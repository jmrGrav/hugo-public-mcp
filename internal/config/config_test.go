package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultOAuthDisabled(t *testing.T) {
	cfg := Default()

	if cfg.OAuth.Enabled {
		t.Fatal("OAuth must be disabled by default")
	}
}

func TestLoadOAuthEnabledFromYAML(t *testing.T) {
	path := writeTempConfig(t, `
site_root: /tmp/public
oauth:
  enabled: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.OAuth.Enabled {
		t.Fatal("expected oauth.enabled=true from YAML")
	}
}

func TestLoadOAuthEnabledFromEnv(t *testing.T) {
	t.Setenv("HUGO_PUBLIC_MCP_OAUTH_ENABLED", "true")
	t.Setenv("HUGO_PUBLIC_MCP_OAUTH_DYNAMIC_CLIENT_REGISTRATION", "true")
	t.Setenv("HUGO_PUBLIC_MCP_OAUTH_REQUIRE_PKCE", "true")
	t.Setenv("HUGO_PUBLIC_MCP_OAUTH_TRUSTED_AUTHORIZE_CIDRS", "127.0.0.1/32,::1/128")
	t.Setenv("HUGO_PUBLIC_MCP_OAUTH_AUTH_CODE_TTL_SECONDS", "123")
	t.Setenv("HUGO_PUBLIC_MCP_OAUTH_ACCESS_TOKEN_TTL_SECONDS", "456")

	cfg, err := Load(writeTempConfig(t, `site_root: /tmp/public`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.OAuth.Enabled {
		t.Fatal("expected OAuth enabled from HUGO_PUBLIC_MCP_OAUTH_ENABLED")
	}
	if !cfg.OAuth.DynamicClientEnabled || !cfg.OAuth.RequirePKCE {
		t.Fatalf("expected OAuth boolean env flags enabled: %#v", cfg.OAuth)
	}
	if got := cfg.OAuth.TrustedAuthorizeCIDRs; len(got) != 2 || got[0] != "127.0.0.1/32" || got[1] != "::1/128" {
		t.Fatalf("unexpected trusted authorize CIDRs: %#v", got)
	}
	if cfg.OAuth.AuthCodeTTLSeconds != 123 || cfg.OAuth.AccessTokenTTLSeconds != 456 {
		t.Fatalf("unexpected OAuth TTLs: %#v", cfg.OAuth)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
