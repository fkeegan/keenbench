package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keenbench/engine/internal/appdirs"
	"keenbench/engine/internal/workbench"
)

func TestEngineMetadataMethods(t *testing.T) {
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

	status, errInfo := eng.ProvidersGetStatus(ctx, nil)
	if errInfo != nil {
		t.Fatalf("status: %v", errInfo)
	}
	providers := status.(map[string]any)["providers"].([]map[string]any)
	if len(providers) == 0 {
		t.Fatalf("expected providers")
	}
	foundOpenAI := false
	foundOpenAICodex := false
	foundMistral := false
	for _, provider := range providers {
		if provider["provider_id"] == ProviderOpenAI {
			foundOpenAI = true
			assertProviderRPIReasoning(t, provider, "medium", "medium", "medium")
		}
		if provider["provider_id"] == ProviderOpenAICodex {
			foundOpenAICodex = true
			if provider["auth_mode"] != "oauth" {
				t.Fatalf("expected openai-codex auth_mode oauth, got %#v", provider["auth_mode"])
			}
			assertProviderRPIReasoning(t, provider, "medium", "medium", "medium")
		}
		if provider["provider_id"] == ProviderMistral {
			foundMistral = true
			if provider["auth_mode"] != "api_key" {
				t.Fatalf("expected mistral auth_mode api_key, got %#v", provider["auth_mode"])
			}
			if _, ok := provider["rpi_reasoning"]; ok {
				t.Fatalf("expected mistral to omit rpi_reasoning, got %#v", provider["rpi_reasoning"])
			}
		}
	}
	if !foundOpenAI {
		t.Fatalf("expected openai provider in status")
	}
	if !foundOpenAICodex {
		t.Fatalf("expected openai-codex provider in status")
	}
	if !foundMistral {
		t.Fatalf("expected mistral provider in status")
	}

	modelsResp, errInfo := eng.ModelsListSupported(ctx, nil)
	if errInfo != nil {
		t.Fatalf("models list: %v", errInfo)
	}
	modelsRaw, ok := modelsResp.(map[string]any)["models"].([]ModelInfo)
	if !ok {
		t.Fatalf("expected models list payload")
	}
	foundMistralModel := false
	for _, model := range modelsRaw {
		if model.ModelID == ModelMistralID {
			foundMistralModel = true
			break
		}
	}
	if !foundMistralModel {
		t.Fatalf("expected mistral model in supported list")
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Meta"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	cancelResp, errInfo := eng.WorkshopCancelRun(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("cancel run: %v", errInfo)
	}
	if cancelResp.(map[string]any)["cancel_requested"] != false {
		t.Fatalf("expected cancel_requested=false when no run is active, got %#v", cancelResp)
	}

	listResp, errInfo := eng.WorkbenchList(ctx, nil)
	if errInfo != nil {
		t.Fatalf("list: %v", errInfo)
	}
	items := listResp.(map[string]any)["workbenches"].([]workbench.Workbench)
	if len(items) == 0 {
		t.Fatalf("expected list items")
	}

	openResp, errInfo := eng.WorkbenchOpen(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("open: %v", errInfo)
	}
	if openResp.(map[string]any)["workbench"].(*workbench.Workbench).ID != workbenchID {
		t.Fatalf("unexpected workbench open")
	}

	filesResp, errInfo := eng.WorkbenchFilesList(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("files list: %v", errInfo)
	}
	files := filesResp.(map[string]any)["files"].([]workbench.FileEntry)
	if len(files) != 0 {
		t.Fatalf("expected empty files")
	}

	workshopState, errInfo := eng.WorkshopGetState(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("workshop state: %v", errInfo)
	}
	if workshopState.(map[string]any)["active_model_id"] == "" {
		t.Fatalf("expected active model id")
	}

	convoResp, errInfo := eng.WorkshopGetConversation(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("conversation: %v", errInfo)
	}
	messages := convoResp.(map[string]any)["messages"].([]conversationMessage)
	if len(messages) != 0 {
		t.Fatalf("expected no messages")
	}

	draftResp, errInfo := eng.DraftGetState(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("draft state: %v", errInfo)
	}
	if draftResp.(map[string]any)["has_draft"] != false {
		t.Fatalf("expected no draft")
	}

	if _, errInfo := eng.DraftDiscard(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("discard: %v", errInfo)
	}

	_, errInfo = eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo == nil {
		t.Fatalf("expected review error without draft")
	}

	_, errInfo = eng.WorkshopGetProposal(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "proposal_id": "missing"}))
	if errInfo == nil {
		t.Fatalf("expected proposal missing error")
	}

	// Add a file and verify scope hash changes
	filePath := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("note"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "source_paths": []string{filePath}})); errInfo != nil {
		t.Fatalf("add file: %v", errInfo)
	}
	consentStatus, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("consent status: %v", errInfo)
	}
	if consentStatus.(map[string]any)["scope_hash"] == "" {
		t.Fatalf("expected scope hash")
	}
}

