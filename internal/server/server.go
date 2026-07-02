package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jmrGrav/hugo-public-mcp/internal/config"
	"github.com/jmrGrav/hugo-public-mcp/internal/oauth"
	"github.com/jmrGrav/hugo-public-mcp/internal/observability"
	"github.com/jmrGrav/hugo-public-mcp/internal/publicmcp"
	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	Name    = "hugo-public-mcp"
	Version = "v0.0.1"
)

var publicToolNames = map[string]struct{}{
	"list_pages":           {},
	"get_page":             {},
	"search_pages":         {},
	"get_recent_posts":     {},
	"list_tags":            {},
	"list_categories":      {},
	"get_sitemap":          {},
	"get_feed":             {},
	"get_site_information": {},
}

type Service struct {
	cfg        config.Config
	index      *site.Index
	server     *mcp.Server
	fullServer *mcp.Server
	oauth      *oauth.Service
}

func New(cfg config.Config) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	index, err := site.BuildIndex(context.Background(), cfg.SiteRoot, site.Config{
		SiteURL:          cfg.SiteURL,
		SiteName:         cfg.SiteName,
		DefaultLanguage:  cfg.DefaultLanguage,
		MaxIndexEntries:  cfg.MaxIndexEntries,
		RejectSymlinks:   cfg.RejectSymlinks,
		RejectHiddenPath: cfg.RejectHiddenPath,
	})
	if err != nil {
		return nil, err
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: Name, Version: Version}, nil)
	publicmcp.Register(srv, publicmcp.Dependencies{Index: index})
	svc := &Service{cfg: cfg, index: index, server: srv}
	if cfg.OAuth.Enabled {
		svc.oauth = oauth.NewService(svc.oauthConfigForRequest(nil))
		if err := svc.initFullServer(); err != nil {
			return nil, err
		}
	}
	return svc, nil
}

// initFullServer creates the OAuth-gated MCP server with public + private tools.
// Called from New() when OAuth.Enabled, and from tests to re-init after config change.
func (s *Service) initFullServer() error {
	if s == nil || !s.cfg.OAuth.Enabled {
		return nil
	}
	full := mcp.NewServer(&mcp.Implementation{Name: Name, Version: Version}, nil)
	publicmcp.Register(full, publicmcp.Dependencies{Index: s.index})
	publicmcp.RegisterPrivate(full, publicmcp.Dependencies{Index: s.index})
	s.fullServer = full
	return nil
}

func (s *Service) MCP() *mcp.Server {
	if s == nil {
		return nil
	}
	return s.server
}

func (s *Service) Index() *site.Index {
	if s == nil {
		return nil
	}
	return s.index
}

func (s *Service) RunStdio(ctx context.Context) error {
	if s == nil || s.server == nil {
		return fmt.Errorf("server not initialized")
	}
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Service) HTTPHandler() http.Handler {
	return s.httpHandler(observability.New())
}

