package postgres

import (
	"SignalForge/internal/infra/db/postgres/sqlc"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxFunc is a function that executes within a transaction
type TxFunc func(*sqlc.Queries) error

// WithTx executes a function within a database transaction
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn TxFunc) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := sqlc.New(tx)

	if err := fn(queries); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// GetTx creates a transaction-aware Queries instance
func GetTx(ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, *sqlc.Queries, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	return tx, sqlc.New(tx), nil
}
