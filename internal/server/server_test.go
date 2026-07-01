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
