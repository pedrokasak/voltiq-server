package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// setupTestDB creates a test database connection
// Note: In production, use a separate test database
func setupTestDB(t *testing.T) (*repository.Database, func()) {
	ctx := context.Background()
	
	db, err := repository.NewDatabase(ctx)
	if err != nil {
		// Try alternative connection string
		db, err = repository.NewDatabase(ctx)
		if err != nil {
			t.Skip("Skipping integration test: database not available")
			return nil, func(){}
		}
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// TestAtomicTransaction_Success tests successful atomic transaction
func TestAtomicTransaction_Success(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	txManager := repository.NewAtomicTransactionManager(db.Pool)

	ctx := context.Background()
	var executed bool

	err := txManager.WithTransaction(ctx, func(tx pgx.Tx) error {
		executed = true
		// Simulate database operations
		_, err := tx.Exec(ctx, "SELECT 1")
		return err
	})

	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	if !executed {
		t.Error("Transaction function was not executed")
	}
}

// TestAtomicTransaction_Rollback tests transaction rollback on error
func TestAtomicTransaction_Rollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	txManager := repository.NewAtomicTransactionManager(db.Pool)

	ctx := context.Background()
	expectedError := "intentional error"

	err := txManager.WithTransaction(ctx, func(tx pgx.Tx) error {
		// Simulate some operation
		_, err := tx.Exec(ctx, "SELECT 1")
		if err != nil {
			return err
		}

		// Intentionally return error to trigger rollback
		return newTestError(expectedError)
	})

	if err == nil {
		t.Error("Expected transaction to fail")
	}
}

// TestConcurrentTransactions tests race condition prevention
func TestConcurrentTransactions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	txManager := repository.NewAtomicTransactionManager(db.Pool)
	ctx := context.Background()

	// Create a test counter table (for this test only)
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_counter (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			value INTEGER DEFAULT 0,
			version INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		t.Skip("Could not create test table")
	}

	// Insert initial counter
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO test_counter (id, value, version) 
		VALUES ('00000000-0000-0000-0000-000000000001', 0, 0)
		ON CONFLICT (id) DO UPDATE SET value = 0, version = 0
	`)
	if err != nil {
		t.Fatalf("Failed to insert test counter: %v", err)
	}

	// Simulate concurrent updates
	var wg sync.WaitGroup
	numGoroutines := 10
	updatesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				err := txManager.WithTransactionAndRetry(ctx, 3, func(tx pgx.Tx) error {
					// Lock the row
					_, err := tx.Exec(ctx, 
						"UPDATE test_counter SET value = value + 1, version = version + 1 WHERE id = $1",
						"00000000-0000-0000-0000-000000000001",
					)
					return err
				})
				if err != nil {
					t.Logf("Transaction failed (expected in concurrent scenario): %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Verify final count
	var finalValue int
	err = db.Pool.QueryRow(ctx, 
		"SELECT value FROM test_counter WHERE id = $1",
		"00000000-0000-0000-0000-000000000001",
	).Scan(&finalValue)

	if err != nil {
		t.Fatalf("Failed to read final value: %v", err)
	}

	expectedValue := numGoroutines * updatesPerGoroutine
	if finalValue != expectedValue {
		t.Errorf("Expected final value %d, got %d", expectedValue, finalValue)
	}

	// Cleanup
	db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_counter")
}

// TestOptimisticLock tests optimistic locking
func TestOptimisticLock(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create test table with version column
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_optimistic (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			data TEXT,
			version INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		t.Skip("Could not create test table")
	}

	// Insert test record
	testID := domain.UUID("11111111-1111-1111-1111-111111111111")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO test_optimistic (id, data, version) 
		VALUES ($1, 'initial', 0)
		ON CONFLICT (id) DO UPDATE SET data = 'initial', version = 0
	`, testID)
	if err != nil {
		t.Fatalf("Failed to insert test record: %v", err)
	}

	txManager := repository.NewAtomicTransactionManager(db.Pool)
	optLock := repository.NewOptimisticLock("test_optimistic", "id", "version")

	// First update should succeed
	err = txManager.WithTransaction(ctx, func(tx pgx.Tx) error {
		return optLock.UpdateWithVersion(ctx, tx, string(testID), func() (map[string]any, error) {
			return map[string]any{
				"data": "updated_by_tx1",
			}, nil
		})
	})

	if err != nil {
		t.Errorf("First update failed: %v", err)
	}

	// Verify update
	var data string
	err = db.Pool.QueryRow(ctx, 
		"SELECT data FROM test_optimistic WHERE id = $1", testID,
	).Scan(&data)

	if err != nil {
		t.Fatalf("Failed to read updated data: %v", err)
	}

	if data != "updated_by_tx1" {
		t.Errorf("Expected 'updated_by_tx1', got '%s'", data)
	}

	// Cleanup
	db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_optimistic")
}

