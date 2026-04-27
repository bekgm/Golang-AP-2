package usecase

import (
	"fmt"
	"log"
	"payment-service/internal/domain"
	"time"

	"github.com/google/uuid"
)

type PaymentUseCase struct {
	repo      domain.PaymentRepository
	publisher domain.EventPublisher
}

func NewPaymentUseCase(repo domain.PaymentRepository, publisher domain.EventPublisher) *PaymentUseCase {
	return &PaymentUseCase{repo: repo, publisher: publisher}
}

type AuthorizeInput struct {
	OrderID       string
	Amount        int64
	CustomerEmail string
}

type AuthorizeOutput struct {
	Payment *domain.Payment
}

func (uc *PaymentUseCase) Authorize(input AuthorizeInput) (*AuthorizeOutput, error) {
	payment := &domain.Payment{
		ID:            uuid.NewString(),
		OrderID:       input.OrderID,
		TransactionID: uuid.NewString(),
		Amount:        input.Amount,
		CreatedAt:     time.Now().UTC(),
	}

	if err := payment.Validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	if !payment.IsWithinLimit() {
		payment.Status = domain.StatusDeclined
		payment.TransactionID = ""
		if err := uc.repo.Save(payment); err != nil {
			return nil, fmt.Errorf("failed to save declined payment: %w", err)
		}
		return &AuthorizeOutput{Payment: payment}, nil
	}

	payment.Status = domain.StatusAuthorized

	if err := uc.repo.Save(payment); err != nil {
		return nil, fmt.Errorf("failed to save payment: %w", err)
	}

	// Publish event asynchronously after the DB transaction is committed.
	event := domain.PaymentCompletedEvent{
		EventID:       uuid.NewString(),
		OrderID:       payment.OrderID,
		PaymentID:     payment.ID,
		Amount:        payment.Amount,
		CustomerEmail: input.CustomerEmail,
		Status:        payment.Status,
		OccurredAt:    time.Now().UTC(),
	}
	if err := uc.publisher.PublishPaymentCompleted(event); err != nil {
		// Log the error but do not fail the request – the payment is already persisted.
		log.Printf("WARNING: failed to publish PaymentCompletedEvent for order %s: %v", payment.OrderID, err)
	}

	return &AuthorizeOutput{Payment: payment}, nil
}

func (uc *PaymentUseCase) GetByOrderID(orderID string) (*domain.Payment, error) {
	payment, err := uc.repo.FindByOrderID(orderID)
	if err != nil {
		return nil, fmt.Errorf("payment not found for order %s: %w", orderID, err)
	}
	return payment, nil
}

type ListPaymentsInput struct {
	MinAmount int64
	MaxAmount int64
}

func (uc *PaymentUseCase) ListPayments(input ListPaymentsInput) ([]*domain.Payment, error) {
	if input.MinAmount < 0 || input.MaxAmount < 0 {
		return nil, fmt.Errorf("min_amount and max_amount must be non-negative")
	}
	if input.MinAmount > 0 && input.MaxAmount > 0 && input.MinAmount > input.MaxAmount {
		return nil, fmt.Errorf("min_amount must be <= max_amount")
	}
	return uc.repo.FindByAmountRange(input.MinAmount, input.MaxAmount)
}
