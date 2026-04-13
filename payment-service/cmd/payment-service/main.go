package main

import (
	"log"
	"net"
	"net/http"
	"os"

	"payment-service/internal/app"
	"payment-service/internal/repository"
	grpchandler "payment-service/internal/transport/grpc"
	httphandler "payment-service/internal/transport/http"
	"payment-service/internal/usecase"

	paymentv1 "github.com/bekgm/ap2-generated/payment/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

func main() {
	cfg := loadConfig()

	db, err := app.NewPostgresDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	paymentRepo := repository.NewPostgresPaymentRepository(db)
	paymentUseCase := usecase.NewPaymentUseCase(paymentRepo)

	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			log.Fatalf("gRPC listen failed: %v", err)
		}
		grpcServer := grpc.NewServer(
			grpc.UnaryInterceptor(grpchandler.LoggingUnaryInterceptor),
		)
		paymentv1.RegisterPaymentServiceServer(grpcServer, grpchandler.NewPaymentGRPCServer(paymentUseCase))
		log.Printf("🔌 Payment gRPC Server starting on :%s", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	paymentHandler := httphandler.NewPaymentHandler(paymentUseCase)
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "payment-service", "status": "ok"})
	})
	paymentHandler.RegisterRoutes(r)

	log.Printf("Payment HTTP Server starting on :%s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func loadConfig() app.Config {
	return app.Config{
		HTTPPort:   getEnv("HTTP_PORT", "8081"),
		GRPCPort:   getEnv("GRPC_PORT", "9091"),
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
