# API Shield

API Shield should only validate the public MCP transport surface.

## Canonical scope

- host: `mcp.arleo.eu`
- path: `/mcp`
- method: `POST`

## What is intentionally excluded

- write endpoints
- auth flows
- admin or filesystem APIs
- source-repository access

## Operational note

Keep validation in monitor mode until the schema and the live traffic are stable.
