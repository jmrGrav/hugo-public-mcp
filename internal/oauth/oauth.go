package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Issuer                string
	Resource              string
	AuthCodeTTLSeconds    int
	AccessTokenTTLSeconds int
	TrustedAuthorizeCIDRs []string
	RequirePKCE           bool
	DynamicClientEnabled  bool
	SupportedScopes       []string
}

type Service struct {
	cfg     Config
	mu      sync.Mutex
	clients map[string]client
	codes   map[string]authCode
	tokens  map[string]time.Time
}

type client struct {
	RedirectURIs []string
}

type authCode struct {
	ClientID            string
	RedirectURI         string
	ExpiresAt           time.Time
	CodeChallenge       string
	CodeChallengeMethod string
}

type RegistrationRequest struct {
	RedirectURIs []string `json:"redirect_uris"`
}

type RegistrationResponse struct {
	ClientID                      string   `json:"client_id"`
	ClientIDIssuedAt              int64    `json:"client_id_issued_at"`
	RedirectURIs                  []string `json:"redirect_uris"`
	GrantTypes                    []string `json:"grant_types"`
	ResponseTypes                 []string `json:"response_types"`
	TokenEndpointAuthMethod       string   `json:"token_endpoint_auth_method"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	Scope                         string   `json:"scope"`
}

type AuthorizeRequest struct {
	SourceIP            string
	ResponseType        string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

type TokenExchangeRequest struct {
	GrantType    string
	ClientID     string
	RedirectURI  string
	Code         string
	CodeVerifier string
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

func NewService(cfg Config) *Service {
	if cfg.AuthCodeTTLSeconds <= 0 {
		cfg.AuthCodeTTLSeconds = 300
	}
	if cfg.AccessTokenTTLSeconds <= 0 {
		cfg.AccessTokenTTLSeconds = 3600
	}
	if len(cfg.TrustedAuthorizeCIDRs) == 0 {
		cfg.TrustedAuthorizeCIDRs = []string{"127.0.0.1/32", "::1/128"}
	}
	if len(cfg.SupportedScopes) == 0 {
		cfg.SupportedScopes = []string{"mcp"}
	}
	return &Service{
		cfg:     cfg,
		clients: make(map[string]client),
		codes:   make(map[string]authCode),
		tokens:  make(map[string]time.Time),
	}
}

func (s *Service) AuthorizationServerMetadata() map[string]interface{} {
	return map[string]interface{}{
		"issuer":                                s.cfg.Issuer,
		"authorization_endpoint":                s.cfg.Issuer + "/authorize",
		"token_endpoint":                        s.cfg.Issuer + "/token",
		"registration_endpoint":                 s.cfg.Issuer + "/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      s.cfg.SupportedScopes,
		"service_documentation":                 s.cfg.Resource,
	}
}

func (s *Service) ProtectedResourceMetadata() map[string]interface{} {
	return map[string]interface{}{
		"resource":                 s.cfg.Resource,
		"authorization_servers":    []string{s.cfg.Issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         s.cfg.SupportedScopes,
		"resource_documentation":   s.cfg.Resource,
	}
}

func (s *Service) RegisterClient(req RegistrationRequest) (*RegistrationResponse, error) {
	if !s.cfg.DynamicClientEnabled {
		return nil, fmt.Errorf("invalid_request: dynamic_client_registration_disabled")
	}
	if len(req.RedirectURIs) == 0 {
		return nil, fmt.Errorf("invalid_request: redirect_uris missing or empty")
	}
	for _, uri := range req.RedirectURIs {
		if !isAllowedRedirectURI(uri) {
			return nil, fmt.Errorf("invalid_redirect_uri")
		}
	}
	id := randomString(24)
	s.mu.Lock()
	s.clients[id] = client{RedirectURIs: append([]string(nil), req.RedirectURIs...)}
	s.mu.Unlock()
	return &RegistrationResponse{
		ClientID:                      id,
		ClientIDIssuedAt:              time.Now().Unix(),
		RedirectURIs:                  append([]string(nil), req.RedirectURIs...),
		GrantTypes:                    []string{"authorization_code"},
		ResponseTypes:                 []string{"code"},
		TokenEndpointAuthMethod:       "none",
		CodeChallengeMethodsSupported: []string{"S256"},
		Scope:                         "mcp",
	}, nil
}

func (s *Service) IssueAuthCode(req AuthorizeRequest) (string, error) {
	if !s.sourceAllowed(req.SourceIP) {
		return "", fmt.Errorf("access_denied: authorize source is not trusted")
	}
	if req.ResponseType != "code" {
		return "", fmt.Errorf("unsupported_response_type")
	}
	if req.State == "" {
		return "", fmt.Errorf("invalid_request: missing state parameter")
	}
	if req.CodeChallenge == "" && s.cfg.RequirePKCE {
		return "", fmt.Errorf("invalid_request: pkce_mandatory")
	}
	if req.CodeChallenge != "" {
		if req.CodeChallengeMethod != "S256" {
			return "", fmt.Errorf("invalid_request: unsupported code_challenge_method")
		}
		if len(req.CodeChallenge) < 43 || len(req.CodeChallenge) > 128 {
			return "", fmt.Errorf("invalid_request: code_challenge length invalid")
		}
	}
	s.mu.Lock()
	c, ok := s.clients[req.ClientID]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("unauthorized_client")
	}
	if !stringInSlice(req.RedirectURI, c.RedirectURIs) {
		return "", fmt.Errorf("invalid_redirect_uri")
	}
	code := randomString(32)
	s.mu.Lock()
	s.codes[code] = authCode{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		ExpiresAt:           time.Now().Add(time.Duration(s.cfg.AuthCodeTTLSeconds) * time.Second),
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
	}
	s.mu.Unlock()
	return code, nil
}

// IsRegisteredRedirectURI reports whether uri is a redirect URI registered for clientID.
// Call this before issuing any redirect to make the validation visible to static analysis.
func (s *Service) IsRegisteredRedirectURI(clientID, uri string) bool {
	s.mu.Lock()
	c, ok := s.clients[clientID]
	s.mu.Unlock()
	return ok && stringInSlice(uri, c.RedirectURIs)
}

func (s *Service) ExchangeToken(req TokenExchangeRequest) (*TokenResponse, error) {
	if req.GrantType != "authorization_code" {
		return nil, fmt.Errorf("unsupported_grant_type")
	}
	s.mu.Lock()
	data, ok := s.codes[req.Code]
	if ok {
		delete(s.codes, req.Code)
	}
	s.mu.Unlock()
	if !ok || data.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("invalid_grant: invalid or expired code")
	}
	if subtle.ConstantTimeCompare([]byte(req.ClientID), []byte(data.ClientID)) != 1 {
		return nil, fmt.Errorf("invalid_client")
	}
	if req.RedirectURI == "" || req.RedirectURI != data.RedirectURI {
		return nil, fmt.Errorf("invalid_grant: redirect_uri mismatch")
	}
	if data.CodeChallenge != "" && !ValidatePKCE(data.CodeChallenge, req.CodeVerifier) {
		return nil, fmt.Errorf("invalid_grant: pkce verification failed")
	}
	token := randomString(32)
	s.mu.Lock()
	s.tokens[HashToken(token)] = time.Now().Add(time.Duration(s.cfg.AccessTokenTTLSeconds) * time.Second)
	s.mu.Unlock()
	return &TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   s.cfg.AccessTokenTTLSeconds,
		Scope:       "mcp",
	}, nil
}

func (s *Service) ValidateAccessToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.tokens[HashToken(token)]
	if !ok {
		return false
	}
	if !expiresAt.After(time.Now()) {
		delete(s.tokens, HashToken(token))
		return false
	}
	return true
}

func (s *Service) sourceAllowed(ipText string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipText))
	if ip == nil {
		return false
	}
	for _, raw := range s.cfg.TrustedAuthorizeCIDRs {
		_, cidr, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func isAllowedRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	if u.Hostname() == "localhost" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

func stringInSlice(v string, items []string) bool {
	for _, item := range items {
		if item == v {
			return true
		}
	}
	return false
}

func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func ValidatePKCE(challenge, verifier string) bool {
	if challenge == "" || verifier == "" {
		return false
	}
	expected := CodeChallengeS256(verifier)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(challenge)) == 1
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomString(bytesLen int) string {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
