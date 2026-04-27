package main

import (
	"log"
	"notification-service/internal/consumer"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	amqpURL := getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/")

	var c *consumer.RabbitMQConsumer
	var err error

	// Retry connecting to RabbitMQ – it may not be ready yet on startup.
	for attempt := 1; attempt <= 10; attempt++ {
		c, err = consumer.New(amqpURL)
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
