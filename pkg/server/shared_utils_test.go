package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringOrStrings_Array(t *testing.T) {
	input := `{"paths":["a","b","c"]}`
	var out struct {
		Paths StringOrStrings `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(input), &out))
	assert.Equal(t, StringOrStrings{"a", "b", "c"}, out.Paths)
}

func TestStringOrStrings_SingleString(t *testing.T) {
	input := `{"paths":"/home/user/photos"}`
	var out struct {
		Paths StringOrStrings `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(input), &out))
	assert.Equal(t, StringOrStrings{"/home/user/photos"}, out.Paths)
}

func TestStringOrStrings_EmptyArray(t *testing.T) {
	input := `{"paths":[]}`
	var out struct {
		Paths StringOrStrings `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(input), &out))
	assert.Equal(t, StringOrStrings{}, out.Paths)
}

func TestStringOrStrings_InvalidType(t *testing.T) {
	input := `{"paths":123}`
	var out struct {
		Paths StringOrStrings `json:"paths"`
	}
	err := json.Unmarshal([]byte(input), &out)
	assert.Error(t, err)
}

func TestStringOrStrings_OmittedField(t *testing.T) {
	input := `{}`
	var out struct {
		Paths StringOrStrings `json:"paths,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(input), &out))
	assert.Nil(t, out.Paths)
}
