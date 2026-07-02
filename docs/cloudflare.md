# Cloudflare

Cloudflare is used only as a front-door control plane for transport security and origin lockdown.

## Recommended controls

- orange-cloud the public MCP hostname
- keep origin access limited to Cloudflare and trusted LAN/admin ranges
- enforce a 1 MiB body limit
- rate-limit `/mcp`
- keep schema validation in monitor mode until the schema is proven stable

## API Shield

The canonical schema file in this repository is:

- `deploy/openapi/mcp.arleo.eu.openapi.json`

It describes only `POST /mcp`.
