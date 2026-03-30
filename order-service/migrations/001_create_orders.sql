-- migrations/001_create_orders.sql
-- Run this against your orders_db PostgreSQL database.

CREATE TABLE IF NOT EXISTS orders (
    id              VARCHAR(36)  PRIMARY KEY,
    customer_id     VARCHAR(255) NOT NULL,
    item_name       VARCHAR(255) NOT NULL,
    amount          BIGINT       NOT NULL CHECK (amount > 0),
    status          VARCHAR(20)  NOT NULL DEFAULT 'Pending',
    idempotency_key VARCHAR(255) UNIQUE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Index for fast idempotency key lookups
CREATE INDEX IF NOT EXISTS idx_orders_idempotency_key ON orders(idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Index for customer queries (useful extension)
CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders(customer_id);
