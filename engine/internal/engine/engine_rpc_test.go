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
	foundAnthropic := false
	foundAnthropicClaude := false
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
		if provider["provider_id"] == ProviderAnthropic {
			foundAnthropic = true
			assertProviderRPIReasoning(t, provider, "medium", "medium", "medium")
		}
		if provider["provider_id"] == ProviderAnthropicClaude {
			foundAnthropicClaude = true
			if provider["auth_mode"] != "setup_token" {
				t.Fatalf("expected anthropic-claude auth_mode setup_token, got %#v", provider["auth_mode"])
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
	if !foundAnthropic {
		t.Fatalf("expected anthropic provider in status")
	}
	if !foundAnthropicClaude {
		t.Fatalf("expected anthropic-claude provider in status")
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

func TestAnthropicClaudeCredentialStorageIsSeparateFromAnthropicAPIKey(t *testing.T) {
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

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderAnthropic,
		"api_key":     "sk-ant-api-key",
	})); errInfo != nil {
		t.Fatalf("set anthropic api key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderAnthropicClaude,
		"api_key":     "sk-ant-setup-token",
	})); errInfo != nil {
		t.Fatalf("set anthropic-claude setup token: %v", errInfo)
	}

	anthropicKey, errInfo := eng.providerKey(ctx, ProviderAnthropic)
	if errInfo != nil {
		t.Fatalf("read anthropic api key: %v", errInfo)
	}
	if anthropicKey != "sk-ant-api-key" {
		t.Fatalf("expected anthropic api key to remain isolated, got %q", anthropicKey)
	}
	anthropicClaudeToken, errInfo := eng.providerKey(ctx, ProviderAnthropicClaude)
	if errInfo != nil {
		t.Fatalf("read anthropic-claude setup token: %v", errInfo)
	}
	if anthropicClaudeToken != "sk-ant-setup-token" {
		t.Fatalf("expected anthropic-claude setup token, got %q", anthropicClaudeToken)
	}

	if _, errInfo := eng.ProvidersClearApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderAnthropicClaude,
	})); errInfo != nil {
		t.Fatalf("clear anthropic-claude setup token: %v", errInfo)
	}

	anthropicKey, errInfo = eng.providerKey(ctx, ProviderAnthropic)
	if errInfo != nil {
		t.Fatalf("read anthropic api key after anthropic-claude clear: %v", errInfo)
	}
	if anthropicKey != "sk-ant-api-key" {
		t.Fatalf("expected anthropic api key to remain after anthropic-claude clear, got %q", anthropicKey)
	}
	anthropicClaudeToken, errInfo = eng.providerKey(ctx, ProviderAnthropicClaude)
	if errInfo != nil {
		t.Fatalf("read anthropic-claude setup token after clear: %v", errInfo)
	}
	if anthropicClaudeToken != "" {
		t.Fatalf("expected anthropic-claude setup token to be cleared, got %q", anthropicClaudeToken)
	}
}

