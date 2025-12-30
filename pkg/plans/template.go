package plans

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/prismon/mcp-space-browser/internal/models"
)

var templateRegex = regexp.MustCompile(`\{\{(\w+(?:\.\w+)*)\}\}`)

// TemplateResolver resolves template variables like {{entry.path}} and {{pref.large.file.size}} in outcome arguments
type TemplateResolver struct {
	preferences map[string]interface{}
}

// NewTemplateResolver creates a new template resolver
func NewTemplateResolver() *TemplateResolver {
	return &TemplateResolver{
		preferences: make(map[string]interface{}),
	}
}

// NewTemplateResolverWithPreferences creates a resolver with plan preferences
func NewTemplateResolverWithPreferences(prefs map[string]interface{}) *TemplateResolver {
	if prefs == nil {
		prefs = make(map[string]interface{})
	}
	return &TemplateResolver{
		preferences: prefs,
	}
}

// SetPreferences updates the preferences map
func (tr *TemplateResolver) SetPreferences(prefs map[string]interface{}) {
	if prefs == nil {
		tr.preferences = make(map[string]interface{})
	} else {
		tr.preferences = prefs
	}
}

// ResolveArguments replaces template variables in arguments with entry values
func (tr *TemplateResolver) ResolveArguments(args map[string]interface{}, entry *models.Entry) map[string]interface{} {
	if args == nil {
		return nil
	}
	resolved := make(map[string]interface{})
	for key, value := range args {
		resolved[key] = tr.resolveValue(value, entry)
	}
	return resolved
}

// resolveValue recursively resolves template variables in any value type
func (tr *TemplateResolver) resolveValue(value interface{}, entry *models.Entry) interface{} {
	switch v := value.(type) {
	case string:
		return tr.resolveString(v, entry)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = tr.resolveValue(item, entry)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = tr.resolveValue(val, entry)
		}
		return result
	default:
		return value
	}
}

// resolveString replaces all {{variable}} occurrences in a string
func (tr *TemplateResolver) resolveString(s string, entry *models.Entry) string {
	return templateRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-2] // Strip {{ and }}
		return tr.getEntryValue(varName, entry)
	})
}

// getEntryValue returns the string value for a template variable
func (tr *TemplateResolver) getEntryValue(varName string, entry *models.Entry) string {
	// Handle preference variables (e.g., pref.large.file.size)
	if strings.HasPrefix(varName, "pref.") {
		prefKey := strings.TrimPrefix(varName, "pref.")
		return tr.getPreferenceValue(prefKey)
	}

	// Handle entry variables
	switch varName {
	case "entry.path":
		return entry.Path
	case "entry.size":
		return strconv.FormatInt(entry.Size, 10)
	case "entry.kind":
		return entry.Kind
	case "entry.mtime":
		return strconv.FormatInt(entry.Mtime, 10)
	case "entry.ctime":
		return strconv.FormatInt(entry.Ctime, 10)
	case "entry.parent":
		if entry.Parent != nil {
			return *entry.Parent
		}
		return ""
	case "entry.id":
		return strconv.FormatInt(entry.ID, 10)
	default:
		// Unknown variable, return original match
		return "{{" + varName + "}}"
	}
}

// getPreferenceValue looks up a preference by key and returns its string representation
func (tr *TemplateResolver) getPreferenceValue(key string) string {
	val, ok := tr.preferences[key]
	if !ok {
		// Check for default preferences
		defaults := models.DefaultPreferences()
		if defVal, ok := defaults[key]; ok {
			return formatPreferenceValue(defVal)
		}
		return "{{pref." + key + "}}"
	}
	return formatPreferenceValue(val)
}

// formatPreferenceValue converts a preference value to string
func formatPreferenceValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
