package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
	"keenbench/engine/internal/toolworker"
	"keenbench/engine/internal/workbench"
)

type captureToolWorker struct {
	method string
	params map[string]any
	resp   map[string]any
}

func (c *captureToolWorker) Call(_ context.Context, method string, params any, result any) error {
	c.method = method
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	c.params = decoded
	if result != nil {
		payload := c.resp
		if payload == nil {
			payload = map[string]any{"ok": true}
		}
		resp, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(resp, result); err != nil {
			return err
		}
	}
	return nil
}

func (c *captureToolWorker) HealthCheck(_ context.Context) error { return nil }
func (c *captureToolWorker) Close() error                        { return nil }

type mapAwareToolWorker struct {
	slideCount int
	calls      []string
}

func (m *mapAwareToolWorker) Call(_ context.Context, method string, _ any, result any) error {
	m.calls = append(m.calls, method)
	if result == nil {
		return nil
	}
	resp := map[string]any{"ok": true}
	if method == "PptxGetMap" {
		resp = map[string]any{"slide_count": m.slideCount}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func (m *mapAwareToolWorker) HealthCheck(_ context.Context) error { return nil }
func (m *mapAwareToolWorker) Close() error                        { return nil }

type errorToolWorker struct {
	err    error
	method string
	params map[string]any
}

func (w *errorToolWorker) Call(_ context.Context, method string, params any, _ any) error {
	w.method = method
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	w.params = decoded
	return w.err
}

func (w *errorToolWorker) HealthCheck(_ context.Context) error { return nil }
func (w *errorToolWorker) Close() error                        { return nil }

func TestToolHandlerListFiles(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "ToolTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	// Add a test file
	testFile := filepath.Join(dataDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{testFile},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	handler := NewToolHandler(eng, workbenchID, ctx)
	result, err := handler.listFiles()
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}

	var files []map[string]any
	if err := json.Unmarshal([]byte(result), &files); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0]["path"] != "test.txt" {
		t.Fatalf("expected test.txt, got %v", files[0]["path"])
	}
	if files[0]["kind"] != "text" {
		t.Fatalf("expected text kind, got %v", files[0]["kind"])
	}
}

func TestToolHandlerReadFile(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "ReadTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	// Add a test file
	testFile := filepath.Join(dataDir, "readme.md")
	content := "# Hello\n\nThis is a test."
	if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{testFile},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "test",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "read_file",
			Arguments: `{"path":"readme.md"}`,
		},
	}
	result, err := handler.Execute(call)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if result != content {
		t.Fatalf("expected %q, got %q", content, result)
	}
}

func TestToolHandlerReadFileWithLegacyTxtFileKind(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "LegacyTxtReadTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	testFile := filepath.Join(dataDir, "budget_notes.txt")
	content := "Budget line item A\nBudget line item B"
	if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{testFile},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	manifestPath := filepath.Join(dataDir, "workbenches", workbenchID, "meta", "files.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest workbench.FileManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(manifest.Files) != 1 {
		t.Fatalf("expected 1 manifest file entry, got %d", len(manifest.Files))
	}
	manifest.Files[0].FileKind = "txt"
	manifest.Files[0].MimeType = "application/octet-stream"
	manifest.Files[0].IsOpaque = true
	updatedManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, updatedManifest, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "test",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "read_file",
			Arguments: `{"path":"budget_notes.txt"}`,
		},
	}
	result, err := handler.Execute(call)
	if err != nil {
		t.Fatalf("read_file with legacy txt kind: %v", err)
	}
	if result != content {
		t.Fatalf("expected %q, got %q", content, result)
	}
}

func TestToolHandlerWriteTextFile(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "WriteTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "test",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "write_text_file",
			Arguments: `{"path":"output.txt","content":"Hello, World!"}`,
		},
	}
	result, err := handler.Execute(call)
	if err != nil {
		t.Fatalf("write_text_file: %v", err)
	}
	if result == "" {
		t.Fatalf("expected non-empty result")
	}

	// Verify draft was created
	ds, err := eng.workbenches.DraftState(workbenchID)
	if err != nil || ds == nil {
		t.Fatalf("expected draft to exist")
	}

	// Verify file content
	content, err := eng.workbenches.ReadFile(workbenchID, "draft", "output.txt")
	if err != nil {
		t.Fatalf("read draft file: %v", err)
	}
	if content != "Hello, World!" {
		t.Fatalf("expected 'Hello, World!', got %q", content)
	}
}

func TestToolHandlerExecuteUnknownTool(t *testing.T) {
	ctx := context.Background()
	handler := &ToolHandler{ctx: ctx}
	call := llm.ToolCall{
		ID:   "test",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "unknown_tool",
			Arguments: `{}`,
		},
	}
	_, err := handler.Execute(call)
	if err == nil {
		t.Fatalf("expected error for unknown tool")
	}
}