// TestPessimisticLock tests pessimistic locking (FOR UPDATE)
func TestPessimisticLock(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create test table
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_pessimistic (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			value INTEGER
		)
	`)
	if err != nil {
		t.Skip("Could not create test table")
	}

	testID := domain.UUID("22222222-2222-2222-2222-222222222222")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO test_pessimistic (id, value) 
		VALUES ($1, 100)
		ON CONFLICT (id) DO UPDATE SET value = 100
	`, testID)
	if err != nil {
		t.Fatalf("Failed to insert test record: %v", err)
	}

	txManager := repository.NewAtomicTransactionManager(db.Pool)
	pessLock := &repository.PessimisticLock{}

	// Test row locking
	err = txManager.WithTransaction(ctx, func(tx pgx.Tx) error {
		return pessLock.LockRowForUpdate(ctx, tx, "test_pessimistic", "id", string(testID))
	})

	if err != nil {
		t.Errorf("Failed to lock row: %v", err)
	}

	// Cleanup
	db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_pessimistic")
}

// TestBatchOperation tests atomic batch operations
func TestBatchOperation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create test table
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_batch (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			value INTEGER
		)
	`)
	if err != nil {
		t.Skip("Could not create test table")
	}

	txManager := repository.NewAtomicTransactionManager(db.Pool)

	// Test batch operations
	err = txManager.WithTransaction(ctx, func(tx pgx.Tx) error {
		batch := repository.NewBatchOperation(tx)
		
		operations := []func(pgx.Tx) error{
			func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "INSERT INTO test_batch (id, value) VALUES ($1, $2)",
					domain.UUID("33333333-3333-3333-3333-333333333331"), 1)
				return err
			},
			func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "INSERT INTO test_batch (id, value) VALUES ($1, $2)",
					domain.UUID("33333333-3333-3333-3333-333333333332"), 2)
				return err
			},
			func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "INSERT INTO test_batch (id, value) VALUES ($1, $2)",
					domain.UUID("33333333-3333-3333-3333-333333333333"), 3)
				return err
			},
		}

		return batch.Execute(ctx, operations)
	})

	if err != nil {
		t.Fatalf("Batch operation failed: %v", err)
	}

	// Verify all records were inserted
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_batch").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if count < 3 {
		t.Errorf("Expected at least 3 records, got %d", count)
	}

	// Cleanup
	db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_batch")
}

// TestTransactionTimeout tests transaction timeout handling
func TestTransactionTimeout(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	txManager := repository.NewAtomicTransactionManager(db.Pool)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Simulate long-running transaction
	startTime := time.Now()
	err := txManager.WithTransaction(ctx, func(tx pgx.Tx) error {
		// Simulate slow operation
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	elapsed := time.Since(startTime)

	if err == nil {
		t.Error("Expected transaction to timeout")
	}

	// Verify timeout occurred reasonably quickly
	if elapsed > 500*time.Millisecond {
		t.Errorf("Transaction took too long to timeout: %v", elapsed)
	}
}

// TestRetryLogic tests retry logic for deadlocks
func TestRetryLogic(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	txManager := repository.NewAtomicTransactionManager(db.Pool)
	ctx := context.Background()

	attemptCount := 0
	maxAttempts := 3

	err := txManager.WithTransactionAndRetry(ctx, maxAttempts, func(tx pgx.Tx) error {
		attemptCount++
		
		// Simulate deadlock on first attempt
		if attemptCount < 2 {
			return newTestError("deadlock detected")
		}
		
		return nil
	})

	if err != nil {
		t.Errorf("Transaction should have succeeded on retry: %v", err)
	}

	if attemptCount < 2 {
		t.Errorf("Expected at least 2 attempts, got %d", attemptCount)
	}

	if attemptCount > maxAttempts {
		t.Errorf("Too many attempts: %d", attemptCount)
	}
}

// Helper to create error
func newTestError(msg string) error {
	return errors.New(msg)
}
