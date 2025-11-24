package plans

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// ConditionEvaluator evaluates RuleCondition against entries
type ConditionEvaluator struct {
	logger *logrus.Entry
}

// NewConditionEvaluator creates a new condition evaluator
func NewConditionEvaluator(logger *logrus.Entry) *ConditionEvaluator {
	return &ConditionEvaluator{
		logger: logger.WithField("component", "condition_evaluator"),
	}
}

// Evaluate checks if an entry matches the condition tree
func (ce *ConditionEvaluator) Evaluate(entry *models.Entry, condition *models.RuleCondition) (bool, error) {
	if condition == nil {
		return true, nil // No condition = always match
	}

	switch condition.Type {
	case "all":
		return ce.evaluateAll(entry, condition.Conditions)
	case "any":
		return ce.evaluateAny(entry, condition.Conditions)
	case "none":
		return ce.evaluateNone(entry, condition.Conditions)
	default:
		return ce.evaluateLeaf(entry, condition)
	}
}

func (ce *ConditionEvaluator) evaluateAll(entry *models.Entry, conditions []*models.RuleCondition) (bool, error) {
	if len(conditions) == 0 {
		return true, nil
	}

	for _, cond := range conditions {
		match, err := ce.Evaluate(entry, cond)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func (ce *ConditionEvaluator) evaluateAny(entry *models.Entry, conditions []*models.RuleCondition) (bool, error) {
	if len(conditions) == 0 {
		return false, nil
	}

	for _, cond := range conditions {
		match, err := ce.Evaluate(entry, cond)
		if err != nil {
			return false, err
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

func (ce *ConditionEvaluator) evaluateNone(entry *models.Entry, conditions []*models.RuleCondition) (bool, error) {
	if len(conditions) == 0 {
		return true, nil
	}

	for _, cond := range conditions {
		match, err := ce.Evaluate(entry, cond)
		if err != nil {
			return false, err
		}
		if match {
			return false, nil
		}
	}
	return true, nil
}

func (ce *ConditionEvaluator) evaluateLeaf(entry *models.Entry, condition *models.RuleCondition) (bool, error) {
	// Size filters
	if condition.MinSize != nil && entry.Size < *condition.MinSize {
		return false, nil
	}
	if condition.MaxSize != nil && entry.Size > *condition.MaxSize {
		return false, nil
	}

	// Time filters
	if condition.MinMtime != nil && entry.Mtime < *condition.MinMtime {
		return false, nil
	}
	if condition.MaxMtime != nil && entry.Mtime > *condition.MaxMtime {
		return false, nil
	}
	if condition.MinCtime != nil && entry.Ctime < *condition.MinCtime {
		return false, nil
	}
	if condition.MaxCtime != nil && entry.Ctime > *condition.MaxCtime {
		return false, nil
	}

	// Path filters
	if condition.PathContains != nil && !strings.Contains(entry.Path, *condition.PathContains) {
		return false, nil
	}
	if condition.PathPrefix != nil && !strings.HasPrefix(entry.Path, *condition.PathPrefix) {
		return false, nil
	}
	if condition.PathSuffix != nil && !strings.HasSuffix(entry.Path, *condition.PathSuffix) {
		return false, nil
	}
	if condition.PathPattern != nil {
		matched, err := regexp.MatchString(*condition.PathPattern, entry.Path)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern %s: %w", *condition.PathPattern, err)
		}
		if !matched {
			return false, nil
		}
	}

	// Media type filter would require media_type field in Entry
	// For now, we can check by extension as a proxy
	if condition.MediaType != nil {
		if !ce.matchesMediaType(entry.Path, *condition.MediaType) {
			return false, nil
		}
	}

	return true, nil
}

// matchesMediaType is a simple heuristic based on file extension
func (ce *ConditionEvaluator) matchesMediaType(path string, mediaType string) bool {
	pathLower := strings.ToLower(path)

	switch mediaType {
	case "video":
		return strings.HasSuffix(pathLower, ".mp4") ||
			strings.HasSuffix(pathLower, ".mkv") ||
			strings.HasSuffix(pathLower, ".avi") ||
			strings.HasSuffix(pathLower, ".mov") ||
			strings.HasSuffix(pathLower, ".webm")
	case "image":
		return strings.HasSuffix(pathLower, ".jpg") ||
			strings.HasSuffix(pathLower, ".jpeg") ||
			strings.HasSuffix(pathLower, ".png") ||
			strings.HasSuffix(pathLower, ".gif") ||
			strings.HasSuffix(pathLower, ".webp") ||
			strings.HasSuffix(pathLower, ".bmp")
	case "audio":
		return strings.HasSuffix(pathLower, ".mp3") ||
			strings.HasSuffix(pathLower, ".flac") ||
			strings.HasSuffix(pathLower, ".wav") ||
			strings.HasSuffix(pathLower, ".m4a") ||
			strings.HasSuffix(pathLower, ".ogg")
	case "document":
		return strings.HasSuffix(pathLower, ".pdf") ||
			strings.HasSuffix(pathLower, ".doc") ||
			strings.HasSuffix(pathLower, ".docx") ||
			strings.HasSuffix(pathLower, ".txt") ||
			strings.HasSuffix(pathLower, ".md") ||
			strings.HasSuffix(pathLower, ".rtf")
	default:
		return false
	}
}
