# Order & Payment Platform — AP2 Assignment 3 (Event-Driven Architecture)

> **Assignment 3** — Event-Driven Architecture with Message Queues  
> **Student:** Taubakabyl Nurlybek  
> **Evolution:** Assignment 2 (REST + gRPC) → Assignment 3 (REST + gRPC + RabbitMQ EDA)

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          Docker Compose Network                          │
│                                                                          │
│  HTTP/REST          gRPC                  AMQP                           │
│  Client ──► Order Service ──► Payment Service ──► RabbitMQ Broker        │
│             (port 8080)       (port 8081/9091)    (port 5672)            │
│                  │                  │                   │                │
│             orders-db          payments-db    payment.completed queue    │
│           (PostgreSQL)        (PostgreSQL)              │                │
│                                                         ▼                │
│                                               Notification Service       │
│                                               (Consumer, no ports)       │
│                                                         │                │
│                                               [Notification] Sent email  │
│                                               to user@example.com for    │
│                                               Order #123. Amount: $9.99  │
└──────────────────────────────────────────────────────────────────────────┘
```

### Event Flow

1. A client sends `POST /payments` (HTTP) or a gRPC `ProcessPayment` call to **Payment Service**.
2. Payment Service validates the request, persists the payment to PostgreSQL.
3. On **success (Authorized)**, Payment Service publishes a `PaymentCompletedEvent` (JSON) to the `payment.completed` **durable queue** in RabbitMQ.
4. **Notification Service** consumes the event, checks idempotency, logs the simulated email, and manually **ACKs** the message.

---

## What Changed vs Assignment 2

| Layer | Assignment 2 | Assignment 3 |
|---|---|---|
| Order→Payment | gRPC ProcessPayment | **UNCHANGED** |
| Payment→Broker | None | **RabbitMQ producer (amqp091-go)** |
| Notification | None | **New Notification Service (consumer)** |
| Graceful Shutdown | None | **os/signal + context timeout** in Payment & Notification |
| Infrastructure | 2 DBs | **2 DBs + RabbitMQ broker** |
| Docker services | 4 | **5 (+ notification-service)** |

---

## Repository Links

| Repository | Purpose | Link |
|---|---|---|
| **Proto Repository (Repo A)** | `.proto` source files + GitHub Actions workflow | [bekgm/ap2-protos](https://github.com/bekgm/ap2-protos) |
| **Generated Code Repository (Repo B)** | Auto-generated `.pb.go` files, imported by services | [bekgm/ap2-generated](https://github.com/bekgm/ap2-generated) |

---

## Idempotency Strategy

Every event published by the Payment Service includes a unique `event_id` (UUID v4 generated at publish time). The Notification Service maintains an **in-memory map** (`map[string]struct{}`) protected by a mutex:

1. When a message arrives, the consumer looks up `event.event_id` in the map.
2. **If found** → the event was already processed; ACK the message and return without logging (safe deduplication).
3. **If not found** → process the event (log the notification), then add the ID to the map, then ACK.

This guarantees that even if RabbitMQ redelivers a message (e.g., after a crash before ACK), the log is only printed once per unique event.

> **Trade-off:** The in-memory store is lost on service restart. For production, a persistent store (Redis `SETNX`, or a DB `processed_events` table with a UNIQUE constraint on `event_id`) would provide cross-restart idempotency.

---

## ACK Logic

Manual acknowledgements are enabled (`auto-ack = false`). The flow:

```
Receive message
      │
      ▼
Unmarshal JSON ──FAIL──► Nack(requeue=false) → message goes to DLQ
      │
      ▼
Idempotency check ──DUPLICATE──► Ack (remove from queue silently)
      │
      ▼
Process (log email)
      │
      ├─ SUCCESS ─► markProcessed(event_id) → Ack
      │
      └─ FAILURE (retries < 3) ─► republish with x-retry-count header → Ack original
                 (retries >= 3) ─► Nack(requeue=false) → message goes to DLQ
