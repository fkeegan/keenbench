package engine

import "strings"

const (
	ProviderOpenAI      = "openai"
	ProviderOpenAICodex = "openai-codex"
	ProviderAnthropic   = "anthropic"
	ProviderGoogle      = "google"
	ProviderMistral     = "mistral"
)

const (
	ModelOpenAIID      = "openai:gpt-5.2"
	ModelOpenAICodexID = "openai-codex:gpt-5.3-codex"
	ModelAnthropicID   = "anthropic:claude-opus-4.5"
	ModelGoogleID      = "google:gemini-3-pro"
	ModelMistralID     = "mistral:mistral-large"
)

type ModelInfo struct {
	ModelID           string `json:"model_id"`
	ProviderID        string `json:"provider_id"`
	DisplayName       string `json:"display_name"`
	ContextTokens     int    `json:"context_tokens_estimate"`
	SupportsFileRead  bool   `json:"supports_file_read"`
	SupportsFileWrite bool   `json:"supports_file_write"`
	CanBeSecondary    bool   `json:"can_be_secondary"`
	RequiresKey       bool   `json:"requires_key"`
}

var modelRegistry = map[string]ModelInfo{
	ModelOpenAIID: {
		ModelID:           ModelOpenAIID,
		ProviderID:        ProviderOpenAI,
		DisplayName:       "OpenAI GPT-5.2",
		ContextTokens:     200000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
	ModelOpenAICodexID: {
		ModelID:           ModelOpenAICodexID,
		ProviderID:        ProviderOpenAICodex,
		DisplayName:       "OpenAI Codex GPT-5.3",
		ContextTokens:     200000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
	ModelAnthropicID: {
		ModelID:           ModelAnthropicID,
		ProviderID:        ProviderAnthropic,
		DisplayName:       "Anthropic Claude Opus 4.5",
		ContextTokens:     200000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
	ModelGoogleID: {
		ModelID:           ModelGoogleID,
		ProviderID:        ProviderGoogle,
		DisplayName:       "Google Gemini 3 Pro",
		ContextTokens:     1000000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    false,
		RequiresKey:       true,
	},
	ModelMistralID: {
		ModelID:           ModelMistralID,
		ProviderID:        ProviderMistral,
		DisplayName:       "Mistral Large",
		ContextTokens:     128000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
}

func listSupportedModels() []ModelInfo {
	return []ModelInfo{
		modelRegistry[ModelOpenAIID],
		modelRegistry[ModelOpenAICodexID],
		modelRegistry[ModelAnthropicID],
		modelRegistry[ModelGoogleID],
		modelRegistry[ModelMistralID],
	}
}

func getModel(modelID string) (ModelInfo, bool) {
	model, ok := modelRegistry[modelID]
	return model, ok
}

func providerModelName(modelID string) string {
	if modelID == ModelMistralID {
		return "mistral-large-latest"
	}
	parts := strings.SplitN(modelID, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return modelID
}
