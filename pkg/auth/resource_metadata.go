package auth

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ProtectedResourceMetadata represents RFC 9728 OAuth 2.0 Protected Resource Metadata
type ProtectedResourceMetadata struct {
	Resource                       string   `json:"resource"`
	AuthorizationServers           []string `json:"authorization_servers"`
	BearerMethodsSupported         []string `json:"bearer_methods_supported"`
	ResourceSigningAlgValues       []string `json:"resource_signing_alg_values_supported"`
	ResourceDocumentation          string   `json:"resource_documentation,omitempty"`
	ResourcePolicyURI              string   `json:"resource_policy_uri,omitempty"`
	ClientRegistrationEndpoint     string   `json:"client_registration_endpoint,omitempty"`
}

// RegisterProtectedResourceMetadataEndpoint registers the RFC 9728 endpoint
func RegisterProtectedResourceMetadataEndpoint(router *gin.Engine, config *AuthConfig, baseURL string) {
	router.GET("/.well-known/oauth-protected-resource", func(c *gin.Context) {
		handleProtectedResourceMetadata(c, config, baseURL)
	})
}

// handleProtectedResourceMetadata returns RFC 9728 Protected Resource Metadata
func handleProtectedResourceMetadata(c *gin.Context, config *AuthConfig, baseURL string) {
	resource := config.ResourceMetadata.Resource
	if resource == "" {
		resource = baseURL
	}

	metadata := ProtectedResourceMetadata{
		Resource:                 resource,
		AuthorizationServers:     []string{config.Issuer},
		BearerMethodsSupported:   config.ResourceMetadata.BearerMethods,
		ResourceSigningAlgValues: config.ResourceMetadata.SigningAlgs,
	}

	// Add DCR endpoint if configured
	if config.DCREndpoint != "" {
		metadata.ClientRegistrationEndpoint = config.DCREndpoint
	}

	c.Header("Content-Type", "application/json")
	c.Header("Cache-Control", "public, max-age=3600")

	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", "  ")
	c.Writer.WriteHeader(http.StatusOK)
	if err := encoder.Encode(metadata); err != nil {
		log.WithError(err).Error("Failed to encode protected resource metadata")
	}
}
