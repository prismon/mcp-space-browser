package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession(t *testing.T) {
	session := NewSession("test-session-id")

	assert.Equal(t, "test-session-id", session.ID)
	assert.Empty(t, session.ActiveProject)
	assert.NotZero(t, session.CreatedAt)
	assert.NotZero(t, session.LastAccess)
	assert.NotNil(t, session.Preferences)
}

func TestSession_Touch(t *testing.T) {
	session := NewSession("test")
	originalAccess := session.LastAccess

	time.Sleep(time.Millisecond)
	session.Touch()

	assert.True(t, session.LastAccess.After(originalAccess))
}

func TestSession_HasActiveProject(t *testing.T) {
	session := NewSession("test")

	assert.False(t, session.HasActiveProject())

	session.ActiveProject = "my-project"
	assert.True(t, session.HasActiveProject())
}

func TestManager_GetOrCreate(t *testing.T) {
	manager := NewManager(time.Hour)

	// Create new session
	session1 := manager.GetOrCreate("session-1")
	assert.Equal(t, "session-1", session1.ID)

	// Get existing session
	session2 := manager.GetOrCreate("session-1")
	assert.Same(t, session1, session2)

	// Create different session
	session3 := manager.GetOrCreate("session-2")
	assert.NotSame(t, session1, session3)
}

func TestManager_Get(t *testing.T) {
	manager := NewManager(time.Hour)

	// Get non-existent session
	session := manager.Get("non-existent")
	assert.Nil(t, session)

	// Create and get session
	manager.GetOrCreate("existing")
	session = manager.Get("existing")
	assert.NotNil(t, session)
	assert.Equal(t, "existing", session.ID)
}

func TestManager_SetActiveProject(t *testing.T) {
	manager := NewManager(time.Hour)

	// Set project for new session
	err := manager.SetActiveProject("session-1", "project-a")
	require.NoError(t, err)

	project, err := manager.GetActiveProject("session-1")
	require.NoError(t, err)
	assert.Equal(t, "project-a", project)

	// Change project
	err = manager.SetActiveProject("session-1", "project-b")
	require.NoError(t, err)

	project, err = manager.GetActiveProject("session-1")
	require.NoError(t, err)
	assert.Equal(t, "project-b", project)
}

func TestManager_GetActiveProject(t *testing.T) {
	manager := NewManager(time.Hour)

	t.Run("returns error for non-existent session", func(t *testing.T) {
		_, err := manager.GetActiveProject("non-existent")
		assert.Error(t, err)

		var noSession *NoSessionError
		assert.ErrorAs(t, err, &noSession)
	})

	t.Run("returns error when no project set", func(t *testing.T) {
		manager.GetOrCreate("no-project")
		_, err := manager.GetActiveProject("no-project")
		assert.Error(t, err)

		var noProject *NoActiveProjectError
		assert.ErrorAs(t, err, &noProject)
	})

	t.Run("returns project when set", func(t *testing.T) {
		manager.SetActiveProject("with-project", "my-project")
		project, err := manager.GetActiveProject("with-project")
		require.NoError(t, err)
		assert.Equal(t, "my-project", project)
	})
}

func TestManager_ClearActiveProject(t *testing.T) {
	manager := NewManager(time.Hour)

	manager.SetActiveProject("session-1", "project-a")
	manager.ClearActiveProject("session-1")

	_, err := manager.GetActiveProject("session-1")
	assert.Error(t, err)

	var noProject *NoActiveProjectError
	assert.ErrorAs(t, err, &noProject)
}

func TestManager_Delete(t *testing.T) {
	manager := NewManager(time.Hour)

	manager.GetOrCreate("to-delete")
	assert.NotNil(t, manager.Get("to-delete"))

	manager.Delete("to-delete")
	assert.Nil(t, manager.Get("to-delete"))
}

func TestManager_Stats(t *testing.T) {
	manager := NewManager(time.Hour)

	// No sessions
	stats := manager.Stats()
	assert.Equal(t, 0, stats.TotalSessions)
	assert.Equal(t, 0, stats.SessionsWithProjects)

	// Add sessions
	manager.GetOrCreate("session-1")
	manager.GetOrCreate("session-2")
	manager.SetActiveProject("session-2", "project")

	stats = manager.Stats()
	assert.Equal(t, 2, stats.TotalSessions)
	assert.Equal(t, 1, stats.SessionsWithProjects)
}

func TestManager_ListSessions(t *testing.T) {
	manager := NewManager(time.Hour)

	manager.GetOrCreate("session-1")
	manager.GetOrCreate("session-2")

	sessions := manager.ListSessions()
	assert.Len(t, sessions, 2)

	ids := make(map[string]bool)
	for _, s := range sessions {
		ids[s.ID] = true
	}
	assert.True(t, ids["session-1"])
	assert.True(t, ids["session-2"])
}

func TestManager_Cleanup(t *testing.T) {
	// Use short timeout for testing
	manager := NewManager(10 * time.Millisecond)

	manager.GetOrCreate("old-session")

	// Wait for session to become stale
	time.Sleep(20 * time.Millisecond)

	// Trigger cleanup
	manager.cleanup()

	// Session should be gone
	assert.Nil(t, manager.Get("old-session"))
}

func TestErrors(t *testing.T) {
	t.Run("NoSessionError", func(t *testing.T) {
		err := &NoSessionError{SessionID: "test-id"}
		assert.Contains(t, err.Error(), "test-id")
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("NoActiveProjectError", func(t *testing.T) {
		err := &NoActiveProjectError{SessionID: "test-id"}
		assert.Contains(t, err.Error(), "test-id")
		assert.Contains(t, err.Error(), "no active project")
		assert.Contains(t, err.Error(), "project-open")
	})
}
