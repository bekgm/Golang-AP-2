package domain

// PaymentRepository is the port that the use case depends on.
type PaymentRepository interface {
	Save(payment *Payment) error
	FindByOrderID(orderID string) (*Payment, error)
}
