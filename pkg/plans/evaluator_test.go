package plans

import (
	"testing"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEvaluator() *ConditionEvaluator {
	logger := logrus.New().WithField("test", "evaluator")
	return NewConditionEvaluator(logger)
}

func TestConditionEvaluator_Evaluate_NilCondition(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/test/file.txt", Size: 100}

	result, err := eval.Evaluate(entry, nil)
	require.NoError(t, err)
	assert.True(t, result, "nil condition should match all")
}

func TestConditionEvaluator_Evaluate_SizeConditions(t *testing.T) {
	eval := newTestEvaluator()

	tests := []struct {
		name     string
		entry    *models.Entry
		cond     *models.RuleCondition
		expected bool
	}{
		{
			name:     "minSize matches",
			entry:    &models.Entry{Size: 1000},
			cond:     &models.RuleCondition{MinSize: ptrInt64(500)},
			expected: true,
		},
		{
			name:     "minSize fails",
			entry:    &models.Entry{Size: 100},
			cond:     &models.RuleCondition{MinSize: ptrInt64(500)},
			expected: false,
		},
		{
			name:     "maxSize matches",
			entry:    &models.Entry{Size: 100},
			cond:     &models.RuleCondition{MaxSize: ptrInt64(500)},
			expected: true,
		},
		{
			name:     "maxSize fails",
			entry:    &models.Entry{Size: 1000},
			cond:     &models.RuleCondition{MaxSize: ptrInt64(500)},
			expected: false,
		},
		{
			name:     "size range matches",
			entry:    &models.Entry{Size: 500},
			cond:     &models.RuleCondition{MinSize: ptrInt64(100), MaxSize: ptrInt64(1000)},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(tt.entry, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_TimeConditions(t *testing.T) {
	eval := newTestEvaluator()

	tests := []struct {
		name     string
		entry    *models.Entry
		cond     *models.RuleCondition
		expected bool
	}{
		{
			name:     "minMtime matches",
			entry:    &models.Entry{Mtime: 1000},
			cond:     &models.RuleCondition{MinMtime: ptrInt64(500)},
			expected: true,
		},
		{
			name:     "minMtime fails",
			entry:    &models.Entry{Mtime: 100},
			cond:     &models.RuleCondition{MinMtime: ptrInt64(500)},
			expected: false,
		},
		{
			name:     "maxMtime matches",
			entry:    &models.Entry{Mtime: 100},
			cond:     &models.RuleCondition{MaxMtime: ptrInt64(500)},
			expected: true,
		},
		{
			name:     "maxMtime fails",
			entry:    &models.Entry{Mtime: 1000},
			cond:     &models.RuleCondition{MaxMtime: ptrInt64(500)},
			expected: false,
		},
		{
			name:     "minCtime matches",
			entry:    &models.Entry{Ctime: 1000},
			cond:     &models.RuleCondition{MinCtime: ptrInt64(500)},
			expected: true,
		},
		{
			name:     "minCtime fails",
			entry:    &models.Entry{Ctime: 100},
			cond:     &models.RuleCondition{MinCtime: ptrInt64(500)},
			expected: false,
		},
		{
			name:     "maxCtime matches",
			entry:    &models.Entry{Ctime: 100},
			cond:     &models.RuleCondition{MaxCtime: ptrInt64(500)},
			expected: true,
		},
		{
			name:     "maxCtime fails",
			entry:    &models.Entry{Ctime: 1000},
			cond:     &models.RuleCondition{MaxCtime: ptrInt64(500)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(tt.entry, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_PathConditions(t *testing.T) {
	eval := newTestEvaluator()

	tests := []struct {
		name     string
		entry    *models.Entry
		cond     *models.RuleCondition
		expected bool
	}{
		{
			name:     "pathContains matches",
			entry:    &models.Entry{Path: "/home/user/documents/file.txt"},
			cond:     &models.RuleCondition{PathContains: ptrString("documents")},
			expected: true,
		},
		{
			name:     "pathContains fails",
			entry:    &models.Entry{Path: "/home/user/photos/file.txt"},
			cond:     &models.RuleCondition{PathContains: ptrString("documents")},
			expected: false,
		},
		{
			name:     "pathPrefix matches",
			entry:    &models.Entry{Path: "/home/user/documents/file.txt"},
			cond:     &models.RuleCondition{PathPrefix: ptrString("/home/user")},
			expected: true,
		},
		{
			name:     "pathPrefix fails",
			entry:    &models.Entry{Path: "/tmp/file.txt"},
			cond:     &models.RuleCondition{PathPrefix: ptrString("/home")},
			expected: false,
		},
		{
			name:     "pathSuffix matches",
			entry:    &models.Entry{Path: "/home/user/file.txt"},
			cond:     &models.RuleCondition{PathSuffix: ptrString(".txt")},
			expected: true,
		},
		{
			name:     "pathSuffix fails",
			entry:    &models.Entry{Path: "/home/user/file.pdf"},
			cond:     &models.RuleCondition{PathSuffix: ptrString(".txt")},
			expected: false,
		},
		{
			name:     "pathPattern matches",
			entry:    &models.Entry{Path: "/home/user/file123.txt"},
			cond:     &models.RuleCondition{PathPattern: ptrString(`file\d+\.txt$`)},
			expected: true,
		},
		{
			name:     "pathPattern fails",
			entry:    &models.Entry{Path: "/home/user/file.txt"},
			cond:     &models.RuleCondition{PathPattern: ptrString(`file\d+\.txt$`)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(tt.entry, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_PathPatternError(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/test/file.txt"}
	cond := &models.RuleCondition{PathPattern: ptrString("[invalid")}

	_, err := eval.Evaluate(entry, cond)
	assert.Error(t, err, "invalid regex should return error")
}

func TestConditionEvaluator_Evaluate_All(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/home/user/file.txt", Size: 500}

	tests := []struct {
		name     string
		cond     *models.RuleCondition
		expected bool
	}{
		{
			name: "all conditions match",
			cond: &models.RuleCondition{
				Type: "all",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(100)},
					{PathPrefix: ptrString("/home")},
				},
			},
			expected: true,
		},
		{
			name: "one condition fails",
			cond: &models.RuleCondition{
				Type: "all",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(100)},
					{PathPrefix: ptrString("/tmp")}, // fails
				},
			},
			expected: false,
		},
		{
			name: "empty conditions",
			cond: &models.RuleCondition{
				Type:       "all",
				Conditions: []*models.RuleCondition{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(entry, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_Any(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/home/user/file.txt", Size: 500}

	tests := []struct {
		name     string
		cond     *models.RuleCondition
		expected bool
	}{
		{
			name: "one condition matches",
			cond: &models.RuleCondition{
				Type: "any",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(1000)}, // fails
					{PathPrefix: ptrString("/home")}, // matches
				},
			},
			expected: true,
		},
		{
			name: "all conditions fail",
			cond: &models.RuleCondition{
				Type: "any",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(1000)},
					{PathPrefix: ptrString("/tmp")},
				},
			},
			expected: false,
		},
		{
			name: "empty conditions",
			cond: &models.RuleCondition{
				Type:       "any",
				Conditions: []*models.RuleCondition{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(entry, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_None(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/home/user/file.txt", Size: 500}

	tests := []struct {
		name     string
		cond     *models.RuleCondition
		expected bool
	}{
		{
			name: "no condition matches",
			cond: &models.RuleCondition{
				Type: "none",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(1000)}, // fails
					{PathPrefix: ptrString("/tmp")}, // fails
				},
			},
			expected: true,
		},
		{
			name: "one condition matches",
			cond: &models.RuleCondition{
				Type: "none",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(100)}, // matches
					{PathPrefix: ptrString("/tmp")},
				},
			},
			expected: false,
		},
		{
			name: "empty conditions",
			cond: &models.RuleCondition{
				Type:       "none",
				Conditions: []*models.RuleCondition{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(entry, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_MediaType(t *testing.T) {
	eval := newTestEvaluator()

	tests := []struct {
		name     string
		path     string
		media    string
		expected bool
	}{
		// Video types
		{"mp4 is video", "/test/video.mp4", "video", true},
		{"mkv is video", "/test/video.mkv", "video", true},
		{"avi is video", "/test/video.avi", "video", true},
		{"mov is video", "/test/video.mov", "video", true},
		{"webm is video", "/test/video.webm", "video", true},
		{"txt is not video", "/test/file.txt", "video", false},

		// Image types
		{"jpg is image", "/test/photo.jpg", "image", true},
		{"jpeg is image", "/test/photo.jpeg", "image", true},
		{"png is image", "/test/photo.png", "image", true},
		{"gif is image", "/test/photo.gif", "image", true},
		{"webp is image", "/test/photo.webp", "image", true},
		{"bmp is image", "/test/photo.bmp", "image", true},
		{"mp4 is not image", "/test/video.mp4", "image", false},

		// Audio types
		{"mp3 is audio", "/test/song.mp3", "audio", true},
		{"flac is audio", "/test/song.flac", "audio", true},
		{"wav is audio", "/test/song.wav", "audio", true},
		{"m4a is audio", "/test/song.m4a", "audio", true},
		{"ogg is audio", "/test/song.ogg", "audio", true},
		{"mp4 is not audio", "/test/video.mp4", "audio", false},

		// Document types
		{"pdf is document", "/test/file.pdf", "document", true},
		{"doc is document", "/test/file.doc", "document", true},
		{"docx is document", "/test/file.docx", "document", true},
		{"txt is document", "/test/file.txt", "document", true},
		{"md is document", "/test/file.md", "document", true},
		{"rtf is document", "/test/file.rtf", "document", true},
		{"mp4 is not document", "/test/video.mp4", "document", false},

		// Unknown type
		{"unknown type", "/test/file.txt", "unknown", false},

		// Case insensitive
		{"MP4 is video (uppercase)", "/test/VIDEO.MP4", "video", true},
		{"JPG is image (uppercase)", "/test/PHOTO.JPG", "image", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &models.Entry{Path: tt.path}
			cond := &models.RuleCondition{MediaType: ptrString(tt.media)}
			result, err := eval.Evaluate(entry, cond)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionEvaluator_Evaluate_NestedConditions(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/home/user/video.mp4", Size: 1000}

	// Complex condition: (size > 500 AND path starts with /home) OR mediaType == video
	cond := &models.RuleCondition{
		Type: "any",
		Conditions: []*models.RuleCondition{
			{
				Type: "all",
				Conditions: []*models.RuleCondition{
					{MinSize: ptrInt64(500)},
					{PathPrefix: ptrString("/home")},
				},
			},
			{MediaType: ptrString("video")},
		},
	}

	result, err := eval.Evaluate(entry, cond)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestConditionEvaluator_Evaluate_AllWithError(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/test/file.txt"}

	cond := &models.RuleCondition{
		Type: "all",
		Conditions: []*models.RuleCondition{
			{PathPattern: ptrString("[invalid")}, // Invalid regex
		},
	}

	_, err := eval.Evaluate(entry, cond)
	assert.Error(t, err)
}

func TestConditionEvaluator_Evaluate_AnyWithError(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/test/file.txt"}

	cond := &models.RuleCondition{
		Type: "any",
		Conditions: []*models.RuleCondition{
			{PathPattern: ptrString("[invalid")}, // Invalid regex
		},
	}

	_, err := eval.Evaluate(entry, cond)
	assert.Error(t, err)
}

func TestConditionEvaluator_Evaluate_NoneWithError(t *testing.T) {
	eval := newTestEvaluator()
	entry := &models.Entry{Path: "/test/file.txt"}

	cond := &models.RuleCondition{
		Type: "none",
		Conditions: []*models.RuleCondition{
			{PathPattern: ptrString("[invalid")}, // Invalid regex
		},
	}

	_, err := eval.Evaluate(entry, cond)
	assert.Error(t, err)
}

// Helper functions
func ptrInt64(v int64) *int64 {
	return &v
}

func ptrString(v string) *string {
	return &v
}
