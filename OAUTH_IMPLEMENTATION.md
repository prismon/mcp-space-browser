# OAuth/OIDC Implementation Summary

## Overview

This document summarizes the OAuth 2.1 / OIDC authentication implementation for MCP Space Browser, which provides unified authentication for both REST API and MCP endpoints.

## Implementation Details

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    MCP Space Browser Server                  │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │           Shared Token Validation Layer                 │ │
│  │  - JWT validation (offline)                             │ │
│  │  - OIDC provider integration (Okta/Auth0/Google)       │ │
│  │  - Token caching (5-min TTL)                            │ │
│  │  - User context extraction                              │ │
│  └────────────────────────────────────────────────────────┘ │
│           ▲                                  ▲               │
│           │                                  │               │
│  ┌────────┴──────────┐           ┌──────────┴────────────┐ │
│  │   Gin Middleware  │           │  HTTP Wrapper         │ │
│  │  (REST endpoints) │           │  (MCP endpoint)       │ │
│  └───────────────────┘           └───────────────────────┘ │
│           │                                  │               │
│  ┌────────┴──────────┐           ┌──────────┴────────────┐ │
│  │  /api/index       │           │      /mcp             │ │
│  │  /api/tree        │           │  (MCP Protocol)       │ │
│  │  /api/inspect     │           │                       │ │
│  │  /api/content     │           │                       │ │
│  └───────────────────┘           └───────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Components Created

#### 1. Configuration System (`pkg/auth/config.go`)
- YAML-based configuration with environment variable overrides
- Supports multiple OAuth providers (Okta, Auth0, Google, Azure, generic)
- Flexible authentication modes (disabled, optional, required)

#### 2. Token Validator (`pkg/auth/validator.go`)
- JWT token parsing and validation
- JWKS-based signature verification
- Token caching with 5-minute TTL
- Auto-discovery of JWKS endpoint from issuer
- Audience and issuer validation

#### 3. REST API Middleware (`pkg/auth/middleware.go`)
- Gin middleware for REST endpoint protection
- RFC 6750 compliant error responses
- User context injection into Gin context
- Flexible auth requirements (per-route or global)

#### 4. MCP Integration (`pkg/auth/mcp_integration.go`)
- HTTP handler wrapper for MCP endpoint protection
- Token validation before MCP request processing
- User context injection into request context
- RFC 6750 compliant error responses

#### 5. Protected Resource Metadata (`pkg/auth/resource_metadata.go`)
- RFC 9728 compliant endpoint implementation
- OAuth discovery for MCP clients
- Dynamic Client Registration (DCR) endpoint exposure

#### 6. Context Utilities (`pkg/auth/context.go`)
- User context management for tool handlers
- Type-safe context key usage

### Files Modified

- **`pkg/server/server.go`**: Integrated OAuth middleware for REST and MCP endpoints
- **`pkg/server/inspect_artifacts.go`**: Removed custom HMAC token system, now uses OAuth
- **`cmd/mcp-space-browser/main.go`**: Added configuration loading and CLI flags
- **`go.mod`**: Added JWT and JWKS dependencies

### Files Created

- **`config.yaml`**: YAML configuration template
- **`pkg/auth/config.go`**: Configuration management
- **`pkg/auth/validator.go`**: Token validation
- **`pkg/auth/middleware.go`**: Gin middleware
- **`pkg/auth/mcp_integration.go`**: MCP OAuth wrapper
- **`pkg/auth/resource_metadata.go`**: RFC 9728 endpoint
- **`pkg/auth/context.go`**: Context utilities
- **`OAUTH_DCR_GUIDE.md`**: Comprehensive DCR documentation

## Configuration

### YAML Configuration (config.yaml)

```yaml
auth:
  enabled: false                    # Enable/disable OAuth
  require_auth: false               # Require authentication for all endpoints
  provider: "okta"                  # okta, auth0, google, azure, generic
  issuer: "https://your-domain.okta.com"
  audience: "api://mcp-space-browser"
  jwks_url: ""                      # Auto-discovered if not provided
  dcr_endpoint: ""                  # For Dynamic Client Registration
  cache_ttl_minutes: 5              # Token cache TTL
```

### Environment Variables

```bash
AUTH_ENABLED=true
AUTH_REQUIRE_AUTH=true
AUTH_PROVIDER=okta
AUTH_ISSUER=https://your-domain.okta.com
AUTH_AUDIENCE=api://mcp-space-browser
AUTH_DCR_ENDPOINT=https://your-domain.okta.com/oauth2/v1/clients
```

### Command Line Flags

```bash
# Start server with config file
./mcp-space-browser server --config=config.yaml

# Override port from CLI
./mcp-space-browser server --config=config.yaml --port=8080
```

## Features

### ✅ Implemented

1. **Unified Authentication**: Single OAuth validator for REST and MCP
2. **OIDC Compliance**: Works with standard OIDC providers
3. **JWT Validation**: Offline token validation using JWKS
4. **Token Caching**: 5-minute cache for performance
5. **Flexible Auth Modes**:
   - Disabled: No authentication
   - Optional: Auth validated but not required
   - Required: All requests must be authenticated
6. **RFC 9728 Compliance**: Protected Resource Metadata endpoint
7. **DCR Support**: Dynamic Client Registration endpoint exposure
8. **User Context**: Authenticated user info available in handlers
9. **YAML Configuration**: Easy configuration management
10. **Environment Overrides**: Support for env vars and CLI flags

### 🔄 Security Improvements

**Before** (Custom Token System):
- HMAC-based content tokens with 10-minute expiry
- Server-generated secrets (lost on restart)
- No user identity tracking
- Content access based on time-limited tokens only

