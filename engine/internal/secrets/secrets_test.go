package secrets

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSecretsRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "secrets.enc"), filepath.Join(root, "master.key"))
	if err := store.SetOpenAIKey("sk-test"); err != nil {
		t.Fatalf("set key: %v", err)
	}
	key, err := store.GetOpenAIKey()
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key != "sk-test" {
		t.Fatalf("expected key roundtrip")
	}
}

func TestOpenAICodexOAuthRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "secrets.enc"), filepath.Join(root, "master.key"))

	expiresAt := time.Date(2026, 2, 17, 15, 4, 5, 0, time.UTC)
	input := &OpenAICodexOAuthCredentials{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		IDToken:      "id-1",
		AccountLabel: "acct_123",
		ExpiresAt:    expiresAt,
	}
	if err := store.SetOpenAICodexOAuthCredentials(input); err != nil {
		t.Fatalf("set oauth credentials: %v", err)
	}

	got, err := store.GetOpenAICodexOAuthCredentials()
	if err != nil {
		t.Fatalf("get oauth credentials: %v", err)
	}
	if got == nil {
		t.Fatalf("expected oauth credentials")
	}
	if got.AccessToken != input.AccessToken {
		t.Fatalf("expected access token %q, got %q", input.AccessToken, got.AccessToken)
	}
	if got.RefreshToken != input.RefreshToken {
		t.Fatalf("expected refresh token %q, got %q", input.RefreshToken, got.RefreshToken)
	}
	if got.AccountLabel != input.AccountLabel {
		t.Fatalf("expected account label %q, got %q", input.AccountLabel, got.AccountLabel)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %s, got %s", expiresAt.Format(time.RFC3339), got.ExpiresAt.Format(time.RFC3339))
	}
}

func TestClearProviderKeyOpenAICodex(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "secrets.enc"), filepath.Join(root, "master.key"))

	if err := store.SetOpenAICodexOAuthCredentials(&OpenAICodexOAuthCredentials{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
	}); err != nil {
		t.Fatalf("set oauth credentials: %v", err)
	}
	if err := store.ClearProviderKey("openai-codex"); err != nil {
		t.Fatalf("clear provider key: %v", err)
	}
	got, err := store.GetOpenAICodexOAuthCredentials()
	if err != nil {
		t.Fatalf("get oauth credentials: %v", err)
	}
	if got != nil {
		t.Fatalf("expected oauth credentials to be cleared, got %#v", got)
	}
}
