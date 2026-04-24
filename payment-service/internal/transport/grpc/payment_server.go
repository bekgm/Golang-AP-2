package grpc

import (
	"context"
	"strings"

	"payment-service/internal/usecase"

	paymentv1 "github.com/bekgm/ap2-generated/payment/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PaymentGRPCServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	uc *usecase.PaymentUseCase
}

func NewPaymentGRPCServer(uc *usecase.PaymentUseCase) *PaymentGRPCServer {
	return &PaymentGRPCServer{uc: uc}
}

func (s *PaymentGRPCServer) ProcessPayment(
	ctx context.Context,
	req *paymentv1.PaymentRequest,
) (*paymentv1.PaymentResponse, error) {

	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be greater than 0")
	}

	output, err := s.uc.Authorize(usecase.AuthorizeInput{
		OrderID: req.GetOrderId(),
		Amount:  req.GetAmount(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "validation error") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, "payment processing failed")
	}

	return &paymentv1.PaymentResponse{
		TransactionId: output.Payment.TransactionID,
		Status:        output.Payment.Status,
		CreatedAt:     timestamppb.New(output.Payment.CreatedAt),
	}, nil
}

func (s *PaymentGRPCServer) ListPayments(
	ctx context.Context,
	req *paymentv1.ListPaymentsRequest,
) (*paymentv1.ListPaymentsResponse, error) {

	if req.GetMinAmount() < 0 || req.GetMaxAmount() < 0 {
		return nil, status.Error(codes.InvalidArgument, "min and max_amount must be non-negative")
	}
	if req.GetMinAmount() > 0 && req.GetMaxAmount() > 0 && req.GetMinAmount() > req.GetMaxAmount() {
		return nil, status.Error(codes.InvalidArgument, "min_amount must be <= max_amount")
	}

	payments, err := s.uc.ListPayments(usecase.ListPaymentsInput{
		MinAmount: req.GetMinAmount(),
		MaxAmount: req.GetMaxAmount(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var resp []*paymentv1.PaymentResponse
	for _, p := range payments {
		resp = append(resp, &paymentv1.PaymentResponse{
			TransactionId: p.TransactionID,
			Status:        p.Status,
			CreatedAt:     timestamppb.New(p.CreatedAt),
		})
	}

	return &paymentv1.ListPaymentsResponse{Payments: resp}, nil
}
