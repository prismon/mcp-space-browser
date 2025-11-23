package classifier

import (
	"fmt"

	"github.com/prismon/mcp-space-browser/pkg/logger"
)

var managerLog = logger.WithName("classifier-manager")

// Manager manages multiple classifiers with fallback support
type Manager struct {
	classifiers []Classifier
}

// NewManager creates a new classifier manager with default classifiers
func NewManager() *Manager {
	m := &Manager{
		classifiers: make([]Classifier, 0),
	}

	// Register classifiers in order of preference
	// FFmpeg is preferred for videos when available
	m.Register(NewFFmpegClassifier())
	// Go classifier is always available as fallback
	m.Register(NewGoClassifier())

	return m
}

// Register adds a classifier to the manager
func (m *Manager) Register(c Classifier) {
	if c == nil {
		return
	}
	m.classifiers = append(m.classifiers, c)
	managerLog.WithField("classifier", c.Name()).
		WithField("available", c.IsAvailable()).
		Debug("Registered classifier")
}

// GetClassifier returns the best available classifier for the given media type
func (m *Manager) GetClassifier(mediaType MediaType) (Classifier, error) {
	for _, c := range m.classifiers {
		if c.CanHandle(mediaType) && c.IsAvailable() {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no available classifier for media type: %s", mediaType)
}

// GenerateThumbnail generates a thumbnail using the best available classifier
func (m *Manager) GenerateThumbnail(req *ArtifactRequest) *ArtifactResult {
	classifier, err := m.GetClassifier(req.MediaType)
	if err != nil {
		return &ArtifactResult{Error: err}
	}

	managerLog.WithField("classifier", classifier.Name()).
		WithField("media_type", req.MediaType).
		WithField("source", req.SourcePath).
		Debug("Generating thumbnail")

	result := classifier.GenerateThumbnail(req)

	// If the preferred classifier failed, try fallback
	if result.Error != nil && len(m.classifiers) > 1 {
		managerLog.WithError(result.Error).
			WithField("classifier", classifier.Name()).
			Warn("Classifier failed, trying fallback")

		// Try the next available classifier
		for _, fallback := range m.classifiers {
			if fallback == classifier {
				continue // Skip the one that just failed
			}
			if !fallback.CanHandle(req.MediaType) || !fallback.IsAvailable() {
				continue
			}

			managerLog.WithField("fallback", fallback.Name()).Debug("Trying fallback classifier")
			result = fallback.GenerateThumbnail(req)
			if result.Error == nil {
				break
			}
		}
	}

	return result
}

// GenerateTimelineFrame generates a timeline frame using the best available classifier
func (m *Manager) GenerateTimelineFrame(req *ArtifactRequest) *ArtifactResult {
	classifier, err := m.GetClassifier(req.MediaType)
	if err != nil {
		return &ArtifactResult{Error: err}
	}

	managerLog.WithField("classifier", classifier.Name()).
		WithField("media_type", req.MediaType).
		WithField("source", req.SourcePath).
		WithField("frame", req.FrameIndex).
		Debug("Generating timeline frame")

	result := classifier.GenerateTimelineFrame(req)

	// If the preferred classifier failed, try fallback
	if result.Error != nil && len(m.classifiers) > 1 {
		managerLog.WithError(result.Error).
			WithField("classifier", classifier.Name()).
			Warn("Classifier failed, trying fallback")

		for _, fallback := range m.classifiers {
			if fallback == classifier {
				continue
			}
			if !fallback.CanHandle(req.MediaType) || !fallback.IsAvailable() {
				continue
			}

			managerLog.WithField("fallback", fallback.Name()).Debug("Trying fallback classifier")
			result = fallback.GenerateTimelineFrame(req)
			if result.Error == nil {
				break
			}
		}
	}

	return result
}

// ListClassifiers returns information about all registered classifiers
func (m *Manager) ListClassifiers() []map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(m.classifiers))
	for _, c := range m.classifiers {
		list = append(list, map[string]interface{}{
			"name":      c.Name(),
			"available": c.IsAvailable(),
			"handles": map[string]bool{
				"image": c.CanHandle(MediaTypeImage),
				"video": c.CanHandle(MediaTypeVideo),
			},
		})
	}
	return list
}
