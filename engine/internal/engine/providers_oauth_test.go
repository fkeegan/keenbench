package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"keenbench/engine/internal/llm"
	"keenbench/engine/internal/openai"
	"keenbench/engine/internal/secrets"
)

type fakeCodexOAuth struct {
	authorizeURL string
	parseCode    string
	parseState   string
	parseErr     error
	exchangeTok  openai.CodexOAuthToken
	exchangeErr  error
	refreshTok   openai.CodexOAuthToken
	refreshErr   error
	accountID    string
}

func (f *fakeCodexOAuth) BuildAuthorizeURL(state, codeChallenge string) (string, error) {
	if f.authorizeURL != "" {
		return f.authorizeURL, nil
	}
	return "https://auth.openai.com/oauth/authorize?state=" + state + "&code_challenge=" + codeChallenge, nil
}

func (f *fakeCodexOAuth) ParseRedirectURL(_ string) (string, string, error) {
	if f.parseErr != nil {
		return "", "", f.parseErr
	}
	return f.parseCode, f.parseState, nil
}

func (f *fakeCodexOAuth) ExchangeAuthorizationCode(_ context.Context, _, _ string) (openai.CodexOAuthToken, error) {
	if f.exchangeErr != nil {
		return openai.CodexOAuthToken{}, f.exchangeErr
	}
	return f.exchangeTok, nil
}

func (f *fakeCodexOAuth) RefreshAccessToken(_ context.Context, _ string) (openai.CodexOAuthToken, error) {
	if f.refreshErr != nil {
		return openai.CodexOAuthToken{}, f.refreshErr
	}
	return f.refreshTok, nil
}

func (f *fakeCodexOAuth) ExtractChatGPTAccountID(_ string) string {
	return f.accountID
}

func TestProvidersOAuthFlowRoundTrip(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	eng.oauthCallbackServer = &http.Server{}
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	eng.now = func() time.Time { return now }
	eng.newOAuthFlowID = func() (string, error) { return "flow-1", nil }
	eng.newCodexPKCE = func() (openai.CodexPKCEValues, error) {
		return openai.CodexPKCEValues{
			State:         "state-1",
			CodeVerifier:  "verifier-1",
			CodeChallenge: "challenge-1",
		}, nil
	}
	eng.codexOAuth = &fakeCodexOAuth{
		authorizeURL: "https://auth.openai.com/oauth/authorize?state=state-1",
		parseCode:    "code-1",
		parseState:   "state-1",
		exchangeTok: openai.CodexOAuthToken{
			AccessToken:  "access-1",
			RefreshToken: "refresh-1",
			IDToken:      "id-1",
			ExpiresAt:    now.Add(1 * time.Hour),
		},
		accountID: "acct_123",
	}

	startRespRaw, errInfo := eng.ProvidersOAuthStart(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
	}))
	if errInfo != nil {
		t.Fatalf("oauth start: %v", errInfo)
	}
	startResp := startRespRaw.(map[string]any)
	if got := startResp["flow_id"].(string); got != "flow-1" {
		t.Fatalf("expected flow-1, got %q", got)
	}

	statusRespRaw, errInfo := eng.ProvidersOAuthStatus(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
		"flow_id":     "flow-1",
	}))
	if errInfo != nil {
		t.Fatalf("oauth status: %v", errInfo)
	}
	statusResp := statusRespRaw.(map[string]any)
	if got := statusResp["status"].(string); got != oauthFlowStatusPending {
		t.Fatalf("expected pending status, got %q", got)
	}

	completeRespRaw, errInfo := eng.ProvidersOAuthComplete(ctx, mustJSON(t, map[string]any{
		"provider_id":  ProviderOpenAICodex,
		"flow_id":      "flow-1",
		"redirect_url": "http://localhost:1455/auth/callback?code=code-1&state=state-1",
	}))
	if errInfo != nil {
		t.Fatalf("oauth complete: %v", errInfo)
	}
	completeResp := completeRespRaw.(map[string]any)
	if connected, _ := completeResp["oauth_connected"].(bool); !connected {
		t.Fatalf("expected oauth_connected=true")
	}
	if label, _ := completeResp["oauth_account_label"].(string); label != "acct_123" {
		t.Fatalf("expected account label acct_123, got %q", label)
	}

	creds, err := eng.secrets.GetOpenAICodexOAuthCredentials()
	if err != nil {
		t.Fatalf("read oauth credentials: %v", err)
	}
	if creds == nil || creds.AccessToken != "access-1" || creds.RefreshToken != "refresh-1" {
		t.Fatalf("unexpected stored credentials: %#v", creds)
	}

	providersRaw, errInfo := eng.ProvidersGetStatus(ctx, nil)
	if errInfo != nil {
		t.Fatalf("providers status: %v", errInfo)
	}
	providers := providersRaw.(map[string]any)["providers"].([]map[string]any)
	found := false
	for _, provider := range providers {
		if provider["provider_id"] != ProviderOpenAICodex {
			continue
		}
		found = true
		if mode, _ := provider["auth_mode"].(string); mode != "oauth" {
			t.Fatalf("expected auth_mode=oauth, got %q", mode)
		}
		if configured, _ := provider["configured"].(bool); !configured {
			t.Fatalf("expected openai-codex configured after oauth complete")
		}
	}
	if !found {
		t.Fatalf("expected openai-codex provider in status")
	}

	if _, errInfo := eng.ProvidersOAuthDisconnect(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
	})); errInfo != nil {
		t.Fatalf("oauth disconnect: %v", errInfo)
	}
	creds, err = eng.secrets.GetOpenAICodexOAuthCredentials()
	if err != nil {
		t.Fatalf("read oauth credentials after disconnect: %v", err)
	}
	if creds != nil {
		t.Fatalf("expected credentials cleared, got %#v", creds)
	}
}