func TestWorkshopToolsSchemaValid(t *testing.T) {
	// Verify all tool schemas are valid JSON
	for _, tool := range WorkshopTools {
		if tool.Type != "function" {
			t.Errorf("tool %s: expected type 'function', got %q", tool.Function.Name, tool.Type)
		}
		if tool.Function.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Function.Description == "" {
			t.Errorf("tool %s: has empty description", tool.Function.Name)
		}
		var params map[string]any
		if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
			t.Errorf("tool %s: invalid parameters JSON: %v", tool.Function.Name, err)
		}
	}
}

func TestWorkshopToolsOfficeSchemasUseCanonicalOperationKeys(t *testing.T) {
	getOperationProps := func(toolName string) map[string]any {
		for _, tool := range WorkshopTools {
			if tool.Function.Name != toolName {
				continue
			}
			params := map[string]any{}
			if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
				t.Fatalf("%s schema: %v", toolName, err)
			}
			rootProps, ok := params["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s schema missing properties", toolName)
			}
			opsField, ok := rootProps["operations"].(map[string]any)
			if !ok {
				t.Fatalf("%s schema missing operations", toolName)
			}
			items, ok := opsField["items"].(map[string]any)
			if !ok {
				t.Fatalf("%s schema missing operations.items", toolName)
			}
			itemProps, ok := items["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s schema missing operations.items.properties", toolName)
			}
			return itemProps
		}
		t.Fatalf("tool not found: %s", toolName)
		return nil
	}

	xlsxProps := getOperationProps("xlsx_operations")
	opSpec, ok := xlsxProps["op"].(map[string]any)
	if !ok {
		t.Fatalf("xlsx_operations schema missing op spec")
	}
	opEnum, ok := opSpec["enum"].([]any)
	if !ok {
		t.Fatalf("xlsx_operations schema missing op enum")
	}
	expectedXlsxOps := map[string]bool{
		"ensure_sheet":          false,
		"set_range":             false,
		"set_cells":             false,
		"summarize_by_category": false,
		"set_column_widths":     false,
		"set_row_heights":       false,
		"freeze_panes":          false,
	}
	for _, raw := range opEnum {
		name, _ := raw.(string)
		if _, ok := expectedXlsxOps[name]; ok {
			expectedXlsxOps[name] = true
		}
	}
	for name, present := range expectedXlsxOps {
		if !present {
			t.Fatalf("xlsx_operations schema missing op %s", name)
		}
	}
	for _, field := range []string{"style", "columns", "rows", "row", "column"} {
		if _, ok := xlsxProps[field]; !ok {
			t.Fatalf("xlsx_operations schema must include %s", field)
		}
	}
	cellsSpec, ok := xlsxProps["cells"].(map[string]any)
	if !ok {
		t.Fatalf("xlsx_operations schema missing cells definition")
	}
	cellItems, ok := cellsSpec["items"].(map[string]any)
	if !ok {
		t.Fatalf("xlsx_operations schema missing cells.items")
	}
	cellProps, ok := cellItems["properties"].(map[string]any)
	if !ok {
		t.Fatalf("xlsx_operations schema missing cells.items.properties")
	}
	for _, field := range []string{"type", "style"} {
		if _, ok := cellProps[field]; !ok {
			t.Fatalf("xlsx_operations cells schema must include %s", field)
		}
	}

	docxProps := getOperationProps("docx_operations")
	if _, ok := docxProps["search"]; !ok {
		t.Fatalf("docx_operations schema must include search")
	}
	if _, ok := docxProps["find"]; ok {
		t.Fatalf("docx_operations schema must not include legacy find")
	}
	for _, field := range []string{"runs", "alignment", "space_before", "space_after", "line_spacing", "indent_left", "indent_right", "indent_first_line", "match_case"} {
		if _, ok := docxProps[field]; !ok {
			t.Fatalf("docx_operations schema must include %s", field)
		}
	}
	paragraphsSpec, ok := docxProps["paragraphs"].(map[string]any)
	if !ok {
		t.Fatalf("docx_operations schema missing paragraphs definition")
	}
	paragraphItems, ok := paragraphsSpec["items"].(map[string]any)
	if !ok {
		t.Fatalf("docx_operations schema missing paragraphs.items")
	}
	paragraphProps, ok := paragraphItems["properties"].(map[string]any)
	if !ok {
		t.Fatalf("docx_operations schema missing paragraphs.items.properties")
	}
	for _, field := range []string{"runs", "alignment"} {
		if _, ok := paragraphProps[field]; !ok {
			t.Fatalf("docx_operations paragraphs schema must include %s", field)
		}
	}

	pptxProps := getOperationProps("pptx_operations")
	if _, ok := pptxProps["index"]; !ok {
		t.Fatalf("pptx_operations schema must include index")
	}
	if _, ok := pptxProps["slide_index"]; ok {
		t.Fatalf("pptx_operations schema must not include legacy slide_index")
	}
	for _, field := range []string{"title_runs", "body_runs", "alignment", "space_before", "space_after", "line_spacing"} {
		if _, ok := pptxProps[field]; !ok {
			t.Fatalf("pptx_operations schema must include %s", field)
		}
	}
}

