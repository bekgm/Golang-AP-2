package main

import (
	"context"
	"io"
	"log"
	"os"

	orderv1 "github.com/bekgm/ap2-generated/order/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: stream-client <order-id>")
	}
	orderID := os.Args[1]

	addr := getEnv("ORDER_GRPC_ADDR", "localhost:9090")
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("connect to %s: %v", addr, err)
	}
	defer conn.Close()

	client := orderv1.NewOrderServiceClient(conn)
	stream, err := client.SubscribeToOrderUpdates(
		context.Background(),
		&orderv1.OrderRequest{OrderId: orderID},
	)
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}

	log.Printf("Subscribed to order %s — waiting for real-time updates...", orderID)

	for {
		update, err := stream.Recv()
		if err == io.EOF {
			log.Println("Stream closed by server (order reached terminal state)")
			return
		}
		if err != nil {
			log.Fatalf("stream error: %v", err)
		}
		log.Printf("order=%-36s  status=%-12s  at=%s",
			update.GetOrderId(),
			update.GetStatus(),
			update.GetUpdatedAt().AsTime().Format("15:04:05.000"),
		)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
