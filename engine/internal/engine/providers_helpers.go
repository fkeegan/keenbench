package engine

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"time"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
	"keenbench/engine/internal/secrets"
)

const oauthRefreshLeadTime = 60 * time.Second

func withProviderID(info *errinfo.ErrorInfo, providerID string) *errinfo.ErrorInfo {
	if info == nil {
		return nil
	}
	copied := *info
	copied.ProviderID = providerID
	return &copied
}

func (e *Engine) clientForProvider(providerID string) (LLMClient, *errinfo.ErrorInfo) {
	client, ok := e.providers[providerID]
	if !ok {
		return nil, withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported provider"), providerID)
	}
	return client, nil
}

func (e *Engine) providerKey(ctx context.Context, providerID string) (string, *errinfo.ErrorInfo) {
	switch providerID {
	case ProviderOpenAI:
		key, err := e.secrets.GetOpenAIKey()
		if err != nil {
			return "", withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
		return key, nil
	case ProviderOpenAICodex:
		creds, errInfo := e.getOpenAICodexCredentials(ctx)
		if errInfo != nil {
			return "", withProviderID(errInfo, providerID)
		}
		return creds.AccessToken, nil
	case ProviderAnthropic:
		key, err := e.secrets.GetAnthropicKey()
		if err != nil {
			return "", withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
		return key, nil
	case ProviderAnthropicClaude:
		token, err := e.secrets.GetAnthropicClaudeSetupToken()
		if err != nil {
			return "", withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
		return token, nil
	case ProviderGoogle:
		key, err := e.secrets.GetGoogleKey()
		if err != nil {
			return "", withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
		return key, nil
	case ProviderMistral:
		key, err := e.secrets.GetMistralKey()
		if err != nil {
			return "", withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
		return key, nil
	default:
		return "", withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported provider"), providerID)
	}
}

func (e *Engine) setProviderKey(providerID, apiKey string) *errinfo.ErrorInfo {
	switch providerID {
	case ProviderOpenAI:
		if err := e.secrets.SetOpenAIKey(strings.TrimSpace(apiKey)); err != nil {
			return withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
	case ProviderAnthropic:
		if err := e.secrets.SetAnthropicKey(strings.TrimSpace(apiKey)); err != nil {
			return withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
	case ProviderAnthropicClaude:
		if err := e.secrets.SetAnthropicClaudeSetupToken(strings.TrimSpace(apiKey)); err != nil {
			return withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
	case ProviderGoogle:
		if err := e.secrets.SetGoogleKey(strings.TrimSpace(apiKey)); err != nil {
			return withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
	case ProviderMistral:
		if err := e.secrets.SetMistralKey(strings.TrimSpace(apiKey)); err != nil {
			return withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
		}
	default:
		return withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported provider"), providerID)
	}
	return nil
}

func (e *Engine) clearProviderKey(providerID string) *errinfo.ErrorInfo {
	if err := e.secrets.ClearProviderKey(providerID); err != nil {
		return withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
	}
	return nil
}

func (e *Engine) providerEnabled(providerID string) (bool, *errinfo.ErrorInfo) {
	settingsData, err := e.settings.Load()
	if err != nil {
		return false, withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
	}
	entry, ok := settingsData.Providers[providerID]
	if !ok {
		return false, nil
	}
	return entry.Enabled, nil
}

func (e *Engine) ensureProviderReadyFor(ctx context.Context, providerID string) *errinfo.ErrorInfo {
	settingsData, err := e.settings.Load()
	if err != nil {
		return withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
	}
	entry, ok := settingsData.Providers[providerID]
	if !ok || !entry.Enabled {
		e.logger.Warn("providers.ready_failed", "provider_id", providerID, "reason", "disabled_or_missing")
		return withProviderID(errinfo.ProviderNotConfigured(errinfo.PhaseSettings), providerID)
	}
	key, errInfo := e.providerKey(ctx, providerID)
	if errInfo != nil {
		e.logger.Warn(
			"providers.ready_failed",
			"provider_id", providerID,
			"reason", "credential_lookup_failed",
			"error_code", errInfo.ErrorCode,
			"detail", errInfo.Detail,
		)
		return withProviderID(errInfo, providerID)
	}
	if strings.TrimSpace(key) == "" {
		e.logger.Warn("providers.ready_failed", "provider_id", providerID, "reason", "credential_empty")
		return withProviderID(errinfo.ProviderNotConfigured(errinfo.PhaseSettings), providerID)
	}
	e.logger.Debug("providers.ready", "provider_id", providerID)
	return nil
}

func (e *Engine) validateProviderKey(ctx context.Context, providerID string) *errinfo.ErrorInfo {
	key, errInfo := e.providerKey(ctx, providerID)
	if errInfo != nil {
		return withProviderID(errInfo, providerID)
	}
	if strings.TrimSpace(key) == "" {
		return withProviderID(errinfo.ProviderNotConfigured(errinfo.PhaseSettings), providerID)
	}
	client, errInfo := e.clientForProvider(providerID)
	if errInfo != nil {
		return withProviderID(errInfo, providerID)
	}
	if err := client.ValidateKey(ctx, key); err != nil {
		return mapLLMError(errinfo.PhaseSettings, providerID, err)
	}
	return nil
}

func (e *Engine) getOpenAICodexCredentials(ctx context.Context) (*secrets.OpenAICodexOAuthCredentials, *errinfo.ErrorInfo) {
	creds, err := e.secrets.GetOpenAICodexOAuthCredentials()
	if err != nil {
		return nil, withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), ProviderOpenAICodex)
	}
	if creds == nil || strings.TrimSpace(creds.RefreshToken) == "" {
		e.logger.Debug("providers.oauth.codex_credentials_missing")
		return nil, withProviderID(errinfo.ProviderNotConfigured(errinfo.PhaseSettings), ProviderOpenAICodex)
	}
	expiresAt := ""
	if !creds.ExpiresAt.IsZero() {
		expiresAt = creds.ExpiresAt.UTC().Format(time.RFC3339)
	}
	expiringSoon := e.openAICodexTokenExpiringSoon(creds.ExpiresAt)
	e.logger.Debug(
		"providers.oauth.codex_credentials_loaded",
		"has_access_token", strings.TrimSpace(creds.AccessToken) != "",
		"has_refresh_token", strings.TrimSpace(creds.RefreshToken) != "",
		"has_id_token", strings.TrimSpace(creds.IDToken) != "",
		"expires_at", expiresAt,
		"expiring_soon", expiringSoon,
		"account_label_present", strings.TrimSpace(creds.AccountLabel) != "",
	)
	if strings.TrimSpace(creds.AccessToken) == "" || expiringSoon {
		e.logger.Debug(
			"providers.oauth.codex_refresh_required",
			"missing_access_token", strings.TrimSpace(creds.AccessToken) == "",
			"expiring_soon", expiringSoon,
		)
		refreshed, errInfo := e.refreshOpenAICodexCredentials(ctx, creds)
		if errInfo != nil {
			e.logger.Warn(
				"providers.oauth.codex_refresh_failed",
				"error_code", errInfo.ErrorCode,
				"detail", errInfo.Detail,
			)
			return nil, errInfo
		}
		creds = refreshed
	}
	if strings.TrimSpace(creds.AccessToken) == "" {
		e.logger.Warn("providers.oauth.codex_access_token_missing_after_refresh")
		return nil, withProviderID(errinfo.ProviderNotConfigured(errinfo.PhaseSettings), ProviderOpenAICodex)
	}
	accessAccountPresent := false
	idAccountPresent := false
	if e.codexOAuth != nil {
		accessAccountPresent = strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(creds.AccessToken)) != ""
		idAccountPresent = strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(creds.IDToken)) != ""
	}
	e.logger.Debug(
		"providers.oauth.codex_credentials_ready",
		"access_account_present", accessAccountPresent,
		"id_account_present", idAccountPresent,
		"account_label_present", strings.TrimSpace(creds.AccountLabel) != "",
	)
	return creds, nil
}

func (e *Engine) openAICodexTokenExpiringSoon(expiresAt time.Time) bool {
	if expiresAt.IsZero() {
		return true
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	return now.Add(oauthRefreshLeadTime).After(expiresAt)
}

func (e *Engine) refreshOpenAICodexCredentials(ctx context.Context, creds *secrets.OpenAICodexOAuthCredentials) (*secrets.OpenAICodexOAuthCredentials, *errinfo.ErrorInfo) {
	if e.codexOAuth == nil {
		return nil, withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth provider unavailable"), ProviderOpenAICodex)
	}
	e.logger.Debug(
		"providers.oauth.codex_refresh_start",
		"has_refresh_token", strings.TrimSpace(creds.RefreshToken) != "",
		"has_access_token", strings.TrimSpace(creds.AccessToken) != "",
		"has_id_token", strings.TrimSpace(creds.IDToken) != "",
	)
	token, err := e.codexOAuth.RefreshAccessToken(ctx, creds.RefreshToken)
	if err != nil {
		return nil, mapOAuthError(err, ProviderOpenAICodex)
	}
	expiresAt := ""
	if !token.ExpiresAt.IsZero() {
		expiresAt = token.ExpiresAt.UTC().Format(time.RFC3339)
	}
	e.logger.Debug(
		"providers.oauth.codex_refresh_response",
		"has_access_token", strings.TrimSpace(token.AccessToken) != "",
		"has_refresh_token", strings.TrimSpace(token.RefreshToken) != "",
		"has_id_token", strings.TrimSpace(token.IDToken) != "",
		"expires_at", expiresAt,
	)
	updated := *creds
	updated.AccessToken = token.AccessToken
	if strings.TrimSpace(token.RefreshToken) != "" {
		updated.RefreshToken = token.RefreshToken
	}
	if strings.TrimSpace(token.IDToken) != "" {
		updated.IDToken = token.IDToken
	}
	if !token.ExpiresAt.IsZero() {
		updated.ExpiresAt = token.ExpiresAt.UTC()
	}
	account := strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(updated.IDToken))
	if account == "" {
		account = strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(updated.AccessToken))
	}
	if account != "" {
		updated.AccountLabel = account
	}
	if err := e.secrets.SetOpenAICodexOAuthCredentials(&updated); err != nil {
		return nil, withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), ProviderOpenAICodex)
	}
	e.logger.Debug(
		"providers.oauth.codex_refresh_persisted",
		"has_access_token", strings.TrimSpace(updated.AccessToken) != "",
		"has_refresh_token", strings.TrimSpace(updated.RefreshToken) != "",
		"has_id_token", strings.TrimSpace(updated.IDToken) != "",
		"account_label_present", strings.TrimSpace(updated.AccountLabel) != "",
	)
	return &updated, nil
}

func mapOAuthError(err error, providerID string) *errinfo.ErrorInfo {
	if errors.Is(err, llm.ErrUnauthorized) {
		return withProviderID(errinfo.ProviderAuthFailed(errinfo.PhaseSettings), providerID)
	}
	if errors.Is(err, llm.ErrEgressBlocked) {
		return withProviderID(errinfo.EgressBlocked(errinfo.PhaseSettings, "provider endpoint not allowed"), providerID)
	}
	if errors.Is(err, llm.ErrUnavailable) {
		return withProviderID(errinfo.ProviderUnavailable(errinfo.PhaseSettings, err.Error()), providerID)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return withProviderID(errinfo.NetworkUnavailable(errinfo.PhaseSettings, err.Error()), providerID)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return withProviderID(errinfo.NetworkUnavailable(errinfo.PhaseSettings, err.Error()), providerID)
	}
	return withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, err.Error()), providerID)
}

