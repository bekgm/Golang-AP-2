package domain

import "time"

// PaymentCompletedEvent is the event published by the Payment Service
// when a payment is successfully authorized.
type PaymentCompletedEvent struct {
	EventID       string    `json:"event_id"`
	OrderID       string    `json:"order_id"`
	PaymentID     string    `json:"payment_id"`
	Amount        int64     `json:"amount"`
	CustomerEmail string    `json:"customer_email"`
	Status        string    `json:"status"`
	OccurredAt    time.Time `json:"occurred_at"`
}
