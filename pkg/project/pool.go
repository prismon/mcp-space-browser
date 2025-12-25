package project

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/database"
)

// DefaultMaxOpen is the default maximum number of open databases
const DefaultMaxOpen = 10

// DefaultIdleTimeout is the default idle timeout for databases
const DefaultIdleTimeout = 30 * time.Minute

// pooledBackend wraps a backend with usage tracking
type pooledBackend struct {
	backend    database.Backend
	project    string
	lastAccess time.Time
	refCount   int32
}

// DatabasePool manages a pool of database backends with idle timeout
type DatabasePool struct {
	backends    map[string]*pooledBackend
	maxOpen     int
	idleTimeout time.Duration
	mu          sync.RWMutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewDatabasePool creates a new database pool
func NewDatabasePool(maxOpen int, idleTimeout time.Duration) *DatabasePool {
	if maxOpen <= 0 {
		maxOpen = DefaultMaxOpen
	}
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}

	return &DatabasePool{
		backends:    make(map[string]*pooledBackend),
		maxOpen:     maxOpen,
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}
}

// Get returns the backend for a project, opening it if necessary
func (p *DatabasePool) Get(project *Project) (database.Backend, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if already open
	if pooled, ok := p.backends[project.Name]; ok {
		atomic.AddInt32(&pooled.refCount, 1)
		pooled.lastAccess = time.Now()
		log.WithField("project", project.Name).Debug("Returning existing database from pool")
		return pooled.backend, nil
	}

	// Check max open limit
	if len(p.backends) >= p.maxOpen {
		// Try to evict an idle backend
		evicted := p.evictOldest()
		if !evicted {
			return nil, fmt.Errorf("maximum number of open databases (%d) reached", p.maxOpen)
		}
	}

	// Create new backend
	log.WithField("project", project.Name).Info("Opening new database for project")
	backend, err := database.NewBackend(project.Path, &project.Config.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to create backend: %w", err)
	}

	// Open the database
	if err := backend.Open(); err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	if err := backend.InitSchema(); err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Add to pool
	p.backends[project.Name] = &pooledBackend{
		backend:    backend,
		project:    project.Name,
		lastAccess: time.Now(),
		refCount:   1,
	}

	return backend, nil
}

// Release decreases the reference count for a project's database
func (p *DatabasePool) Release(projectName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if pooled, ok := p.backends[projectName]; ok {
		atomic.AddInt32(&pooled.refCount, -1)
		pooled.lastAccess = time.Now()
	}
}

// Close closes a specific project's database and removes it from the pool
func (p *DatabasePool) Close(projectName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pooled, ok := p.backends[projectName]
	if !ok {
		return nil // Not in pool
	}

	log.WithField("project", projectName).Info("Closing database from pool")

	if err := pooled.backend.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	delete(p.backends, projectName)
	return nil
}

// CloseAll closes all databases in the pool
func (p *DatabasePool) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Info("Closing all databases in pool")

	var errs []error
	for name, pooled := range p.backends {
		if err := pooled.backend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close %s: %w", name, err))
		}
	}

	p.backends = make(map[string]*pooledBackend)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}

// StartIdleCleanup starts a background goroutine that closes idle databases
func (p *DatabasePool) StartIdleCleanup() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.cleanupIdle()
			}
		}
	}()
}

// StopIdleCleanup stops the background cleanup goroutine
func (p *DatabasePool) StopIdleCleanup() {
	close(p.stopCh)
	p.wg.Wait()
}

// cleanupIdle closes databases that have been idle for too long
func (p *DatabasePool) cleanupIdle() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for name, pooled := range p.backends {
		// Skip if still in use
		if atomic.LoadInt32(&pooled.refCount) > 0 {
			continue
		}

		// Check if idle for too long
		if now.Sub(pooled.lastAccess) > p.idleTimeout {
			log.WithField("project", name).Info("Closing idle database")
			if err := pooled.backend.Close(); err != nil {
				log.WithError(err).WithField("project", name).Warn("Failed to close idle database")
			}
			delete(p.backends, name)
		}
	}
}

// evictOldest closes the oldest idle database to make room
// Must be called with lock held
func (p *DatabasePool) evictOldest() bool {
	var oldest *pooledBackend
	var oldestName string

	for name, pooled := range p.backends {
		// Skip if in use
		if atomic.LoadInt32(&pooled.refCount) > 0 {
			continue
		}

		if oldest == nil || pooled.lastAccess.Before(oldest.lastAccess) {
			oldest = pooled
			oldestName = name
		}
	}

	if oldest == nil {
		return false // All databases are in use
	}

	log.WithField("project", oldestName).Info("Evicting oldest database from pool")
	if err := oldest.backend.Close(); err != nil {
		log.WithError(err).WithField("project", oldestName).Warn("Failed to close evicted database")
	}
	delete(p.backends, oldestName)

	return true
}

// Stats returns statistics about the pool
type PoolStats struct {
	OpenDatabases   int           `json:"openDatabases"`
	MaxOpen         int           `json:"maxOpen"`
	IdleTimeout     time.Duration `json:"idleTimeout"`
	ActiveDatabases int           `json:"activeDatabases"` // Databases with refCount > 0
}

// Stats returns current pool statistics
func (p *DatabasePool) Stats() *PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	active := 0
	for _, pooled := range p.backends {
		if atomic.LoadInt32(&pooled.refCount) > 0 {
			active++
		}
	}

	return &PoolStats{
		OpenDatabases:   len(p.backends),
		MaxOpen:         p.maxOpen,
		IdleTimeout:     p.idleTimeout,
		ActiveDatabases: active,
	}
}

// IsOpen checks if a project's database is currently open
func (p *DatabasePool) IsOpen(projectName string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, ok := p.backends[projectName]
	return ok
}
