package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/toolworker"
)

func TestBuildDocxFocusHint(t *testing.T) {
	hint := buildDocxFocusHint([]map[string]any{
		{"op": "append_paragraph", "text": "intro"},
		{"op": "replace_text", "section_index": 3, "search": "old", "replace": "new"},
		{"op": "replace_text", "section": "1", "search": "a", "replace": "b"},
	})
	if hint == nil {
		t.Fatalf("expected focus hint")
	}
	if got := hint["section_index"]; got != 1 {
		t.Fatalf("expected section_index=1, got %v", got)
	}

	defaultHint := buildDocxFocusHint([]map[string]any{
		{"op": "set_paragraphs", "paragraphs": []any{}},
	})
	if defaultHint == nil || defaultHint["section_index"] != 0 {
		t.Fatalf("expected default section_index=0, got %v", defaultHint)
	}
}

func TestBuildPptxFocusHint(t *testing.T) {
	hint := buildPptxFocusHint([]map[string]any{
		{"op": "add_slide", "index": 0, "title": "new"},
		{"op": "set_slide_text", "index": 3, "title": "updated"},
		{"op": "append_bullets", "slide_index": "1", "bullets": []any{"x"}},
	})
	if hint == nil {
		t.Fatalf("expected focus hint")
	}
	if got := hint["slide_index"]; got != 1 {
		t.Fatalf("expected slide_index=1, got %v", got)
	}

	addOnly := buildPptxFocusHint([]map[string]any{
		{"op": "add_slide", "title": "new"},
	})
	if addOnly != nil {
		t.Fatalf("expected add_slide without explicit index to defer hint resolution, got %v", addOnly)
	}
}

func TestBuildXlsxFocusHintSheetLevelOps(t *testing.T) {
	summaryHint := buildXlsxFocusHint([]map[string]any{
		{
			"op":            "summarize_by_category",
			"sheet":         "Annual",
			"source_sheets": []any{"Q1", "Q2"},
		},
	})
	if summaryHint == nil {
		t.Fatalf("expected summarize_by_category hint")
	}
	if summaryHint["sheet"] != "Annual" {
		t.Fatalf("expected sheet Annual, got %v", summaryHint["sheet"])
	}
	if _, ok := summaryHint["row_start"]; ok {
		t.Fatalf("expected sheet-only summary hint, got %v", summaryHint)
	}

	ensureHint := buildXlsxFocusHint([]map[string]any{
		{"op": "ensure_sheet", "sheet": "Staging"},
	})
	if ensureHint == nil {
		t.Fatalf("expected ensure_sheet hint")
	}
	if ensureHint["sheet"] != "Staging" {
		t.Fatalf("expected sheet Staging, got %v", ensureHint["sheet"])
	}
}

