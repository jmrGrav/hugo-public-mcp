# Threat Model

## Assets

- published site content
- site metadata
- canonical URLs
- index structure
- logs
- service configuration

## Trust boundaries

- Internet client to reverse proxy
- reverse proxy to MCP service
- MCP service to published `public/`
- startup indexing phase to request phase

## Primary threats

### Path traversal

An attacker may try to request `../` or an absolute path to escape the published site tree.

Mitigation:

- never accept file paths from clients
- resolve only slugs already in the index
- canonicalize and reject traversal

### Symlink escape

Published content could contain a symlink that points outside the allowed root.

Mitigation:

- reject symlinks during indexing
- do not follow links when resolving content

### Draft leakage

Drafts or unpublished content may exist in the source tree.

Mitigation:

- index only the published `public/` directory
- do not read Hugo source files

### Request-time filesystem abuse

Repeated requests could force unnecessary disk reads or large file scans.

Mitigation:

- build the index at startup
- serve search from memory
- avoid request-time directory traversal

### Code execution exposure

An attacker may try to turn the public MCP into a general-purpose automation endpoint.

Mitigation:

- no shell
- no rebuild
- no external command execution
- no write tools

### Information leakage

The service may accidentally expose path names, repo layout, internal hosts, or private metadata.

Mitigation:

- only expose canonical public URLs and public metadata
- never expose source paths
- keep logs minimal and redacted

## Non-goals

- public write APIs
- agent authentication
- remote execution
- dynamic content transformation
- source repository browsing

