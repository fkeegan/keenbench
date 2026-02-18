package engine

import (
	"context"
	"encoding/json"
	"time"

	"keenbench/engine/internal/errinfo"
)

func (e *Engine) CheckpointsList(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	items, err := e.workbenches.CheckpointsList(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return map[string]any{"checkpoints": items}, nil
}

func (e *Engine) CheckpointGet(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID  string `json:"workbench_id"`
		CheckpointID string `json:"checkpoint_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	meta, err := e.workbenches.CheckpointGet(req.WorkbenchID, req.CheckpointID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return map[string]any{"checkpoint": meta}, nil
}

func (e *Engine) CheckpointCreate(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkshopUnlocked(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	reason := req.Reason
	if reason == "" {
		reason = "manual"
	}
	checkpointID, err := e.workbenches.CheckpointCreate(req.WorkbenchID, reason, req.Description)
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if e.notify != nil {
		e.notify("CheckpointCreated", map[string]any{
			"workbench_id":  req.WorkbenchID,
			"checkpoint_id": checkpointID,
			"reason":        reason,
			"created_at":    time.Now().UTC().Format(time.RFC3339),
			"description":   req.Description,
		})
	}
	return map[string]any{"checkpoint_id": checkpointID}, nil
}

func (e *Engine) CheckpointRestore(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID  string `json:"workbench_id"`
		CheckpointID string `json:"checkpoint_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkshopUnlocked(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	preID, err := e.workbenches.CheckpointCreate(req.WorkbenchID, "pre_restore", "Before restore")
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if err := e.workbenches.CheckpointRestore(req.WorkbenchID, req.CheckpointID, preID); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	restoredAt := time.Now().UTC().Format(time.RFC3339)
	e.appendCheckpointConversationEvent(req.WorkbenchID, conversationMessage{
		Type:         "system_event",
		Role:         "system",
		Text:         "Restored checkpoint.",
		CreatedAt:    restoredAt,
		EventKind:    "checkpoint_restore",
		CheckpointID: req.CheckpointID,
		Reason:       "restore",
		Timestamp:    restoredAt,
		Metadata: map[string]any{
			"pre_restore_checkpoint_id": preID,
		},
	})
	if e.notify != nil {
		e.notify("CheckpointRestored", map[string]any{
			"workbench_id":  req.WorkbenchID,
			"checkpoint_id": req.CheckpointID,
		})
	}
	return map[string]any{}, nil
}
