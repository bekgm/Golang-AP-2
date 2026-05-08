# Order & Payment Platform — AP2 Assignment 4 (Performance Optimization & External Integrations)

> **Assignment 4** — Caching, Background Jobs & External Integrations  
> **Student:** Bekzat Murat  
> **Evolution:** Assignment 3 (EDA) → Assignment 4 (Redis Caching + Reliable Background Worker)

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            CLIENT (HTTP)                                  │
└────────────────────────────┬─────────────────────────────────────────────┘
                             │ GET /orders/:id  POST /orders  DELETE /orders/:id
                             ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         ORDER SERVICE                                     │
│                                                                           │
│  Rate Limiter Middleware (Redis — 10 req/min per IP)  ← BONUS             │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  OrderUseCase                                                        │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │  Cache-Aside Pattern (domain.OrderCache interface)           │   │ │
│  │  │                                                              │   │ │
│  │  │  GET:    Redis HIT? → return cached order                    │   │ │
│  │  │          Redis MISS? → DB query → SET cache (TTL 5min)       │   │ │
│  │  │                                                              │   │ │
│  │  │  UPDATE / CANCEL:  DB update → DEL cache key (invalidation)  │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│         │ gRPC                          │ SQL                             │
│         ▼                              ▼                                  │
│  Payment Service                   PostgreSQL (orders_db)                 │
└──────────────────────────────────────────────────────────────────────────┘
                                          │ Publishes PaymentCompletedEvent
                                          ▼
                                    RabbitMQ
                               (payment.completed queue)
                                          │
                                          ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                      NOTIFICATION SERVICE (Background Worker)             │
│                                                                           │
│  1. Consume message from queue                                            │
│  2. Redis SETNX idempotency check (key: notification:processed:<eventID>)│
│     → Already processed? ACK & skip                                       │
│  3. sendWithBackoff(event) — exponential backoff: 2s → 4s → 8s           │
│     ┌────────────────────────────────────────────────────────────────┐   │
│     │  domain.EmailSender interface                                  │   │
│     │  ┌──────────────────────┐  ┌──────────────────────────────┐  │   │
│     │  │ SimulatedEmailSender │  │ SMTPEmailSender (REAL mode)  │  │   │
│     │  │ (30% failure rate,   │  │ net/smtp standard library    │  │   │
│     │  │  200ms latency)      │  │                              │  │   │
│     │  └──────────────────────┘  └──────────────────────────────┘  │   │
│     │  Selected via PROVIDER_MODE=SIMULATED|REAL env var            │   │
│     └────────────────────────────────────────────────────────────────┘   │
│  4. Success → Redis SET "done" → ACK message                              │
│  5. All retries exhausted → Clear idempotency key → NACK → DLQ           │
└──────────────────────────────────────────────────────────────────────────┘

Shared Infrastructure
  Redis 7 ── order cache, rate limit counters, notification idempotency
  RabbitMQ 3.13 ── async messaging + dead-letter queue
  PostgreSQL 16 ── orders_db, payments_db
```

---

## What Changed vs Assignment 3

| Feature | Assignment 3 | Assignment 4 |
|---|---|---|
| Order reads | Always hit PostgreSQL | **Cache-aside with Redis (TTL 5 min)** |
| Cache invalidation | N/A | **DEL key on every order mutation** |
| Notification idempotency | In-memory `sync.Map` | **Redis SETNX (survives restarts)** |
| Retry logic | RabbitMQ TTL queue | **In-process exponential backoff (2s→4s→8s)** |
| Email provider | `log.Printf` mock | **`domain.EmailSender` interface + Simulated/SMTP adapters** |
| Provider selection | Hardcoded | **`PROVIDER_MODE=SIMULATED\|REAL` env var** |
| Rate limiting | None | **Redis counter middleware — 10 req/min per IP (bonus)** |
| Infrastructure | Postgres + RabbitMQ | **+ Redis container in docker-compose** |

---

## Cache-Aside Pattern (Invalidation Strategy)

The Order Service uses the **cache-aside** pattern:

- **Read path** (`GET /orders/:id`):
  1. Check Redis for key `order:<id>`.
  2. **HIT** → return the cached `Order` immediately (no DB query).
  3. **MISS** → query PostgreSQL, then `SET order:<id>` with `CACHE_TTL_SECS` (default **300 s**).
- **Write path** (`CreateOrder`, `CancelOrder`):
  - After every successful DB mutation, `DEL order:<id>` from Redis.
  - This **atomic invalidation** guarantees the next read always fetches fresh data.
- The TTL acts as a safety net: even if a bug prevents explicit invalidation, stale data expires within 5 minutes.

---

## Retry & Idempotency Logic (Notification Worker)

### Exponential Backoff
The worker retries failed deliveries in-process without re-queuing the message:

```
attempt 1 → fail → sleep 2s
attempt 2 → fail → sleep 4s
attempt 3 → fail → NACK → DLQ
```

The base delay doubles each attempt (`backoff *= 2`). `MAX_RETRIES` is configurable via env var.

### Redis Idempotency (SETNX)
Before calling `EmailSender.Send()`, the worker calls `SETNX notification:processed:<eventID> "processing" EX 86400`.

- `true` returned → first time seeing this event, proceed.
- `false` returned → duplicate (from re-delivery), ACK and skip.

On success the value is updated to `"done"`. On total failure (all retries exhausted) the key is deleted so future re-delivery can be retried.

---

## Environment Variables

| Variable | Service | Default | Description |
|---|---|---|---|
| `REDIS_ADDR` | order, notification | `localhost:6379` | Redis address |
| `CACHE_TTL_SECS` | order | `300` | Order cache TTL in seconds |
| `RATE_LIMIT_MAX` | order | `10` | Max requests per window per IP |
| `RATE_LIMIT_WINDOW_SECS` | order | `60` | Rate limit window in seconds |
| `PROVIDER_MODE` | notification | `SIMULATED` | `SIMULATED` or `REAL` |
| `MAX_RETRIES` | notification | `3` | Max email delivery attempts |
| `SMTP_HOST` | notification | — | SMTP server host (REAL mode) |
| `SMTP_PORT` | notification | `587` | SMTP server port (REAL mode) |
| `SMTP_USER` | notification | — | SMTP username (REAL mode) |
| `SMTP_PASSWORD` | notification | — | SMTP password (REAL mode) |
| `SMTP_FROM` | notification | — | Sender address (REAL mode) |

---

## Running the Full Stack

```bash
docker compose up --build
```

Services:
- Order Service HTTP: http://localhost:8080
- Payment Service HTTP: http://localhost:8081
- RabbitMQ Management: http://localhost:15672 (guest/guest)

---

## Bonus: Redis Rate Limiter

A Gin middleware (`transport/http/middleware.RateLimiter`) wraps all Order Service routes.
It increments a Redis counter keyed by client IP on every request and returns
**HTTP 429 Too Many Requests** when the count exceeds `RATE_LIMIT_MAX` within the window.
The counter expires automatically after `RATE_LIMIT_WINDOW_SECS` seconds.
Response headers `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `Retry-After` are included.

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
