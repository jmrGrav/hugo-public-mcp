package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jmrGrav/hugo-public-mcp/internal/config"
	"github.com/jmrGrav/hugo-public-mcp/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLiveSiteValidation(t *testing.T) {
	if os.Getenv("HUGO_PUBLIC_MCP_LIVE_VALIDATION") == "" {
		t.Skip("set HUGO_PUBLIC_MCP_LIVE_VALIDATION=1 to run live validation")
	}

	root := "/home/jm/hugo-site/public"
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("site_root unavailable: %v", err)
	}

	before := memAlloc()
	start := time.Now()
	svc, err := server.New(config.Config{
		SiteRoot:         root,
		SiteURL:          "https://www.arleo.eu",
		SiteName:         "arleo.eu",
		DefaultLanguage:  "fr",
		Transport:        "http",
		HTTPBindAddr:     "127.0.0.1",
		HTTPBindPort:     8088,
		StreamingEnabled: true,
		MaxIndexEntries:  50000,
		MaxResultItems:   50,
		MaxRequestBytes:  1 << 20,
		RejectSymlinks:   true,
		RejectHiddenPath: true,
	})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	startupElapsed := time.Since(start)
	after := memAlloc()
	t.Logf("live site startup: elapsed=%s alloc_delta=%d", startupElapsed, deltaAlloc(before, after))

	if got := svc.Index().SiteInformation().SiteURL; got != "https://www.arleo.eu" {
		t.Fatalf("site url = %q", got)
	}
	if got := svc.Index().SiteInformation().PageCount; got == 0 {
		t.Fatal("page count is zero")
	}

	ts := httptest.NewServer(svc.HTTPHandler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "hugo-public-mcp-live-test", Version: "0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("client connect error = %v", err)
	}
	defer session.Close()

	validateTools(t, ctx, session)
}

func validateTools(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Helper()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	expected := []string{
		"list_pages",
		"get_page",
		"search_pages",
		"get_recent_posts",
		"list_tags",
		"list_categories",
		"get_sitemap",
		"get_feed",
		"get_site_information",
	}
	got := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = tool
	}
	for _, name := range expected {
		tool, ok := got[name]
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint || !tool.Annotations.IdempotentHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %q annotations = %#v", name, tool.Annotations)
		}
	}

	listPagesRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_pages", Arguments: map[string]any{"limit": 10}})
	if err != nil {
		t.Fatalf("list_pages error = %v", err)
	}
	if listPagesRes.IsError {
		t.Fatalf("list_pages returned error result: %#v", listPagesRes)
	}
	listPagesPayload, err := json.Marshal(listPagesRes.StructuredContent)
	if err != nil {
		t.Fatalf("list_pages marshal error: %v", err)
	}
	assertNoLeaks(t, "list_pages", string(listPagesPayload))

	sampleSlug, sampleTitle := chooseSamplePage(t, listPagesPayload)

	type toolCall struct {
		name string
		args map[string]any
	}
	calls := []toolCall{
		{name: "get_page", args: map[string]any{"slug": "/"}},
		{name: "get_page", args: map[string]any{"slug": sampleSlug}},
		{name: "search_pages", args: map[string]any{"query": sampleTitle, "limit": 10}},
		{name: "get_recent_posts", args: map[string]any{"limit": 10}},
		{name: "list_tags", args: map[string]any{}},
		{name: "list_categories", args: map[string]any{}},
		{name: "get_sitemap", args: map[string]any{}},
		{name: "get_feed", args: map[string]any{}},
		{name: "get_site_information", args: map[string]any{}},
	}

	for _, tc := range calls {
		callStart := time.Now()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("%s error = %v", tc.name, err)
		}
		if res.IsError {
			t.Fatalf("%s returned error result: %#v", tc.name, res)
		}
		payload, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("%s marshal error: %v", tc.name, err)
		}
		switch tc.name {
		case "get_page":
			assertPageNoInfraLeak(t, tc.name, payload)
		case "get_feed":
			assertFeedNoInfraLeak(t, tc.name, payload)
		default:
			assertNoLeaks(t, tc.name, string(payload))
		}
		t.Logf("%s latency=%s size=%d", tc.name, time.Since(callStart), len(payload))
	}
}

func chooseSamplePage(t *testing.T, payload []byte) (slug string, title string) {
	t.Helper()
	var decoded struct {
		Pages []struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal list_pages payload: %v", err)
	}
	for _, page := range decoded.Pages {
		if page.Slug == "" || page.Slug == "/" {
			continue
		}
		slug = page.Slug
		title = strings.TrimSpace(page.Title)
		break
	}
	if slug == "" {
		t.Fatalf("no non-home page found in list_pages payload: %s", string(payload))
	}
	if title == "" {
		title = strings.TrimPrefix(slug, "/")
	}
	if title == "" {
		title = fmt.Sprintf("page %s", slug)
	}
	return slug, title
}

func assertNoLeaks(t *testing.T, tool string, payload string) {
	t.Helper()
	for _, forbidden := range []string{
		"/home/jm/",
		"hugo-site/public",
		"hugo-site/static",
		".git/",
		"config.yaml",
		"config.toml",
		"config.yml",
		"layouts/",
		"content/",
		"archetypes/",
		"resources/",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("%s leaked forbidden fragment %q in payload: %s", tool, forbidden, payload)
		}
	}
}

func assertPageNoInfraLeak(t *testing.T, tool string, payload []byte) {
	t.Helper()
	var decoded struct {
		Page struct {
			Summary map[string]any `json:"summary"`
		} `json:"page"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("%s unmarshal error: %v", tool, err)
	}
	summary := decoded.Page.Summary
	if summary == nil {
		t.Fatalf("%s missing page summary", tool)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("%s summary marshal error: %v", tool, err)
	}
	assertNoLeaks(t, tool, string(summaryJSON))
}

func assertFeedNoInfraLeak(t *testing.T, tool string, payload []byte) {
	t.Helper()
	var decoded struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("%s unmarshal error: %v", tool, err)
	}
	var feed struct {
		Title       string `json:"title"`
		HomePageURL string `json:"home_page_url"`
		FeedURL     string `json:"feed_url"`
		Description string `json:"description"`
		Items       []struct {
			ID            string `json:"id"`
			URL           string `json:"url"`
			Title         string `json:"title"`
			Summary       string `json:"summary"`
			DatePublished string `json:"date_published"`
			DateModified  string `json:"date_modified"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(decoded.Content), &feed); err != nil {
		t.Fatalf("%s inner feed unmarshal error: %v", tool, err)
	}
	metaJSON, err := json.Marshal(feed)
	if err != nil {
		t.Fatalf("%s feed marshal error: %v", tool, err)
	}
	assertNoLeaks(t, tool, string(metaJSON))
}

func memAlloc() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Alloc
}

func deltaAlloc(before, after uint64) int64 {
	if after < before {
		return -int64(before - after)
	}
	return int64(after - before)
}