func TestWorkshopToolsIncludesStyleAssetTools(t *testing.T) {
	required := map[string]bool{
		"xlsx_get_styles":  false,
		"xlsx_copy_assets": false,
		"docx_get_styles":  false,
		"docx_copy_assets": false,
		"pptx_get_styles":  false,
		"pptx_copy_assets": false,
	}
	for _, tool := range WorkshopTools {
		if _, ok := required[tool.Function.Name]; ok {
			required[tool.Function.Name] = true
		}
	}
	for name, present := range required {
		if !present {
			t.Fatalf("expected tool schema %s", name)
		}
	}
}

func TestWorkshopToolsIncludesTabularTools(t *testing.T) {
	required := map[string]bool{
		"table_get_map":            false,
		"table_describe":           false,
		"table_stats":              false,
		"table_read_rows":          false,
		"table_query":              false,
		"table_export":             false,
		"table_update_from_export": false,
	}
	for _, tool := range WorkshopTools {
		if _, ok := required[tool.Function.Name]; ok {
			required[tool.Function.Name] = true
		}
	}
	for name, present := range required {
		if !present {
			t.Fatalf("expected tool schema %s", name)
		}
	}
}

func TestToolHandlerTableGetMapRoutesToTabularWorker(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "TableMap"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "table-map",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "table_get_map",
			Arguments: `{"path":"sales.csv"}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("table_get_map: %v", err)
	}
	if worker.method != "TabularGetMap" {
		t.Fatalf("expected TabularGetMap, got %s", worker.method)
	}
	if worker.params["root"] != "published" {
		t.Fatalf("expected published root, got %v", worker.params["root"])
	}
}

func TestToolHandlerTableExportEnsuresDraft(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "TableExport"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "table-export",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "table_export",
			Arguments: `{"path":"sales.csv","target_path":"summary.xlsx","format":"xlsx","query":"SELECT * FROM data"}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("table_export: %v", err)
	}
	if worker.method != "TabularExport" {
		t.Fatalf("expected TabularExport, got %s", worker.method)
	}
	if worker.params["target_root"] != "draft" {
		t.Fatalf("expected target_root draft, got %v", worker.params["target_root"])
	}
	if worker.params["root"] != "draft" {
		t.Fatalf("expected source root draft after ensuring draft, got %v", worker.params["root"])
	}
	if ds, err := eng.workbenches.DraftState(workbenchID); err != nil || ds == nil {
		t.Fatalf("expected draft to exist after table_export")
	}
}

