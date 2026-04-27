package domain

import "time"

// PaymentCompletedEvent is published to the message broker after
// a payment is successfully authorized and persisted.
type PaymentCompletedEvent struct {
	EventID       string    `json:"event_id"`
	OrderID       string    `json:"order_id"`
	PaymentID     string    `json:"payment_id"`
	Amount        int64     `json:"amount"`
	CustomerEmail string    `json:"customer_email"`
	Status        string    `json:"status"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// EventPublisher is the port (interface) that the use-case layer depends on.
// The concrete implementation lives in the infrastructure (messaging) layer.
type EventPublisher interface {
	PublishPaymentCompleted(event PaymentCompletedEvent) error
	Close() error
}
