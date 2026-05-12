package provider

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"notification-service/internal/domain"
	"strings"
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
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	tlsConfig := &tls.Config{ServerName: s.host}

	// Port 465 usually expects implicit TLS, while 587/25 use plain SMTP with STARTTLS.
	var client *smtp.Client
	var err error
	if s.port == "465" {
		conn, dialErr := tls.Dial("tcp", addr, tlsConfig)
		if dialErr != nil {
			return fmt.Errorf("smtp: tls dial failed: %w", dialErr)
		}
		client, err = smtp.NewClient(conn, s.host)
	} else {
		client, err = smtp.Dial(addr)
		if err == nil {
			if ok, _ := client.Extension("STARTTLS"); ok {
				if err = client.StartTLS(tlsConfig); err != nil {
					client.Close()
					return fmt.Errorf("smtp: starttls failed: %w", err)
				}
			}
		}
	}
	if err != nil {
		return fmt.Errorf("smtp: dial failed: %w", err)
	}
	defer client.Close()

	// Authenticate
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp: auth failed: %w", err)
	}

	// Prepare email
	subject := fmt.Sprintf("Payment %s for Order %s", event.Status, event.OrderID)
	body := fmt.Sprintf(
		"Hello,\r\n\r\nYour payment of $%.2f for order %s has been %s.\r\n\r\nThank you.",
		float64(event.Amount)/100.0,
		event.OrderID,
		event.Status,
	)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s",
		s.from, event.CustomerEmail, subject, body,
	)

	// Send mail
	if err = client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp: mail failed: %w", err)
	}
	if err = client.Rcpt(event.CustomerEmail); err != nil {
		return fmt.Errorf("smtp: rcpt failed: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data failed: %w", err)
	}
	_, err = wc.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("smtp: fprintf failed: %w", err)
	}
	err = wc.Close()
	if err != nil {
		return fmt.Errorf("smtp: close failed: %w", err)
	}

	if err = client.Quit(); err != nil && !strings.Contains(err.Error(), "EOF") {
		return fmt.Errorf("smtp: quit failed: %w", err)
	}
	log.Printf("[SMTPEmail] Sent notification to %s for order %s", event.CustomerEmail, event.OrderID)
	return nil
}
