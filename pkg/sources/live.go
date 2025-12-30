package sources

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// LifecycleTrigger interface for triggering lifecycle plans
type LifecycleTrigger interface {
	TriggerOnAdd(ctx context.Context, entries []*models.Entry) error
	TriggerOnRemove(ctx context.Context, entries []*models.Entry) error
}

// LiveFilesystemSource watches a filesystem directory for changes in real-time
type LiveFilesystemSource struct {
	config           *SourceConfig
	liveConfig       *LiveFilesystemConfig
	db               *sql.DB
	watcher          *fsnotify.Watcher
	ruleExecutor     RuleExecutor
	lifecycleTrigger LifecycleTrigger
	stats            *SourceStats
	status           SourceStatus
	mu               sync.RWMutex
	stopChan         chan struct{}
	doneChan         chan struct{}
	log              *logrus.Entry
	eventQueue       chan FilesystemEvent
	debounceMap      map[string]*time.Timer
	debounceMu       sync.Mutex
}

// NewLiveFilesystemSource creates a new live filesystem source
func NewLiveFilesystemSource(config *SourceConfig, db *sql.DB) (*LiveFilesystemSource, error) {
	// Parse live config
	liveConfig, err := UnmarshalLiveConfig(config.ConfigJSON)
	if err != nil {
		// Use defaults if config is empty
		liveConfig = &LiveFilesystemConfig{
			WatchRecursive: true,
			DebounceMs:     500,
			BatchSize:      100,
		}
	}

	// Set defaults
	if liveConfig.DebounceMs == 0 {
		liveConfig.DebounceMs = 500
	}
	if liveConfig.BatchSize == 0 {
		liveConfig.BatchSize = 100
	}

	return &LiveFilesystemSource{
		config:      config,
		liveConfig:  liveConfig,
		db:          db,
		stats:       &SourceStats{},
		status:      SourceStatusStopped,
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
		log:         logrus.WithField("source", config.Name),
		eventQueue:  make(chan FilesystemEvent, liveConfig.BatchSize),
		debounceMap: make(map[string]*time.Timer),
	}, nil
}

// Start begins watching the filesystem
func (s *LiveFilesystemSource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == SourceStatusRunning {
		return fmt.Errorf("source already running")
	}

	s.log.WithField("path", s.config.RootPath).Info("Starting live filesystem source")
	s.status = SourceStatusStarting

	// Create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.status = SourceStatusError
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	s.watcher = watcher

	// Add root path to watcher
	if err := s.watcher.Add(s.config.RootPath); err != nil {
		s.watcher.Close()
		s.status = SourceStatusError
		return fmt.Errorf("failed to watch root path: %w", err)
	}

	// If recursive, walk and add all subdirectories
	if s.liveConfig.WatchRecursive {
		if err := s.addRecursiveWatches(s.config.RootPath); err != nil {
			s.log.WithError(err).Warn("Failed to add some recursive watches")
		}
	}

	// Perform initial scan
	s.log.Info("Performing initial filesystem scan")
	if err := s.performInitialScan(); err != nil {
		s.log.WithError(err).Warn("Initial scan failed")
	}

	s.status = SourceStatusRunning
	s.stats.LastUpdate = time.Now()

	// Start event processing goroutine
	go s.processEvents(ctx)

	// Start watch goroutine
	go s.watchLoop(ctx)

	s.log.Info("Live filesystem source started successfully")
	return nil
}

// Stop stops watching the filesystem
func (s *LiveFilesystemSource) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != SourceStatusRunning {
		return fmt.Errorf("source not running")
	}

	s.log.Info("Stopping live filesystem source")
	s.status = SourceStatusStopping

	// Signal stop
	close(s.stopChan)

	// Wait for goroutines to finish with timeout
	select {
	case <-s.doneChan:
		s.log.Debug("Watch loop stopped cleanly")
	case <-time.After(5 * time.Second):
		s.log.Warn("Watch loop did not stop within timeout")
	}

	// Close watcher
	if s.watcher != nil {
		s.watcher.Close()
	}

	s.status = SourceStatusStopped
	s.log.Info("Live filesystem source stopped")
	return nil
}

// Config returns the source configuration
func (s *LiveFilesystemSource) Config() *SourceConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Stats returns current statistics
func (s *LiveFilesystemSource) Stats() *SourceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	statsCopy := *s.stats
	return &statsCopy
}

// Status returns the current status
func (s *LiveFilesystemSource) Status() SourceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetRuleExecutor sets the rule executor for this source
func (s *LiveFilesystemSource) SetRuleExecutor(executor RuleExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ruleExecutor = executor
}

