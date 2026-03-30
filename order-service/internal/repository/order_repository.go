package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"order-service/internal/domain"
)

// PostgresOrderRepository is the concrete implementation of domain.OrderRepository.
// It speaks SQL and knows about the database schema — business logic must NOT live here.
type PostgresOrderRepository struct {
	db *sql.DB
}

// NewPostgresOrderRepository constructs the repository with a shared DB connection.
func NewPostgresOrderRepository(db *sql.DB) *PostgresOrderRepository {
	return &PostgresOrderRepository{db: db}
}

// Save inserts a new order into the database.
func (r *PostgresOrderRepository) Save(order *domain.Order) error {
	query := `
		INSERT INTO orders (id, customer_id, item_name, amount, status, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.Exec(query,
		order.ID,
		order.CustomerID,
		order.ItemName,
		order.Amount,
		order.Status,
		order.IdempotencyKey,
		order.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: save order: %w", err)
	}
	return nil
}

// FindByID retrieves an order by its primary key.
func (r *PostgresOrderRepository) FindByID(id string) (*domain.Order, error) {
	query := `
		SELECT id, customer_id, item_name, amount, status, idempotency_key, created_at
		FROM orders WHERE id = $1
	`
	row := r.db.QueryRow(query, id)

	var order domain.Order
	var idempotencyKey sql.NullString

	err := row.Scan(
		&order.ID,
		&order.CustomerID,
		&order.ItemName,
		&order.Amount,
		&order.Status,
		&idempotencyKey,
		&order.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("order %s not found", id)
		}
		return nil, fmt.Errorf("postgres: find order: %w", err)
	}

	if idempotencyKey.Valid {
		order.IdempotencyKey = idempotencyKey.String
	}

	return &order, nil
}

// Update writes the updated status back to the database.
func (r *PostgresOrderRepository) Update(order *domain.Order) error {
	query := `UPDATE orders SET status = $1 WHERE id = $2`
	result, err := r.db.Exec(query, order.Status, order.ID)
	if err != nil {
		return fmt.Errorf("postgres: update order: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("order %s not found for update", order.ID)
	}
	return nil
}

// FindByIdempotencyKey looks up an existing order by its idempotency key.
// Returns nil, nil if no order exists with that key.
func (r *PostgresOrderRepository) FindByIdempotencyKey(key string) (*domain.Order, error) {
	if key == "" {
		return nil, nil
	}
	query := `
		SELECT id, customer_id, item_name, amount, status, idempotency_key, created_at
		FROM orders WHERE idempotency_key = $1
	`
	row := r.db.QueryRow(query, key)

	var order domain.Order
	var idempotencyKey sql.NullString

	err := row.Scan(
		&order.ID,
		&order.CustomerID,
		&order.ItemName,
		&order.Amount,
		&order.Status,
		&idempotencyKey,
		&order.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: find by idempotency key: %w", err)
	}

	if idempotencyKey.Valid {
		order.IdempotencyKey = idempotencyKey.String
	}

	return &order, nil
}
