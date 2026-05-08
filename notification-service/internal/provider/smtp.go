package provider

import (
	"fmt"
	"log"
	"net/smtp"
	"notification-service/internal/domain"
)

// SMTPEmailSender is the real provider adapter that delivers notifications
// via a standard SMTP server. It is selected when PROVIDER_MODE=REAL.
type SMTPEmailSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

// NewSMTPEmailSender creates an SMTP adapter from configuration.
func NewSMTPEmailSender(host, port, username, password, from string) *SMTPEmailSender {
	return &SMTPEmailSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

// Send delivers an email notification for a completed payment event.
func (s *SMTPEmailSender) Send(event domain.PaymentCompletedEvent) error {
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	subject := fmt.Sprintf("Payment %s for Order %s", event.Status, event.OrderID)
	body := fmt.Sprintf(
		"Hello,\r\n\r\nYour payment of $%.2f for order %s has been %s.\r\n\r\nThank you.",
		float64(event.Amount)/100.0,
		event.OrderID,
		event.Status,
	)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		s.from, event.CustomerEmail, subject, body,
	)

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	if err := smtp.SendMail(addr, auth, s.from, []string{event.CustomerEmail}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp: send failed for event %s: %w", event.EventID, err)
	}

	log.Printf("[SMTPEmail] Sent notification to %s for order %s", event.CustomerEmail, event.OrderID)
	return nil
}
