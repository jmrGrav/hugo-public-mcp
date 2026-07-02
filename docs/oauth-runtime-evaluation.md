# Optional OAuth evaluation with mcp-runtime-go

Status: evaluated, not implemented.

This document evaluates whether `mcp-runtime-go` can be integrated with
`hugo-public-mcp` to expose real OAuth discovery and token handling while keeping
the public MCP endpoint anonymous and read-only.

## Required behavior

`hugo-public-mcp` has one non-negotiable public contract:

- unauthenticated MCP requests remain allowed;
- unauthenticated callers can access only public read-only tools;
- authenticated callers may use the same public read-only tools for now;
- invalid bearer tokens receive a clean `401` with `WWW-Authenticate`;
- no write tools, shell access, Hugo rebuild, source repository access, or admin
  MCP exposure may be introduced.

OAuth must be a real optional layer. The project must not publish fake OAuth,
OIDC, registration, token, JWKS, or protected-resource endpoints just to satisfy
agent-readiness scanners.

## mcp-runtime-go audit

The local `mcp-runtime-go` service is an OAuth 2.0 / PKCE proxy with SQLite WAL
token storage. It exposes:

- `/.well-known/oauth-authorization-server`;
- `/.well-known/oauth-protected-resource`;
- `/register`;
- `/authorize`;
- `/token`;
- `/mcp`;
- `/health`, `/healthz`, `/readyz`, `/metrics`.

Useful capabilities found:

- OAuth Authorization Code flow with mandatory PKCE S256;
- Dynamic Client Registration endpoint;
- SQLite WAL token storage;
- audit logging;
- trusted proxy handling;
- `WWW-Authenticate` metadata for unauthorized MCP requests;
- reverse proxying to a configured MCP backend.

Blocking incompatibility:

- `internal/oauthproxy/proxy.go` currently requires every `/mcp` request to have
  a valid `Authorization: Bearer ...` header.
- Missing bearer tokens return `401` with `"Bearer token required"`.
- Invalid bearer tokens return `401`.
- The runtime tests explicitly assert that unauthenticated `/mcp` reaches the
  proxy handler but returns an auth failure, not an anonymous read-only response.

That behavior is correct for an authenticated admin/protected MCP proxy, but it
does not satisfy this project's public read-only contract.

## Decision

Current decision: **NO-GO for production integration**.

Do not place `mcp-runtime-go` in front of `https://mcp.arleo.eu/mcp` until it
supports an explicit optional-auth mode that preserves anonymous read-only MCP
access.

The current production service should remain:

- `hugo-public-mcp` on the Hugo VM;
- OpenResty reverse proxy on the NUC;
- Cloudflare in front;
- public anonymous read-only MCP tools only;
- no OAuth discovery beyond documented absence of required auth.

## Required upstream changes before reconsideration

`mcp-runtime-go` would need a deliberate optional-auth mode, not a local
workaround:

- a configuration flag such as `ALLOW_ANONYMOUS_MCP=true`;
- anonymous `/mcp` requests proxy or dispatch only to an allowlisted public
  read-only tool set;
- valid bearer tokens are accepted but do not unlock private tools until a future
  explicit authorization model exists;
- invalid bearer tokens still return `401` with correct `WWW-Authenticate`;
- OAuth discovery remains real and consistent with the enabled behavior;
- protected resource metadata must not imply that auth is mandatory for public
  tools if anonymous access is enabled;
- tests must cover anonymous success, invalid-token failure, valid-token success,
  and no write-tool exposure.

If this mode is implemented upstream, it should be validated in local staging
before any production change.

## Staging validation requirements

Before enabling optional OAuth in production:

1. `curl` without `Authorization` against `/mcp` must succeed for public
   read-only tools.
2. `curl` with an invalid bearer token must return `401`.
3. A complete Authorization Code + PKCE flow must issue a token.
4. Dynamic Client Registration must work without exposing private admin access.
5. OAuth metadata must validate against the relevant RFCs.
6. No response may contain local paths, LAN IPs, hostnames, secrets, source Hugo
   files, drafts, or `.git`.
7. `go test ./...`, `go test -race ./...`, `go vet ./...`, and secret scanning
   must pass.
8. IsItAgentReady must not regress from the current public read-only behavior.

## Rollback plan

If optional OAuth is ever staged and fails validation:

1. remove the OAuth runtime from the OpenResty upstream;
2. point OpenResty back directly to `hugo-public-mcp`;
3. reload OpenResty after `openresty -t`;
4. stop the OAuth runtime;
5. purge only the affected Cloudflare URLs if cached;
6. verify anonymous `/mcp`, `.well-known/mcp.json`, `openapi.json`, health, API
   catalog, and agent skills.

