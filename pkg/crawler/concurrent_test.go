package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/stretchr/testify/assert"
)

// TestConcurrentIndexingRejected verifies that concurrent indexing operations are properly rejected
func TestConcurrentIndexingRejected(t *testing.T) {
	// Create test directory
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test")
	err := os.Mkdir(testDir, 0755)
	assert.NoError(t, err)

	// Create many test files to slow down indexing enough to test concurrency
	for i := 0; i < 500; i++ {
		subDir := filepath.Join(testDir, fmt.Sprintf("dir_%d", i))
		err := os.MkdirAll(subDir, 0755)
		assert.NoError(t, err)
		for j := 0; j < 10; j++ {
			err = os.WriteFile(filepath.Join(subDir, fmt.Sprintf("file_%d.txt", j)), []byte("test content for slowing down indexing"), 0644)
			assert.NoError(t, err)
		}
	}

	// Create database
	db, err := database.NewDiskDB(":memory:")
	assert.NoError(t, err)
	defer db.Close()

	var wg sync.WaitGroup
	var firstError, secondError error

	// Start first indexing operation
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, firstError = Index(testDir, db, nil, 0, nil)
	}()

	// Give first operation time to acquire the lock (but not finish)
	time.Sleep(10 * time.Millisecond)

	// Try to start second indexing operation (should be rejected)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, secondError = Index(testDir, db, nil, 0, nil)
	}()

	// Wait for both to complete
	wg.Wait()

	// First operation should succeed
	assert.NoError(t, firstError, "First indexing operation should succeed")

	// Second operation should fail with lock error
	assert.Error(t, secondError, "Second concurrent indexing operation should fail")
	if secondError != nil {
		assert.True(t, strings.Contains(secondError.Error(), "another indexing operation is already in progress"),
			"Error should indicate another indexing operation is in progress, got: %v", secondError)
	}
}
