package engine

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"keenbench/engine/internal/llm"
)

func TestProposalValidationErrors(t *testing.T) {
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, errInfo := eng.parseProposal("not json")
	if errInfo == nil {
		t.Fatalf("expected validation error")
	}
	_, errInfo = eng.parseProposal(`{"summary":"","writes":[]}`)
	if errInfo == nil {
		t.Fatalf("expected missing summary error")
	}
	_, errInfo = eng.parseProposal(`{"summary":"ok","writes":[{"path":"file.exe","content":"x"}]}`)
	if errInfo == nil {
		t.Fatalf("expected unsupported extension error")
	}
	_, errInfo = eng.parseProposal(`{"summary":"ok","writes":[{"path":"nested/file.txt","content":"x"}]}`)
	if errInfo == nil {
		t.Fatalf("expected nested path error")
	}
	_, errInfo = eng.parseProposal(`{"summary":"ok","writes":[{"path":"file.txt","content":"x"}],"delete":[{"path":"file.txt"}]}`)
	if errInfo == nil {
		t.Fatalf("expected delete op error")
	}
	proposalOk := `{"summary":"please delete the old section","writes":[{"path":"file.txt","content":"x"}]}`
	if _, errInfo := eng.parseProposal(proposalOk); errInfo != nil {
		t.Fatalf("expected proposal to allow 'delete' in summary")
	}
	var manyWrites strings.Builder
	manyWrites.WriteString(`{"summary":"ok","writes":[`)
	for i := 0; i < maxProposalWrites+1; i++ {
		if i > 0 {
			manyWrites.WriteString(",")
		}
		manyWrites.WriteString(`{"path":"file`)
		manyWrites.WriteString(strconv.Itoa(i))
		manyWrites.WriteString(`.txt","content":"x"}`)
	}
	manyWrites.WriteString(`]}`)
	if _, errInfo := eng.parseProposal(manyWrites.String()); errInfo == nil {
		t.Fatalf("expected too many writes error")
	}
}

func TestConsentRequiredWhenMissing(t *testing.T) {
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
	eng.providers[ProviderOpenAI] = &testOpenAI{chatResponse: `{"summary":"Summary","writes":[{"path":"summary.md","content":"Done"}]}`}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Test"}))
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

	_, errInfo = eng.WorkshopProposeChanges(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo == nil || errInfo.ErrorCode != "EGRESS_CONSENT_REQUIRED" {
		t.Fatalf("expected consent required error")
	}
}

func TestWorkshopProposalFallbackOnInvalidJSON(t *testing.T) {
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
	eng.providers[ProviderOpenAI] = &testOpenAI{chatResponse: "this is not json"}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"api_key":     "sk-test",
	})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"enabled":     true,
	})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Test"}))
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
		t.Fatalf("add files: %v", errInfo)
	}

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

	proposalResp, errInfo := eng.WorkshopProposeChanges(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("expected fallback proposal, got error: %v", errInfo)
	}
	proposalID := proposalResp.(map[string]any)["proposal_id"].(string)
	proposal, err := eng.readProposal(workbenchID, proposalID)
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	if !proposal.NoChanges {
		t.Fatalf("expected fallback no_changes")
	}
	if len(proposal.Writes) != 0 || len(proposal.Ops) != 0 {
		t.Fatalf("expected no writes/ops for fallback")
	}
}

type testNetErr struct{}

func (testNetErr) Error() string   { return "network unavailable" }
func (testNetErr) Timeout() bool   { return true }
func (testNetErr) Temporary() bool { return true }

func TestProviderValidateErrorMapping(t *testing.T) {
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
		"provider_id": "openai",
		"api_key":     "sk-test",
	})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}

	eng.providers[ProviderOpenAI] = &testOpenAI{validateErr: llm.ErrUnavailable}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo == nil || errInfo.ErrorCode != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("expected provider unavailable error")
	}

	eng.providers[ProviderOpenAI] = &testOpenAI{validateErr: testNetErr{}}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo == nil || errInfo.ErrorCode != "NETWORK_UNAVAILABLE" {
		t.Fatalf("expected network unavailable error")
	}
}
