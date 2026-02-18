package openai

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

const (
	CodexOAuthAuthBaseURL   = "https://auth.openai.com"
	CodexOAuthAuthorizePath = "/oauth/authorize"
	CodexOAuthTokenPath     = "/oauth/token"
	CodexOAuthClientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexOAuthRedirectURI   = "http://localhost:1455/auth/callback"
	CodexOAuthScope         = "openid profile email offline_access"
)

type CodexPKCEValues struct {
	State         string
	CodeVerifier  string
	CodeChallenge string
}

type CodexOAuthToken struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	TokenType    string
	ExpiresAt    time.Time
}

type CodexOAuth struct {
	authBaseURL string
	clientID    string
	redirectURI string
	scope       string
	httpClient  *http.Client
	now         func() time.Time
}

type codexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func NewCodexOAuth() *CodexOAuth {
	transport := egress.NewAllowlistRoundTripper(http.DefaultTransport, []string{"auth.openai.com"})
	return &CodexOAuth{
		authBaseURL: CodexOAuthAuthBaseURL,
		clientID:    CodexOAuthClientID,
		redirectURI: CodexOAuthRedirectURI,
		scope:       CodexOAuthScope,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
		now: time.Now,
	}
}

func GenerateCodexPKCE() (CodexPKCEValues, error) {
	return generateCodexPKCE(rand.Reader)
}

func generateCodexPKCE(reader io.Reader) (CodexPKCEValues, error) {
	verifierBytes := make([]byte, 32)
	if _, err := io.ReadFull(reader, verifierBytes); err != nil {
		return CodexPKCEValues{}, err
	}
	stateBytes := make([]byte, 24)
	if _, err := io.ReadFull(reader, stateBytes); err != nil {
		return CodexPKCEValues{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return CodexPKCEValues{
		State:         state,
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
	}, nil
}

func (c *CodexOAuth) BuildAuthorizeURL(state, codeChallenge string) (string, error) {
	base, err := url.Parse(c.authBaseURL)
	if err != nil {
		return "", err
	}
	base.Path = CodexOAuthAuthorizePath
	query := base.Query()
	query.Set("client_id", c.clientID)
	query.Set("redirect_uri", c.redirectURI)
	query.Set("scope", c.scope)
	query.Set("response_type", "code")
	query.Set("code_challenge_method", "S256")
	query.Set("code_challenge", strings.TrimSpace(codeChallenge))
	query.Set("state", strings.TrimSpace(state))
	query.Set("id_token_add_organizations", "true")
	query.Set("codex_cli_simplified_flow", "true")
	query.Set("originator", "pi")
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *CodexOAuth) ParseRedirectURL(redirectURL string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(redirectURL))
	if err != nil {
		return "", "", err
	}
	query := parsed.Query()
	if oauthErr := strings.TrimSpace(query.Get("error")); oauthErr != "" {
		desc := strings.TrimSpace(query.Get("error_description"))
		if desc != "" {
			return "", "", fmt.Errorf("oauth error: %s: %s", oauthErr, desc)
		}
		return "", "", fmt.Errorf("oauth error: %s", oauthErr)
	}
	code := strings.TrimSpace(query.Get("code"))
	state := strings.TrimSpace(query.Get("state"))
	if code == "" {
		return "", "", errors.New("missing code in redirect URL")
	}
	if state == "" {
		return "", "", errors.New("missing state in redirect URL")
	}
	return code, state, nil
}

func (c *CodexOAuth) ExchangeAuthorizationCode(ctx context.Context, code, codeVerifier string) (CodexOAuthToken, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", strings.TrimSpace(code))
	values.Set("redirect_uri", c.redirectURI)
	values.Set("client_id", c.clientID)
	values.Set("code_verifier", strings.TrimSpace(codeVerifier))
	return c.postToken(ctx, values)
}

func (c *CodexOAuth) RefreshAccessToken(ctx context.Context, refreshToken string) (CodexOAuthToken, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", strings.TrimSpace(refreshToken))
	values.Set("client_id", c.clientID)
	return c.postToken(ctx, values)
}

func (c *CodexOAuth) postToken(ctx context.Context, values url.Values) (CodexOAuthToken, error) {
	endpoint, err := url.Parse(c.authBaseURL)
	if err != nil {
		return CodexOAuthToken{}, err
	}
	endpoint.Path = CodexOAuthTokenPath
	body := bytes.NewBufferString(values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return CodexOAuthToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return CodexOAuthToken{}, llm.ErrEgressBlocked
		}
		return CodexOAuthToken{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		return CodexOAuthToken{}, llm.ErrUnauthorized
	}
	if resp.StatusCode >= 500 {
		return CodexOAuthToken{}, llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorBody, _ := io.ReadAll(resp.Body)
		return CodexOAuthToken{}, fmt.Errorf("oauth token exchange failed: %s - %s", resp.Status, strings.TrimSpace(string(errorBody)))
	}
	var payload codexTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CodexOAuthToken{}, err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return CodexOAuthToken{}, errors.New("oauth token response missing access_token")
	}
	expiresAt := c.now().UTC()
	if payload.ExpiresIn > 0 {
		expiresAt = expiresAt.Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	return CodexOAuthToken{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		IDToken:      payload.IDToken,
		TokenType:    payload.TokenType,
		ExpiresAt:    expiresAt,
	}, nil
}

func (c *CodexOAuth) ExtractChatGPTAccountID(idToken string) string {
	return ExtractCodexChatGPTAccountID(idToken)
}

func ExtractCodexChatGPTAccountID(jwtToken string) string {
	parts := strings.Split(strings.TrimSpace(jwtToken), ".")
	if len(parts) < 2 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return ""
	}
	if accountID := codexAccountIDFromAny(payload["https://api.openai.com/auth.chatgpt_account_id"]); accountID != "" {
		return accountID
	}
	if accountID := codexAccountIDFromAny(payload["chatgpt_account_id"]); accountID != "" {
		return accountID
	}
	authClaimRaw, ok := payload["https://api.openai.com/auth"]
	if !ok {
		return ""
	}
	authClaim, ok := authClaimRaw.(map[string]any)
	if !ok {
		return ""
	}
	accountIDRaw, ok := authClaim["chatgpt_account_id"]
	if !ok {
		return ""
	}
	return codexAccountIDFromAny(accountIDRaw)
}

func codexAccountIDFromAny(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return ""
	}
}
