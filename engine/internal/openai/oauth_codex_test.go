package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type codexOAuthRT struct {
	statusCode int
	body       string
	lastForm   string
}

func (r *codexOAuthRT) RoundTrip(req *http.Request) (*http.Response, error) {
	bodyBytes, _ := io.ReadAll(req.Body)
	r.lastForm = string(bodyBytes)
	status := r.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	body := strings.TrimSpace(r.body)
	if body == "" {
		body = `{"access_token":"at-1","refresh_token":"rt-1","id_token":"id-1","token_type":"Bearer","expires_in":3600}`
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func TestGenerateCodexPKCE(t *testing.T) {
	pkce, err := GenerateCodexPKCE()
	if err != nil {
		t.Fatalf("GenerateCodexPKCE error: %v", err)
	}
	if len(pkce.State) < 20 {
		t.Fatalf("expected state to be populated, got %q", pkce.State)
	}
	if len(pkce.CodeVerifier) < 40 {
		t.Fatalf("expected verifier to be populated, got %q", pkce.CodeVerifier)
	}
	if len(pkce.CodeChallenge) < 40 {
		t.Fatalf("expected challenge to be populated, got %q", pkce.CodeChallenge)
	}
}

func TestCodexOAuthBuildAuthorizeURL(t *testing.T) {
	client := NewCodexOAuth()
	authURL, err := client.BuildAuthorizeURL("state-1", "challenge-1")
	if err != nil {
		t.Fatalf("BuildAuthorizeURL error: %v", err)
	}
	if !strings.HasPrefix(authURL, "https://auth.openai.com/oauth/authorize?") {
		t.Fatalf("unexpected authorize URL: %s", authURL)
	}
	parsed, err := urlParse(authURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") != CodexOAuthClientID {
		t.Fatalf("expected client_id=%s, got %q", CodexOAuthClientID, query.Get("client_id"))
	}
	if query.Get("redirect_uri") != CodexOAuthRedirectURI {
		t.Fatalf("expected redirect_uri=%s, got %q", CodexOAuthRedirectURI, query.Get("redirect_uri"))
	}
	if query.Get("scope") != CodexOAuthScope {
		t.Fatalf("expected scope=%s, got %q", CodexOAuthScope, query.Get("scope"))
	}
	if query.Get("response_type") != "code" {
		t.Fatalf("expected response_type=code, got %q", query.Get("response_type"))
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected code_challenge_method=S256, got %q", query.Get("code_challenge_method"))
	}
	if query.Get("state") != "state-1" {
		t.Fatalf("expected state=state-1, got %q", query.Get("state"))
	}
	if query.Get("code_challenge") != "challenge-1" {
		t.Fatalf("expected code_challenge=challenge-1, got %q", query.Get("code_challenge"))
	}
	if query.Get("id_token_add_organizations") != "true" {
		t.Fatalf("expected id_token_add_organizations=true, got %q", query.Get("id_token_add_organizations"))
	}
	if query.Get("codex_cli_simplified_flow") != "true" {
		t.Fatalf("expected codex_cli_simplified_flow=true, got %q", query.Get("codex_cli_simplified_flow"))
	}
	if query.Get("originator") != "pi" {
		t.Fatalf("expected originator=pi, got %q", query.Get("originator"))
	}
}

func TestCodexOAuthParseRedirectURL(t *testing.T) {
	client := NewCodexOAuth()
	code, state, err := client.ParseRedirectURL("http://localhost:1455/auth/callback?code=abc123&state=s1")
	if err != nil {
		t.Fatalf("ParseRedirectURL error: %v", err)
	}
	if code != "abc123" || state != "s1" {
		t.Fatalf("unexpected parsed values code=%q state=%q", code, state)
	}
}

func TestCodexOAuthExchangeAuthorizationCode(t *testing.T) {
	rt := &codexOAuthRT{}
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	client := &CodexOAuth{
		authBaseURL: CodexOAuthAuthBaseURL,
		clientID:    CodexOAuthClientID,
		redirectURI: CodexOAuthRedirectURI,
		scope:       CodexOAuthScope,
		httpClient:  &http.Client{Transport: rt},
		now: func() time.Time {
			return now
		},
	}
	token, err := client.ExchangeAuthorizationCode(context.Background(), "code-1", "verifier-1")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode error: %v", err)
	}
	if token.AccessToken != "at-1" {
		t.Fatalf("expected access token at-1, got %q", token.AccessToken)
	}
	if token.RefreshToken != "rt-1" {
		t.Fatalf("expected refresh token rt-1, got %q", token.RefreshToken)
	}
	expectedExpiry := now.Add(3600 * time.Second)
	if !token.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("expected expires_at %s, got %s", expectedExpiry.Format(time.RFC3339), token.ExpiresAt.Format(time.RFC3339))
	}
	if !strings.Contains(rt.lastForm, "grant_type=authorization_code") {
		t.Fatalf("expected authorization_code grant, got form %q", rt.lastForm)
	}
	if !strings.Contains(rt.lastForm, "code_verifier=verifier-1") {
		t.Fatalf("expected code_verifier in request, got form %q", rt.lastForm)
	}
}

func TestCodexOAuthRefreshAccessToken(t *testing.T) {
	rt := &codexOAuthRT{}
	client := &CodexOAuth{
		authBaseURL: CodexOAuthAuthBaseURL,
		clientID:    CodexOAuthClientID,
		redirectURI: CodexOAuthRedirectURI,
		scope:       CodexOAuthScope,
		httpClient:  &http.Client{Transport: rt},
		now:         time.Now,
	}
	_, err := client.RefreshAccessToken(context.Background(), "refresh-1")
	if err != nil {
		t.Fatalf("RefreshAccessToken error: %v", err)
	}
	if !strings.Contains(rt.lastForm, "grant_type=refresh_token") {
		t.Fatalf("expected refresh_token grant, got form %q", rt.lastForm)
	}
	if !strings.Contains(rt.lastForm, "refresh_token=refresh-1") {
		t.Fatalf("expected refresh_token in request, got form %q", rt.lastForm)
	}
}

func TestExtractCodexChatGPTAccountID(t *testing.T) {
	payload := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_abc123",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	token := strings.Join([]string{
		"header",
		base64.RawURLEncoding.EncodeToString(body),
		"signature",
	}, ".")
	if got := ExtractCodexChatGPTAccountID(token); got != "acct_abc123" {
		t.Fatalf("expected acct_abc123, got %q", got)
	}
}

func TestExtractCodexChatGPTAccountIDFromFlatClaim(t *testing.T) {
	payload := map[string]any{
		"https://api.openai.com/auth.chatgpt_account_id": "acct_flat123",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	token := strings.Join([]string{
		"header",
		base64.RawURLEncoding.EncodeToString(body),
		"signature",
	}, ".")
	if got := ExtractCodexChatGPTAccountID(token); got != "acct_flat123" {
		t.Fatalf("expected acct_flat123, got %q", got)
	}
}

func TestExtractCodexChatGPTAccountIDFromTopLevelClaim(t *testing.T) {
	payload := map[string]any{
		"chatgpt_account_id": "acct_top123",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	token := strings.Join([]string{
		"header",
		base64.RawURLEncoding.EncodeToString(body),
		"signature",
	}, ".")
	if got := ExtractCodexChatGPTAccountID(token); got != "acct_top123" {
		t.Fatalf("expected acct_top123, got %q", got)
	}
}

func urlParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
