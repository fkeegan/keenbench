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
			IsFree:            isOpenRouterFreeModelID(modelID),
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
	models := make([]ModelInfo, 0, len(e.openrouterModels))
	for _, model := range e.openrouterModels {
		models = append(models, model)
	}
	e.openrouterMu.RUnlock()
	sort.SliceStable(models, func(i, j int) bool {
		return compareOpenRouterModels(models[i], models[j]) < 0
	})
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ModelID)
	}
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

func compareOpenRouterModels(left, right ModelInfo) int {
	if left.IsFree != right.IsFree {
		if left.IsFree {
			return -1
		}
		return 1
	}
	leftName := strings.ToLower(strings.TrimSpace(left.DisplayName))
	rightName := strings.ToLower(strings.TrimSpace(right.DisplayName))
	if leftName < rightName {
		return -1
	}
	if leftName > rightName {
		return 1
	}
	leftID := strings.ToLower(strings.TrimSpace(left.ModelID))
	rightID := strings.ToLower(strings.TrimSpace(right.ModelID))
	if leftID < rightID {
		return -1
	}
	if leftID > rightID {
		return 1
	}
	return 0
}

func isOpenRouterFreeModelID(modelID string) bool {
	modelID = canonicalModelID(modelID)
	return modelID == ModelOpenRouterFreeID || strings.HasSuffix(modelID, ":free")
}
