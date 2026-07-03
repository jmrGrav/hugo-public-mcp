package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrGrav/hugo-public-mcp/internal/config"
	"github.com/jmrGrav/hugo-public-mcp/internal/oauth"
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

func TestHTTPHandlerOAuthEnabledDiscoveryAndTokenFlow(t *testing.T) {
	svc := mustTestService(t, true)
	svc.cfg.OAuth = config.OAuthConfig{
		Enabled:               true,
		DynamicClientEnabled:  true,
		RequirePKCE:           true,
		TrustedAuthorizeCIDRs: []string{"192.0.2.10/32"},
		AuthCodeTTLSeconds:    300,
		AccessTokenTTLSeconds: 3600,
	}
	svc.oauth = oauth.NewService(svc.oauthConfigForRequest(httptest.NewRequest(http.MethodGet, "https://mcp.example.test/", nil)))

	t.Run("serves real OAuth metadata when enabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
		req.Host = "mcp.example.test"
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()

		svc.HTTPHandler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d want 200", rec.Code)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("metadata JSON: %v", err)
		}
		if body["issuer"] != "https://mcp.example.test" || body["token_endpoint"] != "https://mcp.example.test/token" {
			t.Fatalf("unexpected metadata: %#v", body)
		}
	})

	registerBody := []byte(`{"redirect_uris":["https://client.example.test/callback"]}`)
	registerReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(registerBody))
	registerReq.Host = "mcp.example.test"
	registerReq.Header.Set("X-Forwarded-Proto", "https")
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("register status = %d body = %q", registerRec.Code, registerRec.Body.String())
	}
	var registration struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(registerRec.Body.Bytes(), &registration); err != nil {
		t.Fatalf("register JSON: %v", err)
	}

	verifier := "test-verifier-test-verifier-test-verifier"
	challenge := oauth.CodeChallengeS256(verifier)
	authURL := "/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {registration.ClientID},
		"redirect_uri":          {"https://client.example.test/callback"},
		"state":                 {"state-1"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()
	authReq := httptest.NewRequest(http.MethodGet, authURL, nil)
	authReq.RemoteAddr = "192.0.2.10:12345"
	authReq.Host = "mcp.example.test"
	authReq.Header.Set("X-Forwarded-Proto", "https")
	authRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusFound {
		t.Fatalf("authorize status = %d body = %q", authRec.Code, authRec.Body.String())
	}
	location, err := url.Parse(authRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize redirect: %v", err)
	}
	code := location.Query().Get("code")
	if code == "" || location.Query().Get("state") != "state-1" {
		t.Fatalf("unexpected authorize redirect: %s", authRec.Header().Get("Location"))
	}

	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {registration.ClientID},
		"code":          {code},
		"redirect_uri":  {"https://client.example.test/callback"},
		"code_verifier": {verifier},
	}
	tokenReq := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(tokenForm.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(tokenRec, tokenReq)
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token status = %d body = %q", tokenRec.Code, tokenRec.Body.String())
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &token); err != nil {
		t.Fatalf("token JSON: %v", err)
	}
	if token.AccessToken == "" || token.TokenType != "Bearer" {
		t.Fatalf("unexpected token: %#v", token)
	}
}

func TestHTTPHandlerOAuthEnabledKeepsAnonymousReadOnlyAndRejectsInvalidBearer(t *testing.T) {
	svc := mustTestService(t, true)
	svc.cfg.OAuth.Enabled = true
	svc.oauth = oauth.NewService(svc.oauthConfigForRequest(httptest.NewRequest(http.MethodGet, "https://mcp.example.test/", nil)))

	t.Run("anonymous MCP remains allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		svc.HTTPHandler().ServeHTTP(rec, req)

		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Fatalf("anonymous read-only MCP must remain allowed, got %d", rec.Code)
		}
	})

	t.Run("invalid bearer is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer invalid")
		rec := httptest.NewRecorder()

		svc.HTTPHandler().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d want 401", rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
			t.Fatalf("missing bearer challenge: %q", got)
		}
	})

	t.Run("non public tool calls are forbidden before MCP dispatch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish_post","arguments":{}}}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		svc.HTTPHandler().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d want 403 body = %q", rec.Code, rec.Body.String())
		}
	})
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

