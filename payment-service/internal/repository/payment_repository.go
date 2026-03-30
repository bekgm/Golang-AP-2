package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"payment-service/internal/domain"
)

// PostgresPaymentRepository implements domain.PaymentRepository.
type PostgresPaymentRepository struct {
	db *sql.DB
}

func NewPostgresPaymentRepository(db *sql.DB) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{db: db}
}

// Save inserts a payment record.
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

// FindByOrderID retrieves a payment by the associated order ID.
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