func TestToolHandlerTableExportXlsxCollectsFocusHint(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{
		resp: map[string]any{
			"target_path":  "summary.xlsx",
			"format":       "xlsx",
			"row_count":    12,
			"column_count": 3,
			"warnings":     []string{},
			"sheet":        "P1_Orders_Items",
		},
	}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "TableExportHint"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "table-export-hint",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "table_export",
			Arguments: `{"path":"sales.csv","target_path":"summary.xlsx","format":"xlsx","query":"SELECT * FROM data"}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("table_export: %v", err)
	}

	hints := handler.FocusHints()
	hint, ok := hints["summary.xlsx"]
	if !ok {
		t.Fatalf("expected focus hint for summary.xlsx, got %v", hints)
	}
	if hint["sheet"] != "P1_Orders_Items" {
		t.Fatalf("expected sheet P1_Orders_Items, got %v", hint["sheet"])
	}
	rowStart, ok := intFromAny(hint["row_start"])
	if !ok || rowStart != 0 {
		t.Fatalf("expected row_start=0, got %v", hint["row_start"])
	}
	colStart, ok := intFromAny(hint["col_start"])
	if !ok || colStart != 0 {
		t.Fatalf("expected col_start=0, got %v", hint["col_start"])
	}
}

func TestToolHandlerTableUpdateFromExportValidation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "TableUpdateValidation"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "missing target path",
			args: `{"path":"sales.csv","sheet":"Summary","mode":"replace_sheet"}`,
			want: "target_path is required",
		},
		{
			name: "target must be xlsx",
			args: `{"path":"sales.csv","target_path":"summary.csv","sheet":"Summary","mode":"replace_sheet"}`,
			want: "target_path must be a .xlsx file",
		},
		{
			name: "sheet required",
			args: `{"path":"sales.csv","target_path":"summary.xlsx","mode":"replace_sheet"}`,
			want: "sheet is required",
		},
		{
			name: "mode required enum",
			args: `{"path":"sales.csv","target_path":"summary.xlsx","sheet":"Summary","mode":"overwrite_all"}`,
			want: "mode must be replace_sheet, append_rows, or write_range",
		},
		{
			name: "start cell validation for write range",
			args: `{"path":"sales.csv","target_path":"summary.xlsx","sheet":"Summary","mode":"write_range","start_cell":"12"}`,
			want: "start_cell must be an A1-style cell reference",
		},
		{
			name: "clear target range only for write range",
			args: `{"path":"sales.csv","target_path":"summary.xlsx","sheet":"Summary","mode":"append_rows","clear_target_range":true}`,
			want: "clear_target_range is only supported for mode write_range",
		},
	}

	handler := NewToolHandler(eng, workbenchID, ctx)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := llm.ToolCall{
				ID:   "table-update-validation",
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      "table_update_from_export",
					Arguments: tc.args,
				},
			}
			_, execErr := handler.Execute(call)
			if execErr == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(execErr.Error(), errinfo.CodeValidationFailed) {
				t.Fatalf("expected validation error code, got %v", execErr)
			}
			if !strings.Contains(execErr.Error(), tc.want) {
				t.Fatalf("expected %q in error, got %v", tc.want, execErr)
			}
		})
	}
}

func TestToolHandlerTableUpdateFromExportRoutesToTabularWorker(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{
		resp: map[string]any{
			"target_path":   "summary.xlsx",
			"sheet":         "Summary",
			"mode":          "write_range",
			"row_count":     2,
			"column_count":  2,
			"written_range": "",
			"warnings":      []string{},
		},
	}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "TableUpdateRoute"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "table-update-route",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "table_update_from_export",
			Arguments: `{
				"path":"sales.csv",
				"query":"SELECT city, total FROM data",
				"target_path":"summary.xlsx",
				"sheet":"Summary",
				"mode":"write_range",
				"start_cell":"B3",
				"include_header":true,
				"create_workbook_if_missing":true,
				"create_sheet_if_missing":true,
				"clear_target_range":true
			}`,
		},
	}
	result, err := handler.Execute(call)
	if err != nil {
		t.Fatalf("table_update_from_export: %v", err)
	}
	if worker.method != "TabularUpdateFromExport" {
		t.Fatalf("expected TabularUpdateFromExport, got %s", worker.method)
	}
	if worker.params["root"] != "draft" {
		t.Fatalf("expected source root draft, got %v", worker.params["root"])
	}
	if worker.params["target_root"] != "draft" {
		t.Fatalf("expected target_root draft, got %v", worker.params["target_root"])
	}
	if worker.params["target_path"] != "summary.xlsx" {
		t.Fatalf("expected target_path summary.xlsx, got %v", worker.params["target_path"])
	}
	if worker.params["sheet"] != "Summary" {
		t.Fatalf("expected sheet Summary, got %v", worker.params["sheet"])
	}
	if worker.params["mode"] != "write_range" {
		t.Fatalf("expected mode write_range, got %v", worker.params["mode"])
	}
	if worker.params["start_cell"] != "B3" {
		t.Fatalf("expected start_cell B3, got %v", worker.params["start_cell"])
	}
	if worker.params["include_header"] != true {
		t.Fatalf("expected include_header=true, got %v", worker.params["include_header"])
	}
	if worker.params["create_workbook_if_missing"] != true {
		t.Fatalf("expected create_workbook_if_missing=true, got %v", worker.params["create_workbook_if_missing"])
	}
	if worker.params["create_sheet_if_missing"] != true {
		t.Fatalf("expected create_sheet_if_missing=true, got %v", worker.params["create_sheet_if_missing"])
	}
	if worker.params["clear_target_range"] != true {
		t.Fatalf("expected clear_target_range=true, got %v", worker.params["clear_target_range"])
	}

	var payload struct {
		RowCount    int `json:"row_count"`
		ColumnCount int `json:"column_count"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload.RowCount != 2 || payload.ColumnCount != 2 {
		t.Fatalf("unexpected row/column counts in result: %#v", payload)
	}
	if ds, err := eng.workbenches.DraftState(workbenchID); err != nil || ds == nil {
		t.Fatalf("expected draft to exist after table_update_from_export")
	}
	hints := handler.FocusHints()
	hint, ok := hints["summary.xlsx"]
	if !ok {
		t.Fatalf("expected focus hint for summary.xlsx, got %v", hints)
	}
	rowStart, ok := intFromAny(hint["row_start"])
	if !ok || rowStart != 0 {
		t.Fatalf("expected fallback row_start=0, got %v", hint["row_start"])
	}
	colStart, ok := intFromAny(hint["col_start"])
	if !ok || colStart != 0 {
		t.Fatalf("expected fallback col_start=0, got %v", hint["col_start"])
	}
}

