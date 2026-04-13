package domain

import (
	"errors"
	"time"
)

const (
	StatusPending   = "Pending"
	StatusPaid      = "Paid"
	StatusFailed    = "Failed"
	StatusCancelled = "Cancelled"
)

type Order struct {
	ID             string
	CustomerID     string
	ItemName       string
	Amount         int64
	Status         string
	IdempotencyKey string
	CreatedAt      time.Time
}

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
