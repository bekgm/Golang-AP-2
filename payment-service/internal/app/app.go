package app

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

// Config holds all runtime configuration for the Payment Service.
type Config struct {
	HTTPPort   string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
}

// NewPostgresDB opens and verifies a PostgreSQL connection.
func NewPostgresDB(cfg Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	log.Println("✅ Payment Service: connected to PostgreSQL")
	return db, nil
}
