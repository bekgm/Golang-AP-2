package domain

import (
	"errors"
	"time"
)

// Payment statuses
const (
	StatusAuthorized = "Authorized"
	StatusDeclined   = "Declined"
)

// MaxPaymentAmount is the business rule limit (100000 cents = $1000.00).
// Payments exceeding this must be declined.
const MaxPaymentAmount int64 = 100000

// Payment is the core domain entity for the Payment bounded context.
// It is entirely independent — no shared packages with the Order Service.
type Payment struct {
	ID            string
	OrderID       string
	TransactionID string
	Amount        int64 // Amount in cents
	Status        string
	CreatedAt     time.Time
}

// Validate enforces domain invariants.
func (p *Payment) Validate() error {
	if p.OrderID == "" {
		return errors.New("order_id is required")
	}
	if p.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}
	return nil
}

// IsWithinLimit checks the payment limit business rule.
func (p *Payment) IsWithinLimit() bool {
	return p.Amount <= MaxPaymentAmount
}
