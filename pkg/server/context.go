package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prismon/mcp-space-browser/pkg/auth"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/project"
	"github.com/prismon/mcp-space-browser/pkg/session"
)

// contextKey is a type for context keys used in this package
type contextKey string

const (
	// sessionIDKey is the context key for session ID
	sessionIDKey contextKey = "sessionID"
	// requestKey is the context key for the HTTP request
	requestKey contextKey = "httpRequest"
)

// ServerContext holds shared state for the MCP server
type ServerContext struct {
	// ProjectManager manages projects and their databases
	ProjectManager *project.Manager

	// SessionManager manages client sessions
	SessionManager *session.Manager

	// Config holds the server configuration
	Config *auth.Config

	// CacheDir is the path to the shared cache directory
	CacheDir string

	// ContentBaseURL is the base URL for serving content
	ContentBaseURL string
}

// NewServerContext creates a new server context
func NewServerContext(config *auth.Config, projectsPath, cachePath string) (*ServerContext, error) {
	// Create project manager
	projectManager, err := project.NewManager(projectsPath, cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create project manager: %w", err)
	}

	// Create session manager
	sessionManager := session.NewManager(session.DefaultIdleTimeout)
	sessionManager.StartCleanup()

	return &ServerContext{
		ProjectManager: projectManager,
		SessionManager: sessionManager,
		Config:         config,
		CacheDir:       cachePath,
		ContentBaseURL: config.Server.BaseURL,
	}, nil
}

// Close shuts down the server context
func (sc *ServerContext) Close() error {
	sc.SessionManager.StopCleanup()
	return sc.ProjectManager.Close()
}

// GetSessionID extracts the session ID from the context
func GetSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value(sessionIDKey).(string); ok {
		return sessionID
	}

	// Try to get from HTTP request header
	if req, ok := ctx.Value(requestKey).(*http.Request); ok {
		return req.Header.Get("Mcp-Session-Id")
	}

	return ""
}

// WithSessionID adds a session ID to the context
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// WithRequest adds an HTTP request to the context
func WithRequest(ctx context.Context, req *http.Request) context.Context {
	return context.WithValue(ctx, requestKey, req)
}

// GetProjectDB resolves the database backend for the current session's active project
func (sc *ServerContext) GetProjectDB(ctx context.Context) (database.Backend, error) {
	sessionID := GetSessionID(ctx)
	if sessionID == "" {
		return nil, &NoSessionIDError{}
	}

	projectName, err := sc.SessionManager.GetActiveProject(sessionID)
	if err != nil {
		return nil, err
	}

	return sc.ProjectManager.GetProjectDB(projectName)
}

// GetActiveProject returns the active project for the current session
func (sc *ServerContext) GetActiveProject(ctx context.Context) (*project.Project, error) {
	sessionID := GetSessionID(ctx)
	if sessionID == "" {
		return nil, &NoSessionIDError{}
	}

	projectName, err := sc.SessionManager.GetActiveProject(sessionID)
	if err != nil {
		return nil, err
	}

	return sc.ProjectManager.GetProject(projectName)
}

// SetActiveProject sets the active project for the current session
func (sc *ServerContext) SetActiveProject(ctx context.Context, projectName string) error {
	sessionID := GetSessionID(ctx)
	if sessionID == "" {
		return &NoSessionIDError{}
	}

	// Verify project exists
	if !sc.ProjectManager.ProjectExists(projectName) {
		return fmt.Errorf("project not found: %s", projectName)
	}

	return sc.SessionManager.SetActiveProject(sessionID, projectName)
}

// ClearActiveProject clears the active project for the current session
func (sc *ServerContext) ClearActiveProject(ctx context.Context) {
	sessionID := GetSessionID(ctx)
	if sessionID != "" {
		sc.SessionManager.ClearActiveProject(sessionID)
	}
}

// ReleaseProjectDB releases the project database back to the pool
func (sc *ServerContext) ReleaseProjectDB(ctx context.Context) {
	sessionID := GetSessionID(ctx)
	if sessionID == "" {
		return
	}

	projectName, err := sc.SessionManager.GetActiveProject(sessionID)
	if err != nil {
		return
	}

	sc.ProjectManager.ReleaseProjectDB(projectName)
}

// NoSessionIDError is returned when no session ID is available
type NoSessionIDError struct{}

func (e *NoSessionIDError) Error() string {
	return "no session ID in request context. Ensure the Mcp-Session-Id header is set."
}
