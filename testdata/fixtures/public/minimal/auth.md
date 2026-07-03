# Auth.md

## Agent authentication policy

`arleo.eu` offers an optional OAuth 2.0 layer for agents that need authenticated access to richer content tools.
Anonymous read-only access to all public MCP tools remains available without registration.

## Public MCP (no registration required)

- Tools available without authentication: `list_pages`, `get_page`, `search_pages`, `get_recent_posts`, `list_tags`, `list_categories`, `get_sitemap`, `get_feed`, `get_site_information`
- No write capability. No private data. No admin access.
- No OAuth/OIDC login required for these tools.

## Agent registration (optional)

Registration grants access to authenticated-only tools via bearer token.

- **Registration endpoint**: `https://mcp.arleo.eu/register` (Dynamic Client Registration, RFC 7591)
- **Authorization server**: `https://mcp.arleo.eu`
- **Authorization endpoint**: `https://mcp.arleo.eu/authorize`
- **Token endpoint**: `https://mcp.arleo.eu/token`
- **OAuth flow**: Authorization Code + PKCE (RFC 7636)
- **Credential type**: Bearer token
- **Scope**: `mcp`
- **PKCE required**: yes (`S256` method)

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

## Discovery and policy

- Canonical site: `https://www.arleo.eu`
- Canonical MCP endpoint: `https://mcp.arleo.eu/mcp`
- Security contact: see `https://www.arleo.eu/security.txt`

## Scope

This document is public information only.
It does not authorize access to any private service, private host, or administrative action.