func (s *Service) RunHTTP(ctx context.Context, logger *slog.Logger) error {
	if s == nil || s.server == nil {
		return fmt.Errorf("server not initialized")
	}
	if logger == nil {
		logger = observability.New()
	}
	handler := s.httpHandler(logger)
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", s.cfg.HTTPBindAddr, s.cfg.HTTPBindPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}

func (s *Service) httpHandler(logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = observability.New()
	}
	opts := &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		DisableLocalhostProtection: true,
	}
	streaming := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.server
	}, opts)
	var fullStreaming http.Handler
	if s.fullServer != nil {
		fullStreaming = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return s.fullServer
		}, opts)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			logger.Info("public mcp request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"latency_ms", time.Since(start).Milliseconds(),
			)
		}()
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			if !s.cfg.OAuth.Enabled || s.oauth == nil {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodHead {
				return
			}
			_ = json.NewEncoder(w).Encode(s.oauthForRequest(r).AuthorizationServerMetadata())
			return
		case "/.well-known/oauth-protected-resource":
			if !s.cfg.OAuth.Enabled || s.oauth == nil {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodHead {
				return
			}
			_ = json.NewEncoder(w).Encode(s.oauthForRequest(r).ProtectedResourceMetadata())
			return
		case "/register":
			if !s.cfg.OAuth.Enabled || s.oauth == nil {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodPost {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req oauth.RegistrationRequest
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.cfg.MaxRequestBytes)).Decode(&req); err != nil {
				status = http.StatusBadRequest
				writeOAuthError(w, "invalid_request", http.StatusBadRequest)
				return
			}
			resp, err := s.oauth.RegisterClient(req)
			if err != nil {
				status = http.StatusBadRequest
				writeOAuthError(w, "invalid_redirect_uri", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(resp)
			return
		case "/authorize":
			if !s.cfg.OAuth.Enabled || s.oauth == nil {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodGet)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			q := r.URL.Query()
			clientID := q.Get("client_id")
			redirectURI := q.Get("redirect_uri")
			// Validate redirect_uri against registered client URIs before any redirect.
			if !s.oauth.IsRegisteredRedirectURI(clientID, redirectURI) {
				status = http.StatusBadRequest
				http.Error(w, "invalid_redirect_uri", http.StatusBadRequest)
				return
			}
			code, err := s.oauth.IssueAuthCode(oauth.AuthorizeRequest{
				SourceIP:            requestSourceIP(r),
				ResponseType:        q.Get("response_type"),
				ClientID:            clientID,
				RedirectURI:         redirectURI,
				State:               q.Get("state"),
				CodeChallenge:       q.Get("code_challenge"),
				CodeChallengeMethod: q.Get("code_challenge_method"),
			})
			if err != nil {
				status = oauthAuthorizeErrorStatus(err)
				if strings.Contains(err.Error(), "unauthorized_client") || strings.Contains(err.Error(), "access_denied") {
					http.Error(w, oauthAuthorizeErrorCode(err), status)
					return
				}
				params := url.Values{}
				params.Set("error", oauthAuthorizeErrorCode(err))
				if state := q.Get("state"); state != "" {
					params.Set("state", state)
				}
				http.Redirect(w, r, redirectURI+"?"+params.Encode(), http.StatusFound)
				return
			}
			params := url.Values{"code": {code}}
			if state := q.Get("state"); state != "" {
				params.Set("state", state)
			}
			http.Redirect(w, r, redirectURI+"?"+params.Encode(), http.StatusFound)
			return
		case "/token":
			if !s.cfg.OAuth.Enabled || s.oauth == nil {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodPost {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				status = http.StatusBadRequest
				writeOAuthError(w, "invalid_request", http.StatusBadRequest)
				return
			}
			resp, err := s.oauth.ExchangeToken(oauth.TokenExchangeRequest{
				GrantType:    r.FormValue("grant_type"),
				ClientID:     r.FormValue("client_id"),
				RedirectURI:  r.FormValue("redirect_uri"),
				Code:         r.FormValue("code"),
				CodeVerifier: r.FormValue("code_verifier"),
			})
			if err != nil {
				status = oauthTokenErrorStatus(err)
				writeOAuthError(w, oauthTokenErrorCode(err), status)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case "/health":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodHead {
				return
			}
			_ = json.NewEncoder(w).Encode(struct {
				Status   string `json:"status"`
				Service  string `json:"service"`
				Version  string `json:"version"`
				ReadOnly bool   `json:"read_only"`
			}{
				Status:   "ok",
				Service:  Name,
				Version:  Version,
				ReadOnly: true,
			})
			return
		case "/openapi.json":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/openapi+json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=600")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodHead {
				return
			}
			_, _ = w.Write(openAPISchema)
			return
		case "/.well-known/mcp.json":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			card := s.buildDiscoveryCard(r)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodHead {
				return
			}
			_ = json.NewEncoder(w).Encode(card)
			return
		case "/mcp":
			if r.Method != http.MethodPost {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if ct := r.Header.Get("Content-Type"); ct == "" || !containsJSON(ct) {
				status = http.StatusUnsupportedMediaType
				http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxRequestBytes+1))
			_ = r.Body.Close()
			if err != nil {
				status = http.StatusBadRequest
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
			if int64(len(body)) > s.cfg.MaxRequestBytes {
				status = http.StatusRequestEntityTooLarge
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			if s.cfg.OAuth.Enabled && s.oauth != nil {
				auth := strings.TrimSpace(r.Header.Get("Authorization"))
				if auth != "" {
					if !strings.HasPrefix(auth, "Bearer ") || !s.oauth.ValidateAccessToken(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) {
						status = http.StatusUnauthorized
						w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=%q, resource_metadata=%q", requestBaseURL(r), requestBaseURL(r)+"/.well-known/oauth-protected-resource"))
						writeOAuthError(w, "invalid_token", http.StatusUnauthorized)
						return
					}
					// Valid bearer: route to fullServer (public + private tools)
					r.Body = io.NopCloser(bytes.NewReader(body))
					if fullStreaming != nil {
						fullStreaming.ServeHTTP(w, r)
					} else {
						streaming.ServeHTTP(w, r)
					}
					return
				}
				// No bearer: block calls to private tools before MCP dispatch
				if containsForbiddenToolCall(body) {
					status = http.StatusForbidden
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden_tool"})
					return
				}
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			streaming.ServeHTTP(w, r)
		case "/mcp/events":
			if !s.cfg.StreamingEnabled {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			streaming.ServeHTTP(w, r)
		default:
			if s.servePublishedResource(w, r) {
				return
			}
			status = http.StatusNotFound
			http.NotFound(w, r)
		}
	})
}

func containsForbiddenToolCall(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			return false
		}
		for _, raw := range batch {
			if isForbiddenToolCall(raw) {
				return true
			}
		}
		return false
	}
	return isForbiddenToolCall(trimmed)
}

func isForbiddenToolCall(raw []byte) bool {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return false
	}
	if req.Method != "tools/call" {
		return false
	}
	_, ok := publicToolNames[req.Params.Name]
	return !ok
}

func (s *Service) oauthForRequest(r *http.Request) *oauth.Service {
	if s.oauth == nil {
		return nil
	}
	cfg := s.oauthConfigForRequest(r)
	if cfg.Issuer == s.oauth.AuthorizationServerMetadata()["issuer"] {
		return s.oauth
	}
	return oauth.NewService(cfg)
}

func (s *Service) oauthConfigForRequest(r *http.Request) oauth.Config {
	base := requestBaseURL(r)
	issuer := strings.TrimRight(s.cfg.OAuth.Issuer, "/")
	if issuer == "" {
		issuer = base
	}
	resource := strings.TrimSpace(s.cfg.OAuth.Resource)
	if resource == "" {
		resource = issuer + "/mcp"
	}
	return oauth.Config{
		Issuer:                issuer,
		Resource:              resource,
		AuthCodeTTLSeconds:    s.cfg.OAuth.AuthCodeTTLSeconds,
		AccessTokenTTLSeconds: s.cfg.OAuth.AccessTokenTTLSeconds,
		TrustedAuthorizeCIDRs: s.cfg.OAuth.TrustedAuthorizeCIDRs,
		RequirePKCE:           s.cfg.OAuth.RequirePKCE,
		DynamicClientEnabled:  s.cfg.OAuth.DynamicClientEnabled,
		SupportedScopes:       []string{"mcp"},
	}
}

func requestSourceIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.RemoteAddr
	if strings.Contains(host, ":") {
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			host = splitHost
		}
	}
	return host
}