// helpers needed by new tests
func mustTestServiceOAuth(t *testing.T) *Service {
	t.Helper()
	svc := mustTestService(t, true)
	svc.cfg.OAuth = config.OAuthConfig{
		Enabled:               true,
		DynamicClientEnabled:  true,
		RequirePKCE:           true,
		TrustedAuthorizeCIDRs: []string{"192.0.2.10/32"},
		AuthCodeTTLSeconds:    300,
		AccessTokenTTLSeconds: 3600,
	}
	req := httptest.NewRequest(http.MethodGet, "https://mcp.example.test/", nil)
	svc.oauth = oauth.NewService(svc.oauthConfigForRequest(req))
	// rebuild with fullServer
	if err := svc.initFullServer(); err != nil {
		t.Fatalf("initFullServer: %v", err)
	}
	return svc
}

func obtainValidToken(t *testing.T, svc *Service) string {
	t.Helper()
	// DCR
	regBody := []byte(`{"redirect_uris":["https://client.example.test/cb"]}`)
	regReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(regBody))
	regReq.Host = "mcp.example.test"
	regReq.Header.Set("X-Forwarded-Proto", "https")
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register status = %d", regRec.Code)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(regRec.Body.Bytes(), &reg); err != nil {
		t.Fatalf("register JSON: %v", err)
	}

	// authorize
	verifier := "test-verifier-test-verifier-test-verifier"
	challenge := oauth.CodeChallengeS256(verifier)
	authURL := "/authorize?" + url.Values{
		"response_type": {"code"}, "client_id": {reg.ClientID},
		"redirect_uri": {"https://client.example.test/cb"}, "state": {"s1"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}.Encode()
	authReq := httptest.NewRequest(http.MethodGet, authURL, nil)
	authReq.RemoteAddr = "192.0.2.10:1234"
	authReq.Host = "mcp.example.test"
	authReq.Header.Set("X-Forwarded-Proto", "https")
	authRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusFound {
		t.Fatalf("authorize status = %d", authRec.Code)
	}
	loc, _ := url.Parse(authRec.Header().Get("Location"))
	code := loc.Query().Get("code")

	// token
	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {reg.ClientID},
		"code": {code}, "redirect_uri": {"https://client.example.test/cb"},
		"code_verifier": {verifier},
	}
	tokReq := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(tokenForm.Encode()))
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(tokRec, tokReq)
	if tokRec.Code != http.StatusOK {
		t.Fatalf("token status = %d body = %q", tokRec.Code, tokRec.Body.String())
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokRec.Body.Bytes(), &tok); err != nil {
		t.Fatalf("token JSON: %v", err)
	}
	return tok.AccessToken
}

func TestPrivateToolHiddenWithoutBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "get_full_page_markdown") {
		t.Error("get_full_page_markdown must not appear in tools/list without bearer")
	}
}

func TestPrivateToolVisibleWithValidBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "get_full_page_markdown") {
		t.Error("get_full_page_markdown must appear in tools/list with valid bearer")
	}
}

func TestPrivateToolCallForbiddenWithoutBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_full_page_markdown","arguments":{"slug":"/posts/hello"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d want 403", rec.Code)
	}
}

func TestPrivateToolCallInvalidBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_full_page_markdown","arguments":{"slug":"/posts/hello"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer totally-invalid-token")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Errorf("body missing invalid_token: %q", rec.Body.String())
	}
}

func TestPrivateToolCallSucceedsWithValidBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_full_page_markdown","arguments":{"slug":"/posts/hello"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Host = "mcp.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Hello") {
		t.Errorf("response missing page content: %q", rec.Body.String())
	}
}

func TestPrivateToolCallUnknownSlugWithBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_full_page_markdown","arguments":{"slug":"/posts/no-such-page"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	// MCP returns 200 with isError=true in the body (tool-level error, not HTTP error)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 (tool errors are in MCP body)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") && !strings.Contains(rec.Body.String(), "not found") {
		t.Errorf("expected tool error in body: %q", rec.Body.String())
	}
}

