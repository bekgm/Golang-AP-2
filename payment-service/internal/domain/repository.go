package domain

type PaymentRepository interface {
	Save(payment *Payment) error
	FindByOrderID(orderID string) (*Payment, error)
	FindByAmountRange(min, max int64) ([]*Payment, error)
}
