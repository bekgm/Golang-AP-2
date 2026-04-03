package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"order-service/internal/domain"
)

type PostgresOrderRepository struct {
	db *sql.DB
}

func NewPostgresOrderRepository(db *sql.DB) *PostgresOrderRepository {
	return &PostgresOrderRepository{db: db}
}

func (r *PostgresOrderRepository) Save(order *domain.Order) error {
	query := `
		INSERT INTO orders (id, customer_id, item_name, amount, status, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	var idempotencyKey sql.NullString
	if order.IdempotencyKey != "" {
		idempotencyKey = sql.NullString{String: order.IdempotencyKey, Valid: true}
	}

	_, err := r.db.Exec(query,
		order.ID,
		order.CustomerID,
		order.ItemName,
		order.Amount,
		order.Status,
		idempotencyKey,
		order.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: save order: %w", err)
	}
	return nil
}

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
func (r *PostgresOrderRepository) FindRecent(Limit int) ([]*domain.Order, error) {
	query := `
		SELECT id, customer_id, item_name, amount, status, idempotency_key, created_at
		FROM orders ORDER BY created_at DESC LIMIT $1
		`
	rows, err := r.db.Query(query, Limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: find recent orders: %w", err)
	}
	defer rows.Close()

	var orders []*domain.Order
	for rows.Next() {
		var order domain.Order
		var idempotencyKey sql.NullString
		if err := rows.Scan(
			&order.ID,
			&order.CustomerID,
			&order.ItemName,
			&order.Amount,
			&order.Status,
			&idempotencyKey,
			&order.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres:scan recent order: %w", err)
		}
		if idempotencyKey.Valid {
			order.IdempotencyKey = idempotencyKey.String
		}
		orders = append(orders, &order)
	}
	return orders, nil
}
