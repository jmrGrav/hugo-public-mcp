# Changelog

## Unreleased

- Added the MVP implementation for a read-only Hugo public MCP server.
- Defined the security boundary around the published `public/` tree.
- Added security and scale tests, including path traversal and symlink rejection.
- Added CI, linting, and deployment examples for systemd, nginx/openresty, Caddy, Traefik, and Cloudflare.
- Added read-only MCP tools for page discovery, content reading, sitemap, feed, and site metadata.