func TestWorkbenchFilesRemoveRPC(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Remove"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	filePath := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("note"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "source_paths": []string{filePath}})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}
	if _, errInfo := eng.WorkbenchFilesRemove(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "workbench_paths": []string{"notes.txt"}})); errInfo != nil {
		t.Fatalf("remove: %v", errInfo)
	}
	filesResp, errInfo := eng.WorkbenchFilesList(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("list: %v", errInfo)
	}
	files := filesResp.(map[string]any)["files"].([]workbench.FileEntry)
	if len(files) != 0 {
		t.Fatalf("expected empty files")
	}
}

func TestWorkbenchDeleteRPC(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Delete"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	if _, errInfo := eng.WorkbenchDelete(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("delete: %v", errInfo)
	}
	workbenchesDir := appdirs.WorkbenchesDir(dataDir)
	if _, err := os.Stat(filepath.Join(workbenchesDir, workbenchID)); !os.IsNotExist(err) {
		t.Fatalf("expected workbench deleted")
	}
}

func TestWorkbenchCreateUsesUserDefaultModelAsInitialActiveModel(t *testing.T) {
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

	if _, errInfo := eng.UserSetDefaultModel(ctx, mustJSON(t, map[string]any{"model_id": ModelOpenAICodexID})); errInfo != nil {
		t.Fatalf("set user default model: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "ModelDefault"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	workshopState, errInfo := eng.WorkshopGetState(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("workshop state: %v", errInfo)
	}
	if got := workshopState.(map[string]any)["default_model_id"]; got != ModelOpenAICodexID {
		t.Fatalf("expected default_model_id=%q, got %#v", ModelOpenAICodexID, got)
	}
	if got := workshopState.(map[string]any)["active_model_id"]; got != ModelOpenAICodexID {
		t.Fatalf("expected active_model_id=%q, got %#v", ModelOpenAICodexID, got)
	}
}

func TestWorkbenchFilesExtractRPC(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Extract"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	filePath := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("note"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "source_paths": []string{filePath}})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}
	destination := filepath.Join(dataDir, "export")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	resp, errInfo := eng.WorkbenchFilesExtract(ctx, mustJSON(t, map[string]any{
		"workbench_id":    workbenchID,
		"destination_dir": destination,
	}))
	if errInfo != nil {
		t.Fatalf("extract: %v", errInfo)
	}
	results, ok := resp.(map[string]any)["extract_results"].([]workbench.ExtractResult)
	if !ok {
		t.Fatalf("expected extract_results")
	}
	if len(results) != 1 || results[0].Status != "extracted" {
		t.Fatalf("unexpected extract results: %#v", results)
	}
	if _, err := os.Stat(filepath.Join(destination, "notes.txt")); err != nil {
		t.Fatalf("expected exported file: %v", err)
	}
}

func TestWorkbenchGetScopeRPC(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Scope"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	resp, errInfo := eng.WorkbenchGetScope(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("get scope: %v", errInfo)
	}
	payload := resp.(map[string]any)
	limits, ok := payload["limits"].(map[string]any)
	if !ok {
		t.Fatalf("expected limits map")
	}
	if limits["max_files"] != workbenchMaxFiles {
		t.Fatalf("expected max_files=%d, got %v", workbenchMaxFiles, limits["max_files"])
	}
	if limits["max_file_bytes"] != workbenchMaxFileBytes {
		t.Fatalf("expected max_file_bytes=%d, got %v", workbenchMaxFileBytes, limits["max_file_bytes"])
	}
	supported, ok := payload["supported_types"].(map[string]any)
	if !ok {
		t.Fatalf("expected supported_types map")
	}
	editable, ok := supported["editable_extensions"].([]string)
	if !ok || len(editable) == 0 {
		t.Fatalf("expected editable_extensions")
	}
	sandboxRoot, ok := payload["sandbox_root"].(string)
	if !ok || sandboxRoot == "" {
		t.Fatalf("expected sandbox_root")
	}
	if !strings.Contains(sandboxRoot, workbenchID) {
		t.Fatalf("expected sandbox root to include workbench id, got %s", sandboxRoot)
	}
}

