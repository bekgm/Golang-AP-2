package consumer

import (
	"encoding/json"
	"fmt"
	"log"
	"notification-service/internal/domain"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	QueueName    = "payment.completed"
	DLXName      = "payment.dlx"
	DLQName      = "payment.dead-letter"
	MaxRetries   = 3
	RetryHeader  = "x-retry-count"
)

// idempotencyStore is a simple in-memory store for processed event IDs.
type idempotencyStore struct {
	mu      sync.Mutex
	seen    map[string]struct{}
}

func newIdempotencyStore() *idempotencyStore {
	return &idempotencyStore{seen: make(map[string]struct{})}
}

func (s *idempotencyStore) alreadyProcessed(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.seen[id]
	return exists
}

func (s *idempotencyStore) markProcessed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[id] = struct{}{}
}

// RabbitMQConsumer listens to the payment.completed queue.
type RabbitMQConsumer struct {
	conn       *amqp.Connection
	ch         *amqp.Channel
	idempStore *idempotencyStore
	done       chan struct{}
}

// New connects to RabbitMQ and declares the necessary topology.
func New(amqpURL string) (*RabbitMQConsumer, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}

	// Set QoS – process one message at a time for reliability.
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: set qos: %w", err)
	}

	// Declare the Dead-Letter Exchange.
	if err := ch.ExchangeDeclare(
		DLXName, "fanout", true, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare DLX: %w", err)
	}

	// Declare the Dead-Letter Queue and bind it to the DLX.
	dlq, err := ch.QueueDeclare(DLQName, true, false, false, false, nil)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare DLQ: %w", err)
	}
	if err := ch.QueueBind(dlq.Name, "", DLXName, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: bind DLQ: %w", err)
	}

	// Declare the main durable queue with DLX configured.
	_, err = ch.QueueDeclare(
		QueueName,
		true,  // durable – survives broker restart
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-dead-letter-exchange": DLXName,
		},
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare queue: %w", err)
	}

	return &RabbitMQConsumer{
		conn:       conn,
		ch:         ch,
		idempStore: newIdempotencyStore(),
		done:       make(chan struct{}),
	}, nil
}

// Start begins consuming messages. It blocks until Close() is called.
func (c *RabbitMQConsumer) Start() error {
	msgs, err := c.ch.Consume(
		QueueName,
		"",    // consumer tag
		false, // auto-ack DISABLED – we acknowledge manually
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("rabbitmq: consume: %w", err)
	}

	log.Printf("[Notification] Consumer started. Waiting for messages on queue '%s'…", QueueName)

	for {
		select {
		case <-c.done:
			log.Println("[Notification] Consumer shutting down.")
			return nil
		case msg, ok := <-msgs:
			if !ok {
				log.Println("[Notification] Message channel closed.")
				return nil
			}
			c.handleMessage(msg)
		}
	}
}

func (c *RabbitMQConsumer) handleMessage(msg amqp.Delivery) {
	var event domain.PaymentCompletedEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		log.Printf("[Notification] Failed to unmarshal message: %v. Moving to DLQ.", err)
		msg.Nack(false, false) // do not requeue – malformed message
		return
	}

	// --- Idempotency check ---
	if c.idempStore.alreadyProcessed(event.EventID) {
		log.Printf("[Notification] Duplicate event %s detected – skipping.", event.EventID)
		msg.Ack(false) // ack so it is removed from queue
		return
	}

	// --- Retry / DLQ logic ---
	retries := int32(0)
	if v, ok := msg.Headers[RetryHeader]; ok {
		if r, ok := v.(int32); ok {
			retries = r
		}
	}

	if err := c.process(event); err != nil {
		if retries >= MaxRetries-1 {
			log.Printf("[Notification] Max retries (%d) reached for event %s. Sending to DLQ.", MaxRetries, event.EventID)
			msg.Nack(false, false) // nack without requeue → goes to DLX/DLQ
		} else {
			log.Printf("[Notification] Processing failed for event %s (attempt %d). Requeueing.", event.EventID, retries+1)
			time.Sleep(500 * time.Millisecond)
			// Republish with incremented retry count so we can track attempts.
			c.republishWithRetry(msg, event, retries+1)
			msg.Ack(false)
		}
		return
	}

	// Mark as processed BEFORE acking to ensure at-least-once + idempotency.
	c.idempStore.markProcessed(event.EventID)

	// --- Manual ACK: only after successful processing ---
	if err := msg.Ack(false); err != nil {
		log.Printf("[Notification] Failed to ack message %s: %v", event.EventID, err)
	}
}

func (c *RabbitMQConsumer) process(event domain.PaymentCompletedEvent) error {
	log.Printf(
		"[Notification] Sent email to %s for Order #%s. Amount: $%.2f. Status: %s",
		event.CustomerEmail,
		event.OrderID,
		float64(event.Amount)/100.0,
		event.Status,
	)
	return nil
}

func (c *RabbitMQConsumer) republishWithRetry(original amqp.Delivery, event domain.PaymentCompletedEvent, retryCount int32) {
	body, _ := json.Marshal(event)
	headers := amqp.Table{RetryHeader: retryCount}
	err := c.ch.Publish(
		"",        // default exchange
		QueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
		},
	)
	if err != nil {
		log.Printf("[Notification] Failed to republish event %s for retry: %v", event.EventID, err)
	}
}

// Close gracefully shuts down the consumer.
func (c *RabbitMQConsumer) Close() {
	close(c.done)
	c.ch.Close()
	c.conn.Close()
}