func TestProvidersValidateAnthropicClaudeAcceptsNonEmptySetupToken(t *testing.T) {
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

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderAnthropicClaude,
		"api_key":     "sk-ant-oat01-test-token",
	})); errInfo != nil {
		t.Fatalf("set anthropic-claude setup token: %v", errInfo)
	}

	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{
		"provider_id": ProviderAnthropicClaude,
	})); errInfo != nil {
		t.Fatalf("validate anthropic-claude setup token: %v", errInfo)
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

func TestModelsListSupportedIncludesAnthropic46Variants(t *testing.T) {
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

	modelsResp, errInfo := eng.ModelsListSupported(ctx, nil)
	if errInfo != nil {
		t.Fatalf("models list: %v", errInfo)
	}
	modelsRaw, ok := modelsResp.(map[string]any)["models"].([]ModelInfo)
	if !ok {
		t.Fatalf("expected models list payload")
	}
	sonnetIndex := -1
	opusIndex := -1
	sonnetSetupTokenIndex := -1
	opusSetupTokenIndex := -1
	for i, model := range modelsRaw {
		if model.ModelID == ModelAnthropicSonnet46ID {
			sonnetIndex = i
		}
		if model.ModelID == ModelAnthropicOpus46ID {
			opusIndex = i
		}
		if model.ModelID == ModelAnthropicClaudeSonnet46ID {
			sonnetSetupTokenIndex = i
		}
		if model.ModelID == ModelAnthropicClaudeOpus46ID {
			opusSetupTokenIndex = i
		}
		if model.ModelID == modelAnthropicLegacyOpus45ID {
			t.Fatalf("did not expect legacy anthropic model in supported list")
		}
	}
	if sonnetIndex == -1 {
		t.Fatalf("expected %q in supported list", ModelAnthropicSonnet46ID)
	}
	if opusIndex == -1 {
		t.Fatalf("expected %q in supported list", ModelAnthropicOpus46ID)
	}
	if sonnetSetupTokenIndex == -1 {
		t.Fatalf("expected %q in supported list", ModelAnthropicClaudeSonnet46ID)
	}
	if opusSetupTokenIndex == -1 {
		t.Fatalf("expected %q in supported list", ModelAnthropicClaudeOpus46ID)
	}
	if sonnetIndex >= opusIndex {
		t.Fatalf("expected sonnet model before opus model, got sonnet=%d opus=%d", sonnetIndex, opusIndex)
	}
	if sonnetSetupTokenIndex >= opusSetupTokenIndex {
		t.Fatalf(
			"expected setup-token sonnet model before setup-token opus model, got sonnet=%d opus=%d",
			sonnetSetupTokenIndex,
			opusSetupTokenIndex,
		)
	}
}

func TestUserSetDefaultModelCanonicalizesLegacyAnthropicModel(t *testing.T) {
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

	if _, errInfo := eng.UserSetDefaultModel(ctx, mustJSON(t, map[string]any{
		"model_id": modelAnthropicLegacyOpus45ID,
	})); errInfo != nil {
		t.Fatalf("set user default model: %v", errInfo)
	}
	resp, errInfo := eng.UserGetDefaultModel(ctx, nil)
	if errInfo != nil {
		t.Fatalf("get user default model: %v", errInfo)
	}
	if got := resp.(map[string]any)["model_id"]; got != ModelAnthropicSonnet46ID {
		t.Fatalf("expected canonical model id %q, got %#v", ModelAnthropicSonnet46ID, got)
	}
}

func TestUserConsentModeDefaultsToAsk(t *testing.T) {
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

	resp, errInfo := eng.UserGetConsentMode(ctx, nil)
	if errInfo != nil {
		t.Fatalf("get user consent mode: %v", errInfo)
	}
	if got := resp.(map[string]any)["mode"]; got != "ask" {
		t.Fatalf("expected mode=%q, got %#v", "ask", got)
	}
}

func TestUserSetConsentModeRequiresExplicitApproval(t *testing.T) {
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

	if _, errInfo := eng.UserSetConsentMode(ctx, mustJSON(t, map[string]any{
		"mode": "allow_all",
	})); errInfo == nil {
		t.Fatalf("expected explicit approval error")
	}
	if _, errInfo := eng.UserSetConsentMode(ctx, mustJSON(t, map[string]any{
		"mode":     "allow_all",
		"approved": true,
	})); errInfo != nil {
		t.Fatalf("set consent mode allow_all: %v", errInfo)
	}
	resp, errInfo := eng.UserGetConsentMode(ctx, nil)
	if errInfo != nil {
		t.Fatalf("get user consent mode: %v", errInfo)
	}
	if got := resp.(map[string]any)["mode"]; got != "allow_all" {
		t.Fatalf("expected mode=%q, got %#v", "allow_all", got)
	}
}

func TestEgressGetConsentStatusRespectsConsentMode(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "ConsentModeStatus"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	status, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("consent status: %v", errInfo)
	}
	statusMap := status.(map[string]any)
	if got := statusMap["mode"]; got != "ask" {
		t.Fatalf("expected mode=%q, got %#v", "ask", got)
	}
	if got := statusMap["consented"]; got != false {
		t.Fatalf("expected consented=false in ask mode without consent, got %#v", got)
	}

	if _, errInfo := eng.UserSetConsentMode(ctx, mustJSON(t, map[string]any{
		"mode":     "allow_all",
		"approved": true,
	})); errInfo != nil {
		t.Fatalf("set consent mode allow_all: %v", errInfo)
	}

	status, errInfo = eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("consent status in allow_all mode: %v", errInfo)
	}
	statusMap = status.(map[string]any)
	if got := statusMap["mode"]; got != "allow_all" {
		t.Fatalf("expected mode=%q, got %#v", "allow_all", got)
	}
	if got := statusMap["consented"]; got != true {
		t.Fatalf("expected consented=true in allow_all mode, got %#v", got)
	}
}

