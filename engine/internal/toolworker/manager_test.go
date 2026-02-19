package toolworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestManagerRestartOnCrash(t *testing.T) {
	python := requirePython(t)

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	script := filepath.Join(root, "fake_worker.py")
	code := `import sys, json
for line in sys.stdin:
    if not line.strip():
        continue
    req = json.loads(line)
    mid = req.get("method")
    if mid == "Crash":
        sys.exit(0)
    resp = {"jsonrpc":"2.0","id":req.get("id"),"result":{"ok":True}}
    sys.stdout.write(json.dumps(resp)+"\n")
    sys.stdout.flush()
`
	code = "#!/usr/bin/env " + filepath.Base(python) + "\n" + code
	if err := os.WriteFile(script, []byte(code), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}

	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", script)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var info map[string]any
	if err := mgr.Call(ctx, "WorkerGetInfo", map[string]any{}, &info); err != nil {
		t.Fatalf("call: %v", err)
	}

	crashCtx, crashCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer crashCancel()
	if err := mgr.Call(crashCtx, "Crash", map[string]any{}, &info); err == nil {
		t.Fatalf("expected crash error")
	}

	retryCtx, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer retryCancel()
	if err := mgr.Call(retryCtx, "WorkerGetInfo", map[string]any{}, &info); err != nil {
		t.Fatalf("expected restart, got %v", err)
	}

}

