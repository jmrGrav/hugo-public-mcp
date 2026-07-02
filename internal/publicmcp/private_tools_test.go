package publicmcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmrGrav/hugo-public-mcp/internal/publicmcp"
	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mustMinimalIndex(t *testing.T) *site.Index {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "fixtures", "public", "minimal")
	idx, err := site.BuildIndex(context.Background(), root, site.Config{
		SiteURL:  "https://example.test",
		SiteName: "example.test",
	})
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return idx
}

func connectPrivate(t *testing.T, idx *site.Index) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	publicmcp.RegisterPrivate(srv, publicmcp.Dependencies{Index: idx})
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestRegisterPrivate_ToolVisible(t *testing.T) {
	session := connectPrivate(t, mustMinimalIndex(t))

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "get_full_page_markdown" {
			found = true
			break
		}
	}
	if !found {
		t.Error("get_full_page_markdown not found in tools list")
	}
}

func TestRegisterPrivate_CallKnownSlug(t *testing.T) {
	session := connectPrivate(t, mustMinimalIndex(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_full_page_markdown",
		Arguments: map[string]any{"slug": "/posts/hello"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
	// Result must contain structured content with page markdown.
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		t.Fatal("empty tool result")
	}
}

func TestRegisterPrivate_CallUnknownSlug(t *testing.T) {
	session := connectPrivate(t, mustMinimalIndex(t))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_full_page_markdown",
		Arguments: map[string]any{"slug": "/posts/does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for unknown slug")
	}
}

func TestRegisterPrivate_NilServer(t *testing.T) {
	// must not panic
	publicmcp.RegisterPrivate(nil, publicmcp.Dependencies{Index: mustMinimalIndex(t)})
}
