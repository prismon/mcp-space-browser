package project

import (
	"fmt"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

// Standard resource-set names created by lifecycle plans
const (
	ResourceSetLargeFiles = "large-files"
	ResourceSetImages     = "images"
	ResourceSetVideos     = "videos"
	ResourceSetAudio      = "audio"
	ResourceSetDocuments  = "documents"
)

// Lifecycle plan names
const (
	PlanAdded   = "added"
	PlanRemoved = "removed"
	PlanRefresh = "refresh"
)

// StandardResourceSets returns the list of standard resource-sets to create
func StandardResourceSets() []string {
	return []string{
		ResourceSetLargeFiles,
		ResourceSetImages,
		ResourceSetVideos,
		ResourceSetAudio,
		ResourceSetDocuments,
	}
}

// CreateLifecyclePlans creates the standard lifecycle plans and resource-sets
func CreateLifecyclePlans(db *database.DiskDB) error {
	// Create standard resource-sets first
	if err := createStandardResourceSets(db); err != nil {
		return fmt.Errorf("failed to create standard resource-sets: %w", err)
	}

	// Create lifecycle plans
	plans := []*models.Plan{
		createAddedPlan(),
		createRemovedPlan(),
		createRefreshPlan(),
	}

	for _, plan := range plans {
		// Check if plan already exists
		existing, _ := db.GetPlan(plan.Name)
		if existing != nil {
			log.WithField("plan", plan.Name).Debug("Lifecycle plan already exists, skipping")
			continue
		}

		if err := db.CreatePlan(plan); err != nil {
			return fmt.Errorf("failed to create plan %s: %w", plan.Name, err)
		}
		log.WithField("plan", plan.Name).Info("Created lifecycle plan")
	}

	return nil
}

// createStandardResourceSets creates the standard resource-sets if they don't exist
func createStandardResourceSets(db *database.DiskDB) error {
	for _, name := range StandardResourceSets() {
		existing, _ := db.GetResourceSet(name)
		if existing != nil {
			continue
		}

		desc := getResourceSetDescription(name)
		rs := &models.ResourceSet{
			Name:        name,
			Description: &desc,
		}

		if _, err := db.CreateResourceSet(rs); err != nil {
			return fmt.Errorf("failed to create resource-set %s: %w", name, err)
		}
		log.WithField("resource_set", name).Info("Created standard resource-set")
	}

	return nil
}

func getResourceSetDescription(name string) string {
	switch name {
	case ResourceSetLargeFiles:
		return "Files exceeding the configured size threshold"
	case ResourceSetImages:
		return "Image files (jpg, png, gif, webp, etc.)"
	case ResourceSetVideos:
		return "Video files (mp4, mkv, mov, avi, etc.)"
	case ResourceSetAudio:
		return "Audio files (mp3, flac, wav, aac, etc.)"
	case ResourceSetDocuments:
		return "Document files (pdf, doc, txt, etc.)"
	default:
		return ""
	}
}

// createAddedPlan creates the "added" lifecycle plan
// Triggered when files are added to the project
func createAddedPlan() *models.Plan {
	mediaTypeImage := "image"
	mediaTypeVideo := "video"
	mediaTypeAudio := "audio"
	mediaTypeDocument := "document"

	return &models.Plan{
		Name:        PlanAdded,
		Description: strPtr("Processes newly added files: classifies by type and generates features"),
		Mode:        "oneshot",
		Status:      "active",
		Trigger:     "on_add",
		Sources: []models.PlanSource{
			{Type: "entries"}, // Entries passed directly by lifecycle trigger
		},
		Preferences: models.DefaultPreferences(),
		Outcomes: []models.RuleOutcome{
			// Large files - add to large-files resource-set
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetLargeFiles,
					"operation": "add",
					"paths":     []string{"{{entry.path}}"},
				},
				Conditions: &models.RuleCondition{
					Type:    "size",
					MinSize: int64Ptr(524288000), // Will be overridden by pref.large.file.size
				},
			},
			// Images - add to images resource-set AND generate thumbnails
			{
				Conditions: &models.RuleCondition{
					Type:      "media_type",
					MediaType: &mediaTypeImage,
				},
				Outcomes: []*models.RuleOutcome{
					{
						Tool: "resource-set-modify",
						Arguments: map[string]interface{}{
							"name":      ResourceSetImages,
							"operation": "add",
							"paths":     []string{"{{entry.path}}"},
						},
					},
					{
						Tool: "classifier-process",
						Arguments: map[string]interface{}{
							"path":      "{{entry.path}}",
							"artifacts": []string{"thumbnail"},
						},
					},
				},
			},
			// Videos - add to videos resource-set AND generate thumbnails + timeline
			{
				Conditions: &models.RuleCondition{
					Type:      "media_type",
					MediaType: &mediaTypeVideo,
				},
				Outcomes: []*models.RuleOutcome{
					{
						Tool: "resource-set-modify",
						Arguments: map[string]interface{}{
							"name":      ResourceSetVideos,
							"operation": "add",
							"paths":     []string{"{{entry.path}}"},
						},
					},
					{
						Tool: "classifier-process",
						Arguments: map[string]interface{}{
							"path":      "{{entry.path}}",
							"artifacts": []string{"thumbnail", "timeline"},
						},
					},
				},
			},
			// Audio - add to audio resource-set
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetAudio,
					"operation": "add",
					"paths":     []string{"{{entry.path}}"},
				},
				Conditions: &models.RuleCondition{
					Type:      "media_type",
					MediaType: &mediaTypeAudio,
				},
			},
			// Documents - add to documents resource-set
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetDocuments,
					"operation": "add",
					"paths":     []string{"{{entry.path}}"},
				},
				Conditions: &models.RuleCondition{
					Type:      "media_type",
					MediaType: &mediaTypeDocument,
				},
			},
		},
	}
}