```

A message is **acknowledged only after** the notification log has been successfully printed. If the service crashes mid-processing, RabbitMQ will redeliver the message to the next available consumer (at-least-once delivery).

---

## Reliability Guarantees

| Feature | Implementation |
|---|---|
| **Durable queue** | `durable=true` on `payment.completed` — survives broker restart |
| **Persistent messages** | `DeliveryMode: amqp.Persistent` — messages written to disk |
| **Manual ACK** | `auto-ack=false`; ACK only after successful processing |
| **QoS prefetch=1** | Consumer processes one message at a time |
| **At-least-once delivery** | Unacknowledged messages are requeued on consumer crash |
| **Graceful Shutdown** | `os/signal` + `context.WithTimeout` in Payment Service; `done` channel in Notification Service |

---

## Running the Project

```bash
docker compose up --build
```

All five services start automatically:
- `orders-db` — PostgreSQL for orders
- `payments-db` — PostgreSQL for payments
- `rabbitmq` — RabbitMQ broker (Management UI at http://localhost:15672, guest/guest)
- `payment-service` — HTTP :8081, gRPC :9091
- `order-service` — HTTP :8080, gRPC :9090
- `notification-service` — background consumer (no exposed ports)

### Example: trigger a notification

```bash
# Create a payment via REST
curl -X POST http://localhost:8081/payments \
  -H "Content-Type: application/json" \
  -d '{"order_id":"ord-001","amount":9999,"customer_email":"alice@example.com"}'
```

Check `docker compose logs notification-service` to see:
```
[Notification] Sent email to alice@example.com for Order #ord-001. Amount: $99.99. Status: Authorized
```

---

## Clean Architecture Layers

```
cmd/service-name/main.go       ← Composition Root (manual DI, graceful shutdown)
internal/
  domain/     ← Entities + Port interfaces (EventPublisher interface)
  usecase/    ← Business logic (publishes event via domain.EventPublisher)
  messaging/  ← RabbitMQ publisher (implements domain.EventPublisher)
  repository/ ← PostgreSQL implementations
  transport/  ← HTTP (Gin) + gRPC handlers
  consumer/   ← RabbitMQ consumer (notification-service only)
```

The messaging logic is **hidden behind the `domain.EventPublisher` interface**. The use case depends only on the interface, not on RabbitMQ directly — allowing easy substitution (NATS, Kafka, in-memory stub for tests).


> **Assignment 2** — gRPC Migration & Contract-First Development  
> **Student:** Bekzat
> **Evolution:** Assignment 1 (REST) → Assignment 2 (REST + gRPC)

---

## Repository Links

| Repository | Purpose | Link |
|---|---|---|
| **Proto Repository (Repo A)** | `.proto` source files + GitHub Actions workflow | [bekgm/ap2-protos](https://github.com/bekgm/ap2-protos) |
| **Generated Code Repository (Repo B)** | Auto-generated `.pb.go` files, imported by services | [bekgm/ap2-generated](https://github.com/bekgm/ap2-generated) |

> **Contract-First Flow:** On every push to `ap2-protos`, GitHub Actions runs `protoc` and automatically pushes the generated `.pb.go` files to `ap2-generated`. Services import the generated code via `github.com/bekgm/ap2-generated@v1.0.0`.

---

### What Changed vs Assignment 1

| Layer | Assignment 1 | Assignment 2 |
|---|---|---|
| Order→Payment | REST HTTP POST | **gRPC ProcessPayment** |
| Payment transport | Gin REST only | Gin REST + **gRPC Server** |
| Order transport | Gin REST only | Gin REST + **gRPC Streaming Server** |
| Payment client adapter | `HTTPPaymentClient` | **`GRPCPaymentClient`** |
| Real-time updates | None | **Server-Side Streaming + pg_notify** |
| Interceptor | None | **`LoggingUnaryInterceptor`** (bonus) |
| Domain / Use Cases | — | **UNCHANGED** (Clean Architecture preserved) |

---

## Bounded Contexts

| Context | Owns | Does NOT touch |
|---------|------|----------------|
| Order | `orders` table, order lifecycle | `payments` table |
| Payment | `payments` table, authorization logic | `orders` table |

---

## Clean Architecture Layers (per service)

```
cmd/service-name/main.go       ← Composition Root (manual DI)
internal/
  domain/     ← Entities + Port interfaces (no framework deps, UNCHANGED)
  usecase/    ← Business logic (depends only on domain interfaces, UNCHANGED)
  repository/ ← Port implementations: PostgreSQL + gRPC client adapters
  transport/
    http/     ← Gin REST handlers (UNCHANGED)
    grpc/     ← NEW: gRPC server handlers + interceptor
  app/        ← DB connection helper + Config
