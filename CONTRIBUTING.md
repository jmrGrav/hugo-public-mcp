# Contributing

## Principles

- keep the server read-only
- keep the surface area small
- prefer explicit interfaces
- reject hidden filesystem access
- reject accidental mutation paths

## Workflow

1. Update or add a design document.
2. Add or update tests.
3. Keep changes narrowly scoped.
4. Verify security boundaries.
5. Document any tradeoff that increases surface area.
6. Keep GitHub workflow and repository settings aligned with the current MVP scope.

## Review standard

Contributions that add new public capabilities must show:

- why the capability belongs in a read-only public MCP
- why it does not expose private content
- why it does not require shell or rebuild access

## Validation standard

Before requesting merge, run the local test suite and make sure the GitHub checks are green:

- CI
- CodeQL
- secret scans
- dependency updates stay current