// createRemovedPlan creates the "removed" lifecycle plan
// Triggered when files are removed from the project
func createRemovedPlan() *models.Plan {
	return &models.Plan{
		Name:        PlanRemoved,
		Description: strPtr("Cleans up removed files: removes from resource-sets and deletes features"),
		Mode:        "oneshot",
		Status:      "active",
		Trigger:     "on_remove",
		Sources: []models.PlanSource{
			{Type: "entries"}, // Entries passed directly by lifecycle trigger
		},
		Preferences: models.DefaultPreferences(),
		Outcomes: []models.RuleOutcome{
			// Remove from all standard resource-sets
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetLargeFiles,
					"operation": "remove",
					"paths":     []string{"{{entry.path}}"},
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetImages,
					"operation": "remove",
					"paths":     []string{"{{entry.path}}"},
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetVideos,
					"operation": "remove",
					"paths":     []string{"{{entry.path}}"},
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetAudio,
					"operation": "remove",
					"paths":     []string{"{{entry.path}}"},
				},
			},
			{
				Tool: "resource-set-modify",
				Arguments: map[string]interface{}{
					"name":      ResourceSetDocuments,
					"operation": "remove",
					"paths":     []string{"{{entry.path}}"},
				},
			},
			// Clean up features for the removed entry
			{
				Tool: "feature-cleanup",
				Arguments: map[string]interface{}{
					"path": "{{entry.path}}",
				},
			},
		},
	}
}

// createRefreshPlan creates the "refresh" lifecycle plan
// Triggered when files need their features regenerated
func createRefreshPlan() *models.Plan {
	mediaTypeImage := "image"
	mediaTypeVideo := "video"

	return &models.Plan{
		Name:        PlanRefresh,
		Description: strPtr("Refreshes file features: deletes and regenerates thumbnails/metadata"),
		Mode:        "oneshot",
		Status:      "active",
		Trigger:     "on_refresh",
		Sources: []models.PlanSource{
			{Type: "entries"}, // Entries passed directly by lifecycle trigger
		},
		Preferences: models.DefaultPreferences(),
		Outcomes: []models.RuleOutcome{
			// First clean up existing features
			{
				Tool: "feature-cleanup",
				Arguments: map[string]interface{}{
					"path": "{{entry.path}}",
				},
			},
			// Regenerate thumbnails for images
			{
				Tool: "classifier-process",
				Arguments: map[string]interface{}{
					"path":      "{{entry.path}}",
					"artifacts": []string{"thumbnail"},
				},
				Conditions: &models.RuleCondition{
					Type:      "media_type",
					MediaType: &mediaTypeImage,
				},
			},
			// Regenerate thumbnails and timeline for videos
			{
				Tool: "classifier-process",
				Arguments: map[string]interface{}{
					"path":      "{{entry.path}}",
					"artifacts": []string{"thumbnail", "timeline"},
				},
				Conditions: &models.RuleCondition{
					Type:      "media_type",
					MediaType: &mediaTypeVideo,
				},
			},
		},
	}
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
