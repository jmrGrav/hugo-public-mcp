package server

import (
	"bytes"
	"context"
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
	Version = "0.1.0"
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
			status = http.StatusNotFound
			http.NotFound(w, r)
		}
	})
}

func containsJSON(v string) bool {
	return strings.Contains(strings.ToLower(v), "application/json")
}
