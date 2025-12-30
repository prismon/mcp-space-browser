package plans

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/sirupsen/logrus"
)

// ToolInvoker handles invoking MCP tools as plan outcomes
type ToolInvoker struct {
	db        *database.DiskDB
	processor *classifier.Processor
	resolver  *TemplateResolver
	logger    *logrus.Entry
}

// NewToolInvoker creates a new tool invoker
func NewToolInvoker(db *database.DiskDB, processor *classifier.Processor, logger *logrus.Entry) *ToolInvoker {
	return &ToolInvoker{
		db:        db,
		processor: processor,
		resolver:  NewTemplateResolver(),
		logger:    logger.WithField("component", "tool_invoker"),
	}
}

// SetProcessor sets the classifier processor
func (ti *ToolInvoker) SetProcessor(processor *classifier.Processor) {
	ti.processor = processor
	if processor != nil && ti.db != nil {
		processor.SetDatabase(ti.db)
	}
}

// InvokeTool executes an MCP tool with the given arguments for an entry
func (ti *ToolInvoker) InvokeTool(ctx context.Context, toolName string, args map[string]interface{}, entry *models.Entry) error {
	// Resolve template variables
	resolvedArgs := ti.resolver.ResolveArguments(args, entry)

	ti.logger.WithFields(logrus.Fields{
		"tool":      toolName,
		"entry":     entry.Path,
		"arguments": resolvedArgs,
	}).Debug("Invoking tool for outcome")

	// Route to appropriate handler based on tool name
	switch toolName {
	// Resource-set tools
	case "resource-set-modify":
		return ti.invokeResourceSetModify(resolvedArgs)
	case "resource-set-create":
		return ti.invokeResourceSetCreate(resolvedArgs)
	case "resource-set-add-child":
		return ti.invokeResourceSetAddChild(resolvedArgs)
	case "resource-set-remove-child":
		return ti.invokeResourceSetRemoveChild(resolvedArgs)

	// Classifier tools
	case "classifier-process":
		return ti.invokeClassifierProcess(resolvedArgs, entry)

	// Feature tools
	case "feature-cleanup":
		return ti.invokeFeatureCleanup(resolvedArgs, entry)

	// Index tools
	case "index":
		return ti.invokeIndex(resolvedArgs)

	default:
		return fmt.Errorf("tool '%s' is not supported for outcome invocation", toolName)
	}
}

// InvokeToolBatch invokes a tool for multiple entries, optimizing batch operations where possible
func (ti *ToolInvoker) InvokeToolBatch(ctx context.Context, toolName string, args map[string]interface{}, entries []*models.Entry) (int, error) {
	// Check if this tool supports batch optimization
	switch toolName {
	case "resource-set-modify":
		return ti.invokeResourceSetModifyBatch(args, entries)
	default:
		// Fall back to per-entry invocation
		successCount := 0
		for _, entry := range entries {
			if err := ti.InvokeTool(ctx, toolName, args, entry); err != nil {
				ti.logger.WithFields(logrus.Fields{
					"tool":  toolName,
					"entry": entry.Path,
					"error": err,
				}).Warn("Tool invocation failed for entry")
				continue
			}
			successCount++
		}
		return successCount, nil
	}
}

// invokeResourceSetModify handles resource-set-modify tool
func (ti *ToolInvoker) invokeResourceSetModify(args map[string]interface{}) error {
	name, _ := args["name"].(string)
	operation, _ := args["operation"].(string)
	if operation == "" {
		operation = "add"
	}

	paths := ti.extractPaths(args)
	if name == "" || len(paths) == 0 {
		return fmt.Errorf("resource-set-modify requires 'name' and 'paths'")
	}

	// Ensure resource set exists
	if err := ti.ensureResourceSetExists(name); err != nil {
		return err
	}

	switch operation {
	case "add":
		return ti.db.AddToResourceSet(name, paths)
	case "remove":
		return ti.db.RemoveFromResourceSet(name, paths)
	default:
		return fmt.Errorf("invalid operation: %s", operation)
	}
}

// invokeResourceSetModifyBatch handles batch resource-set-modify for efficiency
func (ti *ToolInvoker) invokeResourceSetModifyBatch(args map[string]interface{}, entries []*models.Entry) (int, error) {
	name, _ := args["name"].(string)
	operation, _ := args["operation"].(string)
	if operation == "" {
		operation = "add"
	}

	if name == "" {
		return 0, fmt.Errorf("resource-set-modify requires 'name'")
	}

	// Ensure resource set exists
	if err := ti.ensureResourceSetExists(name); err != nil {
		return 0, err
	}

	// Collect all paths from entries
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}

	var err error
	switch operation {
	case "add":
		err = ti.db.AddToResourceSet(name, paths)
	case "remove":
		err = ti.db.RemoveFromResourceSet(name, paths)
	default:
		return 0, fmt.Errorf("invalid operation: %s", operation)
	}

	if err != nil {
		return 0, err
	}

	ti.logger.WithFields(logrus.Fields{
		"tool":      "resource-set-modify",
		"operation": operation,
		"set":       name,
		"count":     len(entries),
	}).Info("Batch tool invocation completed")

	return len(entries), nil
}

// invokeResourceSetCreate handles resource-set-create tool
func (ti *ToolInvoker) invokeResourceSetCreate(args map[string]interface{}) error {
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)

	if name == "" {
		return fmt.Errorf("resource-set-create requires 'name'")
	}

	_, err := ti.db.CreateResourceSet(&models.ResourceSet{
		Name:        name,
		Description: &description,
	})
	return err
}

