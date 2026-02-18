package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"keenbench/engine/internal/errinfo"
)

type egressEvent struct {
	Timestamp  string `json:"timestamp"`
	Kind       string `json:"kind"`
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	ScopeHash  string `json:"scope_hash"`
}

func (e *Engine) appendEgressEvent(workbenchID, kind, providerID, modelID, scopeHash string) {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	path := filepath.Join(root, "meta", "egress_events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.logger.Warn("egress.event_mkdir_failed", "error", err.Error())
		return
	}
	entry := egressEvent{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Kind:       kind,
		ProviderID: providerID,
		ModelID:    modelID,
		ScopeHash:  scopeHash,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

func (e *Engine) EgressListEvents(_ context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	path := filepath.Join(e.workbenchesRoot(), req.WorkbenchID, "meta", "egress_events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"events": []any{}}, nil
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	lines := bytes.Split(data, []byte("\n"))
	var events []any
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		events = append(events, evt)
	}
	return map[string]any{"events": events}, nil
}
