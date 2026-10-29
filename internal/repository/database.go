package repository

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/voltiq/server/internal/reqctx"
)

// setTenantGUCs sets every session variable read by the RLS policies.
// Migrations 002-006 use app.tenant_id; 001b/006b use app.current_tenant_id
// and app.current_user_email.
const setTenantGUCs = `SELECT set_config('app.tenant_id', $1, false),
	set_config('app.current_tenant_id', $1, false),
	set_config('app.current_user_email', $2, false)`

// Database represents the database connection pool
type Database struct {
	Pool *pgxpool.Pool
}

// NewDatabase creates a new database connection
func NewDatabase(ctx context.Context) (*Database, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/voltiq-sw?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	// Contexto de tenant por conexão: cada conexão adquirida do pool recebe as
	// variáveis usadas pelas políticas de RLS a partir do contexto da requisição,
	// e as perde ao voltar ao pool. Assim nenhuma requisição herda o tenant de outra.
	scoped := &sync.Map{}
	config.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		id, ok := reqctx.FromContext(ctx)
		if !ok || id.TenantID == "" {
			return true
		}
		if _, err := conn.Exec(ctx, setTenantGUCs, string(id.TenantID), id.Email); err != nil {
			// Descarta a conexão em vez de entregar uma sem escopo.
			return false
		}
		scoped.Store(conn, struct{}{})
		return true
	}
	config.AfterRelease = func(conn *pgx.Conn) bool {
		if _, ok := scoped.LoadAndDelete(conn); !ok {
			return true
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := conn.Exec(ctx, setTenantGUCs, "", "")
		// Se o reset falhar a conexão é destruída para não vazar o tenant.
		return err == nil
	}

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

// Close closes the database connection pool
func (db *Database) Close() {
	db.Pool.Close()
}

// InTx runs fn inside a transaction (read committed), committing when it
// returns nil and rolling back otherwise.
func (db *Database) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// WithAdvisoryLock runs fn only if this instance obtains the Postgres
// advisory lock key, so scheduled jobs run once even with several machines.
// Returns false when another instance holds the lock.
func (db *Database) WithAdvisoryLock(ctx context.Context, key int64, fn func(context.Context) error) (bool, error) {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&ok); err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, key)
	}()
	return true, fn(ctx)
}
