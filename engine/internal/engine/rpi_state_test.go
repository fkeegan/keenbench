package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func setupRPITestEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("KEENBENCH_DATA_DIR", dataDir)
	t.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")

	eng, err := New()
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(context.Background(), mustJSON(t, map[string]any{
		"name": "RPI",
	}))
	if errInfo != nil {
		t.Fatalf("create workbench: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	return eng, workbenchID
}

func TestRPIStateReadParseAndMalformedHandling(t *testing.T) {
	eng, workbenchID := setupRPITestEngine(t)

	state := eng.readRPIState(workbenchID)
	if state.HasResearch {
		t.Fatalf("expected HasResearch=false")
	}
	if state.HasPlan {
		t.Fatalf("expected HasPlan=false")
	}
	if state.AllDone {
		t.Fatalf("expected AllDone=false without plan")
	}

	if err := eng.writeRPIArtifact(workbenchID, rpiResearchFile, "research"); err != nil {
		t.Fatalf("write research: %v", err)
	}
	if err := eng.writeRPIArtifact(workbenchID, rpiPlanFile, strings.Join([]string{
		"<!-- original_count: 3 -->",
		"- [ ] 1. Item A — do A",
		"- [x] 1. Item A duplicate label — already done",
		"- [!] 4. Item D — failed previously [Failed: timeout]",
	}, "\n")); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	state = eng.readRPIState(workbenchID)
	if !state.HasResearch {
		t.Fatalf("expected HasResearch=true")
	}
	if !state.HasPlan {
		t.Fatalf("expected HasPlan=true")
	}
	if state.AllDone {
		t.Fatalf("expected AllDone=false with pending item")
	}
	if state.OriginalCount != 3 {
		t.Fatalf("expected OriginalCount=3, got %d", state.OriginalCount)
	}
	if len(state.PlanItems) != 3 {
		t.Fatalf("expected 3 plan items, got %d", len(state.PlanItems))
	}
	if state.PlanItems[0].Index != 1 || state.PlanItems[0].Status != rpiStatusPending {
		t.Fatalf("unexpected first item: %#v", state.PlanItems[0])
	}
	if state.PlanItems[1].Index != 1 || state.PlanItems[1].Status != rpiStatusDone {
		t.Fatalf("unexpected second item: %#v", state.PlanItems[1])
	}
	if state.PlanItems[2].Index != 4 || state.PlanItems[2].Status != rpiStatusFailed {
		t.Fatalf("unexpected third item: %#v", state.PlanItems[2])
	}
	if strings.Contains(state.PlanItems[2].Label, "[Failed:") {
		t.Fatalf("label should not include failure suffix: %q", state.PlanItems[2].Label)
	}

	if err := eng.writeRPIArtifact(workbenchID, rpiPlanFile, "# Execution Plan\n\nNo checklist"); err != nil {
		t.Fatalf("write malformed plan: %v", err)
	}
	state = eng.readRPIState(workbenchID)
	if !state.HasPlan {
		t.Fatalf("expected HasPlan=true for existing malformed plan")
	}
	if len(state.PlanItems) != 0 {
		t.Fatalf("expected 0 parsed items, got %d", len(state.PlanItems))
	}
	if !state.AllDone {
		t.Fatalf("expected AllDone=true for zero actionable plan items")
	}
}

func TestRPIMarkPlanItemAndClearState(t *testing.T) {
	eng, workbenchID := setupRPITestEngine(t)
	plan := strings.Join([]string{
		"<!-- original_count: 2 -->",
		"- [ ] 1. First item — pending",
		"- [ ] 1. Duplicate label index — pending",
	}, "\n")
	if err := eng.writeRPIArtifact(workbenchID, rpiPlanFile, plan); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	if err := eng.markPlanItem(workbenchID, 1, rpiStatusFailed, "timed out"); err != nil {
		t.Fatalf("mark second item failed: %v", err)
	}
	updated, err := eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read updated plan: %v", err)
	}
	if !strings.Contains(updated, "- [!] 1. Duplicate label index — pending [Failed: timed out]") {
		t.Fatalf("expected failed marker on second item, got:\n%s", updated)
	}
	if !strings.Contains(updated, "- [ ] 1. First item — pending") {
		t.Fatalf("expected first item untouched, got:\n%s", updated)
	}

	if err := eng.markPlanItem(workbenchID, 0, rpiStatusDone, ""); err != nil {
		t.Fatalf("mark first item done: %v", err)
	}
	state := eng.readRPIState(workbenchID)
	if len(state.PlanItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(state.PlanItems))
	}
	if state.PlanItems[0].Status != rpiStatusDone {
		t.Fatalf("expected first item done, got %s", state.PlanItems[0].Status)
	}
	if state.PlanItems[1].Status != rpiStatusFailed {
		t.Fatalf("expected second item failed, got %s", state.PlanItems[1].Status)
	}
	if !state.AllDone {
		t.Fatalf("expected AllDone=true with done+failed only")
	}

	if err := eng.clearRPIState(workbenchID); err != nil {
		t.Fatalf("clear state: %v", err)
	}
	if _, err := os.Stat(eng.rpiDir(workbenchID)); !os.IsNotExist(err) {
		t.Fatalf("expected _rpi dir removed, stat err=%v", err)
	}
	state = eng.readRPIState(workbenchID)
	if state.HasResearch || state.HasPlan || state.AllDone {
		t.Fatalf("expected cleared state, got %#v", state)
	}
}