func TestShouldFallbackPptxSlideLegacy(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "validation detail unsupported",
			err: &toolworker.RemoteError{
				Code:    errinfo.CodeValidationFailed,
				Message: "invalid detail",
			},
			want: true,
		},
		{
			name: "validation positioned unsupported",
			err: &toolworker.RemoteError{
				Code:    errinfo.CodeValidationFailed,
				Message: "positioned payload not supported",
			},
			want: true,
		},
		{
			name: "validation unrelated",
			err: &toolworker.RemoteError{
				Code:    errinfo.CodeValidationFailed,
				Message: "invalid slide_index",
			},
			want: false,
		},
		{
			name: "file read failed",
			err: &toolworker.RemoteError{
				Code:    errinfo.CodeFileReadFailed,
				Message: "decode failed",
			},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("boom"),
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackPptxSlideLegacy(tc.err); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestReviewGetDocxContentDiffBaselineSemantics(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "DocxDiff"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "report.docx")
	if err := os.WriteFile(src, []byte("baseline content"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
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
	draftPath := filepath.Join(eng.workbenchesRoot(), workbenchID, "draft", "report.docx")
	if err := os.WriteFile(draftPath, []byte("draft changed"), 0o600); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	resp, errInfo := eng.ReviewGetDocxContentDiff(ctx, mustJSON(t, map[string]any{
		"workbench_id":  workbenchID,
		"path":          "report.docx",
		"section_index": 0,
	}))
	if errInfo != nil {
		t.Fatalf("content diff: %v", errInfo)
	}
	payload := decodeDocxDiffPayload(t, resp)
	if payload.BaselineMissing {
		t.Fatalf("expected baseline to be present")
	}
	if payload.ReferenceSource != "draft_start_snapshot" {
		t.Fatalf("expected reference_source=draft_start_snapshot, got %q", payload.ReferenceSource)
	}
	if payload.SectionCount < 1 {
		t.Fatalf("expected section_count >= 1, got %d", payload.SectionCount)
	}
	if !strings.Contains(firstParagraphText(payload.Baseline), "baseline content") {
		t.Fatalf("expected baseline payload to include source content")
	}
	if !strings.Contains(firstParagraphText(payload.Draft), "draft changed") {
		t.Fatalf("expected draft payload to include draft content")
	}
}

func TestReviewGetPptxContentDiffPublishedFallbackWhenDraftReferenceMissing(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "PptxDiff"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "slides.pptx")
	if err := os.WriteFile(src, []byte("baseline slide"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
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
	if err := os.Remove(eng.baselinePath(workbenchID, draft.DraftID, "slides.pptx")); err != nil {
		t.Fatalf("remove baseline: %v", err)
	}

	resp, errInfo := eng.ReviewGetPptxContentDiff(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"path":         "slides.pptx",
		"slide_index":  0,
	}))
	if errInfo != nil {
		t.Fatalf("content diff: %v", errInfo)
	}
	var payload struct {
		Baseline         map[string]any `json:"baseline"`
		BaselineMissing  bool           `json:"baseline_missing"`
		SlideCount       int            `json:"slide_count"`
		Draft            map[string]any `json:"draft"`
		ReferenceSource  string         `json:"reference_source"`
		ReferenceWarning string         `json:"reference_warning"`
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.BaselineMissing {
		t.Fatalf("expected fallback comparison instead of baseline_missing")
	}
	if payload.SlideCount < 1 {
		t.Fatalf("expected slide_count >= 1, got %d", payload.SlideCount)
	}
	if payload.Baseline == nil {
		t.Fatalf("expected baseline payload from published fallback")
	}
	if payload.Draft == nil {
		t.Fatalf("expected draft payload")
	}
	if payload.ReferenceSource != "published_current_fallback" {
		t.Fatalf("expected reference_source=published_current_fallback, got %q", payload.ReferenceSource)
	}
	if payload.ReferenceWarning == "" {
		t.Fatalf("expected reference_warning for fallback comparison")
	}
}

func TestWorkshopApplyProposalPersistsDocxAndPptxFocusHints(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "FocusHints"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	docxSrc := filepath.Join(dataDir, "report.docx")
	pptxSrc := filepath.Join(dataDir, "slides.pptx")
	if err := os.WriteFile(docxSrc, []byte("baseline"), 0o600); err != nil {
		t.Fatalf("write docx: %v", err)
	}
	if err := os.WriteFile(pptxSrc, []byte("baseline"), 0o600); err != nil {
		t.Fatalf("write pptx: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{docxSrc, pptxSrc},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	proposal := &Proposal{
		ProposalID:    "p-focus-office",
		SchemaVersion: 2,
		Summary:       "focus hints",
		Ops: []ProposalOp{
			{
				Path:    "report.docx",
				Kind:    "docx",
				Summary: "docx update",
				Ops: []map[string]any{
					{"op": "append_paragraph", "text": "Added paragraph"},
				},
			},
			{
				Path:    "slides.pptx",
				Kind:    "pptx",
				Summary: "slide update",
				Ops: []map[string]any{
					{"op": "set_slide_text", "index": 2, "title": "Updated"},
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
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var payload struct {
		Changes []map[string]any `json:"changes"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	docxFound := false
	pptxFound := false
	for _, change := range payload.Changes {
		path, _ := change["path"].(string)
		hint, _ := change["focus_hint"].(map[string]any)
		switch path {
		case "report.docx":
			docxFound = true
			if hint["section_index"] != float64(0) {
				t.Fatalf("expected docx section_index=0, got %v", hint["section_index"])
			}
		case "slides.pptx":
			pptxFound = true
			if hint["slide_index"] != float64(2) {
				t.Fatalf("expected pptx slide_index=2, got %v", hint["slide_index"])
			}
		}
	}
	if !docxFound || !pptxFound {
		t.Fatalf("expected both docx and pptx changes to include focus hints")
	}
}

type docxDiffPayload struct {
	Baseline         map[string]any `json:"baseline"`
	Draft            map[string]any `json:"draft"`
	SectionCount     int            `json:"section_count"`
	BaselineMissing  bool           `json:"baseline_missing"`
	ReferenceSource  string         `json:"reference_source"`
	ReferenceWarning string         `json:"reference_warning"`
}

func decodeDocxDiffPayload(t *testing.T, value any) docxDiffPayload {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var payload docxDiffPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func firstParagraphText(section map[string]any) string {
	paragraphs, ok := section["paragraphs"].([]any)
	if !ok || len(paragraphs) == 0 {
		return ""
	}
	paragraph, ok := paragraphs[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := paragraph["text"].(string)
	return text
}
