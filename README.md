# Order & Payment Platform — AP2 Assignment 1

## Bounded Contexts

| Context | Owns | Does NOT touch |
|---------|------|----------------|
| Order | `orders` table, order lifecycle | `payments` table |
| Payment | `payments` table, authorization logic | `orders` table |

Each service has its **own database**, its **own domain models**, and its **own Go module**. There are zero shared packages between them.

## Clean Architecture Layers (per service)

```
cmd/service-name/main.go   ← Composition Root (manual DI)
internal/
  domain/     ← Entities + Port interfaces (no framework deps)
  usecase/    ← Business logic (depends only on domain interfaces)
  repository/ ← Port implementations: PostgreSQL + HTTP client
  transport/  ← HTTP handlers (Gin) — thin, no business logic
  app/        ← DB connection helper + Config
```

**Dependency Rule:** outer layers depend on inner layers. The `domain` package has zero external imports.

## Business Rules

| Rule | Where enforced |
|------|---------------|
| `amount > 0` | `domain.Order.Validate()` |
| `amount > 100000` → Declined | `domain.Payment.IsWithinLimit()` |
| Paid orders cannot be cancelled | `domain.Order.CanBeCancelled()` |
| Payment timeout = 2 seconds | `main.go` (Composition Root) |
| Payment service down → 503 | `usecase.CreateOrder` + handler |

## Failure Handling Decision

When the Payment Service is **unavailable** (timeout or network error):

- The Order is marked **"Failed"** (not left as "Pending").
- The Order Service returns **HTTP 503**.

**Rationale:** A "Pending" status implies the payment *might still happen*, which is misleading if the payment service never got the request. "Failed" is honest — the authorization attempt definitively did not succeed. The customer can retry, which will create a new order (or reuse the same one via idempotency key).

## Idempotency (Bonus)

Pass an `Idempotency-Key` header on `POST /orders`. If the same key is sent twice, the second request returns the original order without re-calling the Payment Service or inserting a duplicate row.

```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-abc-123" \
  -d '{"customer_id":"c1","item_name":"Book","amount":1500}'
```

## Quick Start

```bash
# 1. Start everything
docker compose up --build

# 2. Run migrations (auto-applied via docker-entrypoint-initdb.d)

# 3. Verify health
curl http://localhost:8080/health
curl http://localhost:8081/health
```

## API Examples

### Create an order (success — amount ≤ 100000)
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"customer-1","item_name":"Laptop Stand","amount":15000}'
```
Response `201`:
```json
{
  "id": "uuid-here",
  "customer_id": "customer-1",
  "item_name": "Laptop Stand",
  "amount": 15000,
  "status": "Paid",
  "created_at": "2026-03-30T10:00:00Z"
}
```

### Create an order (declined — amount > 100000)
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"customer-1","item_name":"Car","amount":200000}'
```
Response `201` with `"status": "Failed"` (payment was declined by Payment Service).

### Get an order
```bash
curl http://localhost:8080/orders/{id}
```

### Cancel an order
```bash
curl -X PATCH http://localhost:8080/orders/{id}/cancel
```
Returns `409 Conflict` if order is already `Paid`.

### Get payment status
```bash
curl http://localhost:8081/payments/{order_id}
```

### Authorize payment directly
```bash
curl -X POST http://localhost:8081/payments \
  -H "Content-Type: application/json" \
  -d '{"order_id":"some-order-id","amount":5000}'
```

## Environment Variables

### Order Service
| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | Listening port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | DB user |
| `DB_PASSWORD` | `postgres` | DB password |
| `DB_NAME` | `orders_db` | Database name |
| `PAYMENT_SERVICE_URL` | `http://localhost:8081` | Payment Service base URL |
| `PAYMENT_TIMEOUT_SECS` | `2` | HTTP client timeout in seconds |

### Payment Service
| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8081` | Listening port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | DB user |
| `DB_PASSWORD` | `postgres` | DB password |
| `DB_NAME` | `payments_db` | Database name |