**After** (OAuth 2.1):
- Industry-standard JWT tokens
- Cryptographic signature validation
- User identity and claims available
- Centralized authorization server
- Audit trail of authenticated users
- Token refresh support

## Usage Examples

### 1. Running Without Auth (Development)

```bash
# Default config has auth disabled
./mcp-space-browser server

# Test endpoints
curl http://localhost:3000/api/tree?path=/tmp
```

### 2. Running With Optional Auth

```yaml
# config.yaml
auth:
  enabled: true
  require_auth: false  # Auth checked but not required
  provider: "okta"
  issuer: "https://dev-12345.okta.com"
  audience: "api://mcp-space-browser"
```

```bash
./mcp-space-browser server

# Works without token
curl http://localhost:3000/api/tree?path=/tmp

# Works with token (user context available)
curl -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/tree?path=/tmp
```

### 3. Running With Required Auth

```yaml
# config.yaml
auth:
  enabled: true
  require_auth: true  # Auth is required
```

```bash
./mcp-space-browser server

# Fails without token (401 Unauthorized)
curl -v http://localhost:3000/api/tree?path=/tmp

# Succeeds with valid token
curl -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/tree?path=/tmp
```

### 4. MCP Client Discovery

```bash
# MCP clients discover OAuth requirements
curl http://localhost:3000/.well-known/oauth-protected-resource

# Response includes:
{
  "resource": "http://localhost:3000",
  "authorization_servers": ["https://dev-12345.okta.com"],
  "bearer_methods_supported": ["header"],
  "resource_signing_alg_values_supported": ["RS256", "ES256"],
  "client_registration_endpoint": "https://dev-12345.okta.com/oauth2/v1/clients"
}
```

### 5. Accessing User Context in Handlers

```go
// REST API handler
func handleIndex(c *gin.Context, db *database.DiskDB) {
    if user, ok := auth.GetUser(c); ok {
        log.Infof("User %s (%s) indexing path", user.Subject, user.Email)
    }
    // ... rest of handler
}

// MCP tool handler
func registerIndexTool(s *server.MCPServer, db *database.DiskDB) {
    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        if user, ok := auth.GetUserFromContext(ctx); ok {
            log.Infof("User %s invoking disk-index tool", user.Subject)
        }
        // ... rest of handler
    })
}
```

## Testing

### Build and Run

```bash
# Build
go build -o mcp-space-browser ./cmd/mcp-space-browser

# Run with default config (auth disabled)
./mcp-space-browser server

# Run with custom config
./mcp-space-browser server --config=prod-config.yaml

# Run with env vars
AUTH_ENABLED=true \
AUTH_ISSUER=https://dev-12345.okta.com \
AUTH_AUDIENCE=api://mcp-space-browser \
./mcp-space-browser server
```

### Test Endpoints

```bash
# Check Protected Resource Metadata
curl http://localhost:3000/.well-known/oauth-protected-resource | jq

# Test REST API
curl http://localhost:3000/api/tree?path=/tmp

# Test with auth
curl -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/tree?path=/tmp

# Test MCP endpoint
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"method":"tools/list"}'
```

## Migration Guide

### From Custom Tokens to OAuth

**Before**:
```
GET /api/content?token=<base64-hmac-token>
```

**After**:
```
GET /api/content?path=/file/path
Authorization: Bearer <jwt-access-token>
```

### Content URLs

**Before** (`inspect_artifacts.go`):
```json
{
  "contentUrl": "http://localhost:3000/api/content?token=eyJhbGc..."
}
```

**After**:
```json
{
  "contentUrl": "http://localhost:3000/api/content?path=/file/path"
}
```

Clients must include `Authorization: Bearer <token>` header when fetching content.

## Security Considerations

1. **HTTPS Required**: Always use HTTPS in production
2. **Token Storage**: JWTs in Authorization header (not in URLs)
3. **Token Expiry**: Tokens expire based on OAuth provider settings
4. **Refresh Tokens**: Use refresh tokens for long-lived sessions
5. **Scope Validation**: Configure appropriate OAuth scopes
6. **DCR Security**: Require initial access tokens for DCR in production
7. **Audit Logging**: Log all authenticated user actions

## Dynamic Client Registration (DCR)

### When to Use DCR

✅ **USE DCR for:**
- MCP clients (Claude Desktop, AI agents)
- Multi-tenant applications
- Microservices architectures
- Enterprise deployments

❌ **DON'T USE DCR for:**
- Browser-based SPAs
- Mobile apps
- Static client credentials

See `OAUTH_DCR_GUIDE.md` for complete DCR documentation.

## Troubleshooting

### Server fails to start

- Check `config.yaml` syntax
- Verify `auth.issuer` and `auth.audience` when auth is enabled
- Ensure database path is writable

### 401 Unauthorized errors

- Verify token hasn't expired (`exp` claim)
- Check `aud` claim matches `config.auth.audience`
- Check `iss` claim matches `config.auth.issuer`
- Verify JWKS URL is accessible

### Token validation slow

- Check token cache TTL (default: 5 minutes)
- Verify JWKS endpoint is responsive
- Consider increasing `cache_ttl_minutes`

## References

- [OAuth 2.1](https://oauth.net/2.1/)
- [RFC 9728 - OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728)
- [RFC 7591 - OAuth 2.0 Dynamic Client Registration](https://datatracker.ietf.org/doc/html/rfc7591)
- [RFC 6750 - OAuth 2.0 Bearer Token Usage](https://datatracker.ietf.org/doc/html/rfc6750)
- [MCP OAuth Specification (2025-03-26)](https://modelcontextprotocol.io/specification/2025-03-26/basic/authorization)
- [OIDC Discovery](https://openid.net/specs/openid-connect-discovery-1_0.html)
