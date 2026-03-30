package main

import (
	"log"
	"net/http"
	"os"

	"payment-service/internal/app"
	"payment-service/internal/repository"
	handler "payment-service/internal/transport/http"
	"payment-service/internal/usecase"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := loadConfig()

	db, err := app.NewPostgresDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// --- Repository (Port implementation) ---
	paymentRepo := repository.NewPostgresPaymentRepository(db)

	// --- Use Case (Business Logic) ---
	paymentUseCase := usecase.NewPaymentUseCase(paymentRepo)

	// --- Delivery Layer ---
	paymentHandler := handler.NewPaymentHandler(paymentUseCase)

	// --- Router ---
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "payment-service", "status": "ok"})
	})
	paymentHandler.RegisterRoutes(r)

	log.Printf("🚀 Payment Service starting on :%s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() app.Config {
	return app.Config{
		HTTPPort:   getEnv("HTTP_PORT", "8081"),
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "payments_db"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
