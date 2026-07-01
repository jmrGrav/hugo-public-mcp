# hugo-public-mcp

`hugo-public-mcp` is a read-only MCP server for static Hugo sites.

It indexes a published `public/` directory at startup and exposes only safe discovery and read APIs for agents:

- `list_pages`
- `get_page`
- `search_pages`
- `get_recent_posts`
- `list_tags`
- `list_categories`
- `get_sitemap`
- `get_feed`
- `get_site_information`

## Security posture

This project is intentionally narrow:

- read-only strict
- no shell
- no rebuild
- no writes
- no access to the Hugo source repo
- no direct file paths in the public API
- no draft content
- no runtime HTML scraping for search
- no disk access during requests when the in-memory index is sufficient

## Project goal

Provide a reusable, secure, Hugo-oriented MCP surface that improves agent discoverability and content reading without creating an execution platform.

## Repository status

This repository currently contains the MVP design documents only.
Implementation comes after design review and validation.

