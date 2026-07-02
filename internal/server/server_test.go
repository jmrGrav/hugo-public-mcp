package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrGrav/hugo-public-mcp/internal/config"
)

func TestHTTPHandlerRejectsWrongMethod(t *testing.T) {
	svc := mustTestService(t, true)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()

	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q want POST", got)
	}
}

func TestHTTPHandlerRejectsWrongContentType(t *testing.T) {
	svc := mustTestService(t, true)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
}

func TestHTTPHandlerRejectsOversizeBody(t *testing.T) {
	svc := mustTestService(t, true)
	svc.cfg.MaxRequestBytes = 8
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHTTPHandlerOAuthDisabledBehaviorUnchanged(t *testing.T) {
	svc := mustTestService(t, true)
	if svc.cfg.OAuth.Enabled {
		t.Fatal("test service must default oauth.enabled=false")
	}

	for _, path := range []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource",
		"/authorize",
		"/token",
		"/register",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			svc.HTTPHandler().ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHTTPHandlerOAuthDisabledKeepsAnonymousReadOnlyMCP(t *testing.T) {
	svc := mustTestService(t, true)

	for _, auth := range []string{"", "Bearer invalid"} {
		t.Run("authorization="+auth, func(t *testing.T) {
			body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			rec := httptest.NewRecorder()

			svc.HTTPHandler().ServeHTTP(rec, req)

			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Fatalf("oauth.disabled must not enforce bearer auth, got %d", rec.Code)
			}
			if rec.Code < 200 || rec.Code >= 500 {
				t.Fatalf("unexpected status = %d body = %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHTTPHandlerDisablesStreamingEndpointWhenConfigured(t *testing.T) {
	svc := mustTestService(t, false)
	req := httptest.NewRequest(http.MethodGet, "/mcp/events", nil)
	rec := httptest.NewRecorder()

	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHTTPHandlerServesDiscoveryCard(t *testing.T) {
	svc := mustTestService(t, true)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/mcp.json", nil)
	req.Host = "mcp.arleo.eu"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q", ct)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("cache-control = %q", got)
	}
	var card struct {
		Name      string   `json:"name"`
		Version   string   `json:"version"`
		Endpoint  string   `json:"endpoint"`
		Discovery string   `json:"discovery"`
		Transport string   `json:"transport"`
		Auth      string   `json:"auth"`
		ReadOnly  bool     `json:"read_only"`
		Tools     []string `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatalf("unmarshal discovery card: %v", err)
	}
	if card.Name != Name {
		t.Fatalf("name = %q want %q", card.Name, Name)
	}
	if card.Version != Version {
		t.Fatalf("version = %q want %q", card.Version, Version)
	}
	if card.Endpoint != "https://mcp.arleo.eu/mcp" {
		t.Fatalf("endpoint = %q", card.Endpoint)
	}
	if card.Discovery != "https://mcp.arleo.eu/.well-known/mcp.json" {
		t.Fatalf("discovery = %q", card.Discovery)
	}
	if card.Transport != "streamable-http" || card.Auth != "none" || !card.ReadOnly {
		t.Fatalf("unexpected discovery card = %#v", card)
	}
	if len(card.Tools) == 0 {
		t.Fatal("expected tools in discovery card")
	}
}

func TestHTTPHandlerServesHealthAndPublishedResources(t *testing.T) {
	svc := mustTestService(t, true)
	cases := []struct {
		name          string
		path          string
		wantStatus    int
		wantCTPrefix  string
		wantSubstring string
	}{
		{name: "health", path: "/health", wantStatus: http.StatusOK, wantCTPrefix: "application/json", wantSubstring: "\"status\":\"ok\""},
		{name: "openapi", path: "/openapi.json", wantStatus: http.StatusOK, wantCTPrefix: "application/openapi+json", wantSubstring: "\"openapi\": \"3.0.3\""},
		{name: "auth", path: "/auth.md", wantStatus: http.StatusOK, wantCTPrefix: "text/markdown", wantSubstring: "no registration required"},
		{name: "api-catalog", path: "/.well-known/api-catalog", wantStatus: http.StatusOK, wantCTPrefix: "application/linkset+json", wantSubstring: "service-desc"},
		{name: "agent-skills", path: "/.well-known/agent-skills/index.json", wantStatus: http.StatusOK, wantCTPrefix: "application/json", wantSubstring: "discover_hugo_site"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Host = "mcp.arleo.eu"
			req.Header.Set("X-Forwarded-Proto", "https")
			rec := httptest.NewRecorder()

			svc.HTTPHandler().ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, tc.wantCTPrefix) {
				t.Fatalf("content-type = %q want prefix %q", ct, tc.wantCTPrefix)
			}
			if !strings.Contains(rec.Body.String(), tc.wantSubstring) {
				t.Fatalf("body = %q want substring %q", rec.Body.String(), tc.wantSubstring)
			}
		})
	}
}

func TestOpenAPISchemaAllowsJSONRPCNumericIDs(t *testing.T) {
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Type     string `json:"type"`
					Nullable bool   `json:"nullable"`
					OneOf    []struct {
						Type string `json:"type"`
					} `json:"oneOf"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(openAPISchema), &doc); err != nil {
		t.Fatalf("unmarshal OpenAPI schema: %v", err)
	}
	for _, schemaName := range []string{"JsonRpcRequest", "JsonRpcResponse"} {
		idSchema, ok := doc.Components.Schemas[schemaName].Properties["id"]
		if !ok {
			t.Fatalf("%s.id schema missing", schemaName)
		}
		if idSchema.Type != "" {
			t.Fatalf("%s.id type = %q, want oneOf without top-level type", schemaName, idSchema.Type)
		}
		if !idSchema.Nullable {
			t.Fatalf("%s.id nullable = false, want true", schemaName)
		}
		types := map[string]bool{}
		for _, option := range idSchema.OneOf {
			types[option.Type] = true
		}
		for _, want := range []string{"string", "number"} {
			if !types[want] {
				t.Fatalf("%s.id oneOf missing %q: %#v", schemaName, want, types)
			}
		}
		if types["integer"] {
			t.Fatalf("%s.id oneOf must not include integer with number because integer ids match both branches", schemaName)
		}
	}
}

func mustTestService(t *testing.T, streaming bool) *Service {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "fixtures", "public", "minimal")
	cfg := config.Config{
		SiteRoot:         root,
		SiteURL:          "https://example.test",
		SiteName:         "example.test",
		DefaultLanguage:  "fr",
		Transport:        "http",
		HTTPBindAddr:     "127.0.0.1",
		HTTPBindPort:     8088,
		StreamingEnabled: streaming,
		MaxIndexEntries:  1000,
		MaxResultItems:   10,
		MaxRequestBytes:  1024,
		RejectSymlinks:   true,
		RejectHiddenPath: true,
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return svc
}

func TestNewBuildsIndexFromPublishedRoot(t *testing.T) {
	svc := mustTestService(t, true)
	if got := svc.Index().SiteInformation().PageCount; got != 3 {
		t.Fatalf("page count = %d want 3", got)
	}
	if svc.MCP() == nil {
		t.Fatal("MCP() returned nil")
	}
}

func TestRunStdioRejectsNilService(t *testing.T) {
	var svc *Service
	if err := svc.RunStdio(context.Background()); err == nil {
		t.Fatal("RunStdio() expected error for nil service")
	}
}
