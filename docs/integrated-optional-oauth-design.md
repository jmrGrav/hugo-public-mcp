# Integrated Optional OAuth Design

Date: 2026-07-02

Status: design only. No production deployment, tag, release, or runtime
implementation is authorized by this document.

## Goal

Keep `mcp.arleo.eu` as a single canonical `hugo-public-mcp` service while adding
real optional OAuth capabilities by reusing mature components from
`mcp-runtime-go`.

The target architecture is:

```text
Cloudflare
  -> OpenResty
  -> mcp.arleo.eu
  -> hugo-public-mcp
       -> public read-only MCP tools
       -> optional OAuth discovery / DCR / PKCE / token endpoint
       -> scope-to-tool ACL
       -> direct in-memory index from Hugo public/
```

Production must not run `mcp-runtime-go` as a separate proxy in front of
`hugo-public-mcp`.

## Non-Goals

- No write tools.
- No Hugo rebuild.
- No shell.
- No access to `hugo-mcp-go` private/admin services.
- No access to the Hugo source repo.
- No double runtime in production.
- No fake OAuth, fake OIDC, fake JWKS, or fake registration.
- No production cutover until staging validates the integrated service.

## Current State

`hugo-public-mcp` currently provides:

- static/public Hugo `public/` indexing at startup;
- public read-only MCP tools;
- static discovery endpoints such as MCP card, OpenAPI, `auth.md`, API Catalog
  resources, and Agent Skills when present in the published site;
- no OAuth runtime.

`mcp-runtime-go` currently provides:

- OAuth Authorization Code + PKCE;
- Dynamic Client Registration;
- OAuth Authorization Server Metadata;
- OAuth Protected Resource Metadata;
- SQLite WAL token storage;
- audit logging;
- trusted proxy/source IP checks;
- optional anonymous MCP mode;
- scope-to-tool ACL for bearer-token traffic;
- MCP reverse-proxy behavior that is not wanted in the final `hugo-public-mcp`
  production architecture.

## Reuse Audit

| Element | Exists in `mcp-runtime-go` | Reusable | Adaptation Needed | New Code in `hugo-public-mcp` |
| --- | --- | --- | --- | --- |
| OAuth discovery metadata | `internal/oauthproxy.HandleMetadata` | Yes | Remove proxy-specific names; derive endpoint URLs from `hugo-public-mcp` request base/config | Route wiring and tests |
| Protected resource metadata | `internal/oauthproxy.HandleProtectedResourceMetadata` | Yes | Keep resource as `/mcp`; expose only when OAuth enabled | Route wiring and tests |
| Dynamic Client Registration | `RegisterClient`, `RegistrationRequest`, `RegistrationResponse` | Mostly | Decide single-client vs durable client registry before production | Config and tests |
| Authorization Code + PKCE | `IssueAuthCode`, `ExchangeToken`, PKCE helpers | Yes | Remove proxy/backend assumptions; keep operator-only authorize option | OAuth service package |
| Token endpoint | `HandleToken`, token response models | Yes | Re-home under shared OAuth component | Route wiring |
| JWKS | Not currently real | No | Do not publish until JWT/JWKS is real | None for MVP |
| SQLite token storage | `internal/storage.SQLiteStore` | Yes | Extract behind a generic `TokenStore` interface | Config, state path, systemd docs |
| Audit logging | `internal/observability.AuditLogger` | Partly | Reuse redaction patterns; integrate with `hugo-public-mcp` logging style | Minimal adapter |
| Request IP resolution | `internal/security.GetRequestInfo` | Yes | Keep trusted-proxy semantics; document OpenResty requirements | Config and tests |
| Redirect URI validation | `internal/security.IsAllowedRedirect` | Yes | Make allowlist config explicit in `hugo-public-mcp` | Config and tests |
| Random generation | `internal/security.GenerateRandomString` | Yes | Extract as utility | None beyond import |
| Scope-to-tool ACL | `AUTHENTICATED_SCOPE_TOOLS`, filter/block logic | Yes | Apply directly before `mcp.Server` dispatch, not as reverse proxy | MCP HTTP wrapper |
| Anonymous tools allowlist | Present in `mcp-runtime-go`; native `hugo-public-mcp` tools are already read-only | Conceptually | Keep anonymous behavior as current; optionally add explicit allowlist config for defense-in-depth | Tests |
| Reverse proxy | `internal/oauthproxy.buildReverseProxy` | No | Production goal explicitly avoids proxying to another MCP runtime | None |
| Backend bearer token injection | `HUGO_TOKEN` proxy behavior | No | Not needed and not wanted | None |
| Metrics | `internal/observability` | Maybe later | Keep out of MVP unless already aligned | None for design phase |

## Recommended Approach

Use an incremental shared-component extraction, not copy-paste and not a third
large refactor up front.

1. In `mcp-runtime-go`, define a clean internal boundary around the reusable
   OAuth domain: request models, metadata builders, PKCE, auth-code issuance,
   token exchange, token store interface, SQLite store, source-IP utilities,
   and scope-to-tool ACL.
