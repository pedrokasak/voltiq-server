package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AtomicTransactionManager handles atomic database operations
type AtomicTransactionManager struct {
	pool *pgxpool.Pool
}

// NewAtomicTransactionManager creates a new AtomicTransactionManager
func NewAtomicTransactionManager(pool *pgxpool.Pool) *AtomicTransactionManager {
	return &AtomicTransactionManager{
		pool: pool,
	}
}

// WithTransaction executes a function within a database transaction
func (m *AtomicTransactionManager) WithTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return m.WithTransactionIso(ctx, pgx.ReadCommitted, fn)
}

// WithTransactionIso executes fn in a transaction with the given isolation.
// READ COMMITTED (padrao) serializa atualizacoes da mesma linha pelo lock
// de linha; use Serializable apenas com WithTransactionAndRetry.
func (m *AtomicTransactionManager) WithTransactionIso(ctx context.Context, iso pgx.TxIsoLevel, fn func(tx pgx.Tx) error) error {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: iso})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// WithTransactionAndRetry executes a function with retry logic for deadlocks
func (m *AtomicTransactionManager) WithTransactionAndRetry(ctx context.Context, maxRetries int, fn func(tx pgx.Tx) error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := m.WithTransaction(ctx, fn)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable (deadlock or serialization failure)
		if !isRetryableError(err) {
			return err
		}

		// Exponential backoff
		backoff := time.Duration(100*(attempt+1)) * time.Millisecond
		select {
		case <-time.After(backoff):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

// isRetryableError reports serialization failures (40001) and deadlocks
// (40P01): the whole transaction was rolled back and can run again.
func isRetryableError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "deadlock detected") || strings.Contains(msg, "could not serialize access")
}

// OptimisticLock helps prevent lost updates
type OptimisticLock struct {
	tableName     string
	idColumn      string
	versionColumn string
}

// NewOptimisticLock creates a new optimistic lock helper
func NewOptimisticLock(tableName, idColumn, versionColumn string) *OptimisticLock {
	return &OptimisticLock{
		tableName:     tableName,
		idColumn:      idColumn,
		versionColumn: versionColumn,
	}
}

// UpdateWithVersion performs an update with version check
func (ol *OptimisticLock) UpdateWithVersion(ctx context.Context, tx pgx.Tx, id string, updateFn func() (map[string]any, error)) error {
	// Get current version
	var currentVersion int
	query := "SELECT " + ol.versionColumn + " FROM " + ol.tableName + " WHERE " + ol.idColumn + " = $1"
	err := tx.QueryRow(ctx, query, id).Scan(&currentVersion)
	if err != nil {
		return err
	}

	// Execute update function
	data, err := updateFn()
	if err != nil {
		return err
	}

	// Build update query with version check (ordem estavel das colunas)
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	setClauses := make([]string, 0, len(data)+1)
	args := make([]any, 0, len(data)+2)
	for _, key := range keys {
		args = append(args, data[key])
		setClauses = append(setClauses, key+" = $"+strconv.Itoa(len(args)))
	}
	setClauses = append(setClauses, ol.versionColumn+" = "+ol.versionColumn+" + 1")
	args = append(args, id, currentVersion)

	query = "UPDATE " + ol.tableName + " SET " + strings.Join(setClauses, ", ") +
		" WHERE " + ol.idColumn + " = $" + strconv.Itoa(len(args)-1) +
		" AND " + ol.versionColumn + " = $" + strconv.Itoa(len(args))

	result, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return errors.New("optimistic lock failed: record was modified by another transaction")
	}

	return nil
}

// PessimisticLock provides row-level locking
type PessimisticLock struct{}

// LockRowForUpdate locks a row for update
func (pl *PessimisticLock) LockRowForUpdate(ctx context.Context, tx pgx.Tx, tableName, idColumn, id string) error {
	query := "SELECT 1 FROM " + tableName + " WHERE " + idColumn + " = $1 FOR UPDATE NOWAIT"
	_, err := tx.Exec(ctx, query, id)
	return err
}

// LockRowsForUpdate locks multiple rows for update
func (pl *PessimisticLock) LockRowsForUpdate(ctx context.Context, tx pgx.Tx, tableName string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "$" + strconv.Itoa(i+1)
		args[i] = id
	}

	query := "SELECT 1 FROM " + tableName + " WHERE id IN (" +
		strings.Join(placeholders, ", ") +
		") FOR UPDATE NOWAIT"

	_, err := tx.Exec(ctx, query, args...)
	return err
}

// BatchOperation helps perform atomic batch operations
type BatchOperation struct {
	tx pgx.Tx
}

// NewBatchOperation creates a new batch operation
func NewBatchOperation(tx pgx.Tx) *BatchOperation {
	return &BatchOperation{tx: tx}
}

// Execute executes multiple operations atomically
func (bo *BatchOperation) Execute(ctx context.Context, operations []func(pgx.Tx) error) error {
	for _, op := range operations {
		if err := op(bo.tx); err != nil {
			return err
		}
	}
	return nil
}

// Database helper methods
func (db *Database) WithTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return NewAtomicTransactionManager(db.Pool).WithTransaction(ctx, fn)
}

func (db *Database) WithTransactionAndRetry(ctx context.Context, maxRetries int, fn func(tx pgx.Tx) error) error {
	return NewAtomicTransactionManager(db.Pool).WithTransactionAndRetry(ctx, maxRetries, fn)
}
