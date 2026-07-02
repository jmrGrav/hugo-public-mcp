# Installation

`hugo-public-mcp` is a read-only MCP server for the published output of a Hugo site.

## Inputs

- `site_root`: the published Hugo `public/` directory
- `site_url`: the canonical public site URL
- `site_name`: a display name for discovery cards and logs

## Runtime

- bind the service to `127.0.0.1`
- place a reverse proxy in front of it
- keep the proxy and origin TLS-only
- keep request bodies small and read-only

## Validation

Use the live validation steps in `docs/mvp-validation.md` after installation.