2. Keep `mcp-runtime-go` behavior unchanged by adapting its existing handlers to
   the new internal boundary.
3. After the boundary is stable and tested, move the reusable code into a small
   shared module only if both repositories can consume it cleanly.
4. Add `oauth.enabled=false` default to `hugo-public-mcp`; default behavior must
   match v0.0.1.
5. Add `oauth.enabled=true` mode in `hugo-public-mcp` that wires real OAuth
   routes into the same HTTP service and applies scope-to-tool ACL before MCP
   dispatch.
6. Validate the integrated service on `staging-mcp.arleo.eu`.

This avoids a production double-runtime and avoids a premature
`hugo-mcp-core`-style refactor.

## Configuration Shape

Default behavior:

```yaml
oauth:
  enabled: false
```

Production-candidate optional OAuth shape:

```yaml
oauth:
  enabled: true
  issuer: https://mcp.arleo.eu
  resource: https://mcp.arleo.eu/mcp
  dynamic_client_registration: true
  authorize_mode: operator_only
  trusted_proxies:
    - 127.0.0.1
    - ::1
  trusted_authorize_cidrs:
    - 82.65.145.189/32
    - 192.168.1.0/24
  redirect_uri_allowlist:
    - https://claude.ai/callback
    - https://chatgpt.com/aip/oauth/callback
  token_store:
    driver: sqlite
    path: /var/lib/hugo-public-mcp/oauth-tokens.db
  scopes:
    mcp:
      tools:
        - list_pages
        - get_page
        - search_pages
        - get_recent_posts
        - list_tags
        - list_categories
        - get_sitemap
        - get_feed
        - get_site_information
```

No private scope is defined for the MVP.

## Runtime Behavior

When `oauth.enabled=false`:

- `/.well-known/oauth-authorization-server` returns `404`;
- `/.well-known/oauth-protected-resource` returns `404`;
- `/authorize`, `/token`, `/register` return `404`;
- `/mcp` behavior remains the current public read-only behavior.

When `oauth.enabled=true`:

- unauthenticated MCP requests continue to work for public read-only tools;
- invalid bearer tokens return `401` with `WWW-Authenticate`;
- valid bearer tokens are checked against the configured scope-to-tool ACL;
- valid bearer tokens currently unlock only the same public read-only tools;
- `/authorize` is operator-only unless a real public consent model is designed;
- DCR is real only if enabled and backed by tested client registration behavior.

## `/authorize` Decision

The current safe default is `authorize_mode: operator_only`.

`authorize_mode: public` is a separate design decision. It requires a real
consent or user-auth model. It must not be enabled only to improve scanner
scores.

## Extraction Plan

Do not extract a new module immediately.

First candidate boundary inside `mcp-runtime-go`:

```text
internal/oauthcore/
  metadata.go
  models.go
  pkce.go
  redirect.go
  service.go
  token_store.go
  scope_tools.go
  request_info.go
  audit_redaction.go
```

After both repositories consume the boundary cleanly, consider a future public
or private module:

```text
github.com/jmrGrav/mcp-oauth-go
```

Do not publish a shared module until:

- `mcp-runtime-go` still passes its existing OAuth tests;
- `hugo-public-mcp` consumes it without importing proxy-specific concepts;
- staging validates the integrated service;
- the API surface is small enough to maintain.

## Staging Plan

Use `staging-mcp.arleo.eu` for the integrated service.

Required staging checks:

- anonymous `tools/list` returns only public read-only tools;
- anonymous public `tools/call` works;
- anonymous non-public `tools/call` returns `403`;
- invalid bearer returns `401` with `WWW-Authenticate`;
- DCR + PKCE + token exchange works when OAuth is enabled;
- valid bearer still sees only public read-only tools;
- `/authorize` respects operator-only CIDR policy;
- no `/home/jm`, `192.168.`, `.git`, token, secret, source path, or private
  hostname appears in any public response;
- IsItAgentReady does not regress compared with current `mcp.arleo.eu`;
- production `mcp.arleo.eu` remains unchanged until explicit cutover approval.

## Rollback

Because production is not touched during design and staging, rollback is:

```bash
sudo systemctl disable --now hugo-public-mcp-staging.service
sudo rm -f /usr/local/openresty/nginx/conf/sites-enabled/staging-mcp.arleo.eu
sudo openresty -t
sudo systemctl reload openresty
```

Production rollback is only needed after a future explicit cutover. That
rollback should restore the current `hugo-public-mcp` v0.0.1 binary and config.

## GO / NO-GO

GO:

- open issues for extraction, optional OAuth mode, integrated staging, and
  `/authorize` policy;
- implement design and tests behind `oauth.enabled=false` default;
- validate on staging only.

NO-GO:

- production cutover;
- copying large blocks from `mcp-runtime-go` into `hugo-public-mcp`;
- fake OAuth/OIDC/JWKS endpoints;
- adding any write/admin/private tools;
- public `/authorize` without consent/user-auth design.
