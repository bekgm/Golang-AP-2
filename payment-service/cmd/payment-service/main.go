package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"payment-service/internal/app"
	"payment-service/internal/messaging"
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

	// --- RabbitMQ Publisher ---
	var publisher *messaging.RabbitMQPublisher
	for attempt := 1; attempt <= 10; attempt++ {
		publisher, err = messaging.NewRabbitMQPublisher(cfg.AMQPURL)
		if err == nil {
			break
		}
		log.Printf("Payment Service: RabbitMQ connection attempt %d/10 failed: %v. Retrying in 3s…", attempt, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("Payment Service: could not connect to RabbitMQ: %v", err)
	}
	defer publisher.Close()

	paymentRepo := repository.NewPostgresPaymentRepository(db)
	paymentUseCase := usecase.NewPaymentUseCase(paymentRepo, publisher)

	// --- gRPC Server ---
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpchandler.LoggingUnaryInterceptor),
	)
	paymentv1.RegisterPaymentServiceServer(grpcServer, grpchandler.NewPaymentGRPCServer(paymentUseCase))

	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			log.Fatalf("gRPC listen failed: %v", err)
		}
		log.Printf("Payment gRPC Server starting on :%s", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()

	// --- HTTP Server ---
	paymentHandler := httphandler.NewPaymentHandler(paymentUseCase)
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "payment-service", "status": "ok"})
	})
	paymentHandler.RegisterRoutes(r)

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: r,
	}

	go func() {
		log.Printf("Payment HTTP Server starting on :%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Payment Service: shutdown signal received. Draining connections…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Payment Service: HTTP shutdown error: %v", err)
	}
	grpcServer.GracefulStop()
	log.Println("Payment Service: exited cleanly.")
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
		AMQPURL:    getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
