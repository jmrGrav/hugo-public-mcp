package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SiteRoot         string `yaml:"site_root"`
	SiteURL          string `yaml:"site_url"`
	SiteName         string `yaml:"site_name"`
	DefaultLanguage  string `yaml:"language_default"`
	Transport        string `yaml:"transport"`
	HTTPBindAddr     string `yaml:"http_bind_addr"`
	HTTPBindPort     int    `yaml:"http_bind_port"`
	StreamingEnabled bool   `yaml:"streaming_enabled"`
	MaxIndexEntries  int    `yaml:"max_index_entries"`
	MaxResultItems   int    `yaml:"max_result_items"`
	MaxRequestBytes  int64  `yaml:"max_request_bytes"`
	RejectSymlinks   bool   `yaml:"reject_symlinks"`
	RejectHiddenPath bool   `yaml:"reject_hidden_paths"`
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
	return nil
}
