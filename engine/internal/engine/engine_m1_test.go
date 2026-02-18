package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keenbench/engine/internal/diff"
	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/toolworker"
)

func TestParseProposalV2OpsOnly(t *testing.T) {
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")
	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	proposal := `{
  "schema_version":2,
  "summary":"Update report",
  "ops":[{"path":"report.docx","kind":"docx","summary":"Report summary","ops":[{"op":"append_paragraph","text":"Note"}]}]
}`
	if _, errInfo := eng.parseProposal(proposal); errInfo != nil {
		t.Fatalf("expected v2 ops-only proposal, got %v", errInfo)
	}
}

func TestParseProposalV2NoChanges(t *testing.T) {
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")
	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	proposal := `{
  "schema_version":2,
  "summary":"No changes needed",
  "no_changes":true,
  "writes":[],
  "ops":[]
}`
	if _, errInfo := eng.parseProposal(proposal); errInfo != nil {
		t.Fatalf("expected no_changes proposal, got %v", errInfo)
	}
}

func TestParseProposalV2RejectsKindMismatch(t *testing.T) {
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")
	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	proposal := `{
  "schema_version":2,
  "summary":"Update report",
  "ops":[{"path":"report.xlsx","kind":"docx","summary":"Bad","ops":[{"op":"append_paragraph","text":"Note"}]}]
}`
	if _, errInfo := eng.parseProposal(proposal); errInfo == nil {
		t.Fatalf("expected kind mismatch error")
	}
}

func TestParseProposalRejectsUnsupportedWriteExtension(t *testing.T) {
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")
	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	proposal := `{
  "schema_version":2,
  "summary":"Update report",
  "writes":[{"path":"report.pdf","content":"nope"}]
}`
	_, errInfo := eng.parseProposal(proposal)
	if errInfo == nil {
		t.Fatalf("expected unsupported write extension error")
	}
	if errInfo.ErrorCode != errinfo.CodeValidationFailed {
		t.Fatalf("expected validation failed, got %v", errInfo.ErrorCode)
	}
}

func TestParseProposalRejectsNestedPath(t *testing.T) {
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")
	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	proposal := `{
  "schema_version":2,
  "summary":"Update report",
  "writes":[{"path":"notes/summary.md","content":"nope"}]
}`
	_, errInfo := eng.parseProposal(proposal)
	if errInfo == nil {
		t.Fatalf("expected nested path error")
	}
	if errInfo.ErrorCode != errinfo.CodeValidationFailed {
		t.Fatalf("expected validation failed, got %v", errInfo.ErrorCode)
	}
}

func TestBaselineExtractionForOffice(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Test"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "report.docx")
	if err := os.WriteFile(src, []byte("baseline"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "source_paths": []string{src}})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}

	draft, err := eng.workbenches.CreateDraft(workbenchID)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := eng.ensureDraftBaseline(ctx, workbenchID, draft.DraftID); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	baselinePath := eng.baselinePath(workbenchID, draft.DraftID, "report.docx")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("expected baseline file, got %v", err)
	}

	// mutate draft file directly and ensure diff uses baseline
	draftPath := filepath.Join(eng.workbenchesRoot(), workbenchID, "draft", "report.docx")
	if err := os.WriteFile(draftPath, []byte("changed"), 0o600); err != nil {
		t.Fatalf("write draft: %v", err)
	}
	resp, errInfo := eng.ReviewGetTextDiff(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "path": "report.docx"}))
	if errInfo != nil {
		t.Fatalf("diff: %v", errInfo)
	}
	payload := resp.(map[string]any)
	if payload["baseline_missing"] == true {
		t.Fatalf("expected baseline")
	}
	if payload["reference_source"] != "draft_start_snapshot" {
		t.Fatalf("expected reference_source=draft_start_snapshot, got %v", payload["reference_source"])
	}
}

func TestReviewGetTextDiffUsesPublishedFallbackWhenDraftReferenceMissing(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Fallback"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "report.docx")
	if err := os.WriteFile(src, []byte("published version"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "source_paths": []string{src}})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}

	draft, err := eng.workbenches.CreateDraft(workbenchID)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := eng.ensureDraftBaseline(ctx, workbenchID, draft.DraftID); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := os.Remove(eng.baselinePath(workbenchID, draft.DraftID, "report.docx")); err != nil {
		t.Fatalf("remove baseline: %v", err)
	}
	draftPath := filepath.Join(eng.workbenchesRoot(), workbenchID, "draft", "report.docx")
	if err := os.WriteFile(draftPath, []byte("draft changed"), 0o600); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	resp, errInfo := eng.ReviewGetTextDiff(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "path": "report.docx"}))
	if errInfo != nil {
		t.Fatalf("diff: %v", errInfo)
	}
	payload := resp.(map[string]any)
	if payload["baseline_missing"] == true {
		t.Fatalf("expected published fallback comparison")
	}
	if payload["reference_source"] != "published_current_fallback" {
		t.Fatalf("expected reference_source=published_current_fallback, got %v", payload["reference_source"])
	}
	warning, _ := payload["reference_warning"].(string)
	if strings.TrimSpace(warning) == "" {
		t.Fatalf("expected reference_warning for fallback")
	}
	hunks, ok := payload["hunks"].([]diff.Hunk)
	if !ok {
		t.Fatalf("expected hunks payload")
	}
	if len(hunks) == 0 {
		t.Fatalf("expected non-empty hunks for fallback diff")
	}
}

