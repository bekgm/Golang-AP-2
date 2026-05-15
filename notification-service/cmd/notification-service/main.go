package main

import (
	"log"
	"notification-service/internal/consumer"
	"notification-service/internal/domain"
	"notification-service/internal/provider"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	amqpURL := getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	maxRetries := getEnvInt("MAX_RETRIES", 5)
	failureRate := getEnvFloat("FAILURE_RATE", 0.80)
	workerCount := getEnvInt("WORKER_COUNT", 5)

	// --- Redis client ---
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	log.Printf("[Notification] Connected to Redis at %s", redisAddr)

	// --- Email provider (selected via PROVIDER_MODE env var) ---
	var sender domain.EmailSender
	mode := getEnv("PROVIDER_MODE", "SIMULATED")
	switch mode {
	case "REAL":
		sender = provider.NewSMTPEmailSender(
			getEnv("SMTP_HOST", "smtp.example.com"),
			getEnv("SMTP_PORT", "587"),
			getEnv("SMTP_USER", ""),
			getEnv("SMTP_PASSWORD", ""),
			getEnv("SMTP_FROM", "noreply@example.com"),
		)
		log.Println("[Notification] Using REAL SMTP email provider")
	default:
		// SIMULATED: configurable failure rate and 200ms simulated latency
		sender = provider.NewSimulatedEmailSender(failureRate, 200*time.Millisecond)
		log.Printf("[Notification] Using SIMULATED email provider (failure rate %.0f%%)", failureRate*100)
	}

	// --- RabbitMQ consumer ---
	var c *consumer.RabbitMQConsumer
	var err error

	for attempt := 1; attempt <= 10; attempt++ {
		c, err = consumer.New(amqpURL, sender, redisClient, maxRetries, workerCount)
		if err == nil {
			break
		}
		log.Printf("[Notification] RabbitMQ connection attempt %d/10 failed: %v. Retrying in 3s…", attempt, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("[Notification] Could not connect to RabbitMQ after 10 attempts: %v", err)
	}
	defer c.Close()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("[Notification] Shutdown signal received. Closing consumer…")
		c.Close()
	}()

	if err := c.Start(); err != nil {
		log.Fatalf("[Notification] Consumer error: %v", err)
	}

	log.Println("[Notification] Service exited cleanly.")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	}
	return fallback
}