func TestPrivateToolPathTraversalRejected(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_full_page_markdown","arguments":{"slug":"../../../etc/passwd"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 (slug validation error in MCP body)", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "passwd") {
		t.Error("response must not contain etc/passwd content")
	}
}

func TestNoPathLeakInErrorResponses(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)
	sensitivePatterns := []string{"/home/jm", "192.168.", ".git", "secret"}

	cases := []struct {
		name string
		body string
		auth string
	}{
		{"invalid_token", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "Bearer invalid"},
		{"forbidden_tool_no_bearer", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"publish_post","arguments":{}}}`, ""},
		{"unknown_slug_with_bearer", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_full_page_markdown","arguments":{"slug":"../../../etc/passwd"}}}`, "Bearer " + token},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			svc.HTTPHandler().ServeHTTP(rec, req)

			got := rec.Body.String()
			for _, pattern := range sensitivePatterns {
				if strings.Contains(got, pattern) {
					t.Errorf("response leaks sensitive pattern %q: %q", pattern, got[:min(len(got), 200)])
				}
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestAnonymousPublicToolsStillWorkWithOAuth(t *testing.T) {
	svc := mustTestServiceOAuth(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_site_information","arguments":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("anonymous public tool must work with OAuth enabled, got %d", rec.Code)
	}
}

func callPrivateTool(t *testing.T, svc *Service, token, toolName, argsJSON string) (int, string) {
	t.Helper()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + toolName + `","arguments":` + argsJSON + `}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestGetPageFrontmatterWithBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)
	code, body := callPrivateTool(t, svc, token, "get_page_frontmatter", `{"slug":"/posts/hello"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", code, body)
	}
	if !strings.Contains(body, "reading_time") {
		t.Errorf("response missing reading_time: %q", body[:min(len(body), 300)])
	}
	if !strings.Contains(body, "Hello") {
		t.Errorf("response missing title: %q", body[:min(len(body), 300)])
	}
}

func TestGetPageFrontmatterForbiddenWithoutBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_page_frontmatter","arguments":{"slug":"/posts/hello"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d want 403", rec.Code)
	}
}

func TestGetRelatedContentWithBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)
	code, body := callPrivateTool(t, svc, token, "get_related_content", `{"slug":"/posts/hello","limit":5}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", code, body)
	}
	// related may be empty in minimal fixture — just check the key exists
	if !strings.Contains(body, "related") {
		t.Errorf("response missing related key: %q", body[:min(len(body), 300)])
	}
}

func TestBuildAgentContextWithBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)
	code, body := callPrivateTool(t, svc, token, "build_agent_context", `{"slug":"/posts/hello"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", code, body)
	}
	if !strings.Contains(body, "markdown_content") {
		t.Errorf("response missing markdown_content: %q", body[:min(len(body), 300)])
	}
	if !strings.Contains(body, "reading_time") {
		t.Errorf("response missing reading_time: %q", body[:min(len(body), 300)])
	}
}

func TestBuildAgentContextForbiddenWithoutBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"build_agent_context","arguments":{"slug":"/posts/hello"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d want 403", rec.Code)
	}
}

func TestExportAgentContextWithBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)
	code, body := callPrivateTool(t, svc, token, "export_agent_context", `{"limit":2}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", code, body)
	}
	if !strings.Contains(body, "pages") || !strings.Contains(body, "total") {
		t.Errorf("response missing export keys: %q", body[:min(len(body), 300)])
	}
}

func TestExportAgentContextForbiddenWithoutBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"export_agent_context","arguments":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d want 403", rec.Code)
	}
}

func TestNewPrivateToolsHiddenWithoutBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	got := rec.Body.String()
	for _, privateTool := range []string{"get_page_frontmatter", "get_related_content", "build_agent_context", "export_agent_context"} {
		if strings.Contains(got, privateTool) {
			t.Errorf("%s must not appear in tools/list without bearer", privateTool)
		}
	}
}

func TestNewPrivateToolsVisibleWithBearer(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	token := obtainValidToken(t, svc)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	for _, privateTool := range []string{"get_page_frontmatter", "get_related_content", "build_agent_context", "export_agent_context"} {
		if !strings.Contains(got, privateTool) {
			t.Errorf("%s must appear in tools/list with valid bearer", privateTool)
		}
	}
}

func TestAgentIdentityAnonymous(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	body := `{"type":"anonymous"}`
	req := httptest.NewRequest(http.MethodPost, "/agent/identity", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 body = %q", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, field := range []string{"registration_id", "registration_type", "identity_assertion", "claim_token"} {
		if resp[field] == nil || resp[field] == "" {
			t.Errorf("missing field %q in agent identity response", field)
		}
	}
	if resp["registration_type"] != "anonymous" {
		t.Errorf("registration_type = %v want anonymous", resp["registration_type"])
	}
}

func TestAgentIdentityUnknownTypeRejects(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	body := `{"type":"service_auth","login_hint":"test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/agent/identity", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400", rec.Code)
	}
}