func writeOAuthError(w http.ResponseWriter, code string, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func oauthAuthorizeErrorCode(err error) string {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unsupported_response_type"):
		return "unsupported_response_type"
	case strings.HasPrefix(msg, "access_denied"):
		return "access_denied"
	case strings.HasPrefix(msg, "unauthorized_client"):
		return "unauthorized_client"
	default:
		return "invalid_request"
	}
}

func oauthAuthorizeErrorStatus(err error) int {
	if strings.HasPrefix(err.Error(), "access_denied") {
		return http.StatusForbidden
	}
	if strings.HasPrefix(err.Error(), "unauthorized_client") {
		return http.StatusUnauthorized
	}
	return http.StatusBadRequest
}

func oauthTokenErrorCode(err error) string {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unsupported_grant_type"):
		return "unsupported_grant_type"
	case strings.HasPrefix(msg, "invalid_client"):
		return "invalid_client"
	case strings.HasPrefix(msg, "invalid_grant"):
		return "invalid_grant"
	default:
		return "invalid_request"
	}
}

func oauthTokenErrorStatus(err error) int {
	if strings.HasPrefix(err.Error(), "invalid_client") {
		return http.StatusUnauthorized
	}
	return http.StatusBadRequest
}

func (s *Service) servePublishedResource(w http.ResponseWriter, r *http.Request) bool {
	if s == nil || s.index == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	content, ok := s.index.GetResource(r.URL.Path)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", resourceMediaTypeForPath(r.URL.Path))
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = w.Write([]byte(content))
	return true
}

