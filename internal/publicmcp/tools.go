package publicmcp

import (
	"context"
	"fmt"

	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Dependencies struct {
	Index *site.Index
}

type listPagesInput struct {
	Limit int `json:"limit,omitempty"`
}

type getPageInput struct {
	Slug string `json:"slug"`
}

type searchPagesInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type getRecentPostsInput struct {
	Limit int `json:"limit,omitempty"`
}

type getSiteInformationInput struct{}

type resourceOutput struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	MediaType string `json:"media_type,omitempty"`
}

type listPagesOutput struct {
	Pages []site.PageSummary `json:"pages"`
}

type getPageOutput struct {
	Page site.PageContent `json:"page"`
}

type searchPagesOutput struct {
	Pages []site.PageSummary `json:"pages"`
}

type getRecentPostsOutput struct {
	Pages []site.PageSummary `json:"pages"`
}

type listTagsOutput struct {
	Tags []site.TagSummary `json:"tags"`
}

type listCategoriesOutput struct {
	Categories []site.CategorySummary `json:"categories"`
}

type getSiteInformationOutput struct {
	Site site.SiteInformation `json:"site"`
}

func Register(s *mcp.Server, deps Dependencies) {
	if s == nil {
		return
	}
	addReadOnlyTool(s, "list_pages", "List pages", "List published Hugo pages from the startup index.", func(ctx context.Context, _ *mcp.CallToolRequest, in listPagesInput) (*mcp.CallToolResult, listPagesOutput, error) {
		if deps.Index == nil {
			return nil, listPagesOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, listPagesOutput{Pages: deps.Index.ListPages(in.Limit)}, nil
	})
	addReadOnlyTool(s, "get_page", "Get page", "Get a published page by slug.", func(ctx context.Context, _ *mcp.CallToolRequest, in getPageInput) (*mcp.CallToolResult, getPageOutput, error) {
		if deps.Index == nil {
			return nil, getPageOutput{}, fmt.Errorf("index not initialized")
		}
		page, err := deps.Index.GetPage(in.Slug)
		if err != nil {
			return nil, getPageOutput{}, err
		}
		return nil, getPageOutput{Page: page}, nil
	})
	addReadOnlyTool(s, "search_pages", "Search pages", "Search the startup index by title, summary, tags, categories, and canonical URL.", func(ctx context.Context, _ *mcp.CallToolRequest, in searchPagesInput) (*mcp.CallToolResult, searchPagesOutput, error) {
		if deps.Index == nil {
			return nil, searchPagesOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, searchPagesOutput{Pages: deps.Index.Search(in.Query, in.Limit)}, nil
	})
	addReadOnlyTool(s, "get_recent_posts", "Get recent posts", "Return recent published posts from the index.", func(ctx context.Context, _ *mcp.CallToolRequest, in getRecentPostsInput) (*mcp.CallToolResult, getRecentPostsOutput, error) {
		if deps.Index == nil {
			return nil, getRecentPostsOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, getRecentPostsOutput{Pages: deps.Index.RecentPosts(in.Limit)}, nil
	})
	addReadOnlyTool(s, "list_tags", "List tags", "List tags discovered from the index.", func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, listTagsOutput, error) {
		if deps.Index == nil {
			return nil, listTagsOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, listTagsOutput{Tags: deps.Index.ListTags()}, nil
	})
	addReadOnlyTool(s, "list_categories", "List categories", "List categories discovered from the index.", func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, listCategoriesOutput, error) {
		if deps.Index == nil {
			return nil, listCategoriesOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, listCategoriesOutput{Categories: deps.Index.ListCategories()}, nil
	})
	addReadOnlyTool(s, "get_sitemap", "Get sitemap", "Return the published sitemap.xml resource.", func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, resourceOutput, error) {
		if deps.Index == nil {
			return nil, resourceOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, resourceFromIndex(deps.Index, "/sitemap.xml"), nil
	})
	addReadOnlyTool(s, "get_feed", "Get feed", "Return the published feed.json resource.", func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, resourceOutput, error) {
		if deps.Index == nil {
			return nil, resourceOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, resourceFromIndex(deps.Index, "/feed.json"), nil
	})
	addReadOnlyTool(s, "get_site_information", "Get site information", "Return basic information about the indexed site.", func(context.Context, *mcp.CallToolRequest, getSiteInformationInput) (*mcp.CallToolResult, getSiteInformationOutput, error) {
		if deps.Index == nil {
			return nil, getSiteInformationOutput{}, fmt.Errorf("index not initialized")
		}
		return nil, getSiteInformationOutput{Site: deps.Index.SiteInformation()}, nil
	})
}

func addReadOnlyTool[In, Out any](s *mcp.Server, name, title, description string, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
	}, handler)
}

func boolPtr(v bool) *bool { return &v }

func resourceFromIndex(idx *site.Index, path string) resourceOutput {
	content, _ := idx.GetResource(path)
	return resourceOutput{Path: path, Content: content, MediaType: mediaTypeForPath(path)}
}

func mediaTypeForPath(path string) string {
	switch path {
	case "/sitemap.xml", "/index.xml":
		return "application/xml"
	case "/feed.json":
		return "application/feed+json"
	case "/robots.txt", "/llms.txt":
		return "text/plain; charset=utf-8"
	default:
		return ""
	}
}
