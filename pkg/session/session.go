package session

import (
	"sync"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("session")
}

// DefaultIdleTimeout is the default idle timeout for sessions
const DefaultIdleTimeout = time.Hour

// Session represents a client session with its associated state
type Session struct {
	// ID is the unique session identifier (from Mcp-Session-Id header)
	ID string `json:"id"`

	// ActiveProject is the currently selected project name
	ActiveProject string `json:"activeProject,omitempty"`

	// CreatedAt is when the session was created
	CreatedAt time.Time `json:"createdAt"`

	// LastAccess is when the session was last accessed
	LastAccess time.Time `json:"lastAccess"`

	// Preferences stores session-specific preferences
	Preferences map[string]interface{} `json:"preferences,omitempty"`
}

// NewSession creates a new session with the given ID
func NewSession(id string) *Session {
	now := time.Now()
	return &Session{
		ID:          id,
		CreatedAt:   now,
		LastAccess:  now,
		Preferences: make(map[string]interface{}),
	}
}

// Touch updates the last access time
func (s *Session) Touch() {
	s.LastAccess = time.Now()
}

// HasActiveProject returns true if the session has an active project
func (s *Session) HasActiveProject() bool {
	return s.ActiveProject != ""
}

// Manager manages sessions and their state
type Manager struct {
	sessions    map[string]*Session
	mu          sync.RWMutex
	idleTimeout time.Duration
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewManager creates a new session manager
func NewManager(idleTimeout time.Duration) *Manager {
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}

	return &Manager{
		sessions:    make(map[string]*Session),
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}
}

// GetOrCreate returns an existing session or creates a new one
func (m *Manager) GetOrCreate(sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[sessionID]; ok {
		session.Touch()
		return session
	}

	session := NewSession(sessionID)
	m.sessions[sessionID] = session
	log.WithField("sessionID", sessionID).Debug("Created new session")

	return session
}

// Get returns a session by ID, or nil if not found
func (m *Manager) Get(sessionID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, ok := m.sessions[sessionID]; ok {
		session.Touch()
		return session
	}
	return nil
}

// SetActiveProject sets the active project for a session
func (m *Manager) SetActiveProject(sessionID, projectName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		session = NewSession(sessionID)
		m.sessions[sessionID] = session
	}

	session.ActiveProject = projectName
	session.Touch()

	log.WithFields(logrus.Fields{
		"sessionID": sessionID,
		"project":   projectName,
	}).Debug("Set active project for session")

	return nil
}

// GetActiveProject returns the active project for a session
func (m *Manager) GetActiveProject(sessionID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return "", &NoSessionError{SessionID: sessionID}
	}

	if session.ActiveProject == "" {
		return "", &NoActiveProjectError{SessionID: sessionID}
	}

	session.Touch()
	return session.ActiveProject, nil
}

// ClearActiveProject clears the active project for a session
func (m *Manager) ClearActiveProject(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[sessionID]; ok {
		session.ActiveProject = ""
		session.Touch()
	}
}

// Delete removes a session
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
	log.WithField("sessionID", sessionID).Debug("Deleted session")
}

// StartCleanup starts a background goroutine that removes stale sessions
func (m *Manager) StartCleanup() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.cleanup()
			}
		}
	}()
}

// StopCleanup stops the background cleanup goroutine
func (m *Manager) StopCleanup() {
	close(m.stopCh)
	m.wg.Wait()
}

// cleanup removes sessions that have been idle for too long
func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, session := range m.sessions {
		if now.Sub(session.LastAccess) > m.idleTimeout {
			delete(m.sessions, id)
			log.WithField("sessionID", id).Debug("Cleaned up stale session")
		}
	}
}

// Stats returns statistics about active sessions
type Stats struct {
	TotalSessions        int `json:"totalSessions"`
	SessionsWithProjects int `json:"sessionsWithProjects"`
}

// Stats returns current session statistics
func (m *Manager) Stats() *Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	withProjects := 0
	for _, session := range m.sessions {
		if session.ActiveProject != "" {
			withProjects++
		}
	}

	return &Stats{
		TotalSessions:        len(m.sessions),
		SessionsWithProjects: withProjects,
	}
}

// ListSessions returns all active sessions
func (m *Manager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// NoSessionError is returned when a session is not found
type NoSessionError struct {
	SessionID string
}

func (e *NoSessionError) Error() string {
	return "session not found: " + e.SessionID
}

// NoActiveProjectError is returned when no active project is set
type NoActiveProjectError struct {
	SessionID string
}

func (e *NoActiveProjectError) Error() string {
	return "no active project for session: " + e.SessionID + ". Use project-open to select a project."
}
