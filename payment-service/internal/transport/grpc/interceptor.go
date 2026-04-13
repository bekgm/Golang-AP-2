package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
)

func LoggingUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	log.Printf("[gRPC interceptor] --> method=%s", info.FullMethod)

	resp, err := handler(ctx, req)

	duration := time.Since(start)
	if err != nil {
		log.Printf("[gRPC interceptor] <-- method=%s | duration=%s | ERROR: %v",
			info.FullMethod, duration, err)
	} else {
		log.Printf("[gRPC interceptor] <-- method=%s | duration=%s | OK",
			info.FullMethod, duration)
	}
	return resp, err
}