func TestToolHandlerTableUpdateFromExportXlsxCollectsFocusHint(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{
		resp: map[string]any{
			"target_path":   "summary.xlsx",
			"sheet":         "P1_Orders_Items",
			"mode":          "write_range",
			"written_range": "C4:F20",
		},
	}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "TableUpdateHint"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "table-update-hint",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "table_update_from_export",
			Arguments: `{"path":"sales.csv","target_path":"summary.xlsx","sheet":"Summary","mode":"write_range"}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("table_update_from_export: %v", err)
	}
	if worker.params["start_cell"] != "A1" {
		t.Fatalf("expected default start_cell A1, got %v", worker.params["start_cell"])
	}

	hints := handler.FocusHints()
	hint, ok := hints["summary.xlsx"]
	if !ok {
		t.Fatalf("expected focus hint for summary.xlsx, got %v", hints)
	}
	if hint["sheet"] != "P1_Orders_Items" {
		t.Fatalf("expected sheet P1_Orders_Items, got %v", hint["sheet"])
	}
	rowStart, ok := intFromAny(hint["row_start"])
	if !ok || rowStart != 3 {
		t.Fatalf("expected row_start=3, got %v", hint["row_start"])
	}
	colStart, ok := intFromAny(hint["col_start"])
	if !ok || colStart != 2 {
		t.Fatalf("expected col_start=2, got %v", hint["col_start"])
	}
}

func TestToolCallFingerprintTableUpdateFromExportStableFields(t *testing.T) {
	call := llm.ToolCall{
		ID:   "table-update-fingerprint",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "table_update_from_export",
			Arguments: `{
				"path":" sales.csv ",
				"query":" SELECT  city, total  FROM data ",
				"target_path":" summary.xlsx ",
				"sheet":" Summary ",
				"mode":"WRITE_RANGE",
				"start_cell":"b2",
				"include_header":true,
				"create_workbook_if_missing":false,
				"create_sheet_if_missing":true,
				"clear_target_range":false,
				"ignored":"value"
			}`,
		},
	}

	got := toolCallFingerprint(call)
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("fingerprint json: %v", err)
	}

	if payload["path"] != "sales.csv" {
		t.Fatalf("expected trimmed path, got %v", payload["path"])
	}
	if payload["query"] != "select city, total from data" {
		t.Fatalf("expected normalized query, got %v", payload["query"])
	}
	if payload["target_path"] != "summary.xlsx" {
		t.Fatalf("expected target_path summary.xlsx, got %v", payload["target_path"])
	}
	if payload["sheet"] != "Summary" {
		t.Fatalf("expected sheet Summary, got %v", payload["sheet"])
	}
	if payload["mode"] != "write_range" {
		t.Fatalf("expected mode write_range, got %v", payload["mode"])
	}
	if payload["start_cell"] != "B2" {
		t.Fatalf("expected start_cell B2, got %v", payload["start_cell"])
	}
	if payload["include_header"] != true {
		t.Fatalf("expected include_header=true, got %v", payload["include_header"])
	}
	if payload["create_workbook_if_missing"] != false {
		t.Fatalf("expected create_workbook_if_missing=false, got %v", payload["create_workbook_if_missing"])
	}
	if payload["create_sheet_if_missing"] != true {
		t.Fatalf("expected create_sheet_if_missing=true, got %v", payload["create_sheet_if_missing"])
	}
	if payload["clear_target_range"] != false {
		t.Fatalf("expected clear_target_range=false, got %v", payload["clear_target_range"])
	}
	if _, exists := payload["ignored"]; exists {
		t.Fatalf("unexpected non-stable field in fingerprint: %v", payload)
	}
}

func TestToolHandlerGetStylesUsesPublishedWhenDraftMissing(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "StyleQuery"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "style-query",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "xlsx_get_styles",
			Arguments: `{"path":"book.xlsx","sheet":"Sheet1"}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("xlsx_get_styles: %v", err)
	}
	if worker.method != "XlsxGetStyles" {
		t.Fatalf("expected XlsxGetStyles, got %s", worker.method)
	}
	if worker.params["root"] != "published" {
		t.Fatalf("expected published root without draft, got %v", worker.params["root"])
	}
}

