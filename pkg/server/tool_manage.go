package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

var manageToolDef = mcp.NewTool("manage",
	mcp.WithDescription("CRUD operations for organizational entities: resource-sets, plans, sources, and jobs."),
	mcp.WithString("entity",
		mcp.Required(),
		mcp.Description("Entity type: resource-set, plan, job"),
		mcp.Enum("resource-set", "plan", "job"),
	),
	mcp.WithString("action",
		mcp.Required(),
		mcp.Description("Action: create, get, list, update, delete"),
		mcp.Enum("create", "get", "list", "update", "delete"),
	),
	mcp.WithString("name",
		mcp.Description("Entity name (for create, get, update, delete)"),
	),
	mcp.WithString("description",
		mcp.Description("Entity description (for create, update)"),
	),
	mcp.WithString("parent",
		mcp.Description("Parent resource-set name (for DAG edge operations)"),
	),
	mcp.WithString("child",
		mcp.Description("Child resource-set name (for DAG edge operations)"),
	),
	mcp.WithString("mode",
		mcp.Description("Plan mode: oneshot or continuous"),
	),
	mcp.WithString("status",
		mcp.Description("Filter by status (for job list)"),
	),
	mcp.WithNumber("id",
		mcp.Description("Entity ID (for job get)"),
	),
	mcp.WithNumber("limit",
		mcp.Description("Max results for list actions (default 100)"),
	),
	mcp.WithString("cursor",
		mcp.Description("Pagination cursor for list actions"),
	),
)

func registerManageTool(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(manageToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleManage(ctx, request, db)
	})
}

func registerManageToolMP(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(manageToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		return handleManage(ctx, request, db)
	})
}

