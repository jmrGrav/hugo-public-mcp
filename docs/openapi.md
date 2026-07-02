# OpenAPI

The Cloudflare API Shield schema for the public MCP transport is stored at:

- `deploy/openapi/mcp.arleo.eu.openapi.json`

It is intentionally minimal and describes only:

- `POST /mcp`
- JSON-RPC request and response envelopes

It does not describe:

- write operations
- filesystem paths
- source repository access
- private infrastructure endpoints
