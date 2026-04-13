# Order & Payment Platform — AP2 Assignment 2 (gRPC Migration)

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
