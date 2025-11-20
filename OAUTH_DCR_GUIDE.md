# OAuth 2.1 and Dynamic Client Registration (DCR) Guide

## Overview

MCP Space Browser now supports OAuth 2.1 authentication for both REST API and MCP endpoints. This guide explains when and how to use Dynamic Client Registration (DCR) for OAuth clients.

## Table of Contents

1. [When to Use DCR](#when-to-use-dcr)
2. [OAuth Architecture](#oauth-architecture)
3. [Client Configuration](#client-configuration)
4. [MCP Client Integration](#mcp-client-integration)
5. [Testing](#testing)

---

## When to Use DCR

### What is Dynamic Client Registration (DCR)?

Dynamic Client Registration (RFC 7591) allows OAuth clients to programmatically register themselves with an authorization server at runtime, without requiring manual pre-registration.

### Use Cases for DCR

**✅ USE DCR when:**

1. **MCP Agents/AI Clients** - MCP clients like Claude Desktop should use DCR to automatically obtain client credentials when first connecting to the MCP server

2. **Multi-Tenant Applications** - When you have many different client applications that need to connect, and manually registering each one is impractical

3. **Microservices Architecture** - Service-to-service communication where services need to act as confidential OAuth clients

4. **Enterprise Deployments** - Large-scale deployments where pre-registering hundreds of clients is not feasible

5. **Development/Testing** - Automatically provision test clients in CI/CD pipelines

**❌ DON'T USE DCR when:**

1. **Browser-Based SPAs** - Single-page applications running in browsers cannot securely store initial access tokens or client secrets required for DCR

2. **Mobile Apps** - Native mobile applications typically cannot use DCR securely since they cannot protect initial access tokens

3. **Static Client Credentials** - When you have a fixed set of clients that can be manually pre-registered in your OAuth provider

4. **Security Restrictions** - When your OAuth provider doesn't support DCR or requires manual approval for all clients

### MCP Client Scenario

For MCP clients (like Claude Desktop, AI agents, or MCP-enabled applications):

```
┌─────────────────────────────────────────────────────────────────┐
│                     First Connection Flow                        │
└─────────────────────────────────────────────────────────────────┘

1. MCP Client attempts to connect to MCP Server
2. MCP Server returns 401 Unauthorized with OAuth metadata
3. MCP Client discovers DCR endpoint from Protected Resource Metadata
4. MCP Client calls DCR endpoint to register itself
5. Authorization Server returns client_id and client_secret
6. MCP Client initiates OAuth 2.1 authorization flow (PKCE)
7. User authenticates and grants consent
8. MCP Client receives access token
9. MCP Client makes authenticated requests to MCP Server
```

---

## OAuth Architecture

### Components

```
┌─────────────────┐         ┌──────────────────┐         ┌─────────────────┐
│                 │         │                  │         │                 │
│   MCP Client    │────────▶│  MCP Server      │────────▶│  Authorization  │
│  (Claude, etc)  │         │  (Resource       │         │  Server         │
│                 │◀────────│   Server)        │◀────────│  (Okta/Auth0/   │
│                 │         │                  │         │   Google/etc)   │
└─────────────────┘         └──────────────────┘         └─────────────────┘
     │                              │                            │
     │   1. GET /mcp (no token)    │                            │
     │────────────────────────────▶│                            │
     │   2. 401 + OAuth metadata   │                            │
     │◀────────────────────────────│                            │
     │   3. GET /.well-known/      │                            │
     │      oauth-protected-       │                            │
     │      resource               │                            │
     │────────────────────────────▶│                            │
     │   4. Returns issuer + DCR   │                            │
     │◀────────────────────────────│                            │
     │   5. POST /oauth2/register  │                            │
     │      (Dynamic Client Reg)                                │
     │────────────────────────────────────────────────────────▶│
     │   6. Returns client_id/     │                            │
     │      client_secret          │                            │
     │◀────────────────────────────────────────────────────────│
     │   7. OAuth 2.1 + PKCE flow  │                            │
     │────────────────────────────────────────────────────────▶│
     │   8. User authenticates     │                            │
     │◀────────────────────────────────────────────────────────│
     │   9. Receive access_token   │                            │
     │◀────────────────────────────────────────────────────────│
     │  10. GET /mcp + Bearer token│                            │
     │────────────────────────────▶│                            │
     │                             │  11. Validate token        │
     │                             │───────────────────────────▶│
     │                             │◀───────────────────────────│
     │  12. Protected MCP response │                            │
     │◀────────────────────────────│                            │
```

### Protected Resource Metadata (RFC 9728)

The MCP server exposes OAuth metadata at:

```
GET /.well-known/oauth-protected-resource
```

Response:
```json
{
  "resource": "http://localhost:3000",
  "authorization_servers": [
    "https://your-domain.okta.com"
  ],
  "bearer_methods_supported": ["header"],
  "resource_signing_alg_values_supported": ["RS256", "ES256"],
  "client_registration_endpoint": "https://your-domain.okta.com/oauth2/v1/clients"
}
```

This tells MCP clients:
- Where to get tokens (`authorization_servers`)
- How to send tokens (`bearer_methods_supported`)
- **Where to register via DCR** (`client_registration_endpoint`)

---

## Client Configuration

### Server Configuration (config.yaml)

```yaml
auth:
  enabled: true
  require_auth: true
  provider: "okta"  # or auth0, google, azure, generic
  issuer: "https://your-domain.okta.com"
  audience: "api://mcp-space-browser"

  # Enable DCR by providing the registration endpoint
  dcr_endpoint: "https://your-domain.okta.com/oauth2/v1/clients"

  resource_metadata:
    resource: "https://your-mcp-server.com"
    bearer_methods: ["header"]
    signing_algs: ["RS256", "ES256"]
```

### Environment Variables

```bash
# Enable OAuth
AUTH_ENABLED=true
AUTH_REQUIRE_AUTH=true
AUTH_PROVIDER=okta
AUTH_ISSUER=https://your-domain.okta.com
AUTH_AUDIENCE=api://mcp-space-browser

# Enable DCR
AUTH_DCR_ENDPOINT=https://your-domain.okta.com/oauth2/v1/clients
```

### OAuth Provider Setup

#### Okta

1. **Create API Service** in Okta Admin Console
2. **Enable Dynamic Client Registration:**
   - Go to Security → API → Authorization Servers
   - Select your authorization server
   - Go to Settings → Dynamic Client Registration
   - Enable "Allow Dynamic Client Registration"
3. **Configure Initial Access Token** (optional but recommended):
   - Require initial access tokens for registration
   - Generate initial access token for your MCP clients
4. **Note the DCR endpoint:**
   ```
   https://your-domain.okta.com/oauth2/v1/clients
   ```

#### Auth0

1. **Enable DCR in Auth0 Dashboard**
   - Go to Applications → Application Settings
   - Enable "Dynamic Client Registration"
2. **DCR endpoint:**
   ```
   https://your-domain.auth0.com/oidc/register
   ```

#### Google Workspace

Google does not support public DCR. Clients must be manually registered in Google Cloud Console.

#### Azure AD

1. **Enable app registration** in Azure Portal
2. **DCR endpoint:**
   ```
   https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/register
   ```

---

## MCP Client Integration

### Example: MCP Client with DCR

```typescript
// Example MCP client implementation with DCR support

class MCPClient {
  private baseURL: string;
  private clientId?: string;
  private clientSecret?: string;
  private accessToken?: string;

  async connect() {
    // 1. Attempt unauthenticated request
    const response = await fetch(`${this.baseURL}/mcp`);

    if (response.status === 401) {
      // 2. Get OAuth metadata
      const metadata = await this.getProtectedResourceMetadata();

      // 3. Check if DCR is available
      if (metadata.client_registration_endpoint) {
        await this.registerClient(metadata.client_registration_endpoint);
      } else {
        throw new Error("OAuth required but DCR not available");
      }

      // 4. Perform OAuth 2.1 flow
      await this.performOAuthFlow(metadata.authorization_servers[0]);
    }
  }

  async getProtectedResourceMetadata() {
    const response = await fetch(
      `${this.baseURL}/.well-known/oauth-protected-resource`
    );
    return await response.json();
  }

  async registerClient(dcrEndpoint: string) {
    const response = await fetch(dcrEndpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        // Optional: include initial access token if required
        // 'Authorization': `Bearer ${initialAccessToken}`
      },
      body: JSON.stringify({
        client_name: 'MCP Client',
        grant_types: ['authorization_code', 'refresh_token'],
        response_types: ['code'],
        redirect_uris: ['http://localhost:8080/callback'],
        token_endpoint_auth_method: 'client_secret_basic'
      })
    });

    const registration = await response.json();
    this.clientId = registration.client_id;
    this.clientSecret = registration.client_secret;
  }

  async performOAuthFlow(issuer: string) {
    // Implement OAuth 2.1 with PKCE
    // 1. Generate code verifier and challenge
    // 2. Redirect user to authorization endpoint
    // 3. Exchange code for access token
    // 4. Store access token
  }

  async callMCPTool(toolName: string, args: any) {
    const response = await fetch(`${this.baseURL}/mcp`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.accessToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        method: 'tools/call',
        params: { name: toolName, arguments: args }
      })
    });

    return await response.json();
  }
}
```

### Go Example

```go
package main

import (
    "encoding/json"
    "net/http"
)

type ProtectedResourceMetadata struct {
    AuthorizationServers      []string `json:"authorization_servers"`
    ClientRegistrationEndpoint string   `json:"client_registration_endpoint"`
}

type DCRRequest struct {
    ClientName              string   `json:"client_name"`
    GrantTypes              []string `json:"grant_types"`
    ResponseTypes           []string `json:"response_types"`
    RedirectURIs            []string `json:"redirect_uris"`
    TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type DCRResponse struct {
    ClientID     string `json:"client_id"`
    ClientSecret string `json:"client_secret"`
}

func registerMCPClient(mcpServerURL string) (*DCRResponse, error) {
    // Get protected resource metadata
    resp, err := http.Get(mcpServerURL + "/.well-known/oauth-protected-resource")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var metadata ProtectedResourceMetadata
    if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
        return nil, err
    }

    // Register via DCR
    dcrReq := DCRRequest{
        ClientName:              "MCP Go Client",
        GrantTypes:              []string{"authorization_code", "refresh_token"},
        ResponseTypes:           []string{"code"},
        RedirectURIs:            []string{"http://localhost:8080/callback"},
        TokenEndpointAuthMethod: "client_secret_basic",
    }

    // ... POST to metadata.ClientRegistrationEndpoint
    // ... return client_id and client_secret
}
```

---

## Testing

### 1. Test Without Auth (Default)

```bash
# Start server with auth disabled
./mcp-space-browser server

# Test REST API
curl http://localhost:3000/api/tree?path=/tmp

# Test MCP endpoint
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"method":"tools/list"}'
```

### 2. Test With OAuth (Permissive Mode)

```yaml
# config.yaml
auth:
  enabled: true
  require_auth: false  # Auth is checked but not required
  provider: "okta"
  issuer: "https://your-domain.okta.com"
  audience: "api://mcp-space-browser"
```

```bash
# Unauthenticated request works
curl http://localhost:3000/api/tree?path=/tmp

# Authenticated request includes user context
curl -H "Authorization: Bearer $ACCESS_TOKEN" \
  http://localhost:3000/api/tree?path=/tmp
```

### 3. Test With OAuth (Strict Mode)

```yaml
# config.yaml
auth:
  enabled: true
  require_auth: true  # Auth is required
```

```bash
# Unauthenticated request fails
curl -v http://localhost:3000/api/tree?path=/tmp
# Returns 401 Unauthorized with WWW-Authenticate header

# Authenticated request succeeds
curl -H "Authorization: Bearer $ACCESS_TOKEN" \
  http://localhost:3000/api/tree?path=/tmp
```

### 4. Test DCR Discovery

```bash
# Get Protected Resource Metadata
curl http://localhost:3000/.well-known/oauth-protected-resource | jq

# Response shows DCR endpoint
{
  "resource": "http://localhost:3000",
  "authorization_servers": [
    "https://your-domain.okta.com"
  ],
  "bearer_methods_supported": ["header"],
  "resource_signing_alg_values_supported": ["RS256", "ES256"],
  "client_registration_endpoint": "https://your-domain.okta.com/oauth2/v1/clients"
}
```

### 5. Test with Mock Tokens (Development)

For local testing without a real OAuth provider, you can use JWT.io to create test tokens:

```bash
# Create a test JWT at https://jwt.io with payload:
{
  "sub": "test-user",
  "iss": "https://your-domain.okta.com",
  "aud": "api://mcp-space-browser",
  "email": "test@example.com",
  "exp": 9999999999
}

# Note: Server will reject this unless you disable signature verification
# This is for development/testing only
```

---

## Security Considerations

1. **DCR Initial Access Tokens**: In production, require initial access tokens for DCR to prevent unauthorized client registration

2. **Client Secret Storage**: MCP clients must securely store client secrets obtained via DCR

3. **Token Expiry**: Access tokens expire and must be refreshed using refresh tokens

4. **HTTPS Required**: Always use HTTPS in production to protect tokens in transit

5. **Scope Validation**: Configure appropriate OAuth scopes for different client types

6. **Rate Limiting**: Implement rate limiting on DCR endpoint to prevent abuse

---

## Troubleshooting

### Client gets 401 even with valid token

- Check that `aud` (audience) claim matches `config.auth.audience`
- Check that `iss` (issuer) claim matches `config.auth.issuer`
- Verify token hasn't expired (`exp` claim)

### DCR endpoint not appearing in metadata

- Ensure `auth.dcr_endpoint` is configured in config.yaml
- Restart the server after configuration changes

### Client cannot validate server tokens

- Check that JWKS URL is accessible
- Verify clock synchronization between client and server
- Check that signing algorithms match

### OAuth provider rejects DCR request

- Verify DCR is enabled in your OAuth provider
- Check if initial access token is required
- Ensure request payload matches provider requirements

---

## References

- [RFC 7591 - OAuth 2.0 Dynamic Client Registration Protocol](https://datatracker.ietf.org/doc/html/rfc7591)
- [RFC 9728 - OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728)
- [MCP OAuth Specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/authorization)
- [OAuth 2.1](https://oauth.net/2.1/)
