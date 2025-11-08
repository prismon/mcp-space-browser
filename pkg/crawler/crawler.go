package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("crawler")
}

// Index performs filesystem indexing using stack-based DFS traversal
func Index(root string, db *database.DiskDB) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	runID := time.Now().Unix()
	stack := []string{abs}

	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
	}).Info("Starting filesystem index")

	filesProcessed := 0
	directoriesProcessed := 0
	var totalSize int64
	errors := 0
	lastProgressLog := time.Now()

	// Begin transaction for better performance
	if err := db.BeginTransaction(); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			db.RollbackTransaction()
			panic(r)
		}
	}()

	for len(stack) > 0 {
		// Pop from stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if logger.IsLevelEnabled(logrus.DebugLevel) {
			log.WithFields(logrus.Fields{
				"path":      current,
				"remaining": len(stack),
			}).Debug("Processing path")
		}

		info, err := os.Stat(current)
		if err != nil {
			errors++
			log.WithFields(logrus.Fields{
				"path":  current,
				"error": err,
			}).Error("Failed to stat path")
			continue
		}

		isDir := info.IsDir()
		parent := filepath.Dir(current)
		if parent == current {
			parent = ""
		}

		var parentPtr *string
		if parent != "" {
			parentPtr = &parent
		}

		entry := &models.Entry{
			Path:        current,
			Parent:      parentPtr,
			Size:        info.Size(),
			Kind:        "file",
			Ctime:       info.ModTime().Unix(), // Go doesn't expose ctime directly
			Mtime:       info.ModTime().Unix(),
			LastScanned: runID,
		}

		if isDir {
			entry.Kind = "directory"
		}

		if err := db.InsertOrUpdate(entry); err != nil {
			errors++
			log.WithFields(logrus.Fields{
				"path":  current,
				"error": err,
			}).Error("Failed to insert/update entry")
			continue
		}

		if isDir {
			directoriesProcessed++

			if logger.IsLevelEnabled(logrus.DebugLevel) {
				log.WithField("path", current).Debug("Scanning directory")
			}

			children, err := os.ReadDir(current)
			if err != nil {
				errors++
				log.WithFields(logrus.Fields{
					"path":  current,
					"error": err,
				}).Error("Failed to read directory")
				continue
			}

			if logger.IsLevelEnabled(logrus.TraceLevel) {
				log.WithFields(logrus.Fields{
					"path":       current,
					"childCount": len(children),
				}).Trace("Directory contents")
			}

			for _, child := range children {
				stack = append(stack, filepath.Join(current, child.Name()))
			}
		} else {
			filesProcessed++
			totalSize += info.Size()

			if logger.IsLevelEnabled(logrus.TraceLevel) {
				log.WithFields(logrus.Fields{
					"path": current,
					"size": info.Size(),
				}).Trace("File processed")
			}
		}

		// Log progress every 5 seconds
		now := time.Now()
		if now.Sub(lastProgressLog) > 5*time.Second {
			log.WithFields(logrus.Fields{
				"filesProcessed": filesProcessed,
				"directoriesProcessed": directoriesProcessed,
				"remaining":      len(stack),
			}).Info("Index progress")
			lastProgressLog = now
		}
	}

	// Commit the transaction
	if err := db.CommitTransaction(); err != nil {
		db.RollbackTransaction()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.WithFields(logrus.Fields{
		"root":                 abs,
		"filesProcessed":       filesProcessed,
		"directoriesProcessed": directoriesProcessed,
		"totalSize":            totalSize,
		"errors":               errors,
		"runID":                runID,
	}).Info("Filesystem scan complete")

	log.WithFields(logrus.Fields{
		"root":  abs,
		"runID": runID,
	}).Debug("Deleting stale entries")

	if err := db.DeleteStale(abs, runID); err != nil {
		return fmt.Errorf("failed to delete stale entries: %w", err)
	}

	log.WithField("root", abs).Debug("Computing aggregate sizes")

	if err := db.ComputeAggregates(abs); err != nil {
		return fmt.Errorf("failed to compute aggregates: %w", err)
	}

	log.WithField("root", abs).Info("Index operation complete")

	return nil
}
