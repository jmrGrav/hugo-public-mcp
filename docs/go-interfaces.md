# Go Interfaces

## Design intent

Keep the public server small by depending on a few narrow interfaces.
The goal is to make the read-only layer easy to test and easy to share later if extraction becomes worthwhile.

## Proposed interfaces

```go
type Indexer interface {
    Build(ctx context.Context, root string) (*SiteIndex, error)
}

type Searcher interface {
    Search(query string, limit int) ([]PageSummary, error)
}

type Reader interface {
    Get(slug string) (PageContent, error)
}

type SiteInfoProvider interface {
    SiteInformation() SiteInformation
}

type ToolRegistrar interface {
    RegisterTools(server MCPServer, deps Dependencies)
}
```

## Core data types

```go
type SiteIndex struct {
    Pages       []PageSummary
    Tags        []TagSummary
    Categories  []CategorySummary
    SitemapURL  string
    FeedURL     string
    SiteURL     string
}

type PageSummary struct {
    Slug        string
    Title       string
    Summary     string
    CanonicalURL string
    Language    string
    Date        time.Time
    Tags        []string
    Categories  []string
    Type        string
}

type PageContent struct {
    Summary PageSummary
    Body    string
}
```

## Annotation requirements

Every exposed tool must set:

- `readOnlyHint = true`
- `destructiveHint = false`
- `idempotentHint = true`
- `openWorldHint = false`

## Dependency rule

The public package must depend on reusable utilities, not on mutation code.
If a helper cannot stay read-only, it does not belong in the public path.

