# Architecture

## Goal

Expose a Hugo site to MCP clients through a strict read-only surface.
The server must index the site from `public/` at startup and serve requests from memory whenever possible.

## High-level flow

```text
Hugo site export
  -> published public/
  -> startup indexer
  -> in-memory site index
  -> read-only MCP tools
  -> reverse proxy / Cloudflare / agent clients
```

## Design rules

- The server only reads from the configured published `public/` root.
- Search is index-based only.
- Requests must be resolved through an indexed slug or canonical URL.
- No file path is accepted from clients.
- No draft content is exposed.
- No write operations exist in the MVP.
- No command execution exists in the MVP.
- No Hugo rebuild exists in the MVP.
- No repository source access exists in the MVP.

## Main components

### Indexer

Builds an in-memory catalog at startup by scanning `public/`.

The index stores:

- slug
- title
- summary
- canonical URL
- language
- date
- tags
- categories
- content type

### Search

Uses the startup index only.

Search scope:

- title
- summary
- tags
- categories
- canonical URL

Search does not parse HTML at request time.

### Content reader

Resolves an already indexed page and returns the published content for that slug.

The reader is not a filesystem API.

### MCP layer

Exposes only read-only tools and advertises read-only annotations.

### Transport layer

Works behind any reverse proxy that can provide HTTPS, logging, rate limiting, and request size limits.

## Future extraction candidates

These are **documented candidates only**. They must not be extracted during the MVP.

- `pathguard`
- `observability`
- helpers routes/langues
- parsing frontmatter
- validation root/path
- metadata MCP/tool annotations
- config/size limits

The rule is simple: keep them local until the public project is validated and the reuse boundary is clear.

