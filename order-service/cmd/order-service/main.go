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
	"order-service/internal/transport/http/middleware"
	"order-service/internal/usecase"

	orderv1 "github.com/bekgm/ap2-generated/order/v1"
	paymentv1 "github.com/bekgm/ap2-generated/payment/v1"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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

	// --- Redis ---
	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	log.Printf("Order Service: connected to Redis at %s", cfg.RedisAddr)

	cacheTTL := time.Duration(cfg.CacheTTLSecs) * time.Second
	orderCache := repo.NewRedisOrderCache(redisClient, cacheTTL)

	// --- Payment gRPC ---
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

	orderUseCase := usecase.NewOrderUseCase(orderRepo, paymentClient, orderCache)

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

	// Rate limiter middleware (bonus): 10 req/min per IP using Redis.
	rateLimitWindow := time.Duration(cfg.RateLimitWindowSecs) * time.Second
	r.Use(middleware.RateLimiter(redisClient, cfg.RateLimitMax, rateLimitWindow))

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
	cacheTTLSecs := 300
	if v := os.Getenv("CACHE_TTL_SECS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cacheTTLSecs = parsed
		}
	}
	rateLimitMax := 10
	if v := os.Getenv("RATE_LIMIT_MAX"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			rateLimitMax = parsed
		}
	}
	rateLimitWindowSecs := 60
	if v := os.Getenv("RATE_LIMIT_WINDOW_SECS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			rateLimitWindowSecs = parsed
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
		RedisAddr:              getEnv("REDIS_ADDR", "localhost:6379"),
		CacheTTLSecs:           cacheTTLSecs,
		RateLimitMax:           rateLimitMax,
		RateLimitWindowSecs:    rateLimitWindowSecs,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
