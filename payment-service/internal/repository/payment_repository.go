package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"payment-service/internal/domain"
)

type PostgresPaymentRepository struct {
	db *sql.DB
}

func NewPostgresPaymentRepository(db *sql.DB) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{db: db}
}

func (r *PostgresPaymentRepository) Save(p *domain.Payment) error {
	query := `
		INSERT INTO payments (id, order_id, transaction_id, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Exec(query,
		p.ID,
		p.OrderID,
		p.TransactionID,
		p.Amount,
		p.Status,
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: save payment: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) FindByAmountRange(min, max int64) ([]*domain.Payment, error) {
	query := `SELECT id, order_id, transaction_id, amount, status, created_at FROM payments WHERE ($1 = 0 OR amount >= $1) AND ($2 = 0 OR amount <= $2) ORDER BY created_at DESC`

	rows, err := r.db.Query(query, min, max)
	if err != nil {
		return nil, fmt.Errorf("postgres: find payments by amount range: %w", err)
	}
	defer rows.Close()

	var payments []*domain.Payment
	for rows.Next() {
		var p domain.Payment
		var transactionID sql.NullString
		if err := rows.Scan(&p.ID, &p.OrderID, &transactionID, &p.Amount, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan payment row: %w", err)
		}
		if transactionID.Valid {
			p.TransactionID = transactionID.String
		}
		payments = append(payments, &p)
	}
	return payments, rows.Err()
}

func (r *PostgresPaymentRepository) FindByOrderID(orderID string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, transaction_id, amount, status, created_at
		FROM payments WHERE order_id = $1
		ORDER BY created_at DESC LIMIT 1
	`
	row := r.db.QueryRow(query, orderID)

	var p domain.Payment
	var transactionID sql.NullString

	err := row.Scan(
		&p.ID,
		&p.OrderID,
		&transactionID,
		&p.Amount,
		&p.Status,
		&p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("payment for order %s not found", orderID)
		}
		return nil, fmt.Errorf("postgres: find payment by order id: %w", err)
	}

	if transactionID.Valid {
		p.TransactionID = transactionID.String
	}

	return &p, nil
}
