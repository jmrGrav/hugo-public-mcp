package oauth

import (
	"strings"
	"testing"
)

func TestServiceRegisterAuthorizeAndExchangePKCE(t *testing.T) {
	svc := NewService(Config{
		Issuer:                "https://mcp.example.test",
		Resource:              "https://mcp.example.test/mcp",
		AuthCodeTTLSeconds:    300,
		AccessTokenTTLSeconds: 3600,
		TrustedAuthorizeCIDRs: []string{"127.0.0.1/32"},
		RequirePKCE:           true,
		DynamicClientEnabled:  true,
		SupportedScopes:       []string{"mcp"},
	})

	reg, err := svc.RegisterClient(RegistrationRequest{RedirectURIs: []string{"https://client.example.test/callback"}})
	if err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}
	if reg.ClientID == "" {
		t.Fatal("expected generated client id")
	}

	verifier := "test-verifier-test-verifier-test-verifier"
	challenge := CodeChallengeS256(verifier)
	code, err := svc.IssueAuthCode(AuthorizeRequest{
		SourceIP:            "127.0.0.1",
		ResponseType:        "code",
		ClientID:            reg.ClientID,
		RedirectURI:         "https://client.example.test/callback",
		State:               "state-1",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("IssueAuthCode() error = %v", err)
	}
	if code == "" {
		t.Fatal("expected auth code")
	}

	token, err := svc.ExchangeToken(TokenExchangeRequest{
		GrantType:    "authorization_code",
		ClientID:     reg.ClientID,
		Code:         code,
		RedirectURI:  "https://client.example.test/callback",
		CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("ExchangeToken() error = %v", err)
	}
	if token.AccessToken == "" || token.TokenType != "Bearer" || token.Scope != "mcp" {
		t.Fatalf("unexpected token response: %#v", token)
	}
	if !svc.ValidateAccessToken(token.AccessToken) {
		t.Fatal("issued token should validate")
	}
}

func TestServiceRejectsUnsafeOAuthRequests(t *testing.T) {
	svc := NewService(Config{
		Issuer:                "https://mcp.example.test",
		Resource:              "https://mcp.example.test/mcp",
		AuthCodeTTLSeconds:    300,
		AccessTokenTTLSeconds: 3600,
		TrustedAuthorizeCIDRs: []string{"127.0.0.1/32"},
		RequirePKCE:           true,
		DynamicClientEnabled:  true,
		SupportedScopes:       []string{"mcp"},
	})

	for _, uri := range []string{"file:///tmp/callback", "http://evil.example/callback"} {
		if _, err := svc.RegisterClient(RegistrationRequest{RedirectURIs: []string{uri}}); err == nil || !strings.Contains(err.Error(), "invalid_redirect_uri") {
			t.Fatalf("expected invalid_redirect_uri for %q, got %v", uri, err)
		}
	}
	reg, err := svc.RegisterClient(RegistrationRequest{RedirectURIs: []string{"https://client.example.test/callback"}})
	if err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}
	if _, err := svc.IssueAuthCode(AuthorizeRequest{
		SourceIP:            "203.0.113.10",
		ResponseType:        "code",
		ClientID:            reg.ClientID,
		RedirectURI:         "https://client.example.test/callback",
		State:               "state-1",
		CodeChallenge:       CodeChallengeS256("test-verifier-test-verifier-test-verifier"),
		CodeChallengeMethod: "S256",
	}); err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("expected access_denied for non-operator source, got %v", err)
	}
}