func modelsForProvider(providerID string) []string {
	var models []string
	for _, model := range listSupportedModels() {
		if model.ProviderID == providerID {
			models = append(models, model.ModelID)
		}
	}
	return models
}

func (e *Engine) resolveActiveModel(workbenchID string) (string, *errinfo.ErrorInfo) {
	state, err := e.readWorkshopState(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if state.ActiveModelID != "" {
		canonicalID := canonicalModelID(state.ActiveModelID)
		if model, ok := getModel(canonicalID); ok {
			if canonicalID != state.ActiveModelID {
				state.ActiveModelID = canonicalID
				if err := e.writeWorkshopState(workbenchID, state); err != nil {
					return "", errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
				}
			}
			return model.ModelID, nil
		}
	}
	wb, err := e.workbenches.Open(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if wb.DefaultModelID != "" {
		canonicalID := canonicalModelID(wb.DefaultModelID)
		if model, ok := getModel(canonicalID); ok {
			if canonicalID != wb.DefaultModelID {
				if errInfo := e.setWorkbenchDefaultModel(workbenchID, canonicalID); errInfo != nil {
					return "", errInfo
				}
			}
			return model.ModelID, nil
		}
	}
	settingsData, err := e.settings.Load()
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
	}
	if settingsData.UserDefaultModelID != "" {
		canonicalID := canonicalModelID(settingsData.UserDefaultModelID)
		if model, ok := getModel(canonicalID); ok {
			if canonicalID != settingsData.UserDefaultModelID {
				settingsData.UserDefaultModelID = canonicalID
				if err := e.settings.Save(settingsData); err != nil {
					return "", errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error())
				}
			}
			return model.ModelID, nil
		}
	}
	return ModelOpenAIID, nil
}

func (e *Engine) setActiveModel(workbenchID, modelID string) *errinfo.ErrorInfo {
	modelID = canonicalModelID(modelID)
	if _, ok := getModel(modelID); !ok {
		return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	state, _ := e.readWorkshopState(workbenchID)
	state.ActiveModelID = modelID
	if err := e.writeWorkshopState(workbenchID, state); err != nil {
		return errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	return nil
}

func (e *Engine) setWorkbenchDefaultModel(workbenchID, modelID string) *errinfo.ErrorInfo {
	modelID = canonicalModelID(modelID)
	if _, ok := getModel(modelID); !ok {
		return errinfo.ValidationFailed(errinfo.PhaseWorkbench, "unsupported model")
	}
	path := filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "workbench.json")
	wb, err := e.workbenches.Open(workbenchID)
	if err != nil {
		return errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	wb.DefaultModelID = modelID
	if err := writeJSON(path, wb); err != nil {
		return errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return nil
}
