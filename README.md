# hugo-public-mcp

`hugo-public-mcp` is a strict read-only MCP server for published Hugo sites.

It builds an in-memory index from a Hugo `public/` tree at startup and exposes only safe discovery and read tools:

- `list_pages`
- `get_page`
- `search_pages`
- `get_recent_posts`
- `list_tags`
- `list_categories`
- `get_sitemap`
- `get_feed`
- `get_site_information`

## Current status

This repository is at release-candidate quality for the MVP scope:

- startup indexing from `public/`
- read-only MCP tools
- request-path validation
- symlink and traversal rejection
- security-oriented HTTP transport guards
- unit, integration, and scale tests
- CI and lint configuration
- deployment examples for systemd, nginx/openresty, Caddy, Traefik, and Cloudflare
- GitHub repository hardening files, templates, and automated security scans

## Security posture

The public server is intentionally narrow:

- read-only strict
- no shell
- no rebuild
- no writes
- no access to the Hugo source repo
- no direct file paths in the public API
- no draft content
- no runtime HTML scraping for search
- no disk access during requests when the in-memory index is sufficient
- no mutation-capable MCP tools

## Repository governance

The repository includes:

- branch protection rules for `main`
- GitHub Actions CI, lint, CodeQL, and secret scanning
- Dependabot updates
- issue and pull request templates
- a CODEOWNERS file and support guidance

## Configuration

See `examples/arleo.eu/config.example.yaml` for a concrete published-site example.
The binary reads its YAML config from `HUGO_PUBLIC_MCP_CONFIG` and supports `stdio` or `http` transport.

## Documentation

- `docs/installation.md`
- `docs/deployment.md`
- `docs/cloudflare.md`
- `docs/api-shield.md`
- `docs/dns-aid.md`
- `docs/openapi.md`
- `docs/oauth-runtime-evaluation.md`
- `docs/architecture.md`
- `docs/threat-model.md`
- `docs/mvp-validation.md`
- `docs/security-tests.md`
- `docs/go-interfaces.md`
- `docs/deployment-pattern.md`
- `docs/release-readiness.md`

## Development

Run the test suite:

```bash
go test ./...
```

Run lint:

```bash
golangci-lint run ./...
```

## Project goal

Provide a reusable, secure, Hugo-oriented MCP surface that improves agent discoverability and content reading without creating an execution platform.
