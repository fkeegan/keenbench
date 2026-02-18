package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"keenbench/engine/internal/llm"
)

type testOpenAI struct {
	validateErr  error
	chatResponse string
}

func (f *testOpenAI) ValidateKey(ctx context.Context, apiKey string) error {
	return f.validateErr
}

func (f *testOpenAI) Chat(ctx context.Context, apiKey, model string, messages []llm.Message) (string, error) {
	return f.chatResponse, nil
}

func (f *testOpenAI) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
	if onDelta != nil {
		onDelta("Hello")
		onDelta(" world")
	}
	return "Hello world", nil
}

func (f *testOpenAI) ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	return llm.ChatResponse{
		Content:      f.chatResponse,
		FinishReason: "stop",
	}, nil
}

func (f *testOpenAI) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	if onDelta != nil {
		onDelta("Hello")
		onDelta(" world")
	}
	return llm.ChatResponse{
		Content:      "Hello world",
		FinishReason: "stop",
	}, nil
}

func TestEngineWorkshopFlow(t *testing.T) {
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

	notifications := []string{}
	eng.SetNotifier(func(method string, params any) {
		notifications = append(notifications, method)
	})

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
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
	_, errInfo = eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "source_paths": []string{src}}))
	if errInfo != nil {
		t.Fatalf("files add: %v", errInfo)
	}

	consentStatus, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("consent status: %v", errInfo)
	}
	scopeHash := consentStatus.(map[string]any)["scope_hash"].(string)
	_, errInfo = eng.EgressGrantWorkshopConsent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"provider_id":  "openai",
		"model_id":     ModelOpenAIID,
		"scope_hash":   scopeHash,
		"persist":      true,
	}))
	if errInfo != nil {
		t.Fatalf("grant consent: %v", errInfo)
	}

	msgResp, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "text": "Summarize"}))
	if errInfo != nil {
		t.Fatalf("send msg: %v", errInfo)
	}
	messageID := msgResp.(map[string]any)["message_id"].(string)
	if _, errInfo := eng.WorkshopStreamAssistantReply(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "message_id": messageID})); errInfo != nil {
		t.Fatalf("stream reply: %v", errInfo)
	}

	proposalResp, errInfo := eng.WorkshopProposeChanges(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("propose: %v", errInfo)
	}
	proposalID := proposalResp.(map[string]any)["proposal_id"].(string)

	_, errInfo = eng.WorkshopApplyProposal(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "proposal_id": proposalID}))
	if errInfo != nil {
		t.Fatalf("apply: %v", errInfo)
	}

	changesResp, errInfo := eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("changes: %v", errInfo)
	}
	raw, err := json.Marshal(changesResp)
	if err != nil {
		t.Fatalf("marshal changes: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal changes: %v", err)
	}
	changes, ok := decoded["changes"].([]any)
	if !ok || len(changes) == 0 {
		t.Fatalf("expected changes")
	}

	_, errInfo = eng.ReviewGetTextDiff(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID, "path": "summary.md"}))
	if errInfo != nil {
		t.Fatalf("diff: %v", errInfo)
	}

	if _, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("publish: %v", errInfo)
	}

	if len(notifications) == 0 {
		t.Fatalf("expected notifications")
	}
}

func mustJSON(t *testing.T, payload any) json.RawMessage {
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
