package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "settings.json"))
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	openAI := settings.Providers[providerOpenAI]
	if openAI.Enabled != true {
		t.Fatalf("expected openai enabled by default")
	}
	if openAI.RPIResearchReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai research reasoning effort to default to medium, got %q", openAI.RPIResearchReasoningEffort)
	}
	if openAI.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai plan reasoning effort to default to medium, got %q", openAI.RPIPlanReasoningEffort)
	}
	if openAI.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai implement reasoning effort to default to medium, got %q", openAI.RPIImplementReasoningEffort)
	}

	codex := settings.Providers[providerOpenAICodex]
	if codex.Enabled != true {
		t.Fatalf("expected openai-codex enabled by default")
	}
	if codex.RPIResearchReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex research reasoning effort to default to medium, got %q", codex.RPIResearchReasoningEffort)
	}
	if codex.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex plan reasoning effort to default to medium, got %q", codex.RPIPlanReasoningEffort)
	}
	if codex.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex implement reasoning effort to default to medium, got %q", codex.RPIImplementReasoningEffort)
	}

	anthropic := settings.Providers[providerAnthropic]
	if anthropic.Enabled != true {
		t.Fatalf("expected anthropic enabled by default")
	}
	if anthropic.RPIResearchReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected anthropic research reasoning effort to default to medium, got %q", anthropic.RPIResearchReasoningEffort)
	}
	if anthropic.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected anthropic plan reasoning effort to default to medium, got %q", anthropic.RPIPlanReasoningEffort)
	}
	if anthropic.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected anthropic implement reasoning effort to default to medium, got %q", anthropic.RPIImplementReasoningEffort)
	}

	mistral := settings.Providers[providerMistral]
	if mistral.Enabled != true {
		t.Fatalf("expected mistral enabled by default")
	}
	if mistral.RPIResearchReasoningEffort != "" {
		t.Fatalf("expected mistral research reasoning effort to be empty, got %q", mistral.RPIResearchReasoningEffort)
	}
	if mistral.RPIPlanReasoningEffort != "" {
		t.Fatalf("expected mistral plan reasoning effort to be empty, got %q", mistral.RPIPlanReasoningEffort)
	}
	if mistral.RPIImplementReasoningEffort != "" {
		t.Fatalf("expected mistral implement reasoning effort to be empty, got %q", mistral.RPIImplementReasoningEffort)
	}
	if settings.UserConsentMode != UserConsentModeAsk {
		t.Fatalf("expected user consent mode to default to %q, got %q", UserConsentModeAsk, settings.UserConsentMode)
	}

	settings.Providers[providerOpenAI] = ProviderSettings{
		Enabled:                     false,
		RPIResearchReasoningEffort:  reasoningEffortLow,
		RPIPlanReasoningEffort:      "HIGH",
		RPIImplementReasoningEffort: reasoningEffortXHigh,
	}
	settings.Providers[providerOpenAICodex] = ProviderSettings{
		Enabled:                     true,
		RPIResearchReasoningEffort:  reasoningEffortNone,
		RPIPlanReasoningEffort:      reasoningEffortMedium,
		RPIImplementReasoningEffort: "not-valid",
	}
	if err := store.Save(settings); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	openAI = loaded.Providers[providerOpenAI]
	if openAI.Enabled != false {
		t.Fatalf("expected openai disabled")
	}
	if openAI.RPIResearchReasoningEffort != reasoningEffortLow {
		t.Fatalf("expected openai research reasoning effort to be %q, got %q", reasoningEffortLow, openAI.RPIResearchReasoningEffort)
	}
	if openAI.RPIPlanReasoningEffort != reasoningEffortHigh {
		t.Fatalf("expected openai plan reasoning effort to be %q, got %q", reasoningEffortHigh, openAI.RPIPlanReasoningEffort)
	}
	if openAI.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai implement reasoning effort to fall back to %q, got %q", reasoningEffortMedium, openAI.RPIImplementReasoningEffort)
	}

	codex = loaded.Providers[providerOpenAICodex]
	if codex.RPIResearchReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex research reasoning effort to fall back to %q, got %q", reasoningEffortMedium, codex.RPIResearchReasoningEffort)
	}
	if codex.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex plan reasoning effort to be %q, got %q", reasoningEffortMedium, codex.RPIPlanReasoningEffort)
	}
	if codex.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex implement reasoning effort to fall back to %q, got %q", reasoningEffortMedium, codex.RPIImplementReasoningEffort)
	}
	if loaded.UserConsentMode != UserConsentModeAsk {
		t.Fatalf("expected user consent mode to normalize to %q, got %q", UserConsentModeAsk, loaded.UserConsentMode)
	}
}

