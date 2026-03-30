package domain

import (
	"errors"
	"time"
)

// Order statuses
const (
	StatusPending   = "Pending"
	StatusPaid      = "Paid"
	StatusFailed    = "Failed"
	StatusCancelled = "Cancelled"
)

// Order is the core domain entity for the Order bounded context.
// It has no dependency on HTTP, JSON, or any framework.
type Order struct {
	ID             string
	CustomerID     string
	ItemName       string
	Amount         int64 // Amount in cents (e.g., 1000 = $10.00)
	Status         string
	IdempotencyKey string
	CreatedAt      time.Time
}

// Validate enforces domain invariants on creation.
func (o *Order) Validate() error {
	if o.CustomerID == "" {
		return errors.New("customer_id is required")
	}
	if o.ItemName == "" {
		return errors.New("item_name is required")
	}
	if o.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}
	return nil
}

// CanBeCancelled checks the domain invariant for cancellation.
func (o *Order) CanBeCancelled() error {
	if o.Status == StatusPaid {
		return errors.New("paid orders cannot be cancelled")
	}
	if o.Status == StatusCancelled {
		return errors.New("order is already cancelled")
	}
	if o.Status == StatusFailed {
		return errors.New("failed orders cannot be cancelled")
	}
	return nil
}