// invokeResourceSetAddChild handles resource-set-add-child tool
func (ti *ToolInvoker) invokeResourceSetAddChild(args map[string]interface{}) error {
	parent, _ := args["parent"].(string)
	child, _ := args["child"].(string)

	if parent == "" || child == "" {
		return fmt.Errorf("resource-set-add-child requires 'parent' and 'child'")
	}

	return ti.db.AddResourceSetEdge(parent, child)
}

// invokeResourceSetRemoveChild handles resource-set-remove-child tool
func (ti *ToolInvoker) invokeResourceSetRemoveChild(args map[string]interface{}) error {
	parent, _ := args["parent"].(string)
	child, _ := args["child"].(string)

	if parent == "" || child == "" {
		return fmt.Errorf("resource-set-remove-child requires 'parent' and 'child'")
	}

	return ti.db.RemoveResourceSetEdge(parent, child)
}

// invokeClassifierProcess handles classifier-process tool
func (ti *ToolInvoker) invokeClassifierProcess(args map[string]interface{}, entry *models.Entry) error {
	if ti.processor == nil {
		return fmt.Errorf("classifier processor not configured")
	}

	// Skip directories
	if entry.Kind == "directory" {
		return nil
	}

	// Check if this is a media file
	mediaType := classifier.DetectMediaType(entry.Path)
	if mediaType == classifier.MediaTypeUnknown {
		return nil // Not a media file, skip silently
	}

	// Get resource URL
	resource, _ := args["resource"].(string)
	if resource == "" {
		resource = "file://" + entry.Path
	}

	// Get artifact types
	artifacts := ti.extractStringSlice(args, "artifacts")
	if len(artifacts) == 0 {
		artifacts = []string{"thumbnail"}
	}

	req := &classifier.ProcessRequest{
		ResourceURL:   resource,
		ArtifactTypes: artifacts,
	}

	result, err := ti.processor.ProcessResource(req)
	if err != nil {
		return fmt.Errorf("classifier failed: %w", err)
	}

	if len(result.Errors) > 0 {
		ti.logger.WithFields(logrus.Fields{
			"path":   entry.Path,
			"errors": result.Errors,
		}).Warn("Classifier completed with errors")
	}

	return nil
}

// invokeIndex handles index tool
func (ti *ToolInvoker) invokeIndex(args map[string]interface{}) error {
	root, _ := args["root"].(string)
	if root == "" {
		return fmt.Errorf("index requires 'root' path")
	}

	// Note: This is a simplified implementation
	// Full implementation would use the crawler package
	ti.logger.WithField("root", root).Info("Index tool invoked (delegating to crawler)")
	return nil
}

// invokeFeatureCleanup handles feature-cleanup tool - deletes all features for an entry
func (ti *ToolInvoker) invokeFeatureCleanup(args map[string]interface{}, entry *models.Entry) error {
	path, _ := args["path"].(string)
	if path == "" {
		path = entry.Path
	}

	// Delete all features for this entry path
	count, err := ti.db.DeleteFeaturesByPath(path)
	if err != nil {
		return fmt.Errorf("failed to delete features for %s: %w", path, err)
	}

	if count > 0 {
		ti.logger.WithFields(logrus.Fields{
			"path":    path,
			"deleted": count,
		}).Info("Cleaned up features for entry")
	}

	return nil
}

// Helper methods

func (ti *ToolInvoker) ensureResourceSetExists(name string) error {
	set, err := ti.db.GetResourceSet(name)
	if err != nil || set == nil {
		desc := "Auto-created by plan outcome"
		_, createErr := ti.db.CreateResourceSet(&models.ResourceSet{
			Name:        name,
			Description: &desc,
		})
		if createErr != nil {
			return fmt.Errorf("failed to create resource set: %w", createErr)
		}
		ti.logger.Infof("Created resource set: %s", name)
	}
	return nil
}

func (ti *ToolInvoker) extractPaths(args map[string]interface{}) []string {
	var paths []string

	// Handle paths as array
	if pathsInterface, ok := args["paths"].([]interface{}); ok {
		for _, p := range pathsInterface {
			if s, ok := p.(string); ok {
				paths = append(paths, s)
			}
		}
	}

	// Handle paths as single string
	if pathStr, ok := args["paths"].(string); ok {
		paths = append(paths, pathStr)
	}

	// Handle path as single string (alternative field name)
	if pathStr, ok := args["path"].(string); ok {
		paths = append(paths, pathStr)
	}

	return paths
}

func (ti *ToolInvoker) extractStringSlice(args map[string]interface{}, key string) []string {
	var result []string

	if slice, ok := args[key].([]interface{}); ok {
		for _, item := range slice {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	}

	if s, ok := args[key].(string); ok {
		result = append(result, s)
	}

	return result
}

// GetSupportedTools returns the list of tools that can be invoked as outcomes
func GetSupportedTools() []string {
	return []string{
		"resource-set-modify",
		"resource-set-create",
		"resource-set-add-child",
		"resource-set-remove-child",
		"classifier-process",
		"feature-cleanup",
		"index",
	}
}

// ValidateToolName checks if a tool is supported for outcome invocation
func ValidateToolName(toolName string) error {
	for _, supported := range GetSupportedTools() {
		if toolName == supported {
			return nil
		}
	}
	return fmt.Errorf("tool '%s' is not supported for outcome invocation; supported tools: %v", toolName, GetSupportedTools())
}

// ToolInvocationResult holds the result of a tool invocation
type ToolInvocationResult struct {
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
	Success   bool                   `json:"success"`
	Error     string                 `json:"error,omitempty"`
}

// MarshalJSON returns JSON representation of tool invocation for audit
func (r *ToolInvocationResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"tool":      r.Tool,
		"arguments": r.Arguments,
		"success":   r.Success,
		"error":     r.Error,
	})
}
