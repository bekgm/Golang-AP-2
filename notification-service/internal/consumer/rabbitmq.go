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
	// Main queue
	QueueName = "payment.completed"
	Exchange  = "payment.exchange"

	// Retry mechanism
	RetryExchange = "payment.retry.exchange"
	RetryQueue    = "payment.retry.queue"
	RetryDuration = 5 * time.Second // Wait 5 seconds between retries

	// Dead Letter
	DLXName = "payment.dlx"
	DLQName = "payment.dead-letter"

	// Constants
	MaxRetries  = 3
	RetryHeader = "x-retry-count"
)

// idempotencyStore is a simple in-memory store for processed event IDs.
type idempotencyStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
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

	// 1. Declare the Dead-Letter Exchange (fanout)
	if err := ch.ExchangeDeclare(
		DLXName, "fanout", true, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare DLX: %w", err)
	}

	// 2. Declare the Dead-Letter Queue and bind it to the DLX
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

	// 3. Declare main exchange
	if err := ch.ExchangeDeclare(
		Exchange, "direct", true, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}

	// 4. Declare the main queue with DLX configured
	_, err = ch.QueueDeclare(
		QueueName,
		true,  // durable
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
		return nil, fmt.Errorf("rabbitmq: declare main queue: %w", err)
	}

	// 5. Bind main queue to exchange
	if err := ch.QueueBind(QueueName, QueueName, Exchange, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: bind main queue: %w", err)
	}

	// 6. Declare retry exchange
	if err := ch.ExchangeDeclare(
		RetryExchange, "direct", true, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare retry exchange: %w", err)
	}

	// 7. Declare retry queue with TTL and DLX (message expired goes back to main queue via retry DLX)
	retryDLX := "payment.retry.dlx"
	if err := ch.ExchangeDeclare(
		retryDLX, "direct", true, false, false, false, nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare retry DLX: %w", err)
	}

	_, err = ch.QueueDeclare(
		RetryQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-message-ttl":             int64(RetryDuration.Milliseconds()),
			"x-dead-letter-exchange":    retryDLX,
			"x-dead-letter-routing-key": QueueName,
		},
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare retry queue: %w", err)
	}

	// 8. Bind retry queue to retry exchange
	if err := ch.QueueBind(RetryQueue, RetryQueue, RetryExchange, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: bind retry queue: %w", err)
	}

	// 9. Bind retry DLX to main queue (so messages re-enter main queue)
	if err := ch.QueueBind(QueueName, QueueName, retryDLX, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: bind retry dlx to main queue: %w", err)
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
		msg.Nack(false, false) // do not requeue – malformed message goes to DLX
		return
	}

	// --- Idempotency check ---
	if c.idempStore.alreadyProcessed(event.EventID) {
		log.Printf("[Notification] Duplicate event %s detected – skipping.", event.EventID)
		msg.Ack(false)
		return
	}

	// --- Get retry count from headers ---
	retries := int32(0)
	if v, ok := msg.Headers[RetryHeader]; ok {
		if r, ok := v.(int32); ok {
			retries = r
		}
	}

	// --- Try to process the message ---
	if err := c.process(event); err != nil {
		if retries >= MaxRetries-1 {
			// Max retries reached - move to DLQ
			log.Printf("[Notification] Max retries (%d) reached for event %s (Order #%s). Moving to DLQ.",
				MaxRetries, event.EventID, event.OrderID)
			msg.Nack(false, false) // nack without requeue → goes to DLX/DLQ
		} else {
			// Send to retry queue with incremented retry count
			newRetries := retries + 1
			log.Printf("[Notification] Processing failed for event %s (attempt %d/%d). Sending to retry queue.",
				event.EventID, newRetries, MaxRetries)
			c.sendToRetryQueue(event, newRetries)
			msg.Ack(false) // ack current message to remove from main queue
		}
		return
	}

	// Mark as processed BEFORE acking to ensure at-least-once + idempotency.
	c.idempStore.markProcessed(event.EventID)

	// --- Manual ACK: only after successful processing ---
	if err := msg.Ack(false); err != nil {
		log.Printf("[Notification] Failed to ack message %s: %v", event.EventID, err)
	} else {
		log.Printf("[Notification] Successfully processed event %s for Order #%s", event.EventID, event.OrderID)
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

func (c *RabbitMQConsumer) sendToRetryQueue(event domain.PaymentCompletedEvent, retryCount int32) {
	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("[Notification] Failed to marshal event %s for retry: %v", event.EventID, err)
		return
	}

	headers := amqp.Table{RetryHeader: retryCount}
	err = c.ch.Publish(
		RetryExchange,
		RetryQueue,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
		},
	)
	if err != nil {
		log.Printf("[Notification] Failed to send event %s to retry queue: %v", event.EventID, err)
	}
}

// Close gracefully shuts down the consumer.
func (c *RabbitMQConsumer) Close() {
	close(c.done)
	c.ch.Close()
	c.conn.Close()
}
