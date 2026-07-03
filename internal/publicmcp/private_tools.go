package publicmcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type getFullPageMarkdownInput struct {
	Slug string `json:"slug"`
}

type getFullPageMarkdownOutput struct {
	Page site.PageMarkdown `json:"page"`
}

type getPageFrontmatterInput struct {
	Slug string `json:"slug"`
}

type getPageFrontmatterOutput struct {
	Page site.PageFrontmatter `json:"page"`
}

type getRelatedContentInput struct {
	Slug  string `json:"slug"`
	Limit int    `json:"limit,omitempty"`
}

type getRelatedContentOutput struct {
	Related []site.RelatedPage `json:"related"`
}

type buildAgentContextInput struct {
	Slug string `json:"slug"`
}

type buildAgentContextOutput struct {
	Context site.AgentContext `json:"context"`
}

type exportAgentContextInput struct {
	Cursor   string `json:"cursor,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Category string `json:"category,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type exportAgentContextOutput struct {
	Export site.ExportResult `json:"export"`
}

// RegisterPrivate registers tools that require a valid bearer token.
// Call this only on the fullServer (OAuth-gated MCP server instance).
func RegisterPrivate(s *mcp.Server, deps Dependencies) {
	if s == nil {
		return
	}
	addReadOnlyTool(s, "get_full_page_markdown", "Get full page Markdown",
		"Return the full Markdown-formatted content of a published page. Requires authentication. Input: indexed slug only — no filesystem paths.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in getFullPageMarkdownInput) (*mcp.CallToolResult, getFullPageMarkdownOutput, error) {
			if deps.Index == nil {
				return nil, getFullPageMarkdownOutput{}, fmt.Errorf("index not initialized")
			}
			pg, err := deps.Index.GetPageMarkdown(in.Slug)
			if err != nil {
				return nil, getFullPageMarkdownOutput{}, err
			}
			return nil, getFullPageMarkdownOutput{Page: pg}, nil
		})

	addReadOnlyTool(s, "get_page_frontmatter", "Get page frontmatter",
		"Return structured metadata for a published page including estimated reading time. Requires authentication. Input: indexed slug only.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in getPageFrontmatterInput) (*mcp.CallToolResult, getPageFrontmatterOutput, error) {
			if deps.Index == nil {
				return nil, getPageFrontmatterOutput{}, fmt.Errorf("index not initialized")
			}
			fm, err := deps.Index.GetFrontmatter(in.Slug)
			if err != nil {
				return nil, getPageFrontmatterOutput{}, err
			}
			return nil, getPageFrontmatterOutput{Page: fm}, nil
		})

	addReadOnlyTool(s, "get_related_content", "Get related content",
		"Return pages related to a given slug by shared tags or categories. Requires authentication. Input: indexed slug only.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in getRelatedContentInput) (*mcp.CallToolResult, getRelatedContentOutput, error) {
			if deps.Index == nil {
				return nil, getRelatedContentOutput{}, fmt.Errorf("index not initialized")
			}
			related, err := deps.Index.RelatedPages(in.Slug, in.Limit)
			if err != nil {
				return nil, getRelatedContentOutput{}, err
			}
			return nil, getRelatedContentOutput{Related: related}, nil
		})

	addReadOnlyTool(s, "build_agent_context", "Build agent context",
		"Return a complete enriched context bundle for a published page: metadata, reading time, full Markdown content, and related pages. Requires authentication. Input: indexed slug only.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in buildAgentContextInput) (*mcp.CallToolResult, buildAgentContextOutput, error) {
			if deps.Index == nil {
				return nil, buildAgentContextOutput{}, fmt.Errorf("index not initialized")
			}
			ac, err := deps.Index.GetAgentContext(in.Slug)
			if err != nil {
				return nil, buildAgentContextOutput{}, err
			}
			return nil, buildAgentContextOutput{Context: ac}, nil
		})

	addReadOnlyTool(s, "export_agent_context", "Export agent context",
		"Paginated export of page context bundles. Each page includes summary, reading time, and full Markdown content. Requires authentication. Use cursor from previous response to fetch the next page.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in exportAgentContextInput) (*mcp.CallToolResult, exportAgentContextOutput, error) {
			if deps.Index == nil {
				return nil, exportAgentContextOutput{}, fmt.Errorf("index not initialized")
			}
			result, err := deps.Index.ExportPages(in.Cursor, in.Tag, in.Category, in.Limit)
			if err != nil {
				return nil, exportAgentContextOutput{}, err
			}
			slog.Info("export_agent_context",
				"cursor", in.Cursor,
				"tag", in.Tag,
				"category", in.Category,
				"pages_returned", len(result.Pages),
				"next_cursor", result.NextCursor,
				"total", result.Total,
			)
			return nil, exportAgentContextOutput{Export: result}, nil
		})
}