func TestToolHandlerCopyAssetsEnsuresDraftAndDraftRoots(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "AssetCopy"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "asset-copy",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "docx_copy_assets",
			Arguments: `{"source_path":"src.docx","target_path":"dst.docx","assets":[{"type":"paragraph_style","name":"Heading 1"}]}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("docx_copy_assets: %v", err)
	}
	if worker.method != "DocxCopyAssets" {
		t.Fatalf("expected DocxCopyAssets, got %s", worker.method)
	}
	if worker.params["root"] != "draft" || worker.params["source_root"] != "draft" || worker.params["target_root"] != "draft" {
		t.Fatalf("expected draft roots, got %#v", worker.params)
	}
	ds, err := eng.workbenches.DraftState(workbenchID)
	if err != nil || ds == nil {
		t.Fatalf("expected draft to exist")
	}
}

func TestToolHandlerCopyAssetsMapsRemoteValidationError(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &errorToolWorker{
		err: &toolworker.RemoteError{
			Code:    errinfo.CodeValidationFailed,
			Message: "unsupported asset type: chart",
		},
	}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "AssetCopyErr"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "asset-copy-error",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      "pptx_copy_assets",
			Arguments: `{"source_path":"src.pptx","target_path":"dst.pptx","assets":[{"type":"chart","id":"1"}]}`,
		},
	}
	_, execErr := handler.Execute(call)
	if execErr == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(execErr.Error(), errinfo.CodeValidationFailed) {
		t.Fatalf("expected validation code in error, got %v", execErr)
	}
}

func TestToolHandlerDocxOperationsNormalizesFindAlias(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "DocxOpsTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "test-docx",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "docx_operations",
			Arguments: `{
				"path":"report.docx",
				"operations":[
					{"op":"replace_text","find":"Old term","replace":"New term"}
				]
			}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("docx_operations: %v", err)
	}

	if worker.method != "DocxApplyOps" {
		t.Fatalf("expected DocxApplyOps, got %s", worker.method)
	}
	if worker.params["root"] != "draft" {
		t.Fatalf("expected root draft, got %v", worker.params["root"])
	}
	ops, ok := worker.params["ops"].([]any)
	if !ok || len(ops) != 1 {
		t.Fatalf("expected one op, got %#v", worker.params["ops"])
	}
	op, ok := ops[0].(map[string]any)
	if !ok {
		t.Fatalf("expected op object, got %#v", ops[0])
	}
	if got := op["search"]; got != "Old term" {
		t.Fatalf("expected search alias to be normalized, got %v", got)
	}

	ds, err := eng.workbenches.DraftState(workbenchID)
	if err != nil || ds == nil {
		t.Fatalf("expected draft to exist after docx operation")
	}
}

