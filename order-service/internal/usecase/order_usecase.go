package usecase

import (
	"fmt"
	"time"

	"order-service/internal/domain"

	"github.com/google/uuid"
)

// OrderUseCase contains all business logic for orders.
// It depends only on interfaces (Ports), never on concrete implementations.
type OrderUseCase struct {
	repo          domain.OrderRepository
	paymentClient domain.PaymentClient
}

// NewOrderUseCase constructs the use case with its dependencies injected.
func NewOrderUseCase(repo domain.OrderRepository, paymentClient domain.PaymentClient) *OrderUseCase {
	return &OrderUseCase{
		repo:          repo,
		paymentClient: paymentClient,
	}
}

// CreateOrderInput is the DTO coming into the use case from the delivery layer.
type CreateOrderInput struct {
	CustomerID      string
	ItemName        string
	Amount          int64
	IdempotencyKey  string
}

// CreateOrderOutput is the DTO returned to the delivery layer.
type CreateOrderOutput struct {
	Order *domain.Order
}

// CreateOrder orchestrates: persist → authorize payment → update status.
// This is the core business flow and lives entirely in the use case layer.
func (uc *OrderUseCase) CreateOrder(input CreateOrderInput) (*CreateOrderOutput, error) {
	order := &domain.Order{
		ID:         uuid.NewString(),
		CustomerID: input.CustomerID,
		ItemName:   input.ItemName,
		Amount:     input.Amount,
		Status:     domain.StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	// Enforce domain invariants before persisting
	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	// Idempotency: check if an order with this key already exists
	if input.IdempotencyKey != "" {
		existing, err := uc.repo.FindByIdempotencyKey(input.IdempotencyKey)
		if err == nil && existing != nil {
			// Return the existing order instead of creating a duplicate
			return &CreateOrderOutput{Order: existing}, nil
		}
	}

	order.IdempotencyKey = input.IdempotencyKey

	// Step 1: Persist with "Pending" status
	if err := uc.repo.Save(order); err != nil {
		return nil, fmt.Errorf("failed to save order: %w", err)
	}

	// Step 2: Call Payment Service (synchronous REST)
	payResp, err := uc.paymentClient.Authorize(domain.PaymentRequest{
		OrderID: order.ID,
		Amount:  order.Amount,
	})

	// Step 3: Update order status based on payment outcome
	if err != nil {
		// Payment service unavailable or timed out — mark as Failed
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

// GetOrder retrieves an order by ID.
func (uc *OrderUseCase) GetOrder(id string) (*domain.Order, error) {
	order, err := uc.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}
	return order, nil
}

// CancelOrder enforces the cancellation invariant and persists the change.
func (uc *OrderUseCase) CancelOrder(id string) (*domain.Order, error) {
	order, err := uc.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	// Domain invariant check — business rule lives here, not in the handler
	if err := order.CanBeCancelled(); err != nil {
		return nil, err
	}

	order.Status = domain.StatusCancelled
	if err := uc.repo.Update(order); err != nil {
		return nil, fmt.Errorf("failed to cancel order: %w", err)
	}

	return order, nil
}