func handleManage(ctx context.Context, request mcp.CallToolRequest, db *database.DiskDB) (*mcp.CallToolResult, error) {
	var args struct {
		Entity      string  `json:"entity"`
		Action      string  `json:"action"`
		Name        string  `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		Parent      string  `json:"parent,omitempty"`
		Child       string  `json:"child,omitempty"`
		Mode        string  `json:"mode,omitempty"`
		Status      string  `json:"status,omitempty"`
		ID          *int64  `json:"id,omitempty"`
		Limit       *int    `json:"limit,omitempty"`
		Cursor      string  `json:"cursor,omitempty"`
	}

	if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	rawArgs, _ := request.Params.Arguments.(map[string]interface{})
	if rawArgs == nil {
		rawArgs = map[string]interface{}{}
	}

	switch args.Entity {
	case "resource-set":
		return handleManageResourceSet(db, args.Action, args.Name, args.Description, args.Parent, args.Child, args.Limit, args.Cursor)
	case "plan":
		return handleManagePlan(db, rawArgs, args.Action, args.Name, args.Description, args.Mode, args.Limit, args.Cursor)
	case "job":
		return handleManageJob(db, args.Action, args.ID, args.Status, args.Limit, args.Cursor)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown entity type: %q", args.Entity)), nil
	}
}

func handleManageResourceSet(db *database.DiskDB, action, name string, description *string, parent, child string, limit *int, cursor string) (*mcp.CallToolResult, error) {
	switch action {
	case "create":
		if name == "" {
			return mcp.NewToolResultError("name is required for create"), nil
		}
		rs := &models.ResourceSet{Name: name, Description: description}
		id, err := db.CreateResourceSet(rs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create resource set: %v", err)), nil
		}
		return jsonResult(map[string]interface{}{"id": id, "name": name, "status": "created"})

	case "get":
		if name == "" {
			return mcp.NewToolResultError("name is required for get"), nil
		}
		rs, err := db.GetResourceSet(name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Resource set not found: %v", err)), nil
		}
		return jsonResult(rs)

	case "list":
		sets, err := db.ListResourceSets()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list resource sets: %v", err)), nil
		}
		lim := 100
		if limit != nil && *limit > 0 {
			lim = *limit
		}
		offset := 0
		if cursor != "" {
			offset = decodeCursorOffset(cursor)
		}
		total := len(sets)
		end := offset + lim
		if end > total {
			end = total
		}
		page := sets
		if offset < total {
			page = sets[offset:end]
		} else {
			page = []*models.ResourceSet{}
		}
		resp := map[string]interface{}{
			"items": page,
			"total": total,
		}
		if end < total {
			resp["next_cursor"] = encodeCursor(end)
		}
		return jsonResult(resp)

	case "update":
		if name == "" {
			return mcp.NewToolResultError("name is required for update"), nil
		}
		if err := db.UpdateResourceSet(name, description); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update resource set: %v", err)), nil
		}

		// Handle DAG edge operations
		if parent != "" {
			if err := addResourceSetEdge(db, parent, name); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to add parent edge: %v", err)), nil
			}
		}
		if child != "" {
			if err := addResourceSetEdge(db, name, child); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to add child edge: %v", err)), nil
			}
		}

		return jsonResult(map[string]interface{}{"name": name, "status": "updated"})

	case "delete":
		if name == "" {
			return mcp.NewToolResultError("name is required for delete"), nil
		}
		if err := db.DeleteResourceSet(name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete resource set: %v", err)), nil
		}
		return jsonResult(map[string]interface{}{"name": name, "status": "deleted"})

	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown action %q for resource-set", action)), nil
	}
}

func addResourceSetEdge(db *database.DiskDB, parentName, childName string) error {
	return db.AddResourceSetEdge(parentName, childName)
}

func handleManagePlan(db *database.DiskDB, rawArgs map[string]interface{}, action, name string, description *string, mode string, limit *int, cursor string) (*mcp.CallToolResult, error) {
	switch action {
	case "create":
		if name == "" {
			return mcp.NewToolResultError("name is required for create"), nil
		}
		plan := &models.Plan{
			Name:        name,
			Description: description,
			Mode:        "oneshot",
			Status:      "active",
		}
		if mode != "" {
			plan.Mode = mode
		}

		// Handle sources if provided
		if sourcesRaw, ok := rawArgs["sources"]; ok {
			sourcesJSON, err := json.Marshal(sourcesRaw)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid sources: %v", err)), nil
			}
			var sources []models.PlanSource
			if err := json.Unmarshal(sourcesJSON, &sources); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid sources format: %v", err)), nil
			}
			plan.Sources = sources
		}

		// Handle outcomes if provided
		if outcomesRaw, ok := rawArgs["outcomes"]; ok {
			outcomesJSON, err := json.Marshal(outcomesRaw)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid outcomes: %v", err)), nil
			}
			var outcomes []models.RuleOutcome
			if err := json.Unmarshal(outcomesJSON, &outcomes); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid outcomes format: %v", err)), nil
			}
			plan.Outcomes = outcomes
		}

		if err := db.CreatePlan(plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create plan: %v", err)), nil
		}
		return jsonResult(map[string]interface{}{"name": name, "status": "created"})

	case "get":
		if name == "" {
			return mcp.NewToolResultError("name is required for get"), nil
		}
		plan, err := db.GetPlan(name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plan not found: %v", err)), nil
		}
		return jsonResult(plan)

	case "list":
		plans, err := db.ListPlans()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list plans: %v", err)), nil
		}
		lim := 100
		if limit != nil && *limit > 0 {
			lim = *limit
		}
		offset := 0
		if cursor != "" {
			offset = decodeCursorOffset(cursor)
		}
		total := len(plans)
		end := offset + lim
		if end > total {
			end = total
		}
		page := plans
		if offset < total {
			page = plans[offset:end]
		} else {
			page = []*models.Plan{}
		}
		resp := map[string]interface{}{
			"items": page,
			"total": total,
		}
		if end < total {
			resp["next_cursor"] = encodeCursor(end)
		}
		return jsonResult(resp)

	case "update":
		if name == "" {
			return mcp.NewToolResultError("name is required for update"), nil
		}
		plan, err := db.GetPlan(name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plan not found: %v", err)), nil
		}
		if description != nil {
			plan.Description = description
		}
		if mode != "" {
			plan.Mode = mode
		}
		if err := db.UpdatePlan(plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update plan: %v", err)), nil
		}
		return jsonResult(map[string]interface{}{"name": name, "status": "updated"})

	case "delete":
		if name == "" {
			return mcp.NewToolResultError("name is required for delete"), nil
		}
		if err := db.DeletePlan(name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete plan: %v", err)), nil
		}
		return jsonResult(map[string]interface{}{"name": name, "status": "deleted"})

	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown action %q for plan", action)), nil
	}
}

func handleManageJob(db *database.DiskDB, action string, id *int64, status string, limit *int, cursor string) (*mcp.CallToolResult, error) {
	switch action {
	case "get":
		if id == nil {
			return mcp.NewToolResultError("id is required for job get"), nil
		}
		job, err := db.GetIndexJob(*id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Job not found: %v", err)), nil
		}
		return jsonResult(job)

	case "list":
		lim := 100
		if limit != nil && *limit > 0 {
			lim = *limit
		}
		var statusPtr *string
		if status != "" {
			statusPtr = &status
		}
		jobs, err := db.ListIndexJobs(statusPtr, lim)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
		}
		return jsonResult(map[string]interface{}{
			"items": jobs,
			"total": len(jobs),
		})

	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown action %q for job (supported: get, list)", action)), nil
	}
}

func jsonResult(data interface{}) (*mcp.CallToolResult, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("JSON marshal error: %v", err)), nil
	}
	return mcp.NewToolResultText(string(payload)), nil
}

func decodeCursorOffset(cursor string) int {
	offset, err := decodeCursor(cursor)
	if err != nil {
		return 0
	}
	return offset
}
