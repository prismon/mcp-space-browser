package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerProjectTools registers all project management MCP tools
func registerProjectTools(s *server.MCPServer, sc *ServerContext) {
	registerProjectCreate(s, sc)
	registerProjectDelete(s, sc)
	registerProjectList(s, sc)
	registerProjectGet(s, sc)
	registerProjectOpen(s, sc)
	registerProjectClose(s, sc)
}

// project-create: Create a new project
func registerProjectCreate(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(mcp.Tool{
		Name:        "project-create",
		Description: "Create a new project with its own database. Projects are isolated workspaces for indexing and managing files.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Unique project name (alphanumeric, underscore, hyphen)",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Optional description of the project",
				},
			},
			Required: []string{"name"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		project, err := sc.ProjectManager.CreateProject(args.Name, args.Description)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create project: %v", err)), nil
		}

		result := map[string]interface{}{
			"success": true,
			"project": map[string]interface{}{
				"name":        project.Name,
				"description": project.Description,
				"path":        project.Path,
				"createdAt":   project.CreatedAt,
			},
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// project-delete: Delete a project
func registerProjectDelete(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(mcp.Tool{
		Name:        "project-delete",
		Description: "Delete a project and all its data. This action is irreversible.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the project to delete",
				},
				"confirm": map[string]interface{}{
					"type":        "boolean",
					"description": "Must be true to confirm deletion",
				},
			},
			Required: []string{"name", "confirm"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name    string `json:"name"`
			Confirm bool   `json:"confirm"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if !args.Confirm {
			return mcp.NewToolResultError("Deletion not confirmed. Set confirm=true to delete the project."), nil
		}

		if err := sc.ProjectManager.DeleteProject(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete project: %v", err)), nil
		}

		result := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Project '%s' deleted successfully", args.Name),
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// project-list: List all projects
func registerProjectList(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(mcp.Tool{
		Name:        "project-list",
		Description: "List all available projects",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := sc.ProjectManager.ListProjects()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list projects: %v", err)), nil
		}

		// Get current session's active project
		activeProject := ""
		sessionID := GetSessionID(ctx)
		if sessionID != "" {
			activeProject, _ = sc.SessionManager.GetActiveProject(sessionID)
		}

		projectList := make([]map[string]interface{}, 0, len(projects))
		for _, p := range projects {
			projectList = append(projectList, map[string]interface{}{
				"name":        p.Name,
				"description": p.Description,
				"createdAt":   p.CreatedAt,
				"updatedAt":   p.UpdatedAt,
				"isActive":    p.Name == activeProject,
			})
		}

		result := map[string]interface{}{
			"projects":      projectList,
			"count":         len(projectList),
			"activeProject": activeProject,
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// project-get: Get information about a project
func registerProjectGet(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(mcp.Tool{
		Name:        "project-get",
		Description: "Get detailed information about a project",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Project name (optional, defaults to active project)",
				},
			},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Use active project if name not specified
		projectName := args.Name
		if projectName == "" {
			sessionID := GetSessionID(ctx)
			if sessionID != "" {
				var err error
				projectName, err = sc.SessionManager.GetActiveProject(sessionID)
				if err != nil {
					return mcp.NewToolResultError("No project specified and no active project. Use project-open to select a project."), nil
				}
			} else {
				return mcp.NewToolResultError("No project specified. Provide 'name' parameter or use project-open to select a project."), nil
			}
		}

		project, err := sc.ProjectManager.GetProject(projectName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get project: %v", err)), nil
		}

		stats, err := project.Stats()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get project stats: %v", err)), nil
		}

		// Check if database is open
		isOpen := sc.ProjectManager.Pool().IsOpen(projectName)

		result := map[string]interface{}{
			"name":           project.Name,
			"description":    project.Description,
			"path":           project.Path,
			"databasePath":   stats.DatabasePath,
			"databaseSizeKB": stats.DatabaseSizeKB,
			"backendType":    stats.BackendType,
			"isOpen":         isOpen,
			"createdAt":      project.CreatedAt,
			"updatedAt":      project.UpdatedAt,
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// project-open: Set active project for session
func registerProjectOpen(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(mcp.Tool{
		Name:        "project-open",
		Description: "Set the active project for this session. All subsequent operations will use this project's database.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the project to open",
				},
			},
			Required: []string{"name"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Verify project exists
		if !sc.ProjectManager.ProjectExists(args.Name) {
			return mcp.NewToolResultError(fmt.Sprintf("Project not found: %s", args.Name)), nil
		}

		// Set active project
		if err := sc.SetActiveProject(ctx, args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to set active project: %v", err)), nil
		}

		// Open database (auto-open via pool)
		_, err := sc.ProjectManager.GetProjectDB(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to open project database: %v", err)), nil
		}

		result := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Project '%s' is now active", args.Name),
			"project": args.Name,
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// project-close: Close the active project
func registerProjectClose(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(mcp.Tool{
		Name:        "project-close",
		Description: "Close the active project and release resources. You will need to use project-open before performing other operations.",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := GetSessionID(ctx)
		if sessionID == "" {
			return mcp.NewToolResultError("No session ID available"), nil
		}

		projectName, err := sc.SessionManager.GetActiveProject(sessionID)
		if err != nil {
			return mcp.NewToolResultError("No active project to close"), nil
		}

		// Clear active project from session
		sc.ClearActiveProject(ctx)

		// Optionally close the database (release resources)
		// Note: The pool will handle idle cleanup, but we can release our reference
		sc.ProjectManager.ReleaseProjectDB(projectName)

		result := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Project '%s' closed", projectName),
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}
