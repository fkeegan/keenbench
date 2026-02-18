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