```

---

## Quick Start

```bash
# 1. Start all containers (DBs + both services)
docker compose up --build

# 2. Health checks
curl http://localhost:8080/health   # order-service
curl http://localhost:8081/health   # payment-service
```

---

## API Examples

### Create an order (triggers gRPC call to Payment Service internally)
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"customer-1","item_name":"Laptop Stand","amount":15000}'
```
Response `201`:
```json
{
  "id": "<uuid>",
  "customer_id": "customer-1",
  "item_name": "Laptop Stand",
  "amount": 15000,
  "status": "Paid",
  "created_at": "2026-04-13T10:00:00Z"
}
```

### Subscribe to real-time order status stream (gRPC)
```bash
# In the order-service directory:
go run ./cmd/stream-client <order-id>
```
The client prints every status update as the DB changes. Cancel an order in another terminal to see it stream live.

### Cancel an order (triggers a real-time stream push)
```bash
curl -X PATCH http://localhost:8080/orders/<id>/cancel
```

### Get an order
```bash
curl http://localhost:8080/orders/<id>
```

### Get recent orders
```bash
curl "http://localhost:8080/orders/recent?limit=5"
```

### Idempotent order creation
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-abc-123" \
  -d '{"customer_id":"c1","item_name":"Book","amount":1500}'
```

---

## Environment Variables

### Order Service
| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | REST API port |
| `GRPC_PORT` | `9090` | gRPC streaming server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | DB user |
| `DB_PASSWORD` | `postgres` | DB password |
| `DB_NAME` | `orders_db` | Database name |
| `PAYMENT_SERVICE_GRPC_ADDR` | `localhost:9091` | Payment Service gRPC address |
| `PAYMENT_TIMEOUT_SECS` | `5` | gRPC call timeout in seconds |

### Payment Service
| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8081` | REST API port |
| `GRPC_PORT` | `9091` | gRPC server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | DB user |
| `DB_PASSWORD` | `postgres` | DB password |
| `DB_NAME` | `payments_db` | Database name |

---

## Business Rules

| Rule | Where enforced |
|------|---------------|
| `amount > 0` | `domain.Order.Validate()` |
| `amount > 100000` → Declined | `domain.Payment.IsWithinLimit()` |
| Paid orders cannot be cancelled | `domain.Order.CanBeCancelled()` |
| Payment timeout | `PAYMENT_TIMEOUT_SECS` env var (Composition Root) |
| gRPC errors → proper status codes | `transport/grpc` layer |
| Every gRPC call logged with duration | `LoggingUnaryInterceptor` (bonus) |

---

## Proto Files

Located in `protos/`:

```
protos/
  payment/v1/payment.proto   ← PaymentService.ProcessPayment RPC
  order/v1/order.proto       ← OrderService.SubscribeToOrderUpdates streaming RPC
  .github/workflows/
    generate.yml             ← GitHub Actions: auto-generates .pb.go → Repo B
```

Generated code is in `generated/` (local mirror of Repo B):
```
generated/
  go.mod                     (module github.com/bekgm/ap2-generated)
  payment/v1/payment.pb.go
  payment/v1/payment_grpc.pb.go
  order/v1/order.pb.go
  order/v1/order_grpc.pb.go
```