func TestDraftGetStateIncludesSourceFields(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "DraftSource"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	if _, err := eng.workbenches.CreateDraftWithSource(workbenchID, "workshop", "agent"); err != nil {
		t.Fatalf("create draft: %v", err)
	}

	resp, errInfo := eng.DraftGetState(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("draft state: %v", errInfo)
	}
	payload := resp.(map[string]any)
	if payload["has_draft"] != true {
		t.Fatalf("expected has_draft=true")
	}
	if payload["source_kind"] != "workshop" {
		t.Fatalf("expected source_kind workshop, got %v", payload["source_kind"])
	}
	if payload["source_ref"] != "agent" {
		t.Fatalf("expected source_ref agent, got %v", payload["source_ref"])
	}
}

func TestWorkbenchFilesChangedNotificationsOnAddAndRemove(t *testing.T) {
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
	var fileEvents []map[string]any
	eng.SetNotifier(func(method string, params any) {
		if method != "WorkbenchFilesChanged" {
			return
		}
		payload, _ := params.(map[string]any)
		fileEvents = append(fileEvents, payload)
	})

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "FileEvents"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("note"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add: %v", errInfo)
	}
	if _, errInfo := eng.WorkbenchFilesRemove(ctx, mustJSON(t, map[string]any{
		"workbench_id":    workbenchID,
		"workbench_paths": []string{"notes.txt"},
	})); errInfo != nil {
		t.Fatalf("remove: %v", errInfo)
	}
	if len(fileEvents) < 2 {
		t.Fatalf("expected at least two file change notifications, got %d", len(fileEvents))
	}
	added, _ := fileEvents[0]["added"].([]string)
	if len(added) != 1 || added[0] != "notes.txt" {
		t.Fatalf("expected added notes.txt, got %v", fileEvents[0]["added"])
	}
	removed, _ := fileEvents[1]["removed"].([]string)
	if len(removed) != 1 || removed[0] != "notes.txt" {
		t.Fatalf("expected removed notes.txt, got %v", fileEvents[1]["removed"])
	}
}

func TestProvidersSetReasoningEffortRoundTripAndValidation(t *testing.T) {
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

	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderOpenAI,
		"research_effort":  " NONE ",
		"plan_effort":      "HIGH",
		"implement_effort": "low",
	})); errInfo != nil {
		t.Fatalf("set openai reasoning effort: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderOpenAICodex,
		"research_effort":  "XHIGH",
		"plan_effort":      "medium",
		"implement_effort": "LOW",
	})); errInfo != nil {
		t.Fatalf("set openai-codex reasoning effort: %v", errInfo)
	}

	status, errInfo := eng.ProvidersGetStatus(ctx, nil)
	if errInfo != nil {
		t.Fatalf("status: %v", errInfo)
	}
	providers := status.(map[string]any)["providers"].([]map[string]any)
	assertProviderRPIReasoning(t, providerStatusByID(t, providers, ProviderOpenAI), "none", "high", "low")
	assertProviderRPIReasoning(t, providerStatusByID(t, providers, ProviderOpenAICodex), "xhigh", "medium", "low")

	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderAnthropic,
		"research_effort":  "low",
		"plan_effort":      "low",
		"implement_effort": "low",
	})); errInfo == nil {
		t.Fatalf("expected unsupported provider validation error")
	}
	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderOpenAI,
		"research_effort":  "xhigh",
		"plan_effort":      "medium",
		"implement_effort": "low",
	})); errInfo == nil {
		t.Fatalf("expected openai invalid reasoning effort error")
	}
	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderOpenAICodex,
		"research_effort":  "high",
		"plan_effort":      "none",
		"implement_effort": "medium",
	})); errInfo == nil {
		t.Fatalf("expected openai-codex invalid reasoning effort error")
	}
}

func providerStatusByID(t *testing.T, providers []map[string]any, providerID string) map[string]any {
	t.Helper()
	for _, provider := range providers {
		if provider["provider_id"] == providerID {
			return provider
		}
	}
	t.Fatalf("provider %q not found", providerID)
	return nil
}

func assertProviderRPIReasoning(t *testing.T, provider map[string]any, research, plan, implement string) {
	t.Helper()
	reasoning, ok := provider["rpi_reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected rpi_reasoning map for provider %v, got %#v", provider["provider_id"], provider["rpi_reasoning"])
	}
	if got := reasoning["research_effort"]; got != research {
		t.Fatalf("expected research_effort=%q, got %#v", research, got)
	}
	if got := reasoning["plan_effort"]; got != plan {
		t.Fatalf("expected plan_effort=%q, got %#v", plan, got)
	}
	if got := reasoning["implement_effort"]; got != implement {
		t.Fatalf("expected implement_effort=%q, got %#v", implement, got)
	}
}
