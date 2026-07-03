package oauth

import (
	"crypto/rand"
	"fmt"
	"time"
)

type agentRegistration struct {
	RegistrationID   string
	AssertionToken   string
	ClaimToken       string
	AssertionExpires time.Time
	ClaimExpires     time.Time
}

type agentClaim struct {
	RegistrationID string
	ClaimAttemptID string
	ExpiresAt      time.Time
}

// AgentClaimInfo is the polling hint returned inside agent identity responses.
type AgentClaimInfo struct {
	UserCode        string `json:"user_code"`
	ExpiresIn       int    `json:"expires_in"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
}

// AgentIdentityResponse is returned from POST /agent/identity.
type AgentIdentityResponse struct {
	RegistrationID    string          `json:"registration_id"`
	RegistrationType  string          `json:"registration_type"`
	IdentityAssertion string          `json:"identity_assertion"`
	AssertionExpires  string          `json:"assertion_expires"`
	PreClaimScopes    []string        `json:"pre_claim_scopes"`
	ClaimURL          string          `json:"claim_url"`
	ClaimToken        string          `json:"claim_token"`
	ClaimTokenExpires string          `json:"claim_token_expires"`
	PostClaimScopes   []string        `json:"post_claim_scopes"`
	Claim             *AgentClaimInfo `json:"claim"`
}

// AgentClaimResponse is returned from POST /agent/identity/claim.
type AgentClaimResponse struct {
	RegistrationID string          `json:"registration_id"`
	ClaimAttemptID string          `json:"claim_attempt_id"`
	Status         string          `json:"status"`
	ExpiresAt      string          `json:"expires_at"`
	ClaimAttempt   *AgentClaimInfo `json:"claim_attempt"`
}

// RegisterAgentAnonymous creates an anonymous agent registration and issues an
// assertion token that can be immediately exchanged for an access token.
func (s *Service) RegisterAgentAnonymous() (*AgentIdentityResponse, error) {
	regID := "reg_" + randomString(20)
	assertion := "arleo_assert_" + randomString(32)
	claimToken := "clm_" + randomString(24)
	now := time.Now()
	assertionExpires := now.Add(time.Hour)
	claimExpires := now.Add(10 * time.Minute)

	s.mu.Lock()
	s.agentRegs[assertion] = agentRegistration{
		RegistrationID:   regID,
		AssertionToken:   assertion,
		ClaimToken:       claimToken,
		AssertionExpires: assertionExpires,
		ClaimExpires:     claimExpires,
	}
	s.agentClaimTokens[claimToken] = assertion
	s.mu.Unlock()

	return &AgentIdentityResponse{
		RegistrationID:    regID,
		RegistrationType:  "anonymous",
		IdentityAssertion: assertion,
		AssertionExpires:  assertionExpires.UTC().Format(time.RFC3339Nano),
		PreClaimScopes:    append([]string(nil), s.cfg.SupportedScopes...),
		ClaimURL:          s.cfg.Issuer + "/agent/identity/claim",
		ClaimToken:        claimToken,
		ClaimTokenExpires: claimExpires.UTC().Format(time.RFC3339Nano),
		PostClaimScopes:   append([]string(nil), s.cfg.SupportedScopes...),
		Claim: &AgentClaimInfo{
			UserCode:        randomDigits(6),
			ExpiresIn:       600,
			VerificationURI: s.cfg.Issuer + "/agent/identity/verify",
			Interval:        5,
		},
	}, nil
}

// ExchangeAgentAssertion validates an agent assertion token and issues an access token.
func (s *Service) ExchangeAgentAssertion(assertion string) (*TokenResponse, error) {
	s.mu.RLock()
	reg, ok := s.agentRegs[assertion]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("invalid_grant")
	}
	if time.Now().After(reg.AssertionExpires) {
		return nil, fmt.Errorf("invalid_grant")
	}

	token := randomString(32)
	hash := HashToken(token)
	ttl := time.Duration(s.cfg.AccessTokenTTLSeconds) * time.Second
	s.mu.Lock()
	s.tokens[hash] = time.Now().Add(ttl)
	s.mu.Unlock()

	return &TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   s.cfg.AccessTokenTTLSeconds,
	}, nil
}

// InitiateClaim starts the claim ceremony for an existing registration.
func (s *Service) InitiateClaim(claimToken string) (*AgentClaimResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assertionKey, ok := s.agentClaimTokens[claimToken]
	if !ok {
		return nil, fmt.Errorf("invalid_claim_token")
	}
	reg, ok := s.agentRegs[assertionKey]
	if !ok {
		return nil, fmt.Errorf("invalid_claim_token")
	}
	if time.Now().After(reg.ClaimExpires) {
		return nil, fmt.Errorf("claim_expired")
	}

	attemptID := "cla_" + randomString(20)
	expiresAt := time.Now().Add(10 * time.Minute)
	s.agentClaims[attemptID] = agentClaim{
		RegistrationID: reg.RegistrationID,
		ClaimAttemptID: attemptID,
		ExpiresAt:      expiresAt,
	}

	return &AgentClaimResponse{
		RegistrationID: reg.RegistrationID,
		ClaimAttemptID: attemptID,
		Status:         "initiated",
		ExpiresAt:      expiresAt.UTC().Format(time.RFC3339Nano),
		ClaimAttempt: &AgentClaimInfo{
			UserCode:        randomDigits(6),
			ExpiresIn:       600,
			VerificationURI: s.cfg.Issuer + "/agent/identity/verify",
			Interval:        5,
		},
	}, nil
}

func randomDigits(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = digits[b[i]%10]
	}
	return string(b)
}
