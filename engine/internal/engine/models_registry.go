package engine

import "strings"

const (
	ProviderOpenAI          = "openai"
	ProviderOpenAICodex     = "openai-codex"
	ProviderAnthropic       = "anthropic"
	ProviderAnthropicClaude = "anthropic-claude"
	ProviderGoogle          = "google"
	ProviderMistral         = "mistral"
)

const (
	ModelOpenAIID                  = "openai:gpt-5.2"
	ModelOpenAICodexID             = "openai-codex:gpt-5.3-codex"
	ModelAnthropicSonnet46ID       = "anthropic:claude-sonnet-4-6"
	ModelAnthropicOpus46ID         = "anthropic:claude-opus-4-6"
	ModelAnthropicClaudeSonnet46ID = "anthropic-claude:claude-sonnet-4-6"
	ModelAnthropicClaudeOpus46ID   = "anthropic-claude:claude-opus-4-6"
	// ModelAnthropicID is kept as a compatibility alias for older internal references.
	ModelAnthropicID = ModelAnthropicSonnet46ID
	ModelGoogleID    = "google:gemini-3-pro"
	ModelMistralID   = "mistral:mistral-large"
)

const modelAnthropicLegacyOpus45ID = "anthropic:claude-opus-4.5"

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
	ModelAnthropicSonnet46ID: {
		ModelID:           ModelAnthropicSonnet46ID,
		ProviderID:        ProviderAnthropic,
		DisplayName:       "Anthropic Claude Sonnet 4.6",
		ContextTokens:     200000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
	ModelAnthropicOpus46ID: {
		ModelID:           ModelAnthropicOpus46ID,
		ProviderID:        ProviderAnthropic,
		DisplayName:       "Anthropic Claude Opus 4.6",
		ContextTokens:     200000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
	ModelAnthropicClaudeSonnet46ID: {
		ModelID:           ModelAnthropicClaudeSonnet46ID,
		ProviderID:        ProviderAnthropicClaude,
		DisplayName:       "Anthropic Claude Sonnet 4.6 (Setup Token)",
		ContextTokens:     200000,
		SupportsFileRead:  true,
		SupportsFileWrite: true,
		CanBeSecondary:    true,
		RequiresKey:       true,
	},
	ModelAnthropicClaudeOpus46ID: {
		ModelID:           ModelAnthropicClaudeOpus46ID,
		ProviderID:        ProviderAnthropicClaude,
		DisplayName:       "Anthropic Claude Opus 4.6 (Setup Token)",
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
		modelRegistry[ModelAnthropicSonnet46ID],
		modelRegistry[ModelAnthropicOpus46ID],
		modelRegistry[ModelAnthropicClaudeSonnet46ID],
		modelRegistry[ModelAnthropicClaudeOpus46ID],
		modelRegistry[ModelGoogleID],
		modelRegistry[ModelMistralID],
	}
}

func getModel(modelID string) (ModelInfo, bool) {
	model, ok := modelRegistry[canonicalModelID(modelID)]
	return model, ok
}

func canonicalModelID(modelID string) string {
	switch strings.TrimSpace(modelID) {
	case modelAnthropicLegacyOpus45ID:
		return ModelAnthropicSonnet46ID
	default:
		return strings.TrimSpace(modelID)
	}
}

func providerModelName(modelID string) string {
	modelID = canonicalModelID(modelID)
	if modelID == ModelMistralID {
		return "mistral-large-latest"
	}
	parts := strings.SplitN(modelID, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return modelID
}