func TestWorkshopGetStateMigratesLegacyAnthropicModelIDs(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "LegacyAnthropic"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	workbenchesDir := appdirs.WorkbenchesDir(dataDir)

	var wb workbench.Workbench
	workbenchPath := filepath.Join(workbenchesDir, workbenchID, "meta", "workbench.json")
	if err := readJSON(workbenchPath, &wb); err != nil {
		t.Fatalf("read workbench: %v", err)
	}
	wb.DefaultModelID = modelAnthropicLegacyOpus45ID
	if err := writeJSON(workbenchPath, &wb); err != nil {
		t.Fatalf("write workbench: %v", err)
	}

	statePath := filepath.Join(workbenchesDir, workbenchID, "meta", "workshop_state.json")
	state := workshopState{ActiveModelID: modelAnthropicLegacyOpus45ID}
	if err := writeJSON(statePath, &state); err != nil {
		t.Fatalf("write workshop state: %v", err)
	}

	workshopStateResp, errInfo := eng.WorkshopGetState(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("workshop state: %v", errInfo)
	}
	payload := workshopStateResp.(map[string]any)
	if got := payload["active_model_id"]; got != ModelAnthropicSonnet46ID {
		t.Fatalf("expected active_model_id=%q, got %#v", ModelAnthropicSonnet46ID, got)
	}
	if got := payload["default_model_id"]; got != ModelAnthropicSonnet46ID {
		t.Fatalf("expected default_model_id=%q, got %#v", ModelAnthropicSonnet46ID, got)
	}

	var persistedState workshopState
	if err := readJSON(statePath, &persistedState); err != nil {
		t.Fatalf("read persisted workshop state: %v", err)
	}
	if persistedState.ActiveModelID != ModelAnthropicSonnet46ID {
		t.Fatalf("expected persisted active_model_id=%q, got %q", ModelAnthropicSonnet46ID, persistedState.ActiveModelID)
	}

	if err := readJSON(workbenchPath, &wb); err != nil {
		t.Fatalf("read persisted workbench: %v", err)
	}
	if wb.DefaultModelID != ModelAnthropicSonnet46ID {
		t.Fatalf("expected persisted default_model_id=%q, got %q", ModelAnthropicSonnet46ID, wb.DefaultModelID)
	}
}

func TestEgressGrantWorkshopConsentCanonicalizesLegacyAnthropicModel(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "ConsentLegacyAnthropic"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	status, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("consent status: %v", errInfo)
	}
	scopeHash := status.(map[string]any)["scope_hash"].(string)
	if _, errInfo := eng.EgressGrantWorkshopConsent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"provider_id":  ProviderAnthropic,
		"model_id":     modelAnthropicLegacyOpus45ID,
		"scope_hash":   scopeHash,
		"persist":      true,
	})); errInfo != nil {
		t.Fatalf("grant consent: %v", errInfo)
	}

	consent, err := eng.workbenches.ReadConsent(workbenchID)
	if err != nil {
		t.Fatalf("read consent: %v", err)
	}
	if consent.Workshop.ModelID != ModelAnthropicSonnet46ID {
		t.Fatalf("expected persisted consent model_id=%q, got %q", ModelAnthropicSonnet46ID, consent.Workshop.ModelID)
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
	if err := os.WriteFile(filepath.Join(destination, "notes.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
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
	if len(results) != 1 || results[0].Status != "extracted" || results[0].FinalPath != "notes(1).txt" {
		t.Fatalf("unexpected extract results: %#v", results)
	}
	if _, err := os.Stat(filepath.Join(destination, "notes(1).txt")); err != nil {
		t.Fatalf("expected renamed exported file: %v", err)
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
	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderAnthropic,
		"research_effort":  "LOW",
		"plan_effort":      "high",
		"implement_effort": "max",
	})); errInfo != nil {
		t.Fatalf("set anthropic reasoning effort: %v", errInfo)
	}

	status, errInfo := eng.ProvidersGetStatus(ctx, nil)
	if errInfo != nil {
		t.Fatalf("status: %v", errInfo)
	}
	providers := status.(map[string]any)["providers"].([]map[string]any)
	assertProviderRPIReasoning(t, providerStatusByID(t, providers, ProviderOpenAI), "none", "high", "low")
	assertProviderRPIReasoning(t, providerStatusByID(t, providers, ProviderOpenAICodex), "xhigh", "medium", "low")
	assertProviderRPIReasoning(t, providerStatusByID(t, providers, ProviderAnthropic), "low", "high", "max")

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
	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderAnthropic,
		"research_effort":  "none",
		"plan_effort":      "medium",
		"implement_effort": "high",
	})); errInfo == nil {
		t.Fatalf("expected anthropic invalid reasoning effort error")
	}
	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderAnthropic,
		"research_effort":  "xhigh",
		"plan_effort":      "medium",
		"implement_effort": "high",
	})); errInfo == nil {
		t.Fatalf("expected anthropic invalid reasoning effort error")
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