type failingExtractWorker struct {
	delegate toolworker.Client
	failPath string
}

func (w *failingExtractWorker) Call(ctx context.Context, method string, params any, result any) error {
	if strings.HasSuffix(method, "ExtractText") {
		payload := map[string]any{}
		data, _ := json.Marshal(params)
		_ = json.Unmarshal(data, &payload)
		if path, _ := payload["path"].(string); path == w.failPath {
			return errors.New("extract failed")
		}
	}
	return w.delegate.Call(ctx, method, params, result)
}

func (w *failingExtractWorker) Close() error {
	if w.delegate == nil {
		return nil
	}
	return w.delegate.Close()
}

func (w *failingExtractWorker) HealthCheck(ctx context.Context) error {
	if w.delegate == nil {
		return nil
	}
	return w.delegate.HealthCheck(ctx)
}

func TestBaselineExtractionBestEffort(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	workbenchesDir := filepath.Join(dataDir, "workbenches")
	if err := os.MkdirAll(workbenchesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	worker := &failingExtractWorker{
		delegate: toolworker.NewFake(workbenchesDir),
		failPath: "bad.docx",
	}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Baseline"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	srcBad := filepath.Join(dataDir, "bad.docx")
	srcGood := filepath.Join(dataDir, "good.docx")
	if err := os.WriteFile(srcBad, []byte("bad"), 0o600); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if err := os.WriteFile(srcGood, []byte("good"), 0o600); err != nil {
		t.Fatalf("write good: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{srcBad, srcGood},
	})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}

	draft, err := eng.workbenches.CreateDraft(workbenchID)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := eng.ensureDraftBaseline(ctx, workbenchID, draft.DraftID); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := os.Stat(eng.baselinePath(workbenchID, draft.DraftID, "good.docx")); err != nil {
		t.Fatalf("expected good baseline, got %v", err)
	}
	if _, err := os.Stat(eng.baselinePath(workbenchID, draft.DraftID, "bad.docx")); err == nil {
		t.Fatalf("unexpected baseline for failing file")
	}
}

func TestXlsxFocusHintFromProposalOps(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Focus"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "metrics.xlsx")
	if err := os.WriteFile(src, []byte("baseline"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}

	proposal := &Proposal{
		ProposalID:    "p-focus",
		SchemaVersion: 2,
		Summary:       "focus",
		Ops: []ProposalOp{
			{
				Path:    "metrics.xlsx",
				Kind:    "xlsx",
				Summary: "update",
				Ops: []map[string]any{
					{
						"op":     "set_range",
						"sheet":  "Summary",
						"start":  "B2",
						"values": []any{[]any{"Metric"}},
					},
				},
			},
		},
	}
	if err := eng.writeProposal(workbenchID, proposal); err != nil {
		t.Fatalf("write proposal: %v", err)
	}
	if _, errInfo := eng.WorkshopApplyProposal(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"proposal_id":  proposal.ProposalID,
	})); errInfo != nil {
		t.Fatalf("apply: %v", errInfo)
	}

	resp, errInfo := eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("changeset: %v", errInfo)
	}
	payload := resp.(map[string]any)
	data, err := json.Marshal(payload["changes"])
	if err != nil {
		t.Fatalf("marshal changes: %v", err)
	}
	var changes []map[string]any
	if err := json.Unmarshal(data, &changes); err != nil {
		t.Fatalf("unmarshal changes: %v", err)
	}
	found := false
	for _, entry := range changes {
		if entry["path"] != "metrics.xlsx" {
			continue
		}
		found = true
		hint, ok := entry["focus_hint"].(map[string]any)
		if !ok {
			t.Fatalf("missing focus hint")
		}
		if hint["sheet"] != "Summary" {
			t.Fatalf("expected sheet Summary, got %v", hint["sheet"])
		}
		if hint["row_start"] != float64(1) {
			t.Fatalf("expected row_start 1, got %v", hint["row_start"])
		}
		if hint["col_start"] != float64(1) {
			t.Fatalf("expected col_start 1, got %v", hint["col_start"])
		}
	}
	if !found {
		t.Fatalf("expected metrics.xlsx change")
	}
}

func TestMapToolWorkerUnavailable(t *testing.T) {
	err := &toolworker.RemoteError{
		Code:    toolworker.CodeToolWorkerUnavailable,
		Message: "LibreOffice not available",
	}
	errInfo := mapToolWorkerError(errinfo.PhaseReview, err)
	if errInfo == nil {
		t.Fatalf("expected mapped error")
	}
	if errInfo.ErrorCode != errinfo.CodeToolWorkerUnavailable {
		t.Fatalf("expected tool worker unavailable, got %v", errInfo.ErrorCode)
	}
}
