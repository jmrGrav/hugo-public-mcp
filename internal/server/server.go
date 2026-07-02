package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jmrGrav/hugo-public-mcp/internal/config"
	"github.com/jmrGrav/hugo-public-mcp/internal/observability"
	"github.com/jmrGrav/hugo-public-mcp/internal/publicmcp"
	"github.com/jmrGrav/hugo-public-mcp/internal/site"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	Name    = "hugo-public-mcp"
	Version = "v0.0.1"
)

type Service struct {
	cfg    config.Config
	index  *site.Index
	server *mcp.Server
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
	return &Service{cfg: cfg, index: index, server: srv}, nil
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
	streaming := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		DisableLocalhostProtection: true,
	})
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
			card := buildDiscoveryCard(r)
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
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Endpoint    string            `json:"endpoint"`
	Discovery   string            `json:"discovery"`
	Transport   string            `json:"transport"`
	Auth        string            `json:"auth"`
	ReadOnly    bool              `json:"read_only"`
	Tools       []string          `json:"tools"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func buildDiscoveryCard(r *http.Request) serverCard {
	base := requestBaseURL(r)
	return serverCard{
		Name:        Name,
		Version:     Version,
		Description: "Read-only MCP server for published Hugo sites.",
		Endpoint:    base + "/mcp",
		Discovery:   base + "/.well-known/mcp.json",
		Transport:   "streamable-http",
		Auth:        "none",
		ReadOnly:    true,
		Tools: []string{
			"list_pages",
			"get_page",
			"search_pages",
			"get_recent_posts",
			"list_tags",
			"list_categories",
			"get_sitemap",
			"get_feed",
			"get_site_information",
		},
		Metadata: map[string]string{
			"scope": "public-read-only",
		},
	}
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
