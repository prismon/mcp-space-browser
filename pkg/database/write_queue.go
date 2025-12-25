package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var wqLog *logrus.Entry

func init() {
	wqLog = logger.WithName("write-queue")
}

// WriteQueue serializes all write operations to avoid SQLite lock contention.
// SQLite only supports a single writer at a time, and concurrent write attempts
// result in "database is locked" errors. The WriteQueue ensures all writes go
// through a single goroutine, eliminating contention.
type WriteQueue struct {
	db      *sql.DB
	queue   chan writeRequest
	done    chan struct{}
	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex
}

// writeRequest represents a single write operation to be executed
type writeRequest struct {
	operation func(db *sql.DB) error
	result    chan error
	ctx       context.Context
}

// WriteQueueConfig configures the write queue behavior
type WriteQueueConfig struct {
	// QueueSize is the buffer size for pending write requests (default: 100)
	QueueSize int
	// WriteTimeout is the maximum time to wait for a write to complete (default: 30s)
	WriteTimeout time.Duration
}

// DefaultWriteQueueConfig returns sensible defaults
func DefaultWriteQueueConfig() *WriteQueueConfig {
	return &WriteQueueConfig{
		QueueSize:    100,
		WriteTimeout: 30 * time.Second,
	}
}

// NewWriteQueue creates a new write queue for the given database connection
func NewWriteQueue(db *sql.DB, config *WriteQueueConfig) *WriteQueue {
	if config == nil {
		config = DefaultWriteQueueConfig()
	}

	wq := &WriteQueue{
		db:    db,
		queue: make(chan writeRequest, config.QueueSize),
		done:  make(chan struct{}),
	}

	return wq
}

// Start begins processing write requests. Must be called before Submit.
func (wq *WriteQueue) Start() {
	wq.mu.Lock()
	defer wq.mu.Unlock()

	if wq.started {
		return
	}

	wq.started = true
	wq.wg.Add(1)

	go wq.worker()

	wqLog.Info("Write queue started")
}

// Stop gracefully shuts down the write queue, waiting for pending writes to complete
func (wq *WriteQueue) Stop() {
	wq.mu.Lock()
	if !wq.started {
		wq.mu.Unlock()
		return
	}
	wq.started = false
	wq.mu.Unlock()

	close(wq.done)
	wq.wg.Wait()

	wqLog.Info("Write queue stopped")
}

// Submit queues a write operation and waits for it to complete.
// The operation function receives the database connection and should perform
// the write. Returns any error from the operation.
func (wq *WriteQueue) Submit(ctx context.Context, operation func(db *sql.DB) error) error {
	wq.mu.Lock()
	if !wq.started {
		wq.mu.Unlock()
		return fmt.Errorf("write queue not started")
	}
	wq.mu.Unlock()

	result := make(chan error, 1)

	req := writeRequest{
		operation: operation,
		result:    result,
		ctx:       ctx,
	}

	select {
	case wq.queue <- req:
		// Request queued successfully
	case <-ctx.Done():
		return ctx.Err()
	case <-wq.done:
		return fmt.Errorf("write queue is shutting down")
	}

	// Wait for result
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-wq.done:
		return fmt.Errorf("write queue shut down while waiting for result")
	}
}

// SubmitTx queues a write operation that runs within a transaction.
// The operation function receives a transaction and should perform writes.
// The transaction is automatically committed on success or rolled back on error.
func (wq *WriteQueue) SubmitTx(ctx context.Context, operation func(tx *sql.Tx) error) error {
	return wq.Submit(ctx, func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		if err := operation(tx); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				wqLog.WithError(rbErr).Error("Failed to rollback transaction after error")
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		return nil
	})
}

// worker is the single goroutine that processes all write requests
func (wq *WriteQueue) worker() {
	defer wq.wg.Done()

	for {
		select {
		case req := <-wq.queue:
			wq.processRequest(req)
		case <-wq.done:
			// Drain remaining requests before shutting down
			wq.drainQueue()
			return
		}
	}
}

// processRequest executes a single write request
func (wq *WriteQueue) processRequest(req writeRequest) {
	// Check if context is already cancelled
	if req.ctx != nil && req.ctx.Err() != nil {
		req.result <- req.ctx.Err()
		return
	}

	// Execute the operation
	err := req.operation(wq.db)

	// Send result
	select {
	case req.result <- err:
	default:
		// Result channel full or closed, log but don't block
		if err != nil {
			wqLog.WithError(err).Warn("Write operation failed but result channel unavailable")
		}
	}
}

// drainQueue processes any remaining requests in the queue during shutdown
func (wq *WriteQueue) drainQueue() {
	for {
		select {
		case req := <-wq.queue:
			wq.processRequest(req)
		default:
			return
		}
	}
}

// QueueLength returns the current number of pending write requests
func (wq *WriteQueue) QueueLength() int {
	return len(wq.queue)
}

// IsStarted returns whether the write queue is running
func (wq *WriteQueue) IsStarted() bool {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	return wq.started
}
