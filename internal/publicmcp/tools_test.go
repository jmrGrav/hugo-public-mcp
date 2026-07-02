package publicmcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterToolsExposesReadOnlyAnnotations(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	Register(server, Dependencies{Index: mustIndex(t)})

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect error = %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Fatalf("session close error = %v", err)
		}
	}()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	got := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = tool
	}
	for _, name := range []string{"list_pages", "get_page", "search_pages", "get_recent_posts", "list_tags", "list_categories", "get_sitemap", "get_feed", "get_site_information"} {
		tool, ok := got[name]
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint || !tool.Annotations.IdempotentHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %q annotations = %#v", name, tool.Annotations)
		}
	}
}

func TestRegisterToolsRoundTrip(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	Register(server, Dependencies{Index: mustIndex(t)})

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect error = %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Fatalf("session close error = %v", err)
		}
	}()

	pageRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_page", Arguments: map[string]any{"slug": "/posts/hello"}})
	if err != nil {
		t.Fatalf("get_page error = %v", err)
	}
	if pageRes.IsError {
		t.Fatalf("get_page returned error result: %#v", pageRes)
	}
	raw, err := json.Marshal(pageRes.StructuredContent)
	if err != nil {
		t.Fatalf("marshal get_page structured content: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal get_page structured content: %v", err)
	}
	pageValue, ok := got["page"]
	if !ok {
		t.Fatal("get_page payload missing page")
	}
	page, ok := pageValue.(map[string]any)
	if !ok {
		t.Fatalf("get_page page type = %T want map[string]any", pageValue)
	}
	summaryValue, ok := page["summary"]
	if !ok {
		t.Fatal("get_page payload missing summary")
	}
	summary, ok := summaryValue.(map[string]any)
	if !ok {
		t.Fatalf("get_page summary type = %T want map[string]any", summaryValue)
	}
	if summary["slug"] != "/posts/hello" {
		t.Fatalf("get_page slug = %v", summary["slug"])
	}
	if summary["language"] != "en" {
		t.Fatalf("get_page language = %v", summary["language"])
	}

	searchRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_pages", Arguments: map[string]any{"query": "security", "limit": 5}})
	if err != nil {
		t.Fatalf("search_pages error = %v", err)
	}
	if searchRes.IsError {
		t.Fatalf("search_pages returned error result: %#v", searchRes)
	}

	infoRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_site_information", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("get_site_information error = %v", err)
	}
	if infoRes.IsError {
		t.Fatalf("get_site_information returned error result: %#v", infoRes)
	}
	raw, err = json.Marshal(infoRes.StructuredContent)
	if err != nil {
		t.Fatalf("marshal get_site_information structured content: %v", err)
	}
	if !strings.Contains(string(raw), "page_count") {
		t.Fatalf("get_site_information payload missing page_count: %s", string(raw))
	}

	sitemapRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_sitemap", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("get_sitemap error = %v", err)
	}
	if sitemapRes.IsError {
		t.Fatalf("get_sitemap returned error result: %#v", sitemapRes)
	}
	raw, err = json.Marshal(sitemapRes.StructuredContent)
	if err != nil {
		t.Fatalf("marshal get_sitemap structured content: %v", err)
	}
	if !strings.Contains(string(raw), "application/xml") {
		t.Fatalf("get_sitemap payload missing media type: %s", string(raw))
	}

	feedRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_feed", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("get_feed error = %v", err)
	}
	if feedRes.IsError {
		t.Fatalf("get_feed returned error result: %#v", feedRes)
	}
	raw, err = json.Marshal(feedRes.StructuredContent)
	if err != nil {
		t.Fatalf("marshal get_feed structured content: %v", err)
	}
	if !strings.Contains(string(raw), "application/feed+json") {
		t.Fatalf("get_feed payload missing media type: %s", string(raw))
	}
}

func mustIndex(t *testing.T) *site.Index {
	t.Helper()
	idx, err := site.BuildIndex(context.Background(), filepath.Join("..", "..", "testdata", "fixtures", "public", "minimal"), site.Config{
		SiteURL:          "https://example.test",
		SiteName:         "example.test",
		DefaultLanguage:  "fr",
		MaxIndexEntries:  1000,
		RejectSymlinks:   true,
		RejectHiddenPath: true,
	})
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	return idx
}
