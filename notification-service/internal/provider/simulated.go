package provider

import (
	"fmt"
	"log"
	"math/rand"
	"notification-service/internal/domain"
	"time"
)

// SimulatedEmailSender is a mock adapter that simulates real-world email
// sending conditions: artificial latency and random transient failures.
// It is selected when PROVIDER_MODE=SIMULATED.
type SimulatedEmailSender struct {
	failureRate float64       // probability of a transient failure [0, 1)
	latency     time.Duration // simulated network latency per send
}

// NewSimulatedEmailSender creates a sender with configurable failure rate and latency.
func NewSimulatedEmailSender(failureRate float64, latency time.Duration) *SimulatedEmailSender {
	return &SimulatedEmailSender{
		failureRate: failureRate,
		latency:     latency,
	}
}

// Send simulates sending an email notification.
// It blocks for the configured latency then randomly returns an error to
// exercise the caller's retry / backoff logic.
func (s *SimulatedEmailSender) Send(event domain.PaymentCompletedEvent) error {
	// Simulate network round-trip time.
	time.Sleep(s.latency)

	// Randomly inject a transient failure.
	if rand.Float64() < s.failureRate {
		return fmt.Errorf("simulated provider error: transient failure for event %s", event.EventID)
	}

	log.Printf(
		"[SimulatedEmail] Sent notification to %s | Order: %s | Amount: $%.2f | Status: %s",
		event.CustomerEmail,
		event.OrderID,
		float64(event.Amount)/100.0,
		event.Status,
	)
	return nil
}