func TestAgentIdentityNotFoundWithoutOAuth(t *testing.T) {
	svc := mustTestService(t, false)
	req := httptest.NewRequest(http.MethodPost, "/agent/identity", strings.NewReader(`{"type":"anonymous"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d want 404", rec.Code)
	}
}

func TestAgentTokenExchangeViaAssertion(t *testing.T) {
	svc := mustTestServiceOAuth(t)

	// Register anonymously.
	regReq := httptest.NewRequest(http.MethodPost, "/agent/identity", strings.NewReader(`{"type":"anonymous"}`))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusOK {
		t.Fatalf("register status = %d", regRec.Code)
	}
	var regResp map[string]interface{}
	_ = json.Unmarshal(regRec.Body.Bytes(), &regResp)
	assertion, _ := regResp["identity_assertion"].(string)
	if assertion == "" {
		t.Fatal("no identity_assertion in register response")
	}

	// Exchange assertion for access token.
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	tokReq := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(tokRec, tokReq)
	if tokRec.Code != http.StatusOK {
		t.Fatalf("token status = %d body = %q", tokRec.Code, tokRec.Body.String())
	}
	var tokResp map[string]interface{}
	_ = json.Unmarshal(tokRec.Body.Bytes(), &tokResp)
	if tokResp["access_token"] == nil {
		t.Fatal("no access_token in token response")
	}
}

func TestAgentIdentityClaimInitiate(t *testing.T) {
	svc := mustTestServiceOAuth(t)

	// Register first.
	regReq := httptest.NewRequest(http.MethodPost, "/agent/identity", strings.NewReader(`{"type":"anonymous"}`))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(regRec, regReq)
	var regResp map[string]interface{}
	_ = json.Unmarshal(regRec.Body.Bytes(), &regResp)
	claimToken, _ := regResp["claim_token"].(string)

	claimBody, _ := json.Marshal(map[string]string{"claim_token": claimToken})
	claimReq := httptest.NewRequest(http.MethodPost, "/agent/identity/claim", bytes.NewReader(claimBody))
	claimReq.Header.Set("Content-Type", "application/json")
	claimRec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(claimRec, claimReq)
	if claimRec.Code != http.StatusOK {
		t.Fatalf("claim status = %d body = %q", claimRec.Code, claimRec.Body.String())
	}
	var claimResp map[string]interface{}
	_ = json.Unmarshal(claimRec.Body.Bytes(), &claimResp)
	if claimResp["claim_attempt_id"] == nil {
		t.Fatal("missing claim_attempt_id")
	}
	if claimResp["status"] != "initiated" {
		t.Errorf("status = %v want initiated", claimResp["status"])
	}
}

func TestAgentEventNotifyAccepted(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	req := httptest.NewRequest(http.MethodPost, "/agent/event/notify", strings.NewReader(`test-event-token`))
	req.Header.Set("Content-Type", "application/secevent+jwt")
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
}

func TestAgentAuthDiscoveryBlock(t *testing.T) {
	svc := mustTestServiceOAuth(t)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()
	svc.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var meta map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &meta)
	agentAuth, ok := meta["agent_auth"].(map[string]interface{})
	if !ok {
		t.Fatal("agent_auth block missing from oauth-authorization-server metadata")
	}
	for _, field := range []string{"skill", "identity_endpoint", "claim_endpoint", "events_endpoint"} {
		if agentAuth[field] == nil {
			t.Errorf("agent_auth.%s is missing", field)
		}
	}
}
