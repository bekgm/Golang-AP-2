package provider

import (
	"fmt"
	"log"
	"math/rand"
	"notification-service/internal/domain"
	"time"
)

type SimulatedEmailSender struct {
	failureRate float64  
	latency     time.Duration
}

// NewSimulatedEmailSender creates a sender with configurable failure rate and latency.
func NewSimulatedEmailSender(failureRate float64, latency time.Duration) *SimulatedEmailSender {
	return &SimulatedEmailSender{
		failureRate: failureRate,
		latency:     latency,
	}
}

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