func TestProvidersOAuthCompleteStateMismatch(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	eng.oauthCallbackServer = &http.Server{}
	eng.newOAuthFlowID = func() (string, error) { return "flow-1", nil }
	eng.newCodexPKCE = func() (openai.CodexPKCEValues, error) {
		return openai.CodexPKCEValues{
			State:         "state-1",
			CodeVerifier:  "verifier-1",
			CodeChallenge: "challenge-1",
		}, nil
	}
	eng.codexOAuth = &fakeCodexOAuth{
		parseCode:  "code-1",
		parseState: "wrong-state",
	}

	if _, errInfo := eng.ProvidersOAuthStart(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
	})); errInfo != nil {
		t.Fatalf("oauth start: %v", errInfo)
	}

	if _, errInfo := eng.ProvidersOAuthComplete(ctx, mustJSON(t, map[string]any{
		"provider_id":  ProviderOpenAICodex,
		"flow_id":      "flow-1",
		"redirect_url": "http://localhost:1455/auth/callback?code=code-1&state=wrong-state",
	})); errInfo == nil || errInfo.ErrorCode != "VALIDATION_FAILED" {
		t.Fatalf("expected validation failed on oauth state mismatch")
	}
}

func TestProvidersOAuthCompleteWithoutRedirectUsesCapturedCode(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	eng.oauthCallbackServer = &http.Server{}
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	eng.now = func() time.Time { return now }
	eng.newOAuthFlowID = func() (string, error) { return "flow-1", nil }
	eng.newCodexPKCE = func() (openai.CodexPKCEValues, error) {
		return openai.CodexPKCEValues{
			State:         "state-1",
			CodeVerifier:  "verifier-1",
			CodeChallenge: "challenge-1",
		}, nil
	}
	eng.codexOAuth = &fakeCodexOAuth{
		exchangeTok: openai.CodexOAuthToken{
			AccessToken:  "access-1",
			RefreshToken: "refresh-1",
			IDToken:      "id-1",
			ExpiresAt:    now.Add(1 * time.Hour),
		},
		accountID: "acct_123",
	}

	if _, errInfo := eng.ProvidersOAuthStart(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
	})); errInfo != nil {
		t.Fatalf("oauth start: %v", errInfo)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=code-1&state=state-1", nil)
	rec := httptest.NewRecorder()
	eng.handleOAuthCallback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected callback status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	completeRespRaw, errInfo := eng.ProvidersOAuthComplete(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
		"flow_id":     "flow-1",
	}))
	if errInfo != nil {
		t.Fatalf("oauth complete: %v", errInfo)
	}
	completeResp := completeRespRaw.(map[string]any)
	if connected, _ := completeResp["oauth_connected"].(bool); !connected {
		t.Fatalf("expected oauth_connected=true")
	}
}

func TestProvidersOAuthCompleteWithoutRedirectRequiresCapturedCode(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	eng.oauthCallbackServer = &http.Server{}
	eng.newOAuthFlowID = func() (string, error) { return "flow-1", nil }
	eng.newCodexPKCE = func() (openai.CodexPKCEValues, error) {
		return openai.CodexPKCEValues{
			State:         "state-1",
			CodeVerifier:  "verifier-1",
			CodeChallenge: "challenge-1",
		}, nil
	}
	eng.codexOAuth = &fakeCodexOAuth{}

	if _, errInfo := eng.ProvidersOAuthStart(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
	})); errInfo != nil {
		t.Fatalf("oauth start: %v", errInfo)
	}

	if _, errInfo := eng.ProvidersOAuthComplete(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
		"flow_id":     "flow-1",
	})); errInfo == nil || errInfo.ErrorCode != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED, got %#v", errInfo)
	}
}

