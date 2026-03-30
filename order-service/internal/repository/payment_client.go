package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"order-service/internal/domain"
)

// paymentRequestDTO is the JSON payload sent to the Payment Service.
type paymentRequestDTO struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

// paymentResponseDTO is the JSON response from the Payment Service.
type paymentResponseDTO struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}

// HTTPPaymentClient implements domain.PaymentClient using a real HTTP client.
// The Timeout on the http.Client is set at the Composition Root (main.go).
type HTTPPaymentClient struct {
	httpClient     *http.Client
	paymentBaseURL string
}

// NewHTTPPaymentClient constructs the adapter.
func NewHTTPPaymentClient(client *http.Client, baseURL string) *HTTPPaymentClient {
	return &HTTPPaymentClient{
		httpClient:     client,
		paymentBaseURL: baseURL,
	}
}

// Authorize sends a POST /payments request to the Payment Service.
// It returns an error (which signals the Order Service to return 503) if:
//   - the HTTP call fails (network error, timeout)
//   - the response cannot be parsed
func (c *HTTPPaymentClient) Authorize(req domain.PaymentRequest) (*domain.PaymentResponse, error) {
	body, err := json.Marshal(paymentRequestDTO{
		OrderID: req.OrderID,
		Amount:  req.Amount,
	})
	if err != nil {
		return nil, fmt.Errorf("payment client: marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.paymentBaseURL+"/payments",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		// This covers timeouts (context deadline exceeded) and network failures
		return nil, fmt.Errorf("payment service unavailable: %w", err)
	}
	defer resp.Body.Close()

	var dto paymentResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return nil, fmt.Errorf("payment client: decode response: %w", err)
	}

	return &domain.PaymentResponse{
		TransactionID: dto.TransactionID,
		Status:        dto.Status,
	}, nil
}
