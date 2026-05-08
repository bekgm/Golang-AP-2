package domain

// EmailSender is the port through which the worker sends notifications.
// Concrete adapters (SMTP, simulated, …) implement this interface so that
// the use-case / worker logic never depends on a specific vendor.
type EmailSender interface {
	Send(event PaymentCompletedEvent) error
}
