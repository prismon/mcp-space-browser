package database

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteQueue_Basic(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a test table
	_, err = db.Exec(`CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()
	defer wq.Stop()

	// Submit a write operation
	err = wq.Submit(context.Background(), func(db *sql.DB) error {
		_, err := db.Exec(`INSERT INTO test (value) VALUES (?)`, "hello")
		return err
	})
	require.NoError(t, err)

	// Verify the write completed
	var value string
	err = db.QueryRow(`SELECT value FROM test WHERE id = 1`).Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "hello", value)
}

func TestWriteQueue_MultipleWrites(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test (id INTEGER PRIMARY KEY, value INTEGER)`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()
	defer wq.Stop()

	// Submit multiple write operations
	for i := 0; i < 10; i++ {
		val := i
		err := wq.Submit(context.Background(), func(db *sql.DB) error {
			_, err := db.Exec(`INSERT INTO test (value) VALUES (?)`, val)
			return err
		})
		require.NoError(t, err)
	}

	// Verify all writes completed
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 10, count)
}

func TestWriteQueue_ConcurrentSubmits(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test (id INTEGER PRIMARY KEY, value INTEGER)`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()
	defer wq.Stop()

	// Submit writes concurrently from multiple goroutines
	var wg sync.WaitGroup
	numGoroutines := 10
	writesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				val := routineID*100 + j
				err := wq.Submit(context.Background(), func(db *sql.DB) error {
					_, err := db.Exec(`INSERT INTO test (value) VALUES (?)`, val)
					return err
				})
				if err != nil {
					t.Errorf("Submit failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all writes completed
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*writesPerGoroutine, count)
}

func TestWriteQueue_ContextCancellation(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	wq := NewWriteQueue(db, nil)
	wq.Start()
	defer wq.Stop()

	// Create an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = wq.Submit(ctx, func(db *sql.DB) error {
		return nil
	})

	assert.ErrorIs(t, err, context.Canceled)
}

func TestWriteQueue_NotStarted(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	wq := NewWriteQueue(db, nil)
	// Note: Not calling Start()

	err = wq.Submit(context.Background(), func(db *sql.DB) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestWriteQueue_SubmitTx(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()
	defer wq.Stop()

	// Submit a transactional write
	err = wq.SubmitTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO test (value) VALUES (?)`, "tx1")
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO test (value) VALUES (?)`, "tx2")
		return err
	})
	require.NoError(t, err)

	// Verify both writes completed
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestWriteQueue_SubmitTxRollback(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT UNIQUE)`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()
	defer wq.Stop()

	// Submit a transactional write that should fail and rollback
	err = wq.SubmitTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO test (value) VALUES (?)`, "unique")
		if err != nil {
			return err
		}
		// This should fail due to UNIQUE constraint
		_, err = tx.Exec(`INSERT INTO test (value) VALUES (?)`, "unique")
		return err
	})
	assert.Error(t, err) // Should fail

	// Verify rollback - no rows should be present
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestWriteQueue_QueueLength(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	wq := NewWriteQueue(db, &WriteQueueConfig{
		QueueSize:    100,
		WriteTimeout: 30 * time.Second,
	})
	wq.Start()
	defer wq.Stop()

	// Queue should be empty initially
	assert.Equal(t, 0, wq.QueueLength())
}

func TestWriteQueue_IsStarted(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	wq := NewWriteQueue(db, nil)

	assert.False(t, wq.IsStarted())

	wq.Start()
	assert.True(t, wq.IsStarted())

	wq.Stop()
	assert.False(t, wq.IsStarted())
}

func TestWriteQueue_GracefulShutdown(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test (id INTEGER PRIMARY KEY, value INTEGER)`)
	require.NoError(t, err)

	wq := NewWriteQueue(db, nil)
	wq.Start()

	// Submit several writes
	var completedWrites int64
	for i := 0; i < 5; i++ {
		val := i
		go func() {
			err := wq.Submit(context.Background(), func(db *sql.DB) error {
				time.Sleep(10 * time.Millisecond) // Simulate some work
				_, err := db.Exec(`INSERT INTO test (value) VALUES (?)`, val)
				if err == nil {
					atomic.AddInt64(&completedWrites, 1)
				}
				return err
			})
			if err != nil {
				t.Logf("Submit error (may be expected during shutdown): %v", err)
			}
		}()
	}

	// Give time for writes to queue
	time.Sleep(20 * time.Millisecond)

	// Stop should wait for pending writes
	wq.Stop()

	// Verify writes completed
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test`).Scan(&count)
	require.NoError(t, err)
	// At least some writes should have completed
	assert.GreaterOrEqual(t, count, 1)
}