// SetLifecycleTrigger sets the lifecycle trigger for this source
func (s *LiveFilesystemSource) SetLifecycleTrigger(trigger LifecycleTrigger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lifecycleTrigger = trigger
}

// watchLoop is the main event loop that watches for filesystem changes
func (s *LiveFilesystemSource) watchLoop(ctx context.Context) {
	defer close(s.doneChan)

	for {
		select {
		case <-s.stopChan:
			return

		case <-ctx.Done():
			return

		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			s.handleFsnotifyEvent(event)

		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			s.log.WithError(err).Error("Watcher error")
			s.mu.Lock()
			s.stats.ErrorCount++
			s.stats.LastError = err.Error()
			s.mu.Unlock()
		}
	}
}

// handleFsnotifyEvent converts fsnotify events to our internal event format
func (s *LiveFilesystemSource) handleFsnotifyEvent(event fsnotify.Event) {
	s.log.WithFields(logrus.Fields{
		"path": event.Name,
		"op":   event.Op.String(),
	}).Trace("Received filesystem event")

	var fsEvent FilesystemEvent
	fsEvent.Path = event.Name
	fsEvent.Time = time.Now()

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		fsEvent.Type = EventTypeCreate
		// If it's a new directory and recursive watching is enabled, add it to watches
		if s.liveConfig.WatchRecursive {
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				s.watcher.Add(event.Name)
			}
		}

	case event.Op&fsnotify.Write == fsnotify.Write:
		fsEvent.Type = EventTypeModify

	case event.Op&fsnotify.Remove == fsnotify.Remove:
		fsEvent.Type = EventTypeDelete

	case event.Op&fsnotify.Rename == fsnotify.Rename:
		fsEvent.Type = EventTypeRename

	default:
		return // Ignore other event types
	}

	// Debounce the event
	s.debounceEvent(fsEvent)
}

// debounceEvent debounces filesystem events to avoid processing too many rapid changes
func (s *LiveFilesystemSource) debounceEvent(event FilesystemEvent) {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()

	// Cancel existing timer for this path
	if timer, exists := s.debounceMap[event.Path]; exists {
		timer.Stop()
	}

	// Create new debounce timer
	s.debounceMap[event.Path] = time.AfterFunc(
		time.Duration(s.liveConfig.DebounceMs)*time.Millisecond,
		func() {
			// Send to event queue after debounce period
			select {
			case s.eventQueue <- event:
			default:
				s.log.Warn("Event queue full, dropping event")
			}

			// Remove from debounce map
			s.debounceMu.Lock()
			delete(s.debounceMap, event.Path)
			s.debounceMu.Unlock()
		},
	)
}

// processEvents processes debounced events from the queue
func (s *LiveFilesystemSource) processEvents(ctx context.Context) {
	for {
		select {
		case <-s.stopChan:
			return

		case <-ctx.Done():
			return

		case event := <-s.eventQueue:
			if err := s.handleEvent(ctx, event); err != nil {
				s.log.WithError(err).WithField("path", event.Path).Error("Failed to handle event")
				s.mu.Lock()
				s.stats.ErrorCount++
				s.stats.LastError = err.Error()
				s.mu.Unlock()
			}
		}
	}
}

