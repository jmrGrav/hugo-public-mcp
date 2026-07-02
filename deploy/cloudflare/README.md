# Cloudflare deployment notes

`hugo-public-mcp` is intended to sit behind Cloudflare or another reverse proxy that can enforce HTTPS, rate limits, body limits, and origin lockdown.

## Recommended rules

- Proxy `mcp.arleo.eu` through Cloudflare.
- Lock the origin so only Cloudflare edge IP ranges and trusted LAN/admin ranges can reach TCP 80/443.
- Apply a rate limit to `/mcp` and `/mcp/events`.
- Keep the MCP origin private; do not expose the source repo or any write-capable endpoint.
- Use TLS end-to-end and keep the upstream on localhost when possible.

## Example policy

- allow: Cloudflare IP ranges
- allow: 192.168.1.0/24 or an equivalent admin subnet
- deny: all other sources

This project does not ship Cloudflare API automation. Keep those controls in your own infrastructure layer.
