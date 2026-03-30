package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"order-service/internal/app"
	repo "order-service/internal/repository"
	"order-service/internal/usecase"
	handler "order-service/internal/transport/http"

	"github.com/gin-gonic/gin"
)

// main is the Composition Root — the single place where all dependencies
// are wired together with manual Dependency Injection. No DI framework needed.
func main() {
	cfg := loadConfig()

	// --- Infrastructure: Database ---
	db, err := app.NewPostgresDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// --- Infrastructure: HTTP client for Payment Service ---
	// Timeout is set here at the Composition Root, not inside the adapter.
	paymentHTTPClient := &http.Client{
		Timeout: time.Duration(cfg.PaymentTimeoutSecs) * time.Second,
	}

	// --- Repositories (Port implementations) ---
	orderRepo := repo.NewPostgresOrderRepository(db)
	paymentClient := repo.NewHTTPPaymentClient(paymentHTTPClient, cfg.PaymentServiceURL)

	// --- Use Cases (Business Logic) ---
	orderUseCase := usecase.NewOrderUseCase(orderRepo, paymentClient)

	// --- Delivery Layer ---
	orderHandler := handler.NewOrderHandler(orderUseCase)

	// --- Router ---
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "order-service", "status": "ok"})
	})
	orderHandler.RegisterRoutes(r)

	log.Printf("🚀 Order Service starting on :%s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() app.Config {
	timeoutSecs := 2
	if v := os.Getenv("PAYMENT_TIMEOUT_SECS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			timeoutSecs = parsed
		}
	}
	return app.Config{
		HTTPPort:           getEnv("HTTP_PORT", "8080"),
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPassword:         getEnv("DB_PASSWORD", "postgres"),
		DBName:             getEnv("DB_NAME", "orders_db"),
		PaymentServiceURL:  getEnv("PAYMENT_SERVICE_URL", "http://localhost:8081"),
		PaymentTimeoutSecs: timeoutSecs,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