// handleEvent processes a filesystem event
func (s *LiveFilesystemSource) handleEvent(ctx context.Context, event FilesystemEvent) error {
	s.log.WithFields(logrus.Fields{
		"type": event.Type,
		"path": event.Path,
	}).Debug("Processing filesystem event")

	switch event.Type {
	case EventTypeCreate, EventTypeModify:
		return s.handleCreateOrModify(ctx, event.Path)

	case EventTypeDelete:
		return s.handleDelete(event.Path)

	case EventTypeRename:
		// For renames, we treat them as delete + create
		// fsnotify doesn't give us the new name, so we just handle the delete
		return s.handleDelete(event.Path)

	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

// handleCreateOrModify handles file creation or modification
func (s *LiveFilesystemSource) handleCreateOrModify(ctx context.Context, path string) error {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted between event and processing
			return s.handleDelete(path)
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Create entry
	entry := &models.Entry{
		Path:        path,
		Size:        info.Size(),
		Ctime:       info.ModTime().Unix(),
		Mtime:       info.ModTime().Unix(),
		LastScanned: time.Now().Unix(),
	}

	// Set kind
	if info.IsDir() {
		entry.Kind = "directory"
	} else {
		entry.Kind = "file"
	}

	// Set parent
	parent := filepath.Dir(path)
	if parent != "." && parent != "/" {
		entry.Parent = &parent
	}

	// Insert or update in database
	if err := s.insertOrUpdateEntry(entry); err != nil {
		return fmt.Errorf("failed to insert/update entry: %w", err)
	}

	// Update stats
	s.mu.Lock()
	if info.IsDir() {
		s.stats.DirsIndexed++
	} else {
		s.stats.FilesIndexed++
	}
	s.stats.BytesIndexed += info.Size()
	s.stats.LastUpdate = time.Now()
	s.mu.Unlock()

	// Execute rules if rule executor is set
	if s.ruleExecutor != nil {
		if err := s.ruleExecutor.ExecuteRulesForPath(ctx, path); err != nil {
			s.log.WithError(err).Warn("Failed to execute rules for path")
		} else {
			s.mu.Lock()
			s.stats.RulesExecuted++
			s.mu.Unlock()
		}
	}

	// Trigger lifecycle "added" plan
	if s.lifecycleTrigger != nil {
		go func() {
			if err := s.lifecycleTrigger.TriggerOnAdd(ctx, []*models.Entry{entry}); err != nil {
				s.log.WithError(err).Warn("Failed to trigger lifecycle plan for added entry")
			}
		}()
	}

	return nil
}

// handleDelete handles file deletion
func (s *LiveFilesystemSource) handleDelete(path string) error {
	// Create entry for lifecycle trigger (before deletion)
	entry := &models.Entry{
		Path: path,
	}

	// Delete entry from database
	if err := s.deleteEntry(path); err != nil {
		return fmt.Errorf("failed to delete entry: %w", err)
	}

	s.log.WithField("path", path).Debug("Deleted entry from database")

	// Trigger lifecycle "removed" plan
	if s.lifecycleTrigger != nil {
		go func() {
			ctx := context.Background()
			if err := s.lifecycleTrigger.TriggerOnRemove(ctx, []*models.Entry{entry}); err != nil {
				s.log.WithError(err).Warn("Failed to trigger lifecycle plan for removed entry")
			}
		}()
	}

	return nil
}

// performInitialScan performs an initial scan of the watched directory
func (s *LiveFilesystemSource) performInitialScan() error {
	runID := time.Now().Unix()

	return filepath.Walk(s.config.RootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			s.log.WithError(err).WithField("path", path).Warn("Error walking path")
			return nil // Continue walking
		}

		// Create entry
		entry := &models.Entry{
			Path:        path,
			Size:        info.Size(),
			Ctime:       info.ModTime().Unix(),
			Mtime:       info.ModTime().Unix(),
			LastScanned: runID,
		}

		if info.IsDir() {
			entry.Kind = "directory"
		} else {
			entry.Kind = "file"
		}

		parent := filepath.Dir(path)
		if parent != "." && parent != "/" {
			entry.Parent = &parent
		}

		// Insert or update
		if err := s.insertOrUpdateEntry(entry); err != nil {
			s.log.WithError(err).WithField("path", path).Warn("Failed to insert entry")
		}

		// Update stats
		s.mu.Lock()
		if info.IsDir() {
			s.stats.DirsIndexed++
		} else {
			s.stats.FilesIndexed++
		}
		s.stats.BytesIndexed += info.Size()
		s.mu.Unlock()

		return nil
	})
}

// addRecursiveWatches adds watches for all subdirectories
func (s *LiveFilesystemSource) addRecursiveWatches(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue
		}

		if !info.IsDir() {
			return nil
		}

		if err := s.watcher.Add(path); err != nil {
			s.log.WithError(err).WithField("path", path).Warn("Failed to add watch")
		}

		return nil
	})
}

// Database helper methods

func (s *LiveFilesystemSource) insertOrUpdateEntry(entry *models.Entry) error {
	_, err := s.db.Exec(`
		INSERT INTO entries (path, parent, size, kind, ctime, mtime, last_scanned, dirty)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(path) DO UPDATE SET
			parent=excluded.parent,
			size=excluded.size,
			kind=excluded.kind,
			ctime=excluded.ctime,
			mtime=excluded.mtime,
			last_scanned=excluded.last_scanned,
			dirty=0
	`, entry.Path, entry.Parent, entry.Size, entry.Kind, entry.Ctime, entry.Mtime, entry.LastScanned)

	return err
}

func (s *LiveFilesystemSource) deleteEntry(path string) error {
	_, err := s.db.Exec(`DELETE FROM entries WHERE path = ? OR path LIKE ?`, path, path+"/%")
	return err
}
