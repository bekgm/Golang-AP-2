package domain

import (
	"errors"
	"time"
)

const (
	StatusAuthorized = "Authorized"
	StatusDeclined   = "Declined"
)

const MaxPaymentAmount int64 = 100000

type Payment struct {
	ID            string
	OrderID       string
	TransactionID string
	Amount        int64
	Status        string
	CreatedAt     time.Time
}

func (p *Payment) Validate() error {
	if p.OrderID == "" {
		return errors.New("order_id is required")
	}
	if p.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}
	return nil
}

func (p *Payment) IsWithinLimit() bool {
	return p.Amount <= MaxPaymentAmount
}
