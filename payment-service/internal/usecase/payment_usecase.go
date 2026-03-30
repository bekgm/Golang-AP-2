package usecase

import (
	"fmt"
	"github.com/google/uuid"
	"payment-service/internal/domain"
	"time"
)

type PaymentUseCase struct {
	repo domain.PaymentRepository
}

func NewPaymentUseCase(repo domain.PaymentRepository) *PaymentUseCase {
	return &PaymentUseCase{repo: repo}
}

type AuthorizeInput struct {
	OrderID string
	Amount  int64
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
		payment.TransactionID = "" // no transaction for declined payments
		if err := uc.repo.Save(payment); err != nil {
			return nil, fmt.Errorf("failed to save declined payment: %w", err)
		}
		return &AuthorizeOutput{Payment: payment}, nil
	}

	payment.Status = domain.StatusAuthorized

	if err := uc.repo.Save(payment); err != nil {
		return nil, fmt.Errorf("failed to save payment: %w", err)
	}

	return &AuthorizeOutput{Payment: payment}, nil
}

// GetByOrderID retrieves payment details for a given order.
func (uc *PaymentUseCase) GetByOrderID(orderID string) (*domain.Payment, error) {
	payment, err := uc.repo.FindByOrderID(orderID)
	if err != nil {
		return nil, fmt.Errorf("payment not found for order %s: %w", orderID, err)
	}
	return payment, nil
}
