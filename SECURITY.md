# Security Policy

## Scope

`hugo-public-mcp` is a read-only service for published Hugo sites.

## Supported behavior

- read-only discovery and content lookup
- startup indexing of the published `public/` directory
- reverse-proxy deployment behind HTTPS

## Not supported

- writes
- shell execution
- Hugo rebuilds
- source repository access
- draft access
- path-based file access

## Reporting security issues

Report any suspected exposure of private content, traversal bypass, symlink escape, or mutation capability as a security issue.

Provide:

- request details
- affected tool
- observed output
- expected boundary

Do not include secrets in the report body.

You can also use GitHub's private security advisory flow for sensitive reports when available.
