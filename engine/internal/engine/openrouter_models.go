package engine

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// loadOpenRouterModelCache reads openrouter_models.json from the data directory
// and populates e.openrouterModels. Called in New() before the engine is shared,
// so no lock is needed.
func (e *Engine) loadOpenRouterModelCache() {
	data, err := os.ReadFile(e.openrouterCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			e.logger.Warn("openrouter.cache_load_failed", "error", err.Error())
		}
		return
	}
	var models map[string]ModelInfo
	if err := json.Unmarshal(data, &models); err != nil {
		e.logger.Warn("openrouter.cache_parse_failed", "error", err.Error())
		return
	}
	e.openrouterModels = models
	e.logger.Debug("openrouter.cache_loaded", "count", len(models))
}

// saveOpenRouterModelCache serializes e.openrouterModels to openrouter_models.json.
// Must be called while holding the write lock (or after it is released, with a copy).
func (e *Engine) saveOpenRouterModelCache() {
	e.openrouterMu.RLock()
	models := e.openrouterModels
	e.openrouterMu.RUnlock()
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		e.logger.Warn("openrouter.cache_marshal_failed", "error", err.Error())
		return
	}
	if err := os.WriteFile(e.openrouterCachePath, data, 0o600); err != nil {
		e.logger.Warn("openrouter.cache_write_failed", "error", err.Error())
	}
}

// fetchAndCacheOpenRouterModels fetches the current model list from OpenRouter,
// converts it to ModelInfo entries, stores them in e.openrouterModels, and
// persists them to disk.
func (e *Engine) fetchAndCacheOpenRouterModels(ctx context.Context) error {
	keyInfo, errInfo := e.providerKey(ctx, ProviderOpenRouter)
	if errInfo != nil {
		return nil // no key configured; skip silently
	}
	if strings.TrimSpace(keyInfo) == "" {
		return nil
	}

	infos, err := e.openrouterClient.FetchModels(ctx, keyInfo)
	if err != nil {
		return err
	}

	models := make(map[string]ModelInfo, len(infos))
	for _, info := range infos {
		displayName := strings.TrimSpace(info.Name)
		if displayName == "" {
			displayName = info.ID
		}
		toolsSupported := false
		for _, p := range info.SupportedParameters {
			if p == "tools" {
				toolsSupported = true
				break
			}
		}
		modelID := "openrouter:" + info.ID
		models[modelID] = ModelInfo{
			ModelID:           modelID,
			ProviderID:        ProviderOpenRouter,
			DisplayName:       displayName,
			ContextTokens:     info.ContextLength,
			SupportsFileRead:  toolsSupported,
			SupportsFileWrite: toolsSupported,
			CanBeSecondary:    true,
			RequiresKey:       true,
		}
	}

	e.openrouterMu.Lock()
	e.openrouterModels = models
	e.openrouterMu.Unlock()

	e.saveOpenRouterModelCache()
	e.logger.Info("openrouter.models_refreshed", "count", len(models))
	return nil
}

// openrouterModelIDs returns a sorted slice of all cached OpenRouter model IDs.
func (e *Engine) openrouterModelIDs() []string {
	e.openrouterMu.RLock()
	ids := make([]string, 0, len(e.openrouterModels))
	for id := range e.openrouterModels {
		ids = append(ids, id)
	}
	e.openrouterMu.RUnlock()
	sort.Strings(ids)
	return ids
}

// MaybeRefreshOpenRouterModels fetches and caches the OpenRouter model list if a
// key is configured. Logs a warning on error but never panics. Safe to call as a
// goroutine.
func (e *Engine) MaybeRefreshOpenRouterModels(ctx context.Context) {
	if err := e.fetchAndCacheOpenRouterModels(ctx); err != nil {
		e.logger.Warn("openrouter.refresh_failed", "error", err.Error())
	}
}
