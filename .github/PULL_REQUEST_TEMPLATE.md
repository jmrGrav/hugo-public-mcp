# Summary

Describe the change and why it is safe for a read-only public MCP surface.

## Validation

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `golangci-lint run ./...`
- [ ] Live validation completed

## Security

- [ ] No new write capability
- [ ] No source-repo access added
- [ ] No local path leak added
- [ ] No secret or token exposure
