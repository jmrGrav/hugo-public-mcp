# DNS-AID

DNS for AI Discovery is treated as an optional discovery surface.

## Current example records

- `_index._agents.arleo.eu`
- `_mcp._agents.arleo.eu`

Both should resolve to the canonical MCP host.

## Security boundary

- discovery only
- no write capability
- no secret material
- no private hostnames or IPs

## Status

This repository documents the pattern but does not depend on DNS-AID for MVP operation.