func TestRPIAppendPlanItemsInflationGuardAndExtraction(t *testing.T) {
	eng, workbenchID := setupRPITestEngine(t)
	initial := strings.Join([]string{
		"<!-- original_count: 2 -->",
		"# Execution Plan",
		"",
		"## Items",
		"- [ ] 1. First",
		"- [ ] 2. Second",
		"",
		"## Notes",
		"- Keep sorted",
	}, "\n")
	if err := eng.writeRPIArtifact(workbenchID, rpiPlanFile, initial); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	if err := eng.appendPlanItems(workbenchID, []string{
		"- [ ] 3. Third",
		"- [ ] 4. Fourth",
		"- [ ] 5. Fifth (should drop)",
	}); err != nil {
		t.Fatalf("append plan items: %v", err)
	}
	updated, err := eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read updated plan: %v", err)
	}
	if strings.Contains(updated, "5. Fifth") {
		t.Fatalf("expected inflation guard to drop excess item, got:\n%s", updated)
	}
	if !strings.Contains(updated, "- [ ] 3. Third\n- [ ] 4. Fourth\n\n## Notes") {
		t.Fatalf("expected appended items after last existing item, got:\n%s", updated)
	}

	if err := eng.writeRPIArtifact(workbenchID, rpiPlanFile, "- [ ] 1. One\n"); err != nil {
		t.Fatalf("rewrite plan without metadata: %v", err)
	}
	if err := eng.appendPlanItems(workbenchID, []string{"- [ ] 2. Two"}); err != nil {
		t.Fatalf("append without metadata: %v", err)
	}
	updated, err = eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read updated plan without metadata: %v", err)
	}
	if !strings.Contains(updated, "- [ ] 2. Two") {
		t.Fatalf("expected append to proceed when original_count metadata missing, got:\n%s", updated)
	}

	items := extractNewPlanItems(strings.Join([]string{
		"Implementation complete.",
		"- [x] 1. done",
		"- [ ] 2. follow-up task",
		"  - [ ] nested should not match",
		"- [ ] 3. another follow-up",
	}, "\n"))
	if len(items) != 2 {
		t.Fatalf("expected 2 extracted items, got %d: %#v", len(items), items)
	}
	if items[0] != "- [ ] 2. follow-up task" || items[1] != "- [ ] 3. another follow-up" {
		t.Fatalf("unexpected extracted items: %#v", items)
	}
}

func TestRPICurrentToolLogSeq(t *testing.T) {
	eng, workbenchID := setupRPITestEngine(t)
	if seq := eng.currentToolLogSeq(workbenchID); seq != 0 {
		t.Fatalf("expected empty log seq 0, got %d", seq)
	}

	if err := eng.appendToolLog(workbenchID, toolLogEntry{ID: 1, Tool: "read_file"}); err != nil {
		t.Fatalf("append log 1: %v", err)
	}
	if err := eng.appendToolLog(workbenchID, toolLogEntry{ID: 3, Tool: "table_query"}); err != nil {
		t.Fatalf("append log 3: %v", err)
	}
	if err := eng.appendToolLog(workbenchID, toolLogEntry{ID: 2, Tool: "write_text_file"}); err != nil {
		t.Fatalf("append log 2: %v", err)
	}
	if seq := eng.currentToolLogSeq(workbenchID); seq != 3 {
		t.Fatalf("expected seq 3, got %d", seq)
	}

	logPath := filepath.Join(eng.workbenchesRoot(), workbenchID, "meta", "workshop", "tool_log.jsonl")
	if err := os.WriteFile(logPath, []byte("{malformed}\n"), 0o600); err != nil {
		t.Fatalf("write malformed log: %v", err)
	}
	if seq := eng.currentToolLogSeq(workbenchID); seq != 0 {
		t.Fatalf("expected malformed-only log seq 0, got %d", seq)
	}
}

func TestRPIToolFiltering(t *testing.T) {
	researchNames := make([]string, 0, len(ResearchTools))
	for _, tool := range ResearchTools {
		researchNames = append(researchNames, tool.Function.Name)
	}
	expectedResearch := []string{
		"list_files",
		"get_file_info",
		"get_file_map",
		"read_file",
		"table_get_map",
		"table_describe",
		"table_stats",
		"table_read_rows",
		"table_query",
		"xlsx_get_styles",
		"docx_get_styles",
		"pptx_get_styles",
		"recall_tool_result",
	}
	for _, name := range expectedResearch {
		if !slices.Contains(researchNames, name) {
			t.Fatalf("expected research tool %q in %+v", name, researchNames)
		}
	}
	for _, forbidden := range []string{
		"table_export",
		"table_update_from_export",
		"write_text_file",
		"xlsx_operations",
		"docx_operations",
		"pptx_operations",
		"xlsx_copy_assets",
		"docx_copy_assets",
		"pptx_copy_assets",
	} {
		if slices.Contains(researchNames, forbidden) {
			t.Fatalf("unexpected write-capable tool %q in research set", forbidden)
		}
	}

	planNames := make([]string, 0, len(PlanTools))
	for _, tool := range PlanTools {
		planNames = append(planNames, tool.Function.Name)
	}
	if len(planNames) != 2 {
		t.Fatalf("expected 2 plan tools, got %d (%+v)", len(planNames), planNames)
	}
	if !slices.Contains(planNames, "read_file") || !slices.Contains(planNames, "recall_tool_result") {
		t.Fatalf("expected plan tools read_file + recall_tool_result, got %+v", planNames)
	}
}
