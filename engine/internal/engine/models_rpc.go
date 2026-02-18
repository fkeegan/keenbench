package engine

import (
	"context"
	"encoding/json"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/settings"
)

func (e *Engine) ModelsListSupported(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	models := listSupportedModels()
	return map[string]any{"models": models}, nil
}

func (e *Engine) ModelsGetCapabilities(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	model, ok := getModel(req.ModelID)
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
	settingsData, err := e.settings.Load()
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
	}
	return map[string]any{"model_id": settingsData.UserDefaultModelID}, nil
}

func (e *Engine) UserSetDefaultModel(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if _, ok := getModel(req.ModelID); !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported model")
	}
	_, err := e.settings.Update(func(s *settings.Settings) {
		s.UserDefaultModelID = req.ModelID
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
	model, ok := getModel(req.ModelID)
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
