# Deployment Pattern for `mcp.arleo.eu`

Recommended production shape:

```text
Cloudflare proxied hostname
  -> origin firewall allows Cloudflare + LAN/admin only
  -> OpenResty reverse proxy
  -> MCP service bound to 127.0.0.1
```

## Origin controls

- Keep `mcp.arleo.eu` orange-cloud proxied in Cloudflare.
- Restrict inbound TCP 80/443 at the origin firewall to Cloudflare IP ranges plus LAN/admin ranges.
- Do not expose the application port directly.
- Keep rate limiting at Cloudflare and optionally at OpenResty.
- Keep request body limits at 1m.
- Send MCP logs to a dedicated file or stream.

## Application controls

- Bind the MCP service to `127.0.0.1` only.
- Use the published Hugo `public/` tree as the only data source.
- Keep the service read-only.
- Do not add write-capable tools or shell access.

## Reverse proxy controls

- Terminate TLS at Cloudflare and/or OpenResty.
- Proxy `/mcp` and `/mcp/events` only.
- Keep proxy buffers and timeout values conservative.
- Use dedicated logs for the MCP host.

## Intentional exclusions

- no direct public access to the app port
- no source-repo access
- no draft exposure
- no API write surface
- no dynamic conversion endpoint

