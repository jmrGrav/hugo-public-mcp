# Release Readiness

This repository is intentionally stopping at release-candidate quality.

## What is done

- read-only MCP server MVP
- startup indexing from the published `public/` tree
- security tests for traversal, symlinks, malformed content, and concurrent reads
- CI and lint configuration
- deployment examples for systemd and common reverse proxies

## Remaining before a first public release

- decide whether to publish a GitHub release or keep the project source-only
- decide the initial version tag
- run a real-site validation against a non-production Hugo export
- confirm the final reverse-proxy and origin-lockdown deployment pattern
- review whether any future shared packages should be extracted with `hugo-mcp-go`

## Intentional non-goals

- no write-capable MCP tools
- no shell access
- no Hugo rebuild path
- no public filesystem path API
- no source-repo access
- no dynamic conversion endpoint

