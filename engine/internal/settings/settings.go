package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const schemaVersion = 1

const (
	providerOpenAI      = "openai"
	providerOpenAICodex = "openai-codex"
	providerAnthropic   = "anthropic"
	providerGoogle      = "google"
	providerMistral     = "mistral"

	reasoningEffortNone   = "none"
	reasoningEffortLow    = "low"
	reasoningEffortMedium = "medium"
	reasoningEffortHigh   = "high"
	reasoningEffortXHigh  = "xhigh"
	reasoningEffortMax    = "max"
)

const (
	defaultUserModelID            = "openai:gpt-5.2"
	anthropicLegacyOpus45ModelID  = "anthropic:claude-opus-4.5"
	anthropicDefaultSonnet46Model = "anthropic:claude-sonnet-4-6"
)

type ProviderSettings struct {
	Enabled                     bool   `json:"enabled"`
	RPIResearchReasoningEffort  string `json:"rpi_research_reasoning_effort,omitempty"`
	RPIPlanReasoningEffort      string `json:"rpi_plan_reasoning_effort,omitempty"`
	RPIImplementReasoningEffort string `json:"rpi_implement_reasoning_effort,omitempty"`
}

type Settings struct {
	SchemaVersion      int                         `json:"schema_version"`
	Providers          map[string]ProviderSettings `json:"providers"`
	UserDefaultModelID string                      `json:"user_default_model_id,omitempty"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (*Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultSettings(), nil
		}
		return nil, err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	backfillSettings(&settings)
	return &settings, nil
}

func (s *Store) Save(settings *Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	backfillSettings(settings)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func defaultSettings() *Settings {
	return &Settings{
		SchemaVersion: schemaVersion,
		Providers: map[string]ProviderSettings{
			providerOpenAI:      defaultProviderSettings(providerOpenAI),
			providerOpenAICodex: defaultProviderSettings(providerOpenAICodex),
			providerAnthropic:   defaultProviderSettings(providerAnthropic),
			providerGoogle:      defaultProviderSettings(providerGoogle),
			providerMistral:     defaultProviderSettings(providerMistral),
		},
		UserDefaultModelID: defaultUserModelID,
	}
}

func (s *Store) Update(fn func(*Settings)) (*Settings, error) {
	settings, err := s.Load()
	if err != nil {
		return nil, err
	}
	fn(settings)
	return settings, s.Save(settings)
}

func backfillSettings(settings *Settings) {
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = schemaVersion
	}
	if settings.Providers == nil {
		settings.Providers = map[string]ProviderSettings{}
	}
	backfillProvider(settings.Providers, providerOpenAI)
	backfillProvider(settings.Providers, providerOpenAICodex)
	backfillProvider(settings.Providers, providerAnthropic)
	backfillProvider(settings.Providers, providerGoogle)
	backfillProvider(settings.Providers, providerMistral)
	if settings.UserDefaultModelID == "" {
		settings.UserDefaultModelID = defaultUserModelID
		return
	}
	switch strings.TrimSpace(settings.UserDefaultModelID) {
	case anthropicLegacyOpus45ModelID:
		settings.UserDefaultModelID = anthropicDefaultSonnet46Model
	default:
		settings.UserDefaultModelID = strings.TrimSpace(settings.UserDefaultModelID)
	}
}

func backfillProvider(providers map[string]ProviderSettings, providerID string) {
	entry, ok := providers[providerID]
	if !ok {
		providers[providerID] = defaultProviderSettings(providerID)
		return
	}
	providers[providerID] = backfillProviderSettings(providerID, entry)
}

func defaultProviderSettings(providerID string) ProviderSettings {
	settings := ProviderSettings{Enabled: true}
	if supportsRPIReasoningEffort(providerID) {
		settings.RPIResearchReasoningEffort = reasoningEffortMedium
		settings.RPIPlanReasoningEffort = reasoningEffortMedium
		settings.RPIImplementReasoningEffort = reasoningEffortMedium
	}
	return settings
}

func backfillProviderSettings(providerID string, entry ProviderSettings) ProviderSettings {
	if !supportsRPIReasoningEffort(providerID) {
		return entry
	}
	entry.RPIResearchReasoningEffort = normalizeProviderReasoningEffort(providerID, entry.RPIResearchReasoningEffort)
	entry.RPIPlanReasoningEffort = normalizeProviderReasoningEffort(providerID, entry.RPIPlanReasoningEffort)
	entry.RPIImplementReasoningEffort = normalizeProviderReasoningEffort(providerID, entry.RPIImplementReasoningEffort)
	return entry
}

func supportsRPIReasoningEffort(providerID string) bool {
	return providerID == providerOpenAI || providerID == providerOpenAICodex || providerID == providerAnthropic
}

func normalizeProviderReasoningEffort(providerID, value string) string {
	effort := strings.ToLower(strings.TrimSpace(value))
	switch providerID {
	case providerOpenAI:
		switch effort {
		case reasoningEffortNone, reasoningEffortLow, reasoningEffortMedium, reasoningEffortHigh:
			return effort
		}
	case providerOpenAICodex:
		switch effort {
		case reasoningEffortLow, reasoningEffortMedium, reasoningEffortHigh, reasoningEffortXHigh:
			return effort
		}
	case providerAnthropic:
		switch effort {
		case reasoningEffortLow, reasoningEffortMedium, reasoningEffortHigh, reasoningEffortMax:
			return effort
		}
	}
	return reasoningEffortMedium
}