func TestToolHandlerPptxOperationsNormalizesSlideIndexAlias(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &captureToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "PptxOpsTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "test-pptx",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "pptx_operations",
			Arguments: `{
				"path":"deck.pptx",
				"operations":[
					{"op":"set_slide_text","slide_index":2,"title":"Updated","body":"Body text"}
				]
			}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("pptx_operations: %v", err)
	}

	if worker.method != "PptxApplyOps" {
		t.Fatalf("expected PptxApplyOps, got %s", worker.method)
	}
	if worker.params["root"] != "draft" {
		t.Fatalf("expected root draft, got %v", worker.params["root"])
	}
	ops, ok := worker.params["ops"].([]any)
	if !ok || len(ops) != 1 {
		t.Fatalf("expected one op, got %#v", worker.params["ops"])
	}
	op, ok := ops[0].(map[string]any)
	if !ok {
		t.Fatalf("expected op object, got %#v", ops[0])
	}
	indexValue, ok := op["index"].(float64)
	if !ok || indexValue != 2 {
		t.Fatalf("expected index alias to be normalized to 2, got %v", op["index"])
	}

	ds, err := eng.workbenches.DraftState(workbenchID)
	if err != nil || ds == nil {
		t.Fatalf("expected draft to exist after pptx operation")
	}
}

func TestToolHandlerPptxOperationsAddSlideCollectsResolvedFocusHint(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &mapAwareToolWorker{slideCount: 4}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "PptxFocusHint"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	call := llm.ToolCall{
		ID:   "test-pptx-add",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "pptx_operations",
			Arguments: `{
				"path":"deck.pptx",
				"operations":[
					{"op":"add_slide","index":0,"title":"New slide"}
				]
			}`,
		},
	}
	if _, err := handler.Execute(call); err != nil {
		t.Fatalf("pptx_operations: %v", err)
	}

	hints := handler.FocusHints()
	hint, ok := hints["deck.pptx"]
	if !ok {
		t.Fatalf("expected focus hint for deck.pptx, got %v", hints)
	}
	slideIndex, ok := intFromAny(hint["slide_index"])
	if !ok || slideIndex != 3 {
		t.Fatalf("expected slide_index=3, got %v", hint["slide_index"])
	}
	if !strings.Contains(strings.Join(worker.calls, ","), "PptxGetMap") {
		t.Fatalf("expected PptxGetMap call, got %v", worker.calls)
	}
}

func TestToolHandlerFocusHintsLastCallWinsForSamePath(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	defer os.Unsetenv("KEENBENCH_DATA_DIR")

	worker := &mapAwareToolWorker{}
	eng, err := New(WithToolWorker(worker))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "HintOverwrite"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	handler := NewToolHandler(eng, workbenchID, ctx)
	firstCall := llm.ToolCall{
		ID:   "xlsx-1",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "xlsx_operations",
			Arguments: `{
				"path":"metrics.xlsx",
				"operations":[
					{"op":"set_range","sheet":"Q1","start":"A1","values":[["Metric"]]}
				]
			}`,
		},
	}
	if _, err := handler.Execute(firstCall); err != nil {
		t.Fatalf("first xlsx_operations: %v", err)
	}
	secondCall := llm.ToolCall{
		ID:   "xlsx-2",
		Type: "function",
		Function: llm.ToolCallFunction{
			Name: "xlsx_operations",
			Arguments: `{
				"path":"metrics.xlsx",
				"operations":[
					{"op":"ensure_sheet","sheet":"Annual"}
				]
			}`,
		},
	}
	if _, err := handler.Execute(secondCall); err != nil {
		t.Fatalf("second xlsx_operations: %v", err)
	}

	hints := handler.FocusHints()
	hint, ok := hints["metrics.xlsx"]
	if !ok {
		t.Fatalf("expected focus hint for metrics.xlsx, got %v", hints)
	}
	if hint["sheet"] != "Annual" {
		t.Fatalf("expected last hint sheet Annual, got %v", hint["sheet"])
	}
	if _, hasRow := hint["row_start"]; hasRow {
		t.Fatalf("expected sheet-only hint from last call, got %v", hint)
	}
}

func TestBuildToolReceiptSmallResultPassesThrough(t *testing.T) {
	result := `{"row_count":3,"rows":[1,2,3]}`
	receipt := buildToolReceipt("table_query", result, 1)
	if receipt != result {
		t.Fatalf("expected small result to pass through, got %q", receipt)
	}
}

func TestBuildToolReceiptLargeTabularResult(t *testing.T) {
	// Build a large tabular result
	rows := make([]map[string]any, 100)
	for i := range rows {
		rows[i] = map[string]any{
			"id":       i,
			"name":     strings.Repeat("x", 50),
			"category": "cat-" + strings.Repeat("y", 20),
		}
	}
	result := map[string]any{
		"row_count":  100,
		"total_rows": 500,
		"has_more":   true,
		"columns":    []string{"id", "name", "category"},
		"rows":       rows,
	}
	resultJSON, _ := json.Marshal(result)
	receipt := buildToolReceipt("table_query", string(resultJSON), 42)

	if len(receipt) >= len(resultJSON) {
		t.Fatalf("receipt should be smaller than result: receipt=%d result=%d", len(receipt), len(resultJSON))
	}
	if !strings.Contains(receipt, "42") {
		t.Fatalf("receipt should contain entry ID 42")
	}
	if !strings.Contains(receipt, "recall_tool_result") {
		t.Fatalf("receipt should mention recall_tool_result")
	}
	if !strings.Contains(receipt, "100 rows") {
		t.Fatalf("receipt should contain row count")
	}
	if !strings.Contains(receipt, "500 total") {
		t.Fatalf("receipt should contain total_rows")
	}
	if !strings.Contains(receipt, "has_more=true") {
		t.Fatalf("receipt should indicate has_more")
	}
}

func TestBuildToolReceiptActionToolPassesThrough(t *testing.T) {
	// Action tools (write_text_file) should not be receipted
	result := strings.Repeat("x", 5000)
	receipt := buildToolReceipt("write_text_file", result, 1)
	if receipt != result {
		t.Fatalf("action tool result should pass through regardless of size")
	}
}

func TestBuildToolReceiptTextResult(t *testing.T) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d: %s", i, strings.Repeat("data ", 20))
	}
	result := strings.Join(lines, "\n")
	receipt := buildToolReceipt("read_file", result, 7)

	if len(receipt) >= len(result) {
		t.Fatalf("receipt should be smaller than result")
	}
	if !strings.Contains(receipt, "7") {
		t.Fatalf("receipt should contain entry ID")
	}
	if !strings.Contains(receipt, "recall_tool_result") {
		t.Fatalf("receipt should mention recall_tool_result")
	}
}

func TestToolLogAppendAndRead(t *testing.T) {
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx := context.Background()
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "LogTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	// Append entries
	for i := 1; i <= 3; i++ {
		entry := toolLogEntry{
			ID:        i,
			Tool:      "table_query",
			Args:      fmt.Sprintf(`{"query":"SELECT %d"}`, i),
			Result:    fmt.Sprintf(`{"row_count":%d}`, i*10),
			Receipt:   fmt.Sprintf("[Receipt — %d rows]", i*10),
			Timestamp: "2026-02-13T17:24:00Z",
			ElapsedMS: int64(i * 100),
		}
		if err := eng.appendToolLog(workbenchID, entry); err != nil {
			t.Fatalf("append entry %d: %v", i, err)
		}
	}

	// Read back entry 2
	entry, err := eng.readToolLogEntry(workbenchID, 2)
	if err != nil {
		t.Fatalf("read entry 2: %v", err)
	}
	if entry.ID != 2 {
		t.Fatalf("expected ID 2, got %d", entry.ID)
	}
	if entry.Tool != "table_query" {
		t.Fatalf("expected table_query, got %s", entry.Tool)
	}
	if !strings.Contains(entry.Result, "20") {
		t.Fatalf("expected result with 20, got %s", entry.Result)
	}

	// Read non-existent
	_, err = eng.readToolLogEntry(workbenchID, 99)
	if err == nil {
		t.Fatalf("expected error for missing entry")
	}
}

func TestEstimatePayloadBytes(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: strings.Repeat("s", 1000)},
		{Role: "user", Content: strings.Repeat("u", 500)},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{Function: llm.ToolCallFunction{Name: "test", Arguments: strings.Repeat("a", 200)}},
		}},
	}
	est := estimatePayloadBytes(messages)
	// Should be at least 1000 + 500 + 2 + 4 + 200 = 1706
	if est < 1700 {
		t.Fatalf("expected estimate >= 1700, got %d", est)
	}
}

func TestTruncateHistoricalToolResult(t *testing.T) {
	result := strings.Repeat("x", 10000)
	truncated := truncateHistoricalToolResult(result)
	if !strings.Contains(truncated, "10000 bytes") {
		t.Fatalf("expected byte count in truncated result, got %q", truncated)
	}
	if len(truncated) >= len(result) {
		t.Fatalf("truncated should be smaller than original")
	}
}

func TestWorkshopRunAgentBasic(t *testing.T) {
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

	eng.providers[ProviderOpenAI] = &scriptedToolOpenAI{
		responses: []llm.ChatResponse{
			{Content: "Research", FinishReason: "stop"},
			{
				Content: strings.Join([]string{
					"# Execution Plan",
					"",
					"## Task",
					"Basic run",
					"",
					"## Items",
					"- [ ] 1. Complete without edits",
				}, "\n"),
				FinishReason: "stop",
			},
			{Content: "Implemented", FinishReason: "stop"},
		},
		streamResponses: []string{"Hello world"},
	}

	// Track notifications
	var notifications []string
	eng.SetNotifier(func(method string, params any) {
		notifications = append(notifications, method)
	})

	// Setup provider
	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	// Create workbench with a file
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "AgentTest"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	testFile := filepath.Join(dataDir, "data.txt")
	if err := os.WriteFile(testFile, []byte("test data"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{testFile},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	// Grant consent
	consentStatus, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("consent status: %v", errInfo)
	}
	scopeHash := consentStatus.(map[string]any)["scope_hash"].(string)
	if _, errInfo := eng.EgressGrantWorkshopConsent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"provider_id":  "openai",
		"model_id":     ModelOpenAIID,
		"scope_hash":   scopeHash,
		"persist":      true,
	})); errInfo != nil {
		t.Fatalf("grant consent: %v", errInfo)
	}

	// Send user message
	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Process the data file",
	})); errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}

	// Run agent
	resp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}

	result := resp.(map[string]any)
	if result["message_id"] == nil || result["message_id"] == "" {
		t.Fatalf("expected message_id in response")
	}

	// Should have received streaming notifications
	hasStreamDelta := false
	hasComplete := false
	for _, n := range notifications {
		if n == "WorkshopAssistantStreamDelta" {
			hasStreamDelta = true
		}
		if n == "WorkshopAssistantMessageComplete" {
			hasComplete = true
		}
	}
	if !hasStreamDelta {
		t.Errorf("expected WorkshopAssistantStreamDelta notification")
	}
	if !hasComplete {
		t.Errorf("expected WorkshopAssistantMessageComplete notification")
	}
}
