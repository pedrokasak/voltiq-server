package repository

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Database represents the database connection pool
type Database struct {
	Pool *pgxpool.Pool
}

// NewDatabase creates a new database connection
func NewDatabase(ctx context.Context) (*Database, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/energybalance?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{
		Pool: pool,
	}, nil
}

// SetTenantID sets the tenant ID for row-level security
func (db *Database) SetTenantID(ctx context.Context, tenantID string) context.Context {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return ctx
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, fmt.Sprintf("SET app.tenant_id = '%s'", tenantID))
	if err != nil {
		return ctx
	}

	return ctx
}

// Close closes the database connection pool
func (db *Database) Close() {
	db.Pool.Close()
}
