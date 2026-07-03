# Auth.md

## Agent authentication policy

`arleo.eu` exposes two complementary authentication layers for AI agents.

## Public MCP (no registration required)

- Tools available without authentication: `list_pages`, `get_page`, `search_pages`, `get_recent_posts`, `list_tags`, `list_categories`, `get_sitemap`, `get_feed`, `get_site_information`
- No write capability. No private data. No admin access.
- No OAuth/OIDC login required for these tools.

## Agent registration (auth.md protocol)

Agents can self-register via the [auth.md](https://auth-md.com/) protocol to obtain an assertion token
and exchange it for a bearer token granting access to enriched content tools.

**Discovery**: `https://mcp.arleo.eu/.well-known/oauth-authorization-server`

### Identity endpoint

```
POST https://mcp.arleo.eu/agent/identity
Content-Type: application/json

{"type": "anonymous"}
```

Response includes `identity_assertion` (assertion token) and optional claim info.

### Token exchange

```
POST https://mcp.arleo.eu/token
Content-Type: application/x-www-form-urlencoded

grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer&assertion=<identity_assertion>
```

### Authenticated tools (bearer required)

- `get_full_page_markdown` — Returns the full Markdown-formatted content of a published page.
- `get_page_frontmatter` — Returns structured metadata for a published page including estimated reading time.
- `get_related_content` — Returns pages related to a given slug by shared tags or categories.
- `build_agent_context` — Returns a complete enriched context bundle: metadata, reading time, full Markdown content, and related pages.
- `export_agent_context` — Paginated export of page context bundles with optional tag or category filter.

### Access policy

- Read-only. No destructive actions.
- No private user data. No admin tools. No write operations.
- Content is limited to publicly published pages already accessible without authentication.
- Authentication grants richer content format, not access to private information.

## OAuth 2.0 (for agents supporting Authorization Code + PKCE)

- **Registration endpoint**: `https://mcp.arleo.eu/register` (Dynamic Client Registration, RFC 7591)
- **Authorization endpoint**: `https://mcp.arleo.eu/authorize`
- **Token endpoint**: `https://mcp.arleo.eu/token`
- **OAuth flow**: Authorization Code + PKCE (RFC 7636), scope `mcp`, `S256` required

## Discovery and policy

- Canonical site: `https://www.arleo.eu`
- Canonical MCP endpoint: `https://mcp.arleo.eu/mcp`
- Security contact: see `https://www.arleo.eu/security.txt`

## Scope

This document is public information only.
It does not authorize access to any private service, private host, or administrative action.
