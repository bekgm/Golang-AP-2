package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"notification-service/internal/domain"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

const (
	// Main queue
	QueueName = "payment.completed"
	Exchange  = "payment.exchange"

	// Dead Letter
	DLXName = "payment.dlx"
	DLQName = "payment.dead-letter"

	// Idempotency key TTL in Redis.
	idempotencyTTL = 24 * time.Hour
)

// RabbitMQConsumer listens to the payment.completed queue and delegates
// notification delivery to the injected EmailSender adapter.
type RabbitMQConsumer struct {
	conn        *amqp.Connection
	ch          *amqp.Channel
	sender      domain.EmailSender
	redisClient *redis.Client
	maxRetries  int
	done        chan struct{}
}

// New connects to RabbitMQ, declares the exchange/queue topology, and returns
// a ready-to-use consumer.
func New(amqpURL string, sender domain.EmailSender, redisClient *redis.Client, maxRetries int) (*RabbitMQConsumer, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}

	// Process one message at a time for reliability.
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: set qos: %w", err)
	}

	// 1. Dead-Letter Exchange (fanout)
	if err := ch.ExchangeDeclare(DLXName, "fanout", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare DLX: %w", err)
	}

	// 2. Dead-Letter Queue
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

	// 3. Main exchange
	if err := ch.ExchangeDeclare(Exchange, "direct", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}

	// 4. Main queue with DLX configured
	if _, err := ch.QueueDeclare(
		QueueName, true, false, false, false,
		amqp.Table{"x-dead-letter-exchange": DLXName},
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare main queue: %w", err)
	}

	// 5. Bind main queue to main exchange
	if err := ch.QueueBind(QueueName, QueueName, Exchange, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: bind main queue: %w", err)
	}

	return &RabbitMQConsumer{
		conn:        conn,
		ch:          ch,
		sender:      sender,
		redisClient: redisClient,
		maxRetries:  maxRetries,
		done:        make(chan struct{}),
	}, nil
}

// Start begins consuming messages. It blocks until Close() is called.
func (c *RabbitMQConsumer) Start() error {
	msgs, err := c.ch.Consume(
		QueueName,
		"",    // consumer tag
		false, // manual ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("rabbitmq: consume: %w", err)
	}

	log.Printf("[Notification] Worker started. Listening on queue '%s'…", QueueName)
	for {
		select {
		case <-c.done:
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("rabbitmq: message channel closed")
			}
			c.handleMessage(msg)
		}
	}
}

func (c *RabbitMQConsumer) handleMessage(msg amqp.Delivery) {
	var event domain.PaymentCompletedEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		log.Printf("[Notification] Malformed message – sending to DLQ: %v", err)
		msg.Nack(false, false)
		return
	}

	// --- Redis Idempotency Check (SET NX) ---
	idempKey := fmt.Sprintf("notification:processed:%s", event.EventID)
	ctx := context.Background()

	set, err := c.redisClient.SetNX(ctx, idempKey, "processing", idempotencyTTL).Result()
	if err != nil {
		log.Printf("[Notification] Redis idempotency check error for %s: %v – processing anyway", event.EventID, err)
	} else if !set {
		log.Printf("[Notification] Event %s already processed – skipping (idempotency)", event.EventID)
		msg.Ack(false)
		return
	}

	// --- Exponential Backoff Retry Loop ---
	if err := c.sendWithBackoff(event); err != nil {
		log.Printf("[Notification] All %d attempts exhausted for event %s: %v – DLQ", c.maxRetries, event.EventID, err)
		// Clear the idempotency marker so external requeue can retry later.
		c.redisClient.Del(ctx, idempKey)
		msg.Nack(false, false)
		return
	}

	// Mark as fully done in Redis.
	c.redisClient.Set(ctx, idempKey, "done", idempotencyTTL)

	if err := msg.Ack(false); err != nil {
		log.Printf("[Notification] Ack failed for event %s: %v", event.EventID, err)
	} else {
		log.Printf("[Notification] Successfully processed event %s for order %s", event.EventID, event.OrderID)
	}
}

// sendWithBackoff attempts notification delivery with exponential backoff.
// Delay schedule: 2s, 4s, 8s, … (doubles each attempt).
func (c *RabbitMQConsumer) sendWithBackoff(event domain.PaymentCompletedEvent) error {
	backoff := 2 * time.Second
	var lastErr error

	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		lastErr = c.sender.Send(event)
		if lastErr == nil {
			return nil
		}

		log.Printf("[Notification] Attempt %d/%d failed for event %s: %v",
			attempt, c.maxRetries, event.EventID, lastErr)

		if attempt < c.maxRetries {
			log.Printf("[Notification] Backing off %s before retry…", backoff)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return fmt.Errorf("all %d attempts failed: %w", c.maxRetries, lastErr)
}

// Close gracefully shuts down the consumer.
func (c *RabbitMQConsumer) Close() {
	defer func() {
		if r := recover(); r != nil {
			// Ignore: channel already closed
		}
	}()
	close(c.done)
	if c.ch != nil {
		c.ch.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}
