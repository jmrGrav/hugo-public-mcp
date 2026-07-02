package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SiteRoot         string      `yaml:"site_root"`
	SiteURL          string      `yaml:"site_url"`
	SiteName         string      `yaml:"site_name"`
	DefaultLanguage  string      `yaml:"language_default"`
	Transport        string      `yaml:"transport"`
	HTTPBindAddr     string      `yaml:"http_bind_addr"`
	HTTPBindPort     int         `yaml:"http_bind_port"`
	StreamingEnabled bool        `yaml:"streaming_enabled"`
	MaxIndexEntries  int         `yaml:"max_index_entries"`
	MaxResultItems   int         `yaml:"max_result_items"`
	MaxRequestBytes  int64       `yaml:"max_request_bytes"`
	RejectSymlinks   bool        `yaml:"reject_symlinks"`
	RejectHiddenPath bool        `yaml:"reject_hidden_paths"`
	OAuth            OAuthConfig `yaml:"oauth"`
}

type OAuthConfig struct {
	Enabled               bool     `yaml:"enabled"`
	Issuer                string   `yaml:"issuer"`
	Resource              string   `yaml:"resource"`
	DynamicClientEnabled  bool     `yaml:"dynamic_client_registration"`
	RequirePKCE           bool     `yaml:"require_pkce"`
	TrustedAuthorizeCIDRs []string `yaml:"trusted_authorize_cidrs"`
	AuthCodeTTLSeconds    int      `yaml:"auth_code_ttl_seconds"`
	AccessTokenTTLSeconds int      `yaml:"access_token_ttl_seconds"`
}

func Default() Config {
	return Config{
		Transport:        "stdio",
		HTTPBindAddr:     "127.0.0.1",
		HTTPBindPort:     8088,
		StreamingEnabled: true,
		DefaultLanguage:  "en",
		MaxIndexEntries:  5000,
		MaxResultItems:   50,
		MaxRequestBytes:  1 << 20,
		RejectSymlinks:   true,
		RejectHiddenPath: true,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, err
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_SITE_ROOT")); v != "" {
		cfg.SiteRoot = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_SITE_URL")); v != "" {
		cfg.SiteURL = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_SITE_NAME")); v != "" {
		cfg.SiteName = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_DEFAULT_LANGUAGE")); v != "" {
		cfg.DefaultLanguage = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_TRANSPORT")); v != "" {
		cfg.Transport = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_STREAMING_ENABLED")); v != "" {
		cfg.StreamingEnabled = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_ENABLED")); v != "" {
		cfg.OAuth.Enabled = parseBool(v)
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_ISSUER")); v != "" {
		cfg.OAuth.Issuer = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_RESOURCE")); v != "" {
		cfg.OAuth.Resource = v
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_DYNAMIC_CLIENT_REGISTRATION")); v != "" {
		cfg.OAuth.DynamicClientEnabled = parseBool(v)
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_REQUIRE_PKCE")); v != "" {
		cfg.OAuth.RequirePKCE = parseBool(v)
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_TRUSTED_AUTHORIZE_CIDRS")); v != "" {
		cfg.OAuth.TrustedAuthorizeCIDRs = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_AUTH_CODE_TTL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.OAuth.AuthCodeTTLSeconds = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("HUGO_PUBLIC_MCP_OAUTH_ACCESS_TOKEN_TTL_SECONDS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.OAuth.AccessTokenTTLSeconds = parsed
		}
	}
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.SiteRoot) == "" {
		return fmt.Errorf("missing required site_root")
	}
	if c.HTTPBindAddr == "" {
		c.HTTPBindAddr = "127.0.0.1"
	}
	if c.HTTPBindPort <= 0 {
		c.HTTPBindPort = 8088
	}
	if c.MaxIndexEntries <= 0 {
		c.MaxIndexEntries = 5000
	}
	if c.MaxResultItems <= 0 {
		c.MaxResultItems = 50
	}
	if c.MaxRequestBytes <= 0 {
		c.MaxRequestBytes = 1 << 20
	}
	if c.Transport == "" {
		c.Transport = "stdio"
	}
	if c.Transport != "stdio" && c.Transport != "http" {
		return fmt.Errorf("transport must be stdio or http")
	}
	if c.OAuth.Enabled {
		if c.OAuth.AuthCodeTTLSeconds <= 0 {
			c.OAuth.AuthCodeTTLSeconds = 300
		}
		if c.OAuth.AccessTokenTTLSeconds <= 0 {
			c.OAuth.AccessTokenTTLSeconds = 3600
		}
		if len(c.OAuth.TrustedAuthorizeCIDRs) == 0 {
			c.OAuth.TrustedAuthorizeCIDRs = []string{"127.0.0.1/32", "::1/128"}
		}
		if !c.OAuth.DynamicClientEnabled {
			c.OAuth.DynamicClientEnabled = true
		}
		if !c.OAuth.RequirePKCE {
			c.OAuth.RequirePKCE = true
		}
	}
	return nil
}

func parseBool(v string) bool {
	return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if cleaned := strings.TrimSpace(part); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}