func TestOpenAICodexCredentialRefreshOnProviderKeyRead(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	eng.now = func() time.Time { return now }
	eng.codexOAuth = &fakeCodexOAuth{
		refreshTok: openai.CodexOAuthToken{
			AccessToken:  "refreshed-access",
			RefreshToken: "refreshed-refresh",
			IDToken:      "new-id-token",
			ExpiresAt:    now.Add(2 * time.Hour),
		},
		accountID: "acct_refresh",
	}

	if err := eng.secrets.SetOpenAICodexOAuthCredentials(&secrets.OpenAICodexOAuthCredentials{
		AccessToken:  "",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed oauth credentials: %v", err)
	}

	token, errInfo := eng.providerKey(ctx, ProviderOpenAICodex)
	if errInfo != nil {
		t.Fatalf("provider key: %v", errInfo)
	}
	if token != "refreshed-access" {
		t.Fatalf("expected refreshed access token, got %q", token)
	}

	updated, err := eng.secrets.GetOpenAICodexOAuthCredentials()
	if err != nil {
		t.Fatalf("read refreshed credentials: %v", err)
	}
	if updated == nil || updated.RefreshToken != "refreshed-refresh" || updated.AccountLabel != "acct_refresh" {
		t.Fatalf("unexpected refreshed credentials: %#v", updated)
	}
}

func TestProvidersOAuthCompleteUnauthorizedMapsToProviderAuthFailed(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	eng.oauthCallbackServer = &http.Server{}
	eng.newOAuthFlowID = func() (string, error) { return "flow-1", nil }
	eng.newCodexPKCE = func() (openai.CodexPKCEValues, error) {
		return openai.CodexPKCEValues{
			State:         "state-1",
			CodeVerifier:  "verifier-1",
			CodeChallenge: "challenge-1",
		}, nil
	}
	eng.codexOAuth = &fakeCodexOAuth{
		parseCode:   "code-1",
		parseState:  "state-1",
		exchangeErr: llm.ErrUnauthorized,
	}

	if _, errInfo := eng.ProvidersOAuthStart(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderOpenAICodex,
	})); errInfo != nil {
		t.Fatalf("oauth start: %v", errInfo)
	}

	if _, errInfo := eng.ProvidersOAuthComplete(ctx, mustJSON(t, map[string]any{
		"provider_id":  ProviderOpenAICodex,
		"flow_id":      "flow-1",
		"redirect_url": "http://localhost:1455/auth/callback?code=code-1&state=state-1",
	})); errInfo == nil || errInfo.ErrorCode != "PROVIDER_AUTH_FAILED" {
		t.Fatalf("expected PROVIDER_AUTH_FAILED, got %#v", errInfo)
	} else if errInfo.ProviderID != ProviderOpenAICodex {
		t.Fatalf("expected provider_id=%q, got %#v", ProviderOpenAICodex, errInfo)
	}
}

func TestProvidersGetStatusOpenAICodexExpiredFlag(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	now := time.Date(2026, 2, 17, 11, 0, 0, 0, time.UTC)
	eng.now = func() time.Time { return now }
	if err := eng.secrets.SetOpenAICodexOAuthCredentials(&secrets.OpenAICodexOAuthCredentials{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		AccountLabel: "acct_1",
		ExpiresAt:    now.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("seed oauth credentials: %v", err)
	}

	respRaw, errInfo := eng.ProvidersGetStatus(ctx, nil)
	if errInfo != nil {
		t.Fatalf("providers status: %v", errInfo)
	}
	providers := respRaw.(map[string]any)["providers"].([]map[string]any)
	for _, provider := range providers {
		if provider["provider_id"] != ProviderOpenAICodex {
			continue
		}
		if expired, _ := provider["oauth_expired"].(bool); !expired {
			t.Fatalf("expected oauth_expired=true")
		}
		return
	}
	t.Fatalf("openai-codex not found in providers status")
}

func TestOpenAICodexCredentialRefreshErrorMapping(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	eng.now = func() time.Time { return now }
	eng.codexOAuth = &fakeCodexOAuth{refreshErr: llm.ErrUnavailable}

	if err := eng.secrets.SetOpenAICodexOAuthCredentials(&secrets.OpenAICodexOAuthCredentials{
		AccessToken:  "",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed oauth credentials: %v", err)
	}

	if _, errInfo := eng.providerKey(ctx, ProviderOpenAICodex); errInfo == nil || errInfo.ErrorCode != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("expected PROVIDER_UNAVAILABLE, got %#v", errInfo)
	} else if errInfo.ProviderID != ProviderOpenAICodex {
		t.Fatalf("expected provider_id=%q, got %#v", ProviderOpenAICodex, errInfo)
	}
}

func TestEnsureProviderReadyForOpenAICodexMissingCredentialsIncludesProviderID(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if errInfo := eng.ensureProviderReadyFor(ctx, ProviderOpenAICodex); errInfo == nil || errInfo.ErrorCode != "PROVIDER_NOT_CONFIGURED" {
		t.Fatalf("expected PROVIDER_NOT_CONFIGURED, got %#v", errInfo)
	} else if errInfo.ProviderID != ProviderOpenAICodex {
		t.Fatalf("expected provider_id=%q, got %#v", ProviderOpenAICodex, errInfo)
	}
}
