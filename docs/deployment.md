# Deployment

Use the deployment pattern in `docs/deployment-pattern.md`.

The supported production layout is:

```text
Cloudflare proxied host
  -> reverse proxy
  -> MCP service bound to 127.0.0.1
  -> Hugo public/ tree
```

Do not expose the application port directly to the Internet.
