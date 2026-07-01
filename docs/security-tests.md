# Security Tests

## Required test cases

### Index boundary tests

- indexer refuses roots outside the configured `public/`
- indexer rejects symlinked entries that escape the root
- indexer skips files not meant to be public

### Path handling tests

- `../` is rejected
- absolute paths are rejected
- Windows-style backslashes are rejected
- empty or whitespace-only routes are normalized safely

### Tool contract tests

- unknown tools are rejected
- read-only tools carry read-only annotations
- the tool metadata does not advertise mutation capabilities

### Data exposure tests

- no source path appears in any returned object
- no draft content appears in search results
- no hidden directories are returned
- no content outside the public tree is reachable

### Runtime tests

- no shell command is invoked
- no Hugo rebuild is invoked
- no writes occur during read requests
- request-time access does not depend on scanning the filesystem repeatedly

### Reverse proxy tests

- HTTPS works through the proxy
- request-size limits are enforced
- logs are present and do not leak secrets
- rate limiting can be applied without changing the application

## Negative tests

These must fail:

- request a file path directly
- request a draft slug
- request a path outside the root
- request an unknown tool
- request a mutation

