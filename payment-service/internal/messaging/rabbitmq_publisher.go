package messaging

import (
	"encoding/json"
	"fmt"

	"payment-service/internal/domain"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	queueName    = "payment.completed"
	exchangeName = "payment.exchange"
	dlxName      = "payment.dlx"
)

// RabbitMQPublisher implements domain.EventPublisher.
type RabbitMQPublisher struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewRabbitMQPublisher connects to RabbitMQ and declares the durable queue and exchange.
func NewRabbitMQPublisher(amqpURL string) (*RabbitMQPublisher, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq publisher: dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher: open channel: %w", err)
	}

	// Declare the main exchange
	if err := ch.ExchangeDeclare(
		exchangeName,
		"direct",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // args
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher: declare exchange: %w", err)
	}

	// Declare the DLX (Dead-Letter Exchange)
	if err := ch.ExchangeDeclare(
		dlxName,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher: declare dlx: %w", err)
	}

	// Declare the queue with DLX configured
	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-dead-letter-exchange": dlxName,
		},
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher: declare queue: %w", err)
	}

	// Bind queue to exchange
	if err := ch.QueueBind(queueName, queueName, exchangeName, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher: bind queue: %w", err)
	}

	return &RabbitMQPublisher{conn: conn, ch: ch}, nil
}

// PublishPaymentCompleted serialises the event to JSON and publishes it as a
// persistent message so it survives a broker restart.
func (p *RabbitMQPublisher) PublishPaymentCompleted(event domain.PaymentCompletedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("rabbitmq publisher: marshal event: %w", err)
	}

	err = p.ch.Publish(
		exchangeName, // exchange
		queueName,    // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // message survives broker restart
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("rabbitmq publisher: publish: %w", err)
	}
	return nil
}

// Close releases the channel and connection.
func (p *RabbitMQPublisher) Close() error {
	if err := p.ch.Close(); err != nil {
		return err
	}
	return p.conn.Close()
}
