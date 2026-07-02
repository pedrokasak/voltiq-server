package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/energybalance/server/internal/domain"
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
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable, // Highest isolation level to prevent race conditions
	})
	if err != nil {
		return errors.New("failed to begin transaction")
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.New("failed to rollback transaction")
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.New("failed to commit transaction")
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

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	
	// PostgreSQL deadlock detected
	if contains(errStr, "deadlock detected") {
		return true
	}
	
	// Serialization failure
	if contains(errStr, "serialization failure") {
		return true
	}
	
	// Connection issues
	if contains(errStr, "connection reset") || 
	   contains(errStr, "connection refused") ||
	   contains(errStr, "broken pipe") {
		return true
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// OptimisticLock helps prevent lost updates
type OptimisticLock struct {
	tableName string
	idColumn  string
	versionColumn string
}

// NewOptimisticLock creates a new optimistic lock helper
func NewOptimisticLock(tableName, idColumn, versionColumn string) *OptimisticLock {
	return &OptimisticLock{
		tableName: tableName,
		idColumn: idColumn,
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

	// Build update query with version check
	setClauses := make([]string, 0, len(data)+1)
	args := make([]any, 0, len(data)+2)
	argIndex := 1

	for key, value := range data {
		setClauses = append(setClauses, key+" = $"+string(rune(argIndex)))
		args = append(args, value)
		argIndex++
	}

	// Increment version
	setClauses = append(setClauses, ol.versionColumn+" = "+ol.versionColumn+" + 1")
	args = append(args, currentVersion+1)
	args = append(args, id)
	args = append(args, currentVersion)

	query = "UPDATE " + ol.tableName + " SET " + 
		joinStrings(setClauses, ", ") + 
		" WHERE " + ol.idColumn + " = $" + string(rune(argIndex)) + 
		" AND " + ol.versionColumn + " = $" + string(rune(argIndex+1))

	result, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return errors.New("optimistic lock failed: record was modified by another transaction")
	}

	return nil
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
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
		placeholders[i] = "$" + string(rune(i+1))
		args[i] = id
	}

	query := "SELECT 1 FROM " + tableName + " WHERE id IN (" + 
		joinStrings(placeholders, ", ") + 
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
