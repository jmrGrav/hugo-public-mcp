package publicmcp

import (
	"context"
	"fmt"

	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type getFullPageMarkdownInput struct {
	Slug string `json:"slug"`
}

type getFullPageMarkdownOutput struct {
	Page site.PageMarkdown `json:"page"`
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
}
