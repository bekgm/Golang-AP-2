package domain

type PaymentRequest struct {
	OrderID string
	Amount  int64
}

type PaymentResponse struct {
	TransactionID string
	Status        string
}

type PaymentClient interface {
	Authorize(req PaymentRequest) (*PaymentResponse, error)
}
