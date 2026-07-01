package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jmrGrav/hugo-public-mcp/internal/config"
	"github.com/jmrGrav/hugo-public-mcp/internal/observability"
	"github.com/jmrGrav/hugo-public-mcp/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "hugo-public-mcp: %s\n", observability.RedactString(err.Error()))
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfgPath := os.Getenv("HUGO_PUBLIC_MCP_CONFIG")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	svc, err := server.New(cfg)
	if err != nil {
		return err
	}
	logger := observability.New()
	if cfg.Transport == "http" {
		return svc.RunHTTP(ctx, logger)
	}
	return svc.RunStdio(ctx)
}
