package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"order-service/internal/app"
	repo "order-service/internal/repository"
	grpchandler "order-service/internal/transport/grpc"
	handler "order-service/internal/transport/http"
	"order-service/internal/usecase"

	orderv1 "github.com/bekgm/ap2-generated/order/v1"
	paymentv1 "github.com/bekgm/ap2-generated/payment/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := loadConfig()

	db, err := app.NewPostgresDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	paymentConn, err := grpc.NewClient(
		cfg.PaymentServiceGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to dial payment gRPC service: %v", err)
	}
	defer paymentConn.Close()

	dsn := app.BuildDSN(cfg)
	orderNotifier, err := repo.NewPGOrderNotifier(dsn)
	if err != nil {
		log.Fatalf("failed to create order status notifier: %v", err)
	}

	orderRepo := repo.NewPostgresOrderRepository(db)
	paymentClient := repo.NewGRPCPaymentClient(
		paymentv1.NewPaymentServiceClient(paymentConn),
		time.Duration(cfg.PaymentTimeoutSecs)*time.Second,
	)

	orderUseCase := usecase.NewOrderUseCase(orderRepo, paymentClient)

	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			log.Fatalf("gRPC listen failed: %v", err)
		}
		grpcServer := grpc.NewServer()
		orderv1.RegisterOrderServiceServer(grpcServer,
			grpchandler.NewOrderGRPCServer(orderRepo, orderNotifier),
		)
		log.Printf("Order gRPC Server starting on :%s", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	orderHandler := handler.NewOrderHandler(orderUseCase)
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "order-service", "status": "ok"})
	})
	orderHandler.RegisterRoutes(r)

	log.Printf("Order REST Server starting on :%s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig() app.Config {
	timeoutSecs := 5
	if v := os.Getenv("PAYMENT_TIMEOUT_SECS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			timeoutSecs = parsed
		}
	}
	return app.Config{
		HTTPPort:               getEnv("HTTP_PORT", "8080"),
		GRPCPort:               getEnv("GRPC_PORT", "9090"),
		DBHost:                 getEnv("DB_HOST", "localhost"),
		DBPort:                 getEnv("DB_PORT", "5432"),
		DBUser:                 getEnv("DB_USER", "postgres"),
		DBPassword:             getEnv("DB_PASSWORD", "postgres"),
		DBName:                 getEnv("DB_NAME", "orders_db"),
		PaymentServiceGRPCAddr: getEnv("PAYMENT_SERVICE_GRPC_ADDR", "localhost:9091"),
		PaymentTimeoutSecs:     timeoutSecs,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
