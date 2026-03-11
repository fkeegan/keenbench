package engine

import (
	"context"
	"encoding/json"
	"strings"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/settings"
)

func (e *Engine) ModelsListSupported(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	models := make([]ModelInfo, 0, len(listSupportedModels())+len(e.openrouterModels))
	seen := map[string]struct{}{}
	for _, model := range listSupportedModels() {
		models = append(models, model)
		seen[model.ModelID] = struct{}{}
	}
	for _, modelID := range e.openrouterModelIDs() {
		model, ok := e.findModel(modelID)
		if !ok {
			continue
		}
		if _, ok := seen[model.ModelID]; ok {
			continue
		}
		models = append(models, model)
	}
	return map[string]any{"models": models}, nil
}

func (e *Engine) ModelsGetCapabilities(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	req.ModelID = canonicalModelID(req.ModelID)
	model, ok := e.findModel(req.ModelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported model")
	}
	return map[string]any{
		"capabilities": map[string]any{
			"supports_file_read":  model.SupportsFileRead,
			"supports_file_write": model.SupportsFileWrite,
			"context_tokens":      model.ContextTokens,
			"can_be_secondary":    model.CanBeSecondary,
		},
	}, nil
}

func (e *Engine) UserGetDefaultModel(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	result, errInfo := e.UserGetDefaultSelection(ctx, nil)
	if errInfo != nil {
		return nil, errInfo
	}
	selection := result.(map[string]any)
	return map[string]any{"model_id": selection["model_id"]}, nil
}

func (e *Engine) UserSetDefaultModel(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	req.ModelID = canonicalModelID(req.ModelID)
	model, ok := e.findModel(req.ModelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported model")
	}
	selectionParams, err := json.Marshal(map[string]any{
		"provider_id": model.ProviderID,
		"model_id":    req.ModelID,
	})
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	return e.UserSetDefaultSelection(ctx, selectionParams)
}

func (e *Engine) UserGetDefaultSelection(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	settingsData, err := e.settings.Load()
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
	}
	selection, changed := e.resolveDefaultSelection(settingsData)
	if changed {
		if err := e.settings.Save(settingsData); err != nil {
			return nil, errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error())
		}
	}
	return map[string]any{
		"provider_id": selection.ProviderID,
		"model_id":    selection.ModelID,
	}, nil
}

func (e *Engine) UserSetDefaultSelection(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
		ModelID    string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.ModelID = canonicalModelID(req.ModelID)
	model, ok := e.findModel(req.ModelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported model")
	}
	if req.ProviderID == "" {
		req.ProviderID = model.ProviderID
	}
	if model.ProviderID != req.ProviderID {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "provider/model mismatch")
	}
	if _, errInfo := e.clientForProvider(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	_, err := e.settings.Update(func(s *settings.Settings) {
		s.UserDefaultProviderID = req.ProviderID
		s.UserDefaultModelID = req.ModelID
	})
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error())
	}
	return map[string]any{}, nil
}

func (e *Engine) UserGetConsentMode(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	settingsData, err := e.settings.Load()
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
	}
	return map[string]any{
		"mode": settings.NormalizeUserConsentMode(settingsData.UserConsentMode),
	}, nil
}

func (e *Engine) UserSetConsentMode(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		Mode     string `json:"mode"`
		Approved bool   `json:"approved"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case settings.UserConsentModeAsk:
	case settings.UserConsentModeAllowAll:
		if !req.Approved {
			return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "explicit approval required for allow_all mode")
		}
	default:
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported consent mode")
	}
	_, err := e.settings.Update(func(s *settings.Settings) {
		s.UserConsentMode = mode
	})
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error())
	}
	return map[string]any{}, nil
}

func (e *Engine) WorkbenchGetDefaultModel(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	wb, err := e.workbenches.Open(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	canonicalID := canonicalModelID(wb.DefaultModelID)
	if canonicalID != "" && canonicalID != wb.DefaultModelID {
		if errInfo := e.setWorkbenchDefaultModel(req.WorkbenchID, canonicalID); errInfo != nil {
			return nil, errInfo
		}
		wb.DefaultModelID = canonicalID
	}
	return map[string]any{"model_id": wb.DefaultModelID}, nil
}

func (e *Engine) WorkbenchSetDefaultModel(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		ModelID     string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.setWorkbenchDefaultModel(req.WorkbenchID, req.ModelID); errInfo != nil {
		return nil, errInfo
	}
	return map[string]any{}, nil
}

func (e *Engine) WorkshopSetActiveModel(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		ModelID     string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	req.ModelID = canonicalModelID(req.ModelID)
	model, ok := e.findModel(req.ModelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	if errInfo := e.ensureProviderReadyFor(ctx, model.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := e.setActiveModel(req.WorkbenchID, req.ModelID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := e.setWorkbenchDefaultModel(req.WorkbenchID, req.ModelID); errInfo != nil {
		return nil, errInfo
	}
	e.emitClutterChanged(req.WorkbenchID)
	if e.notify != nil {
		e.notify("WorkshopModelChanged", map[string]any{
			"workbench_id": req.WorkbenchID,
			"model_id":     req.ModelID,
		})
	}
	return map[string]any{}, nil
}
