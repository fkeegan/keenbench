package engine

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/workbench"
)

const (
	clutterLightThreshold    = 0.40
	clutterModerateThreshold = 0.70
	largeTextTokensCap       = 25000
	baseDocTokens            = 4000
	baseOfficeTokens         = 6000
	baseImageTokens          = 4000
	baseOpaqueTokens         = 500
	clutterEmitInterval      = 250 * time.Millisecond
)

func (e *Engine) WorkbenchGetClutter(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	state, errInfo := e.computeClutter(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	return map[string]any{
		"score":                state.Score,
		"level":                state.Level,
		"model_id":             state.ModelID,
		"context_items_weight": state.ContextItemsWeight,
		"context_share":        state.ContextShare,
		"context_warning":      state.ContextWarning,
	}, nil
}

type clutterState struct {
	Score              float64
	Level              string
	ModelID            string
	ContextItemsWeight float64
	ContextShare       float64
	ContextWarning     bool
}

func (e *Engine) computeClutter(workbenchID string) (*clutterState, *errinfo.ErrorInfo) {
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	modelID, errInfo := e.resolveActiveModel(workbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	model, ok := getModel(modelID)
	contextTokens := 32000
	if ok && model.ContextTokens > 0 {
		contextTokens = model.ContextTokens
	}
	usage := 0.0
	usage += float64(len(files)) * 500
	for _, file := range files {
		usage += fileWeight(file)
	}
	_, contextItemsWeight := e.buildWorkbenchContextInjection(workbenchID)
	usage += contextItemsWeight
	conversationTokens := 0.0
	for _, item := range items {
		if item.Text == "" {
			continue
		}
		conversationTokens += float64(len(item.Text)) / 4.0
	}
	usage += conversationTokens
	usage += e.pendingClutterTokens(workbenchID)
	contextShare := 0.0
	if contextTokens > 0 {
		contextShare = contextItemsWeight / float64(contextTokens)
	}
	contextWarning := contextShare >= 0.35
	score := usage / float64(contextTokens)
	score = math.Max(0, math.Min(1, score))
	level := "Light"
	if score >= clutterModerateThreshold {
		level = "Heavy"
	} else if score >= clutterLightThreshold {
		level = "Moderate"
	}
	return &clutterState{
		Score:              score,
		Level:              level,
		ModelID:            modelID,
		ContextItemsWeight: contextItemsWeight,
		ContextShare:       contextShare,
		ContextWarning:     contextWarning,
	}, nil
}

func fileWeight(file workbench.FileEntry) float64 {
	kind := file.FileKind
	if kind == "" {
		kind, _ = workbench.FileKindForPath(file.Path)
	}
	size := float64(file.Size)
	switch kind {
	case workbench.FileKindText:
		if size > 100*1024 {
			return math.Min(float64(largeTextTokensCap), size/4.0)
		}
		return size / 4.0
	case workbench.FileKindDocx, workbench.FileKindOdt, workbench.FileKindPdf:
		return float64(baseDocTokens) + size/6.0
	case workbench.FileKindXlsx, workbench.FileKindPptx:
		return float64(baseOfficeTokens) + size/8.0
	case workbench.FileKindImage:
		return float64(baseImageTokens)
	default:
		return float64(baseOpaqueTokens)
	}
}

func (e *Engine) emitClutterChanged(workbenchID string) {
	state, errInfo := e.computeClutter(workbenchID)
	if errInfo != nil {
		return
	}
	if e.notify != nil {
		e.notify("WorkbenchClutterChanged", map[string]any{
			"workbench_id":         workbenchID,
			"score":                state.Score,
			"level":                state.Level,
			"model_id":             state.ModelID,
			"context_items_weight": state.ContextItemsWeight,
			"context_share":        state.ContextShare,
			"context_warning":      state.ContextWarning,
		})
	}
}

// addPendingClutter records in-flight assistant stream text before it is
// persisted in conversation history. Returns true when callers should emit
// a throttled clutter notification.
func (e *Engine) addPendingClutter(workbenchID, delta string) bool {
	if workbenchID == "" || delta == "" {
		return false
	}
	e.clutterMu.Lock()
	e.pendingClutter[workbenchID] += len(delta)
	now := time.Now()
	last := e.clutterEmitAt[workbenchID]
	shouldEmit := now.Sub(last) >= clutterEmitInterval
	if shouldEmit {
		e.clutterEmitAt[workbenchID] = now
	}
	e.clutterMu.Unlock()
	return shouldEmit
}

func (e *Engine) clearPendingClutter(workbenchID string) {
	if workbenchID == "" {
		return
	}
	e.clutterMu.Lock()
	delete(e.pendingClutter, workbenchID)
	delete(e.clutterEmitAt, workbenchID)
	e.clutterMu.Unlock()
}

func (e *Engine) pendingClutterTokens(workbenchID string) float64 {
	if workbenchID == "" {
		return 0
	}
	e.clutterMu.Lock()
	chars := e.pendingClutter[workbenchID]
	e.clutterMu.Unlock()
	return float64(chars) / 4.0
}
