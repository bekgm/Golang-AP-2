package usecase

import (
	"fmt"
	"time"

	"order-service/internal/domain"

	"github.com/google/uuid"
)

type OrderUseCase struct {
	repo          domain.OrderRepository
	paymentClient domain.PaymentClient
}

func NewOrderUseCase(repo domain.OrderRepository, paymentClient domain.PaymentClient) *OrderUseCase {
	return &OrderUseCase{
		repo:          repo,
		paymentClient: paymentClient,
	}
}

type CreateOrderInput struct {
	CustomerID     string
	ItemName       string
	Amount         int64
	IdempotencyKey string
}

type CreateOrderOutput struct {
	Order *domain.Order
}

func (uc *OrderUseCase) CreateOrder(input CreateOrderInput) (*CreateOrderOutput, error) {
	order := &domain.Order{
		ID:         uuid.NewString(),
		CustomerID: input.CustomerID,
		ItemName:   input.ItemName,
		Amount:     input.Amount,
		Status:     domain.StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	if input.IdempotencyKey != "" {
		existing, err := uc.repo.FindByIdempotencyKey(input.IdempotencyKey)
		if err == nil && existing != nil {

			return &CreateOrderOutput{Order: existing}, nil
		}
	}

	order.IdempotencyKey = input.IdempotencyKey

	if err := uc.repo.Save(order); err != nil {
		return nil, fmt.Errorf("failed to save order: %w", err)
	}

	payResp, err := uc.paymentClient.Authorize(domain.PaymentRequest{
		OrderID: order.ID,
		Amount:  order.Amount,
	})

	if err != nil {

		order.Status = domain.StatusFailed
		_ = uc.repo.Update(order)
		return nil, fmt.Errorf("payment service unavailable: %w", err)
	}

	if payResp.Status == "Authorized" {
		order.Status = domain.StatusPaid
	} else {
		order.Status = domain.StatusFailed
	}

	if err := uc.repo.Update(order); err != nil {
		return nil, fmt.Errorf("failed to update order status: %w", err)
	}

	return &CreateOrderOutput{Order: order}, nil
}

func (uc *OrderUseCase) GetOrder(id string) (*domain.Order, error) {
	order, err := uc.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}
	return order, nil
}

func (uc *OrderUseCase) CancelOrder(id string) (*domain.Order, error) {
	order, err := uc.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	if err := order.CanBeCancelled(); err != nil {
		return nil, err
	}

	order.Status = domain.StatusCancelled
	if err := uc.repo.Update(order); err != nil {
		return nil, fmt.Errorf("failed to cancel order: %w", err)
	}

	return order, nil
}
func (uc *OrderUseCase) GetRecentOrders(limit int) ([]*domain.Order, error) {
	if limit < 1 {
		limit = 10
	}
	return uc.repo.FindRecent(limit)
}
