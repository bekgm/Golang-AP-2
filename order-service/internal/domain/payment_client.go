package domain

// PaymentRequest is the outbound DTO for calling the Payment Service.
type PaymentRequest struct {
	OrderID string
	Amount  int64
}

// PaymentResponse is the inbound DTO from the Payment Service.
type PaymentResponse struct {
	TransactionID string
	Status        string // "Authorized" or "Declined"
}

// PaymentClient is the port for outbound communication with the Payment Service.
// This abstraction lets us swap the real HTTP client for a mock in tests.
type PaymentClient interface {
	Authorize(req PaymentRequest) (*PaymentResponse, error)
}