func TestManagerCallTimeout(t *testing.T) {
	python := requirePython(t)

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	script := filepath.Join(root, "sleep_worker.py")
	code := `import sys, json, time
for line in sys.stdin:
    if not line.strip():
        continue
    time.sleep(5)
    req = json.loads(line)
    resp = {"jsonrpc":"2.0","id":req.get("id"),"result":{"ok":True}}
    sys.stdout.write(json.dumps(resp)+"\n")
    sys.stdout.flush()
`
	code = "#!/usr/bin/env " + filepath.Base(python) + "\n" + code
	if err := os.WriteFile(script, []byte(code), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}

	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", script)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var info map[string]any
	err := mgr.Call(ctx, "WorkerGetInfo", map[string]any{}, &info)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestWorkerMergedCells(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-merged"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	xlsxPath := filepath.Join(draftDir, "merged.xlsx")

	makeScript := filepath.Join(root, "make_xlsx.py")
	makeCode := `import openpyxl, sys
wb = openpyxl.Workbook()
ws = wb.active
ws["A1"] = "Merged"
ws.merge_cells("A1:B1")
wb.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, xlsxPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := filepath.Join(root, "worker.sh")
	workerPath := filepath.Clean(filepath.Join("..", "..", "tools", "pyworker", "worker.py"))
	absWorkerPath, err := filepath.Abs(filepath.Join("..", "..", "tools", "pyworker", "worker.py"))
	if err == nil {
		workerPath = absWorkerPath
	}
	wrapperCode := "#!/usr/bin/env bash\nexec " + python + " " + workerPath + "\n"
	if err := os.WriteFile(workerWrapper, []byte(wrapperCode), 0o700); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}

	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	var resp struct {
		Cells [][]map[string]any `json:"cells"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Call(ctx, "XlsxRenderGrid", map[string]any{
		"workbench_id": wbID,
		"path":         "merged.xlsx",
		"root":         "draft",
		"row_start":    0,
		"row_count":    1,
		"col_start":    0,
		"col_count":    2,
	}, &resp); err != nil {
		t.Fatalf("render grid: %v", err)
	}
	if len(resp.Cells) < 1 || len(resp.Cells[0]) < 2 {
		t.Fatalf("unexpected cell grid")
	}
	if resp.Cells[0][0]["value"] != "Merged" {
		t.Fatalf("expected merged value, got %v", resp.Cells[0][0]["value"])
	}
	if _, ok := resp.Cells[0][1]["value"]; ok && resp.Cells[0][1]["value"] != nil {
		t.Fatalf("expected merged follower cell to be null, got %v", resp.Cells[0][1]["value"])
	}
}

func TestWorkerXlsxSummarizeByCategoryTotals(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-xlsx-summary"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	xlsxPath := filepath.Join(draftDir, "quarterly_data.xlsx")

	makeScript := filepath.Join(root, "make_quarterly_xlsx.py")
	makeCode := `import openpyxl, sys
wb = openpyxl.Workbook()
ws = wb.active
ws.title = "Q1"
ws.append(["Date", "Category", "Amount", "Description"])
ws.append(["2026-01-01", "Marketing", 100.0, "Campaign"])
ws.append(["2026-01-02", "Software", 49.99, "Tools"])

ws = wb.create_sheet("Q2")
ws.append(["Date", "Category", "Amount", "Description"])
ws.append(["2026-04-01", "Marketing", 150.0, "Ads"])
ws.append(["2026-04-02", "Office Supplies", 25.0, "Notebooks"])

ws = wb.create_sheet("Q3")
ws.append(["Date", "Category", "Amount", "Description"])
ws.append(["2026-07-01", "Marketing", 175.0, "Conference"])
ws.append(["2026-07-02", "Software", 75.0, "Licenses"])

ws = wb.create_sheet("Q4")
ws.append(["Date", "Category", "Amount", "Description"])
ws.append(["2026-10-01", "Marketing", 200.0, "Campaign"])
ws.append(["2026-10-02", "Office Supplies", 30.0, "Pens"])

wb.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, xlsxPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var applyResp map[string]any
	if err := mgr.Call(ctx, "XlsxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "quarterly_data.xlsx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op":            "summarize_by_category",
				"sheet":         "Annual",
				"source_sheets": []string{"Q1", "Q2", "Q3", "Q4"},
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("XlsxApplyOps: %v", err)
	}

	var readResp struct {
		Text string `json:"text"`
	}
	if err := mgr.Call(ctx, "XlsxReadRange", map[string]any{
		"workbench_id": wbID,
		"path":         "quarterly_data.xlsx",
		"root":         "draft",
		"sheet":        "Annual",
		"range":        "A1:F6",
	}, &readResp); err != nil {
		t.Fatalf("XlsxReadRange: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(readResp.Text), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected summary rows in Annual sheet, got: %q", readResp.Text)
	}
	if lines[1] != "Category\tQ1_Total\tQ2_Total\tQ3_Total\tQ4_Total\tGrand_Total" {
		t.Fatalf("unexpected headers: %q", lines[1])
	}

	expectedRows := []string{
		"Marketing\t100\t150\t175\t200\t625",
		"Office Supplies\t0\t25\t0\t30\t55",
		"Software\t49.99\t0\t75\t0\t124.99",
	}
	for _, row := range expectedRows {
		if !strings.Contains(readResp.Text, row) {
			t.Fatalf("expected row %q in summary output, got: %q", row, readResp.Text)
		}
	}
}

func TestWorkerXlsxApplyOpsNewWorkbookReusesDefaultSheet(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-xlsx-apply-reuse-default"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var applyResp map[string]any
	if err := mgr.Call(ctx, "XlsxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "report.xlsx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op":    "set_range",
				"sheet": "Summary",
				"start": "A1",
				"values": []any{
					[]any{"Metric", "Value"},
				},
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("XlsxApplyOps: %v", err)
	}

	var stylesResp map[string]any
	if err := mgr.Call(ctx, "XlsxGetStyles", map[string]any{
		"workbench_id": wbID,
		"path":         "report.xlsx",
		"root":         "draft",
	}, &stylesResp); err != nil {
		t.Fatalf("XlsxGetStyles: %v", err)
	}

	sheetsRaw, ok := stylesResp["sheets"].([]any)
	if !ok {
		t.Fatalf("expected sheets array, got %T", stylesResp["sheets"])
	}
	if len(sheetsRaw) != 1 {
		t.Fatalf("expected exactly one sheet after creation, got %v", sheetsRaw)
	}
	sheet, ok := sheetsRaw[0].(string)
	if !ok {
		t.Fatalf("expected sheet entry to be string, got %T", sheetsRaw[0])
	}
	if sheet != "Summary" {
		t.Fatalf("expected single sheet Summary, got %q", sheet)
	}
}

func TestWorkerXlsxCopyAssetsNewTargetReusesDefaultSheet(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-xlsx-copy-reuse-default"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	srcPath := filepath.Join(draftDir, "source.xlsx")

	makeScript := filepath.Join(root, "make_source_xlsx.py")
	makeCode := `import sys
import openpyxl

wb = openpyxl.Workbook()
ws = wb.active
ws.title = "Source"
ws["A1"] = "Style Source"
wb.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, srcPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var copyResp map[string]any
	if err := mgr.Call(ctx, "XlsxCopyAssets", map[string]any{
		"workbench_id": wbID,
		"source_path":  "source.xlsx",
		"target_path":  "target.xlsx",
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
		"assets": []map[string]any{
			{
				"type":         "cell_style",
				"source_sheet": "Source",
				"source_cell":  "A1",
				"target_sheet": "Report",
				"target_cell":  "B2",
			},
		},
	}, &copyResp); err != nil {
		t.Fatalf("XlsxCopyAssets: %v", err)
	}

	var stylesResp map[string]any
	if err := mgr.Call(ctx, "XlsxGetStyles", map[string]any{
		"workbench_id": wbID,
		"path":         "target.xlsx",
		"root":         "draft",
	}, &stylesResp); err != nil {
		t.Fatalf("XlsxGetStyles: %v", err)
	}

	sheetsRaw, ok := stylesResp["sheets"].([]any)
	if !ok {
		t.Fatalf("expected sheets array, got %T", stylesResp["sheets"])
	}
	if len(sheetsRaw) != 1 {
		t.Fatalf("expected exactly one sheet after target creation, got %v", sheetsRaw)
	}
	sheet, ok := sheetsRaw[0].(string)
	if !ok {
		t.Fatalf("expected sheet entry to be string, got %T", sheetsRaw[0])
	}
	if sheet != "Report" {
		t.Fatalf("expected single sheet Report, got %q", sheet)
	}
}

func TestWorkerXlsxApplyOpsExistingWorkbookKeepsNonEmptyDefaultSheet(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-xlsx-keep-default"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	existingPath := filepath.Join(draftDir, "existing.xlsx")

	makeScript := filepath.Join(root, "make_existing_xlsx.py")
	makeCode := `import sys
import openpyxl

wb = openpyxl.Workbook()
ws = wb.active
ws["A1"] = "Existing data"
wb.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, existingPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var applyResp map[string]any
	if err := mgr.Call(ctx, "XlsxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "existing.xlsx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op":    "set_cells",
				"sheet": "Summary",
				"cells": []map[string]any{
					{"cell": "A1", "value": "Summary data"},
				},
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("XlsxApplyOps: %v", err)
	}

	var stylesResp map[string]any
	if err := mgr.Call(ctx, "XlsxGetStyles", map[string]any{
		"workbench_id": wbID,
		"path":         "existing.xlsx",
		"root":         "draft",
	}, &stylesResp); err != nil {
		t.Fatalf("XlsxGetStyles: %v", err)
	}

	sheetsRaw, ok := stylesResp["sheets"].([]any)
	if !ok {
		t.Fatalf("expected sheets array, got %T", stylesResp["sheets"])
	}
	if len(sheetsRaw) != 2 {
		t.Fatalf("expected two sheets for existing workbook, got %v", sheetsRaw)
	}
	found := map[string]bool{
		"Sheet":   false,
		"Summary": false,
	}
	for _, raw := range sheetsRaw {
		sheet, ok := raw.(string)
		if !ok {
			t.Fatalf("expected sheet entry to be string, got %T", raw)
		}
		if _, exists := found[sheet]; exists {
			found[sheet] = true
		}
	}
	for sheet, ok := range found {
		if !ok {
			t.Fatalf("expected sheet %q to remain present, got %v", sheet, sheetsRaw)
		}
	}
}

func TestWorkerXlsxGetStylesAndCopyCellStyle(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-xlsx-style-copy"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	srcPath := filepath.Join(draftDir, "source.xlsx")
	dstPath := filepath.Join(draftDir, "target.xlsx")

	makeScript := filepath.Join(root, "make_style_xlsx.py")
	makeCode := `import sys
import openpyxl
from openpyxl.styles import Font, PatternFill

src = openpyxl.Workbook()
ws = src.active
ws.title = "Sheet1"
ws["A1"] = 42
ws["A1"].font = Font(name="Calibri", bold=True)
ws["A1"].fill = PatternFill(fill_type="solid", fgColor="FFCC00")
ws["A1"].number_format = "0.00"
src.save(sys.argv[1])

dst = openpyxl.Workbook()
ws2 = dst.active
ws2.title = "Sheet1"
ws2["B2"] = 0
dst.save(sys.argv[2])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, srcPath, dstPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var copyResp map[string]any
	if err := mgr.Call(ctx, "XlsxCopyAssets", map[string]any{
		"workbench_id": wbID,
		"source_path":  "source.xlsx",
		"target_path":  "target.xlsx",
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
		"assets": []map[string]any{
			{
				"type":         "cell_style",
				"source_sheet": "Sheet1",
				"source_cell":  "A1",
				"target_sheet": "Sheet1",
				"target_cell":  "B2",
			},
		},
	}, &copyResp); err != nil {
		t.Fatalf("XlsxCopyAssets: %v", err)
	}

	var stylesResp map[string]any
	if err := mgr.Call(ctx, "XlsxGetStyles", map[string]any{
		"workbench_id": wbID,
		"path":         "target.xlsx",
		"root":         "draft",
		"sheet":        "Sheet1",
	}, &stylesResp); err != nil {
		t.Fatalf("XlsxGetStyles: %v", err)
	}
	cellAssets, ok := stylesResp["cell_style_assets"].([]any)
	if !ok {
		t.Fatalf("expected cell_style_assets list")
	}
	found := false
	for _, raw := range cellAssets {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if entry["cell"] == "B2" {
			if entry["style_id"] == float64(0) || entry["style_id"] == nil {
				t.Fatalf("expected non-default style_id for B2, got %v", entry["style_id"])
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected copied style asset for B2")
	}
}

func TestWorkerXlsxApplyOpsInlineStyleAndLayoutOps(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-xlsx-inline-style"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	xlsxPath := filepath.Join(draftDir, "styled.xlsx")

	makeScript := filepath.Join(root, "make_styled_xlsx.py")
	makeCode := `import sys
import openpyxl

wb = openpyxl.Workbook()
ws = wb.active
ws.title = "Styled"
wb.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, xlsxPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var applyResp map[string]any
	if err := mgr.Call(ctx, "XlsxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "styled.xlsx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op":    "ensure_sheet",
				"sheet": "Styled",
			},
			{
				"op":    "set_cells",
				"sheet": "Styled",
				"cells": []map[string]any{
					{
						"cell":  "A1",
						"value": "Category",
						"style": map[string]any{
							"font_name":         "Calibri",
							"font_size":         12,
							"font_bold":         true,
							"font_italic":       "bad-bool",
							"font_color":        "#FFFFFF",
							"fill_color":        "#2F5496",
							"fill_pattern":      "solid",
							"h_align":           "center",
							"v_align":           "center",
							"wrap_text":         true,
							"number_format":     "@",
							"border_bottom":     map[string]any{"style": "medium", "color": "#1F3864"},
							"unknown_style_key": "ignored",
						},
					},
				},
			},
			{
				"op":    "set_range",
				"sheet": "Styled",
				"start": "A2",
				"values": []any{
					[]any{123.45},
					[]any{67.89},
				},
				"style": map[string]any{
					"number_format": "#,##0.00",
					"h_align":       "right",
					"wrap_text":     "invalid",
					"fill_color":    "#F2F2F2",
					"border_top":    map[string]any{"style": "invalid-style", "color": "#000000"},
					"border_left":   map[string]any{"style": "thin", "color": "#GGGGGG"},
				},
			},
			{
				"op":    "set_column_widths",
				"sheet": "Styled",
				"columns": []map[string]any{
					{"column": "A", "width": 22.5},
					{"column": "B", "width": "bad"},
					{"column": "bad-col", "width": 10},
				},
			},
			{
				"op":    "set_row_heights",
				"sheet": "Styled",
				"rows": []map[string]any{
					{"row": 1, "height": 25},
					{"row": "bad", "height": 18},
				},
			},
			{
				"op":     "freeze_panes",
				"sheet":  "Styled",
				"row":    1,
				"column": 1,
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("XlsxApplyOps: %v", err)
	}

	verifyScript := filepath.Join(root, "verify_styled_xlsx.py")
	verifyCode := `import json
import sys
import openpyxl

def color_hex(color_obj):
    if color_obj is None:
        return ""
    rgb = getattr(color_obj, "rgb", None)
    if rgb is None:
        return ""
    text = str(rgb)
    return text[-6:].upper()

wb = openpyxl.load_workbook(sys.argv[1])
ws = wb["Styled"]
out = {
    "a1_bold": bool(ws["A1"].font.bold),
    "a1_font_name": ws["A1"].font.name or "",
    "a1_font_size": float(ws["A1"].font.sz or 0),
    "a1_font_color": color_hex(ws["A1"].font.color),
    "a1_fill_pattern": ws["A1"].fill.patternType or "",
    "a1_fill_color": color_hex(ws["A1"].fill.fgColor),
    "a1_h_align": ws["A1"].alignment.horizontal or "",
    "a1_v_align": ws["A1"].alignment.vertical or "",
    "a1_wrap_text": bool(ws["A1"].alignment.wrap_text),
    "a2_number_format": ws["A2"].number_format or "",
    "a2_h_align": ws["A2"].alignment.horizontal or "",
    "column_a_width": float(ws.column_dimensions["A"].width or 0),
    "column_b_width": ws.column_dimensions["B"].width,
    "row1_height": float(ws.row_dimensions[1].height or 0),
    "row2_height": ws.row_dimensions[2].height,
    "freeze_panes": str(ws.freeze_panes) if ws.freeze_panes else "",
}
print(json.dumps(out))
`
	verifyCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + verifyCode
	if err := os.WriteFile(verifyScript, []byte(verifyCode), 0o700); err != nil {
		t.Fatalf("write verify script: %v", err)
	}
	out, err := exec.Command(python, verifyScript, xlsxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("verify xlsx: %v (%s)", err, string(out))
	}
	var verify struct {
		A1Bold        bool     `json:"a1_bold"`
		A1FontName    string   `json:"a1_font_name"`
		A1FontSize    float64  `json:"a1_font_size"`
		A1FontColor   string   `json:"a1_font_color"`
		A1FillPattern string   `json:"a1_fill_pattern"`
		A1FillColor   string   `json:"a1_fill_color"`
		A1HAlign      string   `json:"a1_h_align"`
		A1VAlign      string   `json:"a1_v_align"`
		A1WrapText    bool     `json:"a1_wrap_text"`
		A2NumberFmt   string   `json:"a2_number_format"`
		A2HAlign      string   `json:"a2_h_align"`
		ColumnAWidth  float64  `json:"column_a_width"`
		ColumnBWidth  *float64 `json:"column_b_width"`
		Row1Height    float64  `json:"row1_height"`
		Row2Height    *float64 `json:"row2_height"`
		FreezePanes   string   `json:"freeze_panes"`
	}
	if err := json.Unmarshal(out, &verify); err != nil {
		t.Fatalf("unmarshal verify output: %v (%s)", err, string(out))
	}

	if !verify.A1Bold {
		t.Fatalf("expected A1 bold style")
	}
	if verify.A1FontName != "Calibri" {
		t.Fatalf("expected A1 font name Calibri, got %q", verify.A1FontName)
	}
	if verify.A1FontSize < 11.9 || verify.A1FontSize > 12.1 {
		t.Fatalf("expected A1 font size 12, got %v", verify.A1FontSize)
	}
	if verify.A1FontColor != "FFFFFF" {
		t.Fatalf("expected A1 font color FFFFFF, got %q", verify.A1FontColor)
	}
	if verify.A1FillPattern != "solid" {
		t.Fatalf("expected A1 fill pattern solid, got %q", verify.A1FillPattern)
	}
	if verify.A1FillColor != "2F5496" {
		t.Fatalf("expected A1 fill color 2F5496, got %q", verify.A1FillColor)
	}
	if verify.A1HAlign != "center" || verify.A1VAlign != "center" {
		t.Fatalf("expected A1 center alignment, got h=%q v=%q", verify.A1HAlign, verify.A1VAlign)
	}
	if !verify.A1WrapText {
		t.Fatalf("expected A1 wrap text true")
	}
	if verify.A2NumberFmt != "#,##0.00" {
		t.Fatalf("expected A2 number format #,##0.00, got %q", verify.A2NumberFmt)
	}
	if verify.A2HAlign != "right" {
		t.Fatalf("expected A2 h_align right, got %q", verify.A2HAlign)
	}
	if verify.ColumnAWidth < 22.4 || verify.ColumnAWidth > 22.6 {
		t.Fatalf("expected column A width 22.5, got %v", verify.ColumnAWidth)
	}
	if verify.ColumnBWidth != nil && *verify.ColumnBWidth > 14.9 && *verify.ColumnBWidth < 15.1 {
		t.Fatalf("expected invalid column B width to be skipped, got %v", *verify.ColumnBWidth)
	}
	if verify.Row1Height < 24.9 || verify.Row1Height > 25.1 {
		t.Fatalf("expected row 1 height 25, got %v", verify.Row1Height)
	}
	if verify.Row2Height != nil && *verify.Row2Height > 17.9 && *verify.Row2Height < 18.1 {
		t.Fatalf("expected invalid row 2 height to be skipped, got %v", *verify.Row2Height)
	}
	if verify.FreezePanes != "B2" {
		t.Fatalf("expected freeze panes B2, got %q", verify.FreezePanes)
	}
}

func TestWorkerTabularCSVFlow(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "duckdb") {
		t.Skip("duckdb not available")
	}
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-tabular"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	csvPath := filepath.Join(draftDir, "sales.csv")
	initialCSV := "region,amount,active\nwest,10,true\nwest,5,false\neast,3,true\n"
	if err := os.WriteFile(csvPath, []byte(initialCSV), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mapResp struct {
		Format   string           `json:"format"`
		RowCount int              `json:"row_count"`
		Columns  []map[string]any `json:"columns"`
		Chunks   []map[string]any `json:"chunks"`
		Encoding string           `json:"encoding_detected"`
	}
	if err := mgr.Call(ctx, "TabularGetMap", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
	}, &mapResp); err != nil {
		t.Fatalf("TabularGetMap: %v", err)
	}
	if mapResp.Format != "csv" || mapResp.RowCount != 3 {
		t.Fatalf("unexpected map response: %#v", mapResp)
	}
	if len(mapResp.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(mapResp.Columns))
	}
	if mapResp.Encoding == "" {
		t.Fatalf("expected detected encoding")
	}

	var describeResp struct {
		RowCount int              `json:"row_count"`
		Columns  []map[string]any `json:"columns"`
	}
	if err := mgr.Call(ctx, "TabularDescribe", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
	}, &describeResp); err != nil {
		t.Fatalf("TabularDescribe: %v", err)
	}
	if describeResp.RowCount != 3 || len(describeResp.Columns) != 3 {
		t.Fatalf("unexpected describe response: %#v", describeResp)
	}
	describeByName := map[string]map[string]any{}
	for _, col := range describeResp.Columns {
		name, _ := col["name"].(string)
		if name == "" {
			continue
		}
		describeByName[name] = col
	}
	if describeByName["region"] == nil || int(describeByName["region"]["non_null_count"].(float64)) != 3 {
		t.Fatalf("unexpected region describe stats: %#v", describeByName["region"])
	}
	if describeByName["amount"] == nil || int(describeByName["amount"]["non_null_count"].(float64)) != 3 {
		t.Fatalf("unexpected amount describe stats: %#v", describeByName["amount"])
	}
	if describeByName["active"] == nil || int(describeByName["active"]["non_null_count"].(float64)) != 3 {
		t.Fatalf("unexpected active describe stats: %#v", describeByName["active"])
	}

	var statsResp struct {
		Columns []map[string]any `json:"columns"`
	}
	if err := mgr.Call(ctx, "TabularGetStats", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
	}, &statsResp); err != nil {
		t.Fatalf("TabularGetStats: %v", err)
	}
	if len(statsResp.Columns) != 3 {
		t.Fatalf("expected stats for 3 columns, got %d", len(statsResp.Columns))
	}
	statsByName := map[string]map[string]any{}
	for _, col := range statsResp.Columns {
		name, _ := col["name"].(string)
		if name == "" {
			continue
		}
		statsByName[name] = col
	}
	amountStats := statsByName["amount"]
	if amountStats == nil {
		t.Fatalf("missing stats for amount column")
	}
	if int(amountStats["min"].(float64)) != 3 || int(amountStats["max"].(float64)) != 10 {
		t.Fatalf("unexpected amount min/max stats: %#v", amountStats)
	}
	if int(amountStats["sum"].(float64)) != 18 {
		t.Fatalf("unexpected amount sum stat: %#v", amountStats)
	}
	regionStats := statsByName["region"]
	if regionStats == nil {
		t.Fatalf("missing stats for region column")
	}
	if int(regionStats["min_length"].(float64)) != 4 || int(regionStats["max_length"].(float64)) != 4 {
		t.Fatalf("unexpected region length stats: %#v", regionStats)
	}
	mostCommon, ok := regionStats["most_common"].([]any)
	if !ok || len(mostCommon) != 2 {
		t.Fatalf("expected 2 most_common entries for region, got %#v", regionStats["most_common"])
	}
	firstCommon, ok := mostCommon[0].(map[string]any)
	if !ok || firstCommon["value"] != "west" || int(firstCommon["count"].(float64)) != 2 {
		t.Fatalf("unexpected top region frequency: %#v", mostCommon[0])
	}
	activeStats := statsByName["active"]
	if activeStats == nil {
		t.Fatalf("missing stats for active column")
	}
	if int(activeStats["true_count"].(float64)) != 2 || int(activeStats["false_count"].(float64)) != 1 {
		t.Fatalf("unexpected active boolean stats: %#v", activeStats)
	}
	if _, exists := activeStats["min_length"]; exists {
		t.Fatalf("did not expect string length stats for boolean column: %#v", activeStats)
	}

	var rowsResp struct {
		Columns   []string `json:"columns"`
		Rows      [][]any  `json:"rows"`
		HasMore   bool     `json:"has_more"`
		TotalRows int      `json:"total_rows"`
	}
	if err := mgr.Call(ctx, "TabularReadRows", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
		"row_start":    2,
		"row_count":    2,
	}, &rowsResp); err != nil {
		t.Fatalf("TabularReadRows: %v", err)
	}
	if rowsResp.TotalRows != 3 || len(rowsResp.Rows) != 2 {
		t.Fatalf("unexpected rows response: %#v", rowsResp)
	}
	if len(rowsResp.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(rowsResp.Columns))
	}

	var queryResp struct {
		Columns       []string `json:"columns"`
		Rows          [][]any  `json:"rows"`
		TotalRowCount int      `json:"total_row_count"`
	}
	if err := mgr.Call(ctx, "TabularQuery", map[string]any{
		"workbench_id":  wbID,
		"path":          "sales.csv",
		"root":          "draft",
		"query":         "SELECT region, SUM(amount) AS total FROM data GROUP BY region ORDER BY region",
		"window_rows":   10,
		"window_offset": 0,
	}, &queryResp); err != nil {
		t.Fatalf("TabularQuery: %v", err)
	}
	if len(queryResp.Rows) != 2 || queryResp.TotalRowCount != 2 {
		t.Fatalf("unexpected query response: %#v", queryResp)
	}

	var offsetResp struct {
		Rows          [][]any `json:"rows"`
		TotalRowCount int     `json:"total_row_count"`
		HasMore       bool    `json:"has_more"`
	}
	if err := mgr.Call(ctx, "TabularQuery", map[string]any{
		"workbench_id":  wbID,
		"path":          "sales.csv",
		"root":          "draft",
		"query":         "SELECT region, SUM(amount) AS total FROM data GROUP BY region ORDER BY region",
		"window_rows":   10,
		"window_offset": 10,
	}, &offsetResp); err != nil {
		t.Fatalf("TabularQuery offset window: %v", err)
	}
	if len(offsetResp.Rows) != 0 || offsetResp.TotalRowCount != 2 || offsetResp.HasMore {
		t.Fatalf("unexpected offset query response: %#v", offsetResp)
	}

	var exportResp struct {
		TargetPath string `json:"target_path"`
		Format     string `json:"format"`
	}
	if err := mgr.Call(ctx, "TabularExport", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
		"target_path":  "sales_summary.xlsx",
		"target_root":  "draft",
		"format":       "xlsx",
		"query":        "SELECT region, SUM(amount) AS total FROM data GROUP BY region ORDER BY region",
	}, &exportResp); err != nil {
		t.Fatalf("TabularExport: %v", err)
	}
	if exportResp.TargetPath != "sales_summary.xlsx" || exportResp.Format != "xlsx" {
		t.Fatalf("unexpected export response: %#v", exportResp)
	}
	if _, err := os.Stat(filepath.Join(draftDir, "sales_summary.xlsx")); err != nil {
		t.Fatalf("expected export file: %v", err)
	}

	sum := sha256.Sum256([]byte("sales.csv"))
	key := hex.EncodeToString(sum[:])
	dbPath := filepath.Join(workbenches, wbID, "meta", "tabular", key+".duckdb")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected tabular cache db file: %v", err)
	}

	var overwriteResp struct {
		TargetPath string `json:"target_path"`
		Format     string `json:"format"`
		RowCount   int    `json:"row_count"`
	}
	if err := mgr.Call(ctx, "TabularExport", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
		"target_path":  "sales.csv",
		"target_root":  "draft",
		"format":       "csv",
		"query":        "SELECT region, amount, active FROM data WHERE region <> 'east' ORDER BY rowid",
	}, &overwriteResp); err != nil {
		t.Fatalf("TabularExport overwrite: %v", err)
	}
	if overwriteResp.TargetPath != "sales.csv" || overwriteResp.Format != "csv" || overwriteResp.RowCount != 2 {
		t.Fatalf("unexpected overwrite response: %#v", overwriteResp)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected tabular cache db to be proactively removed after csv export overwrite")
	}

	var afterOverwrite struct {
		RowCount int `json:"row_count"`
	}
	if err := mgr.Call(ctx, "TabularGetMap", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
	}, &afterOverwrite); err != nil {
		t.Fatalf("TabularGetMap after overwrite: %v", err)
	}
	if afterOverwrite.RowCount != 2 {
		t.Fatalf("expected row_count 2 after overwrite, got %d", afterOverwrite.RowCount)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected tabular cache db file to be rebuilt: %v", err)
	}

	updatedCSV := "region,amount,active\nwest,10,true\nwest,5,false\neast,3,true\nnorth,7,false\n"
	if err := os.WriteFile(csvPath, []byte(updatedCSV), 0o600); err != nil {
		t.Fatalf("update csv: %v", err)
	}
	var refreshed struct {
		RowCount int `json:"row_count"`
	}
	if err := mgr.Call(ctx, "TabularGetMap", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
	}, &refreshed); err != nil {
		t.Fatalf("TabularGetMap refresh: %v", err)
	}
	if refreshed.RowCount != 4 {
		t.Fatalf("expected refreshed row_count 4, got %d", refreshed.RowCount)
	}
}

func TestWorkerTabularUpdateFromExportModes(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "duckdb") {
		t.Skip("duckdb not available")
	}
	if !hasPythonModule(python, "openpyxl") {
		t.Skip("openpyxl not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-tabular-update-export"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	csvPath := filepath.Join(draftDir, "sales.csv")
	csvData := "region,amount\nwest,10\neast,3\nnorth,7\n"
	if err := os.WriteFile(csvPath, []byte(csvData), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	xlsxPath := filepath.Join(draftDir, "report.xlsx")

	makeScript := filepath.Join(root, "make_report_xlsx.py")
	makeCode := `import openpyxl
import sys

wb = openpyxl.Workbook()
summary = wb.active
summary.title = "Summary"
summary["A1"] = "old_header"
summary["A2"] = "old_value"
summary["B2"] = 999
keep = wb.create_sheet("KeepMe")
keep["A1"] = "untouched"
wb.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, xlsxPath).CombinedOutput(); err != nil {
		t.Fatalf("make xlsx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	type gridCell struct {
		Value any `json:"value"`
	}
	type gridResp struct {
		Cells [][]gridCell `json:"cells"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	readGrid := func(sheet string, rowStart, rowCount, colStart, colCount int) gridResp {
		t.Helper()
		var resp gridResp
		if err := mgr.Call(ctx, "XlsxRenderGrid", map[string]any{
			"workbench_id": wbID,
			"path":         "report.xlsx",
			"root":         "draft",
			"sheet":        sheet,
			"row_start":    rowStart,
			"row_count":    rowCount,
			"col_start":    colStart,
			"col_count":    colCount,
		}, &resp); err != nil {
			t.Fatalf("XlsxRenderGrid %s: %v", sheet, err)
		}
		return resp
	}

	containsWarning := func(warnings []string, token string) bool {
		for _, warning := range warnings {
			if strings.Contains(warning, token) {
				return true
			}
		}
		return false
	}

	var replaceResp struct {
		TargetPath   string   `json:"target_path"`
		Sheet        string   `json:"sheet"`
		Mode         string   `json:"mode"`
		RowCount     int      `json:"row_count"`
		ColumnCount  int      `json:"column_count"`
		WrittenRange string   `json:"written_range"`
		Warnings     []string `json:"warnings"`
	}
	if err := mgr.Call(ctx, "TabularUpdateFromExport", map[string]any{
		"workbench_id": wbID,
		"path":         "sales.csv",
		"root":         "draft",
		"target_path":  "report.xlsx",
		"target_root":  "draft",
		"sheet":        "Summary",
		"mode":         "replace_sheet",
		"query":        "SELECT region, amount FROM data WHERE region IN ('east', 'west') ORDER BY region",
	}, &replaceResp); err != nil {
		t.Fatalf("TabularUpdateFromExport replace_sheet: %v", err)
	}
	if replaceResp.TargetPath != "report.xlsx" || replaceResp.Sheet != "Summary" || replaceResp.Mode != "replace_sheet" {
		t.Fatalf("unexpected replace response identity: %#v", replaceResp)
	}
	if replaceResp.RowCount != 2 || replaceResp.ColumnCount != 2 {
		t.Fatalf("unexpected replace row/column counts: %#v", replaceResp)
	}
	if replaceResp.WrittenRange != "A1:B3" {
		t.Fatalf("expected replace written_range A1:B3, got %q", replaceResp.WrittenRange)
	}
	if len(replaceResp.Warnings) != 0 {
		t.Fatalf("expected no replace warnings, got %#v", replaceResp.Warnings)
	}

	keepGrid := readGrid("KeepMe", 0, 1, 0, 1)
	if len(keepGrid.Cells) != 1 || len(keepGrid.Cells[0]) != 1 || keepGrid.Cells[0][0].Value != "untouched" {
		t.Fatalf("expected KeepMe sheet to be preserved, got %#v", keepGrid.Cells)
	}

	summaryGrid := readGrid("Summary", 0, 4, 0, 2)
	if summaryGrid.Cells[0][0].Value != "region" || summaryGrid.Cells[0][1].Value != "amount" {
		t.Fatalf("unexpected replace headers: %#v", summaryGrid.Cells[0])
	}
	if summaryGrid.Cells[1][0].Value != "east" || int(summaryGrid.Cells[1][1].Value.(float64)) != 3 {
		t.Fatalf("unexpected replace first row: %#v", summaryGrid.Cells[1])
	}
	if summaryGrid.Cells[2][0].Value != "west" || int(summaryGrid.Cells[2][1].Value.(float64)) != 10 {
		t.Fatalf("unexpected replace second row: %#v", summaryGrid.Cells[2])
	}

	var appendResp struct {
		Mode         string   `json:"mode"`
		RowCount     int      `json:"row_count"`
		ColumnCount  int      `json:"column_count"`
		WrittenRange string   `json:"written_range"`
		Warnings     []string `json:"warnings"`
	}
	if err := mgr.Call(ctx, "TabularUpdateFromExport", map[string]any{
		"workbench_id":   wbID,
		"path":           "sales.csv",
		"root":           "draft",
		"target_path":    "report.xlsx",
		"target_root":    "draft",
		"sheet":          "Summary",
		"mode":           "append_rows",
		"query":          "SELECT region, amount FROM data WHERE region = 'north' ORDER BY region",
		"include_header": true,
	}, &appendResp); err != nil {
		t.Fatalf("TabularUpdateFromExport append_rows: %v", err)
	}
	if appendResp.Mode != "append_rows" || appendResp.RowCount != 1 || appendResp.ColumnCount != 2 {
		t.Fatalf("unexpected append response counts: %#v", appendResp)
	}
	if appendResp.WrittenRange != "A4:B4" {
		t.Fatalf("expected append written_range A4:B4, got %q", appendResp.WrittenRange)
	}
	if !containsWarning(appendResp.Warnings, "header_skipped_on_append") {
		t.Fatalf("expected append header skip warning, got %#v", appendResp.Warnings)
	}

	afterAppend := readGrid("Summary", 0, 5, 0, 2)
	if afterAppend.Cells[3][0].Value != "north" || int(afterAppend.Cells[3][1].Value.(float64)) != 7 {
		t.Fatalf("unexpected appended row: %#v", afterAppend.Cells[3])
	}

	var writeRangeResp struct {
		Mode         string   `json:"mode"`
		RowCount     int      `json:"row_count"`
		ColumnCount  int      `json:"column_count"`
		WrittenRange string   `json:"written_range"`
		Warnings     []string `json:"warnings"`
	}
	if err := mgr.Call(ctx, "TabularUpdateFromExport", map[string]any{
		"workbench_id":       wbID,
		"path":               "sales.csv",
		"root":               "draft",
		"target_path":        "report.xlsx",
		"target_root":        "draft",
		"sheet":              "Summary",
		"mode":               "write_range",
		"start_cell":         "D2",
		"clear_target_range": true,
		"query":              "SELECT region, amount FROM data WHERE region = 'west'",
	}, &writeRangeResp); err != nil {
		t.Fatalf("TabularUpdateFromExport write_range: %v", err)
	}
	if writeRangeResp.Mode != "write_range" || writeRangeResp.RowCount != 1 || writeRangeResp.ColumnCount != 2 {
		t.Fatalf("unexpected write_range response counts: %#v", writeRangeResp)
	}
	if writeRangeResp.WrittenRange != "D2:E3" {
		t.Fatalf("expected write_range written_range D2:E3, got %q", writeRangeResp.WrittenRange)
	}
	if !containsWarning(writeRangeResp.Warnings, "query_has_no_order_by") {
		t.Fatalf("expected no-order warning for write_range query, got %#v", writeRangeResp.Warnings)
	}

	rangeGrid := readGrid("Summary", 1, 2, 3, 2)
	if rangeGrid.Cells[0][0].Value != "region" || rangeGrid.Cells[0][1].Value != "amount" {
		t.Fatalf("unexpected write_range headers: %#v", rangeGrid.Cells[0])
	}
	if rangeGrid.Cells[1][0].Value != "west" || int(rangeGrid.Cells[1][1].Value.(float64)) != 10 {
		t.Fatalf("unexpected write_range data row: %#v", rangeGrid.Cells[1])
	}
}

func TestWorkerTabularQueryTimeoutGuardrail(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "duckdb") {
		t.Skip("duckdb not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-tabular-timeout"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	csvPath := filepath.Join(draftDir, "values.csv")
	builder := strings.Builder{}
	builder.WriteString("value\n")
	for i := 1; i <= 1200; i++ {
		builder.WriteString(strconv.Itoa(i))
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(csvPath, []byte(builder.String()), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	os.Setenv("KEENBENCH_TABULAR_QUERY_TIMEOUT_MS", "100")
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")
	defer os.Unsetenv("KEENBENCH_TABULAR_QUERY_TIMEOUT_MS")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := mgr.Call(ctx, "TabularQuery", map[string]any{
		"workbench_id": wbID,
		"path":         "values.csv",
		"root":         "draft",
		"query": "SELECT SUM(CAST(a.value AS DOUBLE) * CAST(b.value AS DOUBLE) * CAST(c.value AS DOUBLE)) AS total " +
			"FROM data a CROSS JOIN data b CROSS JOIN data c",
		"window_rows":   1,
		"window_offset": 0,
	}, &map[string]any{})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("expected remote error, got %T: %v", err, err)
	}
	if remoteErr.Code != "FILE_READ_FAILED" {
		t.Fatalf("expected FILE_READ_FAILED, got %s (%s)", remoteErr.Code, remoteErr.Message)
	}
	if !strings.Contains(strings.ToLower(remoteErr.Message), "timed out") {
		t.Fatalf("expected timeout message, got %q", remoteErr.Message)
	}
}

func TestWorkerTabularQueryValidationGuards(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "duckdb") {
		t.Skip("duckdb not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-tabular-validation"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	csvPath := filepath.Join(draftDir, "sales.csv")
	if err := os.WriteFile(csvPath, []byte("region,amount\nwest,10\n"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		query     string
		wantError string
	}{
		{
			name:      "rejects path reader table functions",
			query:     "SELECT * FROM read_csv_auto('/etc/passwd')",
			wantError: "path-reading functions are not allowed",
		},
		{
			name:      "rejects multiple statements",
			query:     "SELECT * FROM data; SELECT 1",
			wantError: "single statement",
		},
		{
			name:      "rejects non-select statements",
			query:     "DELETE FROM data",
			wantError: "only SELECT/CTE queries are allowed",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := mgr.Call(ctx, "TabularQuery", map[string]any{
				"workbench_id":  wbID,
				"path":          "sales.csv",
				"root":          "draft",
				"query":         tc.query,
				"window_rows":   10,
				"window_offset": 0,
			}, &map[string]any{})
			if err == nil {
				t.Fatalf("expected query validation error")
			}
			var remoteErr *RemoteError
			if !errors.As(err, &remoteErr) {
				t.Fatalf("expected remote error, got %T: %v", err, err)
			}
			if remoteErr.Code != "VALIDATION_FAILED" {
				t.Fatalf("expected VALIDATION_FAILED, got %s (%s)", remoteErr.Code, remoteErr.Message)
			}
			if !strings.Contains(strings.ToLower(remoteErr.Message), strings.ToLower(tc.wantError)) {
				t.Fatalf("expected error containing %q, got %q", tc.wantError, remoteErr.Message)
			}
		})
	}
}

func TestWorkerDocxGetStylesAndCopyStyle(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "docx") {
		t.Skip("python-docx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-docx-style-copy"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	srcPath := filepath.Join(draftDir, "source.docx")
	dstPath := filepath.Join(draftDir, "target.docx")

	makeScript := filepath.Join(root, "make_style_docx.py")
	makeCode := `import sys
from docx import Document
from docx.enum.style import WD_STYLE_TYPE
from docx.shared import Pt

src = Document()
brand = src.styles.add_style("BrandHeading", WD_STYLE_TYPE.PARAGRAPH)
brand.font.name = "Calibri"
brand.font.size = Pt(18)
brand.font.bold = True
src.add_paragraph("Brand title", style="BrandHeading")
src.save(sys.argv[1])

dst = Document()
dst.add_paragraph("Target body")
dst.save(sys.argv[2])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, srcPath, dstPath).CombinedOutput(); err != nil {
		t.Fatalf("make docx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var copyResp map[string]any
	if err := mgr.Call(ctx, "DocxCopyAssets", map[string]any{
		"workbench_id": wbID,
		"source_path":  "source.docx",
		"target_path":  "target.docx",
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
		"assets": []map[string]any{
			{"type": "paragraph_style", "name": "BrandHeading"},
		},
	}, &copyResp); err != nil {
		t.Fatalf("DocxCopyAssets: %v", err)
	}

	var stylesResp map[string]any
	if err := mgr.Call(ctx, "DocxGetStyles", map[string]any{
		"workbench_id": wbID,
		"path":         "target.docx",
		"root":         "draft",
	}, &stylesResp); err != nil {
		t.Fatalf("DocxGetStyles: %v", err)
	}
	styles, ok := stylesResp["styles"].([]any)
	if !ok {
		t.Fatalf("expected styles list")
	}
	found := false
	for _, raw := range styles {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if entry["name"] == "BrandHeading" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected BrandHeading style in target docx")
	}
}

func TestWorkerDocxApplyOpsRunsAndParagraphFormatting(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "docx") {
		t.Skip("python-docx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-docx-inline-style"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	docxPath := filepath.Join(draftDir, "styled.docx")

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var applyResp map[string]any
	if err := mgr.Call(ctx, "DocxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "styled.docx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op": "set_paragraphs",
				"paragraphs": []map[string]any{
					{
						"text":              "ignored paragraph text",
						"style":             "Heading1",
						"alignment":         "center",
						"space_before":      6,
						"space_after":       12,
						"line_spacing":      1.2,
						"indent_left":       0.25,
						"indent_first_line": "bad-indent",
						"unknown_style_key": "ignored",
						"runs": []map[string]any{
							{
								"text":            "Revenue ",
								"font_name":       "Calibri",
								"font_size":       14,
								"bold":            true,
								"font_color":      "#112233",
								"unknown_run_key": "ignored",
							},
							{
								"text":            "+23%",
								"italic":          true,
								"underline":       true,
								"font_color":      "not-a-color",
								"highlight_color": "yellow",
							},
						},
					},
				},
			},
			{
				"op":           "append_paragraph",
				"text":         "ignored append text",
				"alignment":    "justify",
				"space_after":  8,
				"line_spacing": 1.1,
				"indent_right": 0.2,
				"space_before": "bad-space",
				"runs": []map[string]any{
					{
						"text":            "Next steps",
						"font_name":       "Arial",
						"font_size":       11,
						"bold":            true,
						"highlight_color": "invalid-color",
					},
				},
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("DocxApplyOps: %v", err)
	}

	verifyScript := filepath.Join(root, "verify_styled_docx.py")
	verifyCode := `import json
import sys
from docx import Document
from docx.enum.text import WD_ALIGN_PARAGRAPH

def pt(value):
    if value is None:
        return 0.0
    return round(float(value.pt), 1)

def inches(value):
    if value is None:
        return 0.0
    return round(float(value.inches), 2)

def color_hex(font):
    try:
        if font is None or font.color is None or font.color.rgb is None:
            return ""
        return str(font.color.rgb)[-6:].upper()
    except Exception:
        return ""

doc = Document(sys.argv[1])
paragraphs = [p for p in doc.paragraphs if (p.text or "").strip()]
p0 = paragraphs[0]
p1 = paragraphs[1]
r0 = p0.runs[0]
r1 = p0.runs[1]
p1r0 = p1.runs[0]
out = {
    "paragraph_count": len(paragraphs),
    "p0_text": p0.text,
    "p0_center": p0.alignment == WD_ALIGN_PARAGRAPH.CENTER,
    "p0_space_after": pt(p0.paragraph_format.space_after),
    "p0_indent_left": inches(p0.paragraph_format.left_indent),
    "p0_first_line_is_none": p0.paragraph_format.first_line_indent is None,
    "run0_bold": bool(r0.bold),
    "run0_font_name": r0.font.name or "",
    "run0_font_size": pt(r0.font.size),
    "run0_color": color_hex(r0.font),
    "run1_italic": bool(r1.italic),
    "run1_underline": bool(r1.underline),
    "run1_highlight_set": r1.font.highlight_color is not None,
    "p1_text": p1.text,
    "p1_justify": p1.alignment == WD_ALIGN_PARAGRAPH.JUSTIFY,
    "p1_space_after": pt(p1.paragraph_format.space_after),
    "p1_indent_right": inches(p1.paragraph_format.right_indent),
    "p1_run0_highlight_none": p1r0.font.highlight_color is None,
}
print(json.dumps(out))
`
	verifyCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + verifyCode
	if err := os.WriteFile(verifyScript, []byte(verifyCode), 0o700); err != nil {
		t.Fatalf("write verify script: %v", err)
	}
	out, err := exec.Command(python, verifyScript, docxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("verify docx: %v (%s)", err, string(out))
	}
	var verify struct {
		ParagraphCount      int     `json:"paragraph_count"`
		P0Text              string  `json:"p0_text"`
		P0Center            bool    `json:"p0_center"`
		P0SpaceAfter        float64 `json:"p0_space_after"`
		P0IndentLeft        float64 `json:"p0_indent_left"`
		P0FirstLineIsNone   bool    `json:"p0_first_line_is_none"`
		Run0Bold            bool    `json:"run0_bold"`
		Run0FontName        string  `json:"run0_font_name"`
		Run0FontSize        float64 `json:"run0_font_size"`
		Run0Color           string  `json:"run0_color"`
		Run1Italic          bool    `json:"run1_italic"`
		Run1Underline       bool    `json:"run1_underline"`
		Run1HighlightSet    bool    `json:"run1_highlight_set"`
		P1Text              string  `json:"p1_text"`
		P1Justify           bool    `json:"p1_justify"`
		P1SpaceAfter        float64 `json:"p1_space_after"`
		P1IndentRight       float64 `json:"p1_indent_right"`
		P1Run0HighlightNone bool    `json:"p1_run0_highlight_none"`
	}
	if err := json.Unmarshal(out, &verify); err != nil {
		t.Fatalf("unmarshal verify output: %v (%s)", err, string(out))
	}

	if verify.ParagraphCount < 2 {
		t.Fatalf("expected at least 2 paragraphs, got %d", verify.ParagraphCount)
	}
	if verify.P0Text != "Revenue +23%" {
		t.Fatalf("expected first paragraph text from runs, got %q", verify.P0Text)
	}
	if !verify.P0Center {
		t.Fatalf("expected first paragraph alignment center")
	}
	if verify.P0SpaceAfter < 11.9 || verify.P0SpaceAfter > 12.1 {
		t.Fatalf("expected first paragraph space_after 12, got %v", verify.P0SpaceAfter)
	}
	if verify.P0IndentLeft < 0.24 || verify.P0IndentLeft > 0.26 {
		t.Fatalf("expected first paragraph left indent 0.25, got %v", verify.P0IndentLeft)
	}
	if !verify.P0FirstLineIsNone {
		t.Fatalf("expected invalid first-line indent to be skipped")
	}
	if !verify.Run0Bold || verify.Run0FontName != "Calibri" {
		t.Fatalf("expected first run bold Calibri style, got bold=%v font=%q", verify.Run0Bold, verify.Run0FontName)
	}
	if verify.Run0FontSize < 13.9 || verify.Run0FontSize > 14.1 {
		t.Fatalf("expected first run font size 14, got %v", verify.Run0FontSize)
	}
	if verify.Run0Color != "112233" {
		t.Fatalf("expected first run color 112233, got %q", verify.Run0Color)
	}
	if !verify.Run1Italic || !verify.Run1Underline || !verify.Run1HighlightSet {
		t.Fatalf("expected second run italic+underline+highlight, got italic=%v underline=%v highlight=%v", verify.Run1Italic, verify.Run1Underline, verify.Run1HighlightSet)
	}
	if verify.P1Text != "Next steps" {
		t.Fatalf("expected append paragraph text from runs, got %q", verify.P1Text)
	}
	if !verify.P1Justify {
		t.Fatalf("expected append paragraph alignment justify")
	}
	if verify.P1SpaceAfter < 7.9 || verify.P1SpaceAfter > 8.1 {
		t.Fatalf("expected append paragraph space_after 8, got %v", verify.P1SpaceAfter)
	}
	if verify.P1IndentRight < 0.19 || verify.P1IndentRight > 0.21 {
		t.Fatalf("expected append paragraph indent_right 0.2, got %v", verify.P1IndentRight)
	}
	if !verify.P1Run0HighlightNone {
		t.Fatalf("expected invalid append highlight to be skipped")
	}
}

func TestWorkerPptxCopyTextStyle(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "pptx") {
		t.Skip("python-pptx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-pptx-style-copy"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	srcPath := filepath.Join(draftDir, "source.pptx")
	dstPath := filepath.Join(draftDir, "target.pptx")

	makeScript := filepath.Join(root, "make_style_pptx.py")
	makeCode := `import sys
from pptx import Presentation
from pptx.util import Pt

src = Presentation()
s1 = src.slides.add_slide(src.slide_layouts[1])
s1.shapes.title.text = "Source"
p = s1.placeholders[1].text_frame.paragraphs[0]
r = p.add_run()
r.text = "Styled text"
r.font.name = "Courier New"
r.font.size = Pt(30)
r.font.bold = True
src.save(sys.argv[1])

dst = Presentation()
s2 = dst.slides.add_slide(dst.slide_layouts[1])
s2.shapes.title.text = "Target"
p2 = s2.placeholders[1].text_frame.paragraphs[0]
r2 = p2.add_run()
r2.text = "Target text"
r2.font.name = "Arial"
r2.font.size = Pt(14)
dst.save(sys.argv[2])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, srcPath, dstPath).CombinedOutput(); err != nil {
		t.Fatalf("make pptx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var copyResp map[string]any
	if err := mgr.Call(ctx, "PptxCopyAssets", map[string]any{
		"workbench_id": wbID,
		"source_path":  "source.pptx",
		"target_path":  "target.pptx",
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
		"assets": []map[string]any{
			{
				"type":               "text_style",
				"source_slide_index": 0,
				"source_shape_index": 1,
				"target_slide_index": 0,
				"target_shape_index": 1,
			},
		},
	}, &copyResp); err != nil {
		t.Fatalf("PptxCopyAssets: %v", err)
	}

	var slideResp map[string]any
	if err := mgr.Call(ctx, "PptxGetSlideContent", map[string]any{
		"workbench_id": wbID,
		"path":         "target.pptx",
		"root":         "draft",
		"slide_index":  0,
		"detail":       "positioned",
	}, &slideResp); err != nil {
		t.Fatalf("PptxGetSlideContent: %v", err)
	}
	slide, ok := slideResp["slide"].(map[string]any)
	if !ok {
		t.Fatalf("expected slide payload")
	}
	positioned, ok := slide["positioned"].(map[string]any)
	if !ok {
		t.Fatalf("expected positioned payload")
	}
	positionedShapes, ok := positioned["positioned_shapes"].([]any)
	if !ok || len(positionedShapes) < 2 {
		t.Fatalf("expected positioned shapes, got %v", positioned["positioned_shapes"])
	}
	foundFont := false
	for _, raw := range positionedShapes {
		shape, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		textRuns, ok := shape["text_runs"].([]any)
		if !ok {
			continue
		}
		for _, runRaw := range textRuns {
			run, ok := runRaw.(map[string]any)
			if !ok {
				continue
			}
			if run["font_name"] == "Courier New" {
				foundFont = true
				break
			}
		}
	}
	if !foundFont {
		t.Fatalf("expected copied font_name Courier New in target positioned payload")
	}
}

func TestWorkerPptxApplyOpsRunsAndParagraphFormatting(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "pptx") {
		t.Skip("python-pptx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-pptx-inline-style"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	pptxPath := filepath.Join(draftDir, "styled.pptx")

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var applyResp map[string]any
	if err := mgr.Call(ctx, "PptxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "styled.pptx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op":           "add_slide",
				"layout":       "title_and_content",
				"title":        "ignored title",
				"body":         "ignored body",
				"alignment":    "center",
				"space_after":  6,
				"line_spacing": 1.1,
				"space_before": "bad-space",
				"title_runs": []map[string]any{
					{
						"text":            "Q4 ",
						"font_name":       "Arial",
						"font_size":       28,
						"bold":            true,
						"font_color":      "#1A1A1A",
						"unknown_run_key": "ignored",
					},
					{
						"text":       "Results",
						"italic":     true,
						"font_color": "bad-color",
					},
				},
				"body_runs": []map[string]any{
					{
						"text":       "Revenue: ",
						"font_name":  "Calibri",
						"font_size":  18,
						"font_color": "#1A1A1A",
					},
					{
						"text":            "$4.2M",
						"bold":            true,
						"underline":       true,
						"font_color":      "#2E7D32",
						"highlight_color": "yellow",
					},
				},
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("PptxApplyOps: %v", err)
	}

	verifyScript := filepath.Join(root, "verify_styled_pptx.py")
	verifyCode := `import json
import sys
from pptx import Presentation
from pptx.enum.text import PP_ALIGN

def find_body_shape(slide):
    title_shape = slide.shapes.title
    for shape in slide.placeholders:
        if shape is title_shape or not getattr(shape, "has_text_frame", False):
            continue
        return shape
    for shape in slide.shapes:
        if shape is title_shape or not getattr(shape, "has_text_frame", False):
            continue
        return shape
    return None

def color_hex(font):
    try:
        if font is None or font.color is None or font.color.rgb is None:
            return ""
        return str(font.color.rgb)[-6:].upper()
    except Exception:
        return ""

def pt_value(value):
    if value is None:
        return 0.0
    if hasattr(value, "pt"):
        return round(float(value.pt), 1)
    return round(float(value), 2)

def line_spacing_value(value):
    if value is None:
        return 0.0
    try:
        return round(float(value), 2)
    except Exception:
        return pt_value(value)

prs = Presentation(sys.argv[1])
slide = prs.slides[0]
title = slide.shapes.title
body = find_body_shape(slide)
title_p = title.text_frame.paragraphs[0]
body_p = body.text_frame.paragraphs[0]
title_runs = list(title_p.runs)
body_runs = list(body_p.runs)

out = {
    "slide_count": len(prs.slides),
    "title_text": title.text or "",
    "title_center": title_p.alignment == PP_ALIGN.CENTER,
    "title_run_count": len(title_runs),
    "title_run0_bold": bool(title_runs[0].font.bold),
    "title_run0_font_name": title_runs[0].font.name or "",
    "title_run0_font_size": pt_value(title_runs[0].font.size),
    "title_run0_color": color_hex(title_runs[0].font),
    "title_run1_italic": bool(title_runs[1].font.italic),
    "body_text": body.text or "",
    "body_center": body_p.alignment == PP_ALIGN.CENTER,
    "body_run_count": len(body_runs),
    "body_run0_font_name": body_runs[0].font.name or "",
    "body_run1_bold": bool(body_runs[1].font.bold),
    "body_run1_underline": bool(body_runs[1].font.underline),
    "body_run1_color": color_hex(body_runs[1].font),
    "body_space_after": pt_value(body_p.space_after),
    "body_line_spacing": line_spacing_value(body_p.line_spacing),
}
print(json.dumps(out))
`
	verifyCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + verifyCode
	if err := os.WriteFile(verifyScript, []byte(verifyCode), 0o700); err != nil {
		t.Fatalf("write verify script: %v", err)
	}
	out, err := exec.Command(python, verifyScript, pptxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("verify pptx: %v (%s)", err, string(out))
	}
	var verify struct {
		SlideCount        int     `json:"slide_count"`
		TitleText         string  `json:"title_text"`
		TitleCenter       bool    `json:"title_center"`
		TitleRunCount     int     `json:"title_run_count"`
		TitleRun0Bold     bool    `json:"title_run0_bold"`
		TitleRun0Name     string  `json:"title_run0_font_name"`
		TitleRun0Size     float64 `json:"title_run0_font_size"`
		TitleRun0Color    string  `json:"title_run0_color"`
		TitleRun1Italic   bool    `json:"title_run1_italic"`
		BodyText          string  `json:"body_text"`
		BodyCenter        bool    `json:"body_center"`
		BodyRunCount      int     `json:"body_run_count"`
		BodyRun0Name      string  `json:"body_run0_font_name"`
		BodyRun1Bold      bool    `json:"body_run1_bold"`
		BodyRun1Underline bool    `json:"body_run1_underline"`
		BodyRun1Color     string  `json:"body_run1_color"`
		BodySpaceAfter    float64 `json:"body_space_after"`
		BodyLineSpacing   float64 `json:"body_line_spacing"`
	}
	if err := json.Unmarshal(out, &verify); err != nil {
		t.Fatalf("unmarshal verify output: %v (%s)", err, string(out))
	}

	if verify.SlideCount != 1 {
		t.Fatalf("expected 1 slide, got %d", verify.SlideCount)
	}
	if verify.TitleText != "Q4 Results" {
		t.Fatalf("expected title from title_runs, got %q", verify.TitleText)
	}
	if !verify.TitleCenter {
		t.Fatalf("expected centered title paragraph")
	}
	if verify.TitleRunCount != 2 {
		t.Fatalf("expected 2 title runs, got %d", verify.TitleRunCount)
	}
	if !verify.TitleRun0Bold || verify.TitleRun0Name != "Arial" {
		t.Fatalf("expected first title run styled bold Arial, got bold=%v name=%q", verify.TitleRun0Bold, verify.TitleRun0Name)
	}
	if verify.TitleRun0Size < 27.9 || verify.TitleRun0Size > 28.1 {
		t.Fatalf("expected first title run size 28, got %v", verify.TitleRun0Size)
	}
	if verify.TitleRun0Color != "1A1A1A" {
		t.Fatalf("expected first title run color 1A1A1A, got %q", verify.TitleRun0Color)
	}
	if !verify.TitleRun1Italic {
		t.Fatalf("expected second title run italic")
	}
	if verify.BodyText != "Revenue: $4.2M" {
		t.Fatalf("expected body from body_runs, got %q", verify.BodyText)
	}
	if !verify.BodyCenter {
		t.Fatalf("expected centered body paragraph")
	}
	if verify.BodyRunCount != 2 {
		t.Fatalf("expected 2 body runs, got %d", verify.BodyRunCount)
	}
	if verify.BodyRun0Name != "Calibri" {
		t.Fatalf("expected first body run font Calibri, got %q", verify.BodyRun0Name)
	}
	if !verify.BodyRun1Bold || !verify.BodyRun1Underline {
		t.Fatalf("expected second body run bold+underline, got bold=%v underline=%v", verify.BodyRun1Bold, verify.BodyRun1Underline)
	}
	if verify.BodyRun1Color != "2E7D32" {
		t.Fatalf("expected second body run color 2E7D32, got %q", verify.BodyRun1Color)
	}
	if verify.BodySpaceAfter < 5.9 || verify.BodySpaceAfter > 6.1 {
		t.Fatalf("expected body space_after 6, got %v", verify.BodySpaceAfter)
	}
	if verify.BodyLineSpacing < 1.09 || verify.BodyLineSpacing > 1.11 {
		t.Fatalf("expected body line_spacing 1.1, got %v", verify.BodyLineSpacing)
	}
}

func TestWorkerDocxSectionContentDeterministic(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "docx") {
		t.Skip("python-docx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-docx"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	docxPath := filepath.Join(draftDir, "report.docx")

	makeScript := filepath.Join(root, "make_docx.py")
	makeCode := `import sys
from docx import Document

doc = Document()
doc.add_heading("Executive Summary", level=1)
paragraph = doc.add_paragraph("Revenue increased by ")
run = paragraph.add_run("12%")
run.bold = True
table = doc.add_table(rows=2, cols=2)
table.cell(0, 0).text = "Metric"
table.cell(0, 1).text = "Value"
table.cell(1, 0).text = "Revenue"
table.cell(1, 1).text = "120"
doc.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, docxPath).CombinedOutput(); err != nil {
		t.Fatalf("make docx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	call := func() map[string]any {
		t.Helper()
		var resp map[string]any
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mgr.Call(ctx, "DocxGetSectionContent", map[string]any{
			"workbench_id":  wbID,
			"path":          "report.docx",
			"root":          "draft",
			"section_index": 0,
		}, &resp); err != nil {
			t.Fatalf("DocxGetSectionContent: %v", err)
		}
		return resp
	}

	first := call()
	second := call()

	if got := int(first["section_count"].(float64)); got < 1 {
		t.Fatalf("expected section_count >= 1, got %d", got)
	}
	section, ok := first["section"].(map[string]any)
	if !ok {
		t.Fatalf("missing section payload")
	}
	if _, ok := section["paragraphs"].([]any); !ok {
		t.Fatalf("expected paragraphs list in section payload")
	}
	if _, ok := section["tables"].([]any); !ok {
		t.Fatalf("expected tables list in section payload")
	}
	if _, ok := section["images"].([]any); !ok {
		t.Fatalf("expected images list in section payload")
	}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first payload: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second payload: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("expected deterministic payloads, got %s vs %s", string(firstJSON), string(secondJSON))
	}
}

func TestWorkerPptxSlideContentDeterministic(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "pptx") {
		t.Skip("python-pptx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-pptx"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	pptxPath := filepath.Join(draftDir, "slides.pptx")

	makeScript := filepath.Join(root, "make_pptx.py")
	makeCode := `import sys
from pptx import Presentation

prs = Presentation()
slide = prs.slides.add_slide(prs.slide_layouts[1])
slide.shapes.title.text = "Overview"
slide.placeholders[1].text = "Revenue\nMargin"
prs.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, pptxPath).CombinedOutput(); err != nil {
		t.Fatalf("make pptx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	call := func() map[string]any {
		t.Helper()
		var resp map[string]any
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mgr.Call(ctx, "PptxGetSlideContent", map[string]any{
			"workbench_id": wbID,
			"path":         "slides.pptx",
			"root":         "draft",
			"slide_index":  0,
			"detail":       "positioned",
		}, &resp); err != nil {
			t.Fatalf("PptxGetSlideContent: %v", err)
		}
		return resp
	}

	first := call()
	second := call()

	if got := int(first["slide_count"].(float64)); got < 1 {
		t.Fatalf("expected slide_count >= 1, got %d", got)
	}
	slide, ok := first["slide"].(map[string]any)
	if !ok {
		t.Fatalf("missing slide payload")
	}
	shapes, ok := slide["shapes"].([]any)
	if !ok || len(shapes) == 0 {
		t.Fatalf("expected shapes payload")
	}
	shape, ok := shapes[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first shape map")
	}
	if _, ok := shape["text_blocks"].([]any); !ok {
		t.Fatalf("expected text_blocks in shape payload")
	}
	positioned, ok := slide["positioned"].(map[string]any)
	if !ok {
		t.Fatalf("expected positioned payload in slide")
	}
	positionedShapes, ok := positioned["positioned_shapes"].([]any)
	if !ok || len(positionedShapes) == 0 {
		t.Fatalf("expected positioned_shapes payload")
	}
	firstPos, ok := positionedShapes[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first positioned shape map")
	}
	bounds, ok := firstPos["bounds"].(map[string]any)
	if !ok {
		t.Fatalf("expected positioned bounds map")
	}
	if _, ok := bounds["x"]; !ok {
		t.Fatalf("expected bounds.x")
	}
	if _, ok := bounds["width"]; !ok {
		t.Fatalf("expected bounds.width")
	}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first payload: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second payload: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("expected deterministic payloads, got %s vs %s", string(firstJSON), string(secondJSON))
	}
}

func TestWorkerDocxReplaceTextIncludesTableCells(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "docx") {
		t.Skip("python-docx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-docx-table-replace"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	docxPath := filepath.Join(draftDir, "invoice_template.docx")

	makeScript := filepath.Join(root, "make_invoice_docx.py")
	makeCode := `import sys
from docx import Document

doc = Document()
doc.add_heading("Invoice", 0)
doc.add_paragraph("Company: {{company}}")
doc.add_paragraph("Date: {{date}}")
table = doc.add_table(rows=2, cols=4)
table.cell(0, 0).text = "Item"
table.cell(0, 1).text = "Quantity"
table.cell(0, 2).text = "Unit Price"
table.cell(0, 3).text = "Subtotal"
table.cell(1, 0).text = "{{items_table}}"
doc.add_paragraph("Total: {{total}}")
doc.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, docxPath).CombinedOutput(); err != nil {
		t.Fatalf("make docx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var applyResp map[string]any
	if err := mgr.Call(ctx, "DocxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "invoice_template.docx",
		"root":         "draft",
		"ops": []map[string]any{
			{"op": "replace_text", "search": "{{company}}", "replace": "Acme Corp"},
			{"op": "replace_text", "search": "{{date}}", "replace": "2026-03-01"},
			{"op": "replace_text", "search": "{{items_table}}", "replace": "Widget | 50 | 24.99 | 1249.50"},
			{"op": "replace_text", "search": "{{total}}", "replace": "1249.50"},
		},
	}, &applyResp); err != nil {
		t.Fatalf("DocxApplyOps: %v", err)
	}

	var textResp map[string]any
	if err := mgr.Call(ctx, "DocxExtractText", map[string]any{
		"workbench_id": wbID,
		"path":         "invoice_template.docx",
		"root":         "draft",
	}, &textResp); err != nil {
		t.Fatalf("DocxExtractText: %v", err)
	}

	text, _ := textResp["text"].(string)
	if strings.Contains(text, "{{items_table}}") {
		t.Fatalf("expected table placeholder to be replaced, got text: %q", text)
	}
	if !strings.Contains(text, "Widget | 50 | 24.99 | 1249.50") {
		t.Fatalf("expected replaced table content in extracted text, got: %q", text)
	}
}

func TestWorkerDocxExtractSectionIncludesTableRows(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "docx") {
		t.Skip("python-docx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-docx-section-table"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	docxPath := filepath.Join(draftDir, "invoice_template.docx")

	makeScript := filepath.Join(root, "make_section_docx.py")
	makeCode := `import sys
from docx import Document

doc = Document()
doc.add_heading("Invoice", 1)
doc.add_paragraph("Company: {{company}}")
table = doc.add_table(rows=2, cols=2)
table.cell(0, 0).text = "Item"
table.cell(0, 1).text = "Quantity"
table.cell(1, 0).text = "{{items_table}}"
table.cell(1, 1).text = ""
doc.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, docxPath).CombinedOutput(); err != nil {
		t.Fatalf("make docx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var resp map[string]any
	if err := mgr.Call(ctx, "DocxExtractText", map[string]any{
		"workbench_id": wbID,
		"path":         "invoice_template.docx",
		"root":         "draft",
		"section":      "Invoice",
	}, &resp); err != nil {
		t.Fatalf("DocxExtractText: %v", err)
	}

	text, _ := resp["text"].(string)
	if !strings.Contains(text, "{{items_table}}") {
		t.Fatalf("expected section extraction to include table placeholder text, got: %q", text)
	}
}

func TestWorkerPptxAddSlideWritesBodyTextForObjectPlaceholder(t *testing.T) {
	python := requirePython(t)
	if !hasPythonModule(python, "pptx") {
		t.Skip("python-pptx not available")
	}

	root := t.TempDir()
	workbenches := filepath.Join(root, "workbenches")
	if err := os.MkdirAll(workbenches, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wbID := "wb-pptx-body-text"
	draftDir := filepath.Join(workbenches, wbID, "draft")
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatalf("draft dir: %v", err)
	}
	pptxPath := filepath.Join(draftDir, "slides.pptx")

	makeScript := filepath.Join(root, "make_base_pptx.py")
	makeCode := `import sys
from pptx import Presentation

prs = Presentation()
slide = prs.slides.add_slide(prs.slide_layouts[1])
slide.shapes.title.text = "Overview"
slide.placeholders[1].text = "Current status"
prs.save(sys.argv[1])
`
	makeCode = "#!/usr/bin/env " + filepath.Base(python) + "\n" + makeCode
	if err := os.WriteFile(makeScript, []byte(makeCode), 0o700); err != nil {
		t.Fatalf("write make script: %v", err)
	}
	if out, err := exec.Command(python, makeScript, pptxPath).CombinedOutput(); err != nil {
		t.Fatalf("make pptx: %v (%s)", err, string(out))
	}

	workerWrapper := makeWorkerWrapper(t, root, python)
	os.Setenv("KEENBENCH_TOOL_WORKER_PATH", workerWrapper)
	os.Setenv("KEENBENCH_WORKBENCHES_DIR", workbenches)
	defer os.Unsetenv("KEENBENCH_TOOL_WORKER_PATH")
	defer os.Unsetenv("KEENBENCH_WORKBENCHES_DIR")

	mgr := New(workbenches, nil)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var applyResp map[string]any
	if err := mgr.Call(ctx, "PptxApplyOps", map[string]any{
		"workbench_id": wbID,
		"path":         "slides.pptx",
		"root":         "draft",
		"ops": []map[string]any{
			{
				"op":     "add_slide",
				"layout": "Title and Content",
				"title":  "Next Steps",
				"body":   "1) Finalize Q1 priorities and owners\n2) Share timeline and milestones with stakeholders\n3) Schedule follow-up review meeting",
			},
		},
	}, &applyResp); err != nil {
		t.Fatalf("PptxApplyOps: %v", err)
	}

	var textResp map[string]any
	if err := mgr.Call(ctx, "PptxExtractText", map[string]any{
		"workbench_id": wbID,
		"path":         "slides.pptx",
		"root":         "draft",
		"slide_index":  1,
	}, &textResp); err != nil {
		t.Fatalf("PptxExtractText: %v", err)
	}

	text, _ := textResp["text"].(string)
	if !strings.Contains(text, "Next Steps") {
		t.Fatalf("expected title in new slide text, got: %q", text)
	}
	if !strings.Contains(text, "Finalize Q1 priorities and owners") ||
		!strings.Contains(text, "Share timeline and milestones with stakeholders") ||
		!strings.Contains(text, "Schedule follow-up review meeting") {
		t.Fatalf("expected body action items in new slide text, got: %q", text)
	}
}

func makeWorkerWrapper(t *testing.T, root, python string) string {
	t.Helper()
	workerWrapper := filepath.Join(root, "worker.sh")
	workerPath := filepath.Clean(filepath.Join("..", "..", "tools", "pyworker", "worker.py"))
	absWorkerPath, err := filepath.Abs(filepath.Join("..", "..", "tools", "pyworker", "worker.py"))
	if err == nil {
		workerPath = absWorkerPath
	}
	wrapperCode := "#!/usr/bin/env bash\nexec " + python + " " + workerPath + "\n"
	if err := os.WriteFile(workerWrapper, []byte(wrapperCode), 0o700); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	return workerWrapper
}

func requirePython(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("python3"); err == nil {
		return path
	}
	if path, err := exec.LookPath("python"); err == nil {
		return path
	}
	t.Skip("python not available")
	return ""
}

func hasPythonModule(python, module string) bool {
	cmd := exec.Command(python, "-c", "import "+module)
	return cmd.Run() == nil
}