func TestLoadBackfillsOpenAICodexProviderAndRPIReasoningEffort(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	legacy := `{
  "schema_version": 1,
  "providers": {
    "openai": {
      "enabled": true,
      "rpi_research_reasoning_effort": "LOW",
      "rpi_implement_reasoning_effort": "invalid"
    },
    "anthropic": {"enabled": true},
    "google": {"enabled": true}
  },
  "user_default_model_id": "openai:gpt-5.2"
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	store := NewStore(path)
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	entry, ok := settings.Providers[providerOpenAICodex]
	if !ok {
		t.Fatalf("expected openai-codex provider to be backfilled")
	}
	if !entry.Enabled {
		t.Fatalf("expected openai-codex provider to default to enabled")
	}
	if entry.RPIResearchReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex research reasoning effort to default to %q, got %q", reasoningEffortMedium, entry.RPIResearchReasoningEffort)
	}
	if entry.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex plan reasoning effort to default to %q, got %q", reasoningEffortMedium, entry.RPIPlanReasoningEffort)
	}
	if entry.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai-codex implement reasoning effort to default to %q, got %q", reasoningEffortMedium, entry.RPIImplementReasoningEffort)
	}

	anthropic, ok := settings.Providers[providerAnthropic]
	if !ok {
		t.Fatalf("expected anthropic provider to be backfilled")
	}
	if !anthropic.Enabled {
		t.Fatalf("expected anthropic provider to default to enabled")
	}
	if anthropic.RPIResearchReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected anthropic research reasoning effort to default to %q, got %q", reasoningEffortMedium, anthropic.RPIResearchReasoningEffort)
	}
	if anthropic.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected anthropic plan reasoning effort to default to %q, got %q", reasoningEffortMedium, anthropic.RPIPlanReasoningEffort)
	}
	if anthropic.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected anthropic implement reasoning effort to default to %q, got %q", reasoningEffortMedium, anthropic.RPIImplementReasoningEffort)
	}

	mistral, ok := settings.Providers[providerMistral]
	if !ok {
		t.Fatalf("expected mistral provider to be backfilled")
	}
	if !mistral.Enabled {
		t.Fatalf("expected mistral provider to default to enabled")
	}
	if mistral.RPIResearchReasoningEffort != "" {
		t.Fatalf("expected mistral research reasoning effort to stay empty, got %q", mistral.RPIResearchReasoningEffort)
	}
	if mistral.RPIPlanReasoningEffort != "" {
		t.Fatalf("expected mistral plan reasoning effort to stay empty, got %q", mistral.RPIPlanReasoningEffort)
	}
	if mistral.RPIImplementReasoningEffort != "" {
		t.Fatalf("expected mistral implement reasoning effort to stay empty, got %q", mistral.RPIImplementReasoningEffort)
	}

	openAI := settings.Providers[providerOpenAI]
	if openAI.RPIResearchReasoningEffort != reasoningEffortLow {
		t.Fatalf("expected openai research reasoning effort to normalize to %q, got %q", reasoningEffortLow, openAI.RPIResearchReasoningEffort)
	}
	if openAI.RPIPlanReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai plan reasoning effort to default to %q, got %q", reasoningEffortMedium, openAI.RPIPlanReasoningEffort)
	}
	if openAI.RPIImplementReasoningEffort != reasoningEffortMedium {
		t.Fatalf("expected openai implement reasoning effort to default to %q for invalid legacy value, got %q", reasoningEffortMedium, openAI.RPIImplementReasoningEffort)
	}
}

func TestLoadMigratesLegacyAnthropicUserDefaultModel(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	legacy := `{
  "schema_version": 1,
  "providers": {
    "openai": {"enabled": true},
    "openai-codex": {"enabled": true},
    "anthropic": {"enabled": true},
    "google": {"enabled": true},
    "mistral": {"enabled": true}
  },
  "user_default_model_id": "anthropic:claude-opus-4.5"
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	store := NewStore(path)
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if settings.UserDefaultModelID != anthropicDefaultSonnet46Model {
		t.Fatalf("expected user_default_model_id=%q, got %q", anthropicDefaultSonnet46Model, settings.UserDefaultModelID)
	}
	if settings.UserConsentMode != UserConsentModeAsk {
		t.Fatalf("expected user consent mode to default to %q for legacy settings, got %q", UserConsentModeAsk, settings.UserConsentMode)
	}
}

func TestLoadNormalizesUserConsentMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	legacy := `{
  "schema_version": 1,
  "providers": {
    "openai": {"enabled": true},
    "openai-codex": {"enabled": true},
    "anthropic": {"enabled": true},
    "google": {"enabled": true},
    "mistral": {"enabled": true}
  },
  "user_default_model_id": "openai:gpt-5.2",
  "user_consent_mode": "ALLOW_ALL"
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	store := NewStore(path)
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if settings.UserConsentMode != UserConsentModeAllowAll {
		t.Fatalf("expected user_consent_mode=%q, got %q", UserConsentModeAllowAll, settings.UserConsentMode)
	}

	settings.UserConsentMode = "invalid"
	if err := store.Save(settings); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.UserConsentMode != UserConsentModeAsk {
		t.Fatalf("expected invalid user_consent_mode to normalize to %q, got %q", UserConsentModeAsk, reloaded.UserConsentMode)
	}
}
