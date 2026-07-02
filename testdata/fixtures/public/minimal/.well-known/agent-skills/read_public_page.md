---
name: read_public_page
description: Read a published public page by indexed slug.
---

# read_public_page

Usage: read a published public page by slug.

Endpoint: `https://mcp.arleo.eu/mcp`

Inputs:
- slug

Outputs:
- page summary
- page content

Security:
- read-only
- no auth
- no writes

Limitations:
- slug must be indexed