func resourceMediaTypeForPath(path string) string {
	switch path {
	case "/sitemap.xml", "/index.xml":
		return "application/xml; charset=utf-8"
	case "/feed.json", "/.well-known/agent-skills/index.json":
		return "application/json; charset=utf-8"
	case "/.well-known/api-catalog":
		return "application/linkset+json; profile=\"https://www.rfc-editor.org/info/rfc9727\"; charset=utf-8"
	case "/robots.txt", "/llms.txt", "/auth.md", "/.well-known/agent-skills/discover_hugo_site.md", "/.well-known/agent-skills/search_public_pages.md", "/.well-known/agent-skills/read_public_page.md", "/.well-known/agent-skills/list_public_tags.md", "/.well-known/agent-skills/list_public_categories.md", "/.well-known/agent-skills/get_public_feed.md", "/.well-known/agent-skills/get_public_sitemap.md":
		return "text/markdown; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

func containsJSON(v string) bool {
	return strings.Contains(strings.ToLower(v), "application/json")
}

type serverCard struct {
	Name               string            `json:"name"`
	Version            string            `json:"version"`
	Description        string            `json:"description"`
	Endpoint           string            `json:"endpoint"`
	Discovery          string            `json:"discovery"`
	Transport          string            `json:"transport"`
	Auth               string            `json:"auth"`
	ReadOnly           bool              `json:"read_only"`
	Tools              []string          `json:"tools"`
	AuthenticatedTools []string          `json:"authenticated_tools,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

func (s *Service) buildDiscoveryCard(r *http.Request) serverCard {
	base := requestBaseURL(r)
	card := serverCard{
		Name:        Name,
		Version:     Version,
		Description: "Read-only MCP server for published Hugo sites.",
		Endpoint:    base + "/mcp",
		Discovery:   base + "/.well-known/mcp.json",
		Transport:   "streamable-http",
		ReadOnly:    true,
		Tools: []string{
			"list_pages", "get_page", "search_pages", "get_recent_posts",
			"list_tags", "list_categories", "get_sitemap", "get_feed", "get_site_information",
		},
		Metadata: map[string]string{"scope": "public-read-only"},
	}
	if s.cfg.OAuth.Enabled {
		card.Auth = "oauth2-optional"
		card.AuthenticatedTools = []string{"get_full_page_markdown"}
	} else {
		card.Auth = "none"
	}
	return card
}

func requestBaseURL(r *http.Request) string {
	if r == nil {
		return "https://mcp.arleo.eu"
	}
	scheme := "https"
	if xf := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xf != "" {
		scheme = strings.ToLower(xf)
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		host = "mcp.arleo.eu"
	}
	return scheme + "://" + host
}
