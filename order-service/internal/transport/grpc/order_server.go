package grpc

import (
	"log"
	"time"

	orderv1 "github.com/bekgm/ap2-generated/order/v1"
	"order-service/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type OrderGRPCServer struct {
	orderv1.UnimplementedOrderServiceServer
	repo     domain.OrderRepository
	notifier domain.OrderStatusNotifier
}

func NewOrderGRPCServer(repo domain.OrderRepository, notifier domain.OrderStatusNotifier) *OrderGRPCServer {
	return &OrderGRPCServer{repo: repo, notifier: notifier}
}

func (s *OrderGRPCServer) SubscribeToOrderUpdates(
	req *orderv1.OrderRequest,
	stream orderv1.OrderService_SubscribeToOrderUpdatesServer,
) error {
	orderID := req.GetOrderId()
	if orderID == "" {
		return status.Error(codes.InvalidArgument, "order_id is required")
	}

	order, err := s.repo.FindByID(orderID)
	if err != nil {
		return status.Errorf(codes.NotFound, "order %s not found", orderID)
	}

	if err := stream.Send(&orderv1.OrderStatusUpdate{
		OrderId:   order.ID,
		Status:    order.Status,
		UpdatedAt: timestamppb.New(time.Now().UTC()),
	}); err != nil {
		return err
	}

	if isTerminal(order.Status) {
		return nil
	}

	ctx := stream.Context()
	updates, err := s.notifier.Subscribe(ctx, orderID)
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe failed: %v", err)
	}

	log.Printf("[streaming] client subscribed to order %s (current: %s)", orderID, order.Status)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[streaming] client disconnected from order %s", orderID)
			return nil

		case newStatus, ok := <-updates:
			if !ok {
				return nil
			}
			log.Printf("[streaming] order %s → %s", orderID, newStatus)
			if err := stream.Send(&orderv1.OrderStatusUpdate{
				OrderId:   orderID,
				Status:    newStatus,
				UpdatedAt: timestamppb.New(time.Now().UTC()),
			}); err != nil {
				return err
			}

			if isTerminal(newStatus) {
				log.Printf("[streaming] order %s reached terminal state, closing stream", orderID)
				return nil
			}
		}
	}
}

func isTerminal(s string) bool {
	return s == domain.StatusPaid || s == domain.StatusFailed || s == domain.StatusCancelled
}
