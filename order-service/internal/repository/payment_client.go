package repository

import (
	"context"
	"fmt"
	"time"

	paymentv1 "github.com/bekgm/ap2-generated/payment/v1"
	"order-service/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCPaymentClient struct {
	client  paymentv1.PaymentServiceClient
	timeout time.Duration
}

func NewGRPCPaymentClient(client paymentv1.PaymentServiceClient, timeout time.Duration) *GRPCPaymentClient {
	return &GRPCPaymentClient{client: client, timeout: timeout}
}

func (c *GRPCPaymentClient) Authorize(req domain.PaymentRequest) (*domain.PaymentResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.client.ProcessPayment(ctx, &paymentv1.PaymentRequest{
		OrderId: req.OrderID,
		Amount:  req.Amount,
	})
	if err != nil {
		st, _ := status.FromError(err)
		switch st.Code() {
		case codes.InvalidArgument:
			return nil, fmt.Errorf("validation error: %s", st.Message())
		case codes.Unavailable, codes.DeadlineExceeded:
			return nil, fmt.Errorf("payment service unavailable: %s", st.Message())
		default:
			return nil, fmt.Errorf("payment service error: %s", st.Message())
		}
	}

	return &domain.PaymentResponse{
		TransactionID: resp.GetTransactionId(),
		Status:        resp.GetStatus(),
	}, nil
}
