package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keenbench/engine/internal/llm"
)

type scriptedToolOpenAI struct {
	responses       []llm.ChatResponse
	streamResponses []string
	index           int
	streamIndex     int
}

func (s *scriptedToolOpenAI) ValidateKey(ctx context.Context, apiKey string) error {
	return nil
}

func (s *scriptedToolOpenAI) Chat(ctx context.Context, apiKey, model string, messages []llm.Message) (string, error) {
	return "", nil
}

func (s *scriptedToolOpenAI) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
	text := s.nextStreamResponse()
	if onDelta != nil && text != "" {
		onDelta(text)
	}
	return text, nil
}

func (s *scriptedToolOpenAI) ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	return s.nextResponse(), nil
}

func (s *scriptedToolOpenAI) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	resp := s.nextResponse()
	if onDelta != nil && resp.Content != "" {
		onDelta(resp.Content)
	}
	return resp, nil
}

func (s *scriptedToolOpenAI) nextResponse() llm.ChatResponse {
	if s.index >= len(s.responses) {
		return llm.ChatResponse{Content: "", FinishReason: "stop"}
	}
	resp := s.responses[s.index]
	s.index++
	return resp
}

func (s *scriptedToolOpenAI) nextStreamResponse() string {
	if s.streamIndex >= len(s.streamResponses) {
		return "Summary"
	}
	resp := s.streamResponses[s.streamIndex]
	s.streamIndex++
	return resp
}

type testNotification struct {
	method string
	params map[string]any
}

func rpiPlanWithItems(items ...string) string {
	lines := []string{
		"# Execution Plan",
		"",
		"## Task",
		"Execute scripted test plan",
		"",
		"## Items",
	}
	lines = append(lines, items...)
	return strings.Join(lines, "\n")
}

func TestWorkshopRunAgentEmitsClutterWhileStreaming(t *testing.T) {
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
			{
				Content:      "Research notes.",
				FinishReason: "stop",
			},
			{
				Content: rpiPlanWithItems(
					"- [ ] 1. Respond — provide summary output",
				),
				FinishReason: "stop",
			},
			{
				Content:      "Implement item done.",
				FinishReason: "stop",
			},
		},
		streamResponses: []string{"Streaming response for clutter updates."},
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "StreamingClutter"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("seed"), 0o600); err != nil {
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

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Summarize and respond",
	})); errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}

	var notifications []string
	eng.SetNotifier(func(method string, params any) {
		notifications = append(notifications, method)
	})

	if _, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}

	clutterChanges := 0
	streamDeltas := 0
	for _, method := range notifications {
		if method == "WorkbenchClutterChanged" {
			clutterChanges++
		}
		if method == "WorkshopAssistantStreamDelta" {
			streamDeltas++
		}
	}
	if streamDeltas == 0 {
		t.Fatalf("expected stream deltas during run")
	}
	if clutterChanges < 2 {
		t.Fatalf("expected clutter updates during stream and final persist, got %d", clutterChanges)
	}
}

func TestWorkshopRunAgentPersistsDraftSummaryFallback(t *testing.T) {
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

	finalAssistantText := "Draft complete with key updates."
	eng.providers[ProviderOpenAI] = &scriptedToolOpenAI{
		responses: []llm.ChatResponse{
			{
				Content:      "Research notes.",
				FinishReason: "stop",
			},
			{
				Content: rpiPlanWithItems(
					"- [ ] 1. Update summary.md — write requested content",
				),
				FinishReason: "stop",
			},
			{
				Content: "Applying requested change.",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      "write_text_file",
							Arguments: `{"path":"summary.md","content":"Updated"}`,
						},
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Content:      "Item completed.",
				FinishReason: "stop",
			},
		},
		streamResponses: []string{finalAssistantText},
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "AgentSummary"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("seed"), 0o600); err != nil {
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

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Update the summary file",
	})); errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}

	resp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}
	result := resp.(map[string]any)
	if result["has_draft"] != true {
		t.Fatalf("expected has_draft=true, got %v", result["has_draft"])
	}

	draft, err := eng.workbenches.DraftState(workbenchID)
	if err != nil || draft == nil {
		t.Fatalf("expected draft: %v", err)
	}
	draftSummary, err := eng.readDraftSummary(workbenchID, draft.DraftID)
	if err != nil {
		t.Fatalf("read draft summary: %v", err)
	}
	if draftSummary != finalAssistantText {
		t.Fatalf("expected draft summary %q, got %q", finalAssistantText, draftSummary)
	}

	changesResp, errInfo := eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("changes: %v", errInfo)
	}
	payload := changesResp.(map[string]any)
	if payload["draft_summary"] != finalAssistantText {
		t.Fatalf("expected draft_summary %q, got %v", finalAssistantText, payload["draft_summary"])
	}
}

func TestReviewGetChangeSetIncludesDraftSummaryAndPreservesPerFileSummary(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "ReviewSummary"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "metrics.xlsx")
	if err := os.WriteFile(src, []byte("seed"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	proposal := &Proposal{
		ProposalID:    "p-summary",
		SchemaVersion: 2,
		Summary:       "update",
		Ops: []ProposalOp{
			{
				Path:    "metrics.xlsx",
				Kind:    "xlsx",
				Summary: "Per-file summary",
				Ops: []map[string]any{
					{
						"op":     "set_range",
						"sheet":  "Summary",
						"start":  "A1",
						"values": []any{[]any{"Metric", "Value"}, []any{"Q1", 120}},
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

	draft, err := eng.workbenches.DraftState(workbenchID)
	if err != nil || draft == nil {
		t.Fatalf("expected draft: %v", err)
	}
	fallback := "Assistant fallback summary"
	if err := eng.writeDraftSummary(workbenchID, draft.DraftID, fallback); err != nil {
		t.Fatalf("write draft summary: %v", err)
	}

	resp, errInfo := eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("changeset: %v", errInfo)
	}
	payload := resp.(map[string]any)
	if payload["draft_summary"] != fallback {
		t.Fatalf("expected draft_summary %q, got %v", fallback, payload["draft_summary"])
	}
	data, err := json.Marshal(payload["changes"])
	if err != nil {
		t.Fatalf("marshal changes: %v", err)
	}
	var changes []map[string]any
	if err := json.Unmarshal(data, &changes); err != nil {
		t.Fatalf("unmarshal changes: %v", err)
	}
	found := false
	for _, change := range changes {
		if change["path"] != "metrics.xlsx" {
			continue
		}
		found = true
		if change["summary"] != "Per-file summary" {
			t.Fatalf("expected per-file summary preserved, got %v", change["summary"])
		}
	}
	if !found {
		t.Fatalf("expected metrics.xlsx change")
	}
}

func TestWorkshopRunAgentPersistsOfficeFocusHintsFromToolCalls(t *testing.T) {
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
			{
				Content:      "Research workbook.",
				FinishReason: "stop",
			},
			{
				Content: rpiPlanWithItems(
					"- [ ] 1. Build annual sheet — summarize workbook",
				),
				FinishReason: "stop",
			},
			{
				Content: "Applying workbook summary.",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-office-1",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name: "xlsx_operations",
							Arguments: `{
								"path":"quarterly_data.xlsx",
								"operations":[
									{"op":"summarize_by_category","sheet":"Annual","source_sheets":["Q1","Q2","Q3","Q4"]}
								]
							}`,
						},
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Content:      "Annual sheet done.",
				FinishReason: "stop",
			},
		},
		streamResponses: []string{"Done"},
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "AgentOfficeHints"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

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

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Create an annual summary sheet",
	})); errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}

	runResp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}
	if runResp.(map[string]any)["has_draft"] != true {
		t.Fatalf("expected has_draft=true")
	}

	resp, errInfo := eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("changeset: %v", errInfo)
	}
	data, err := json.Marshal(resp.(map[string]any)["changes"])
	if err != nil {
		t.Fatalf("marshal changes: %v", err)
	}
	var changes []map[string]any
	if err := json.Unmarshal(data, &changes); err != nil {
		t.Fatalf("unmarshal changes: %v", err)
	}
	found := false
	for _, change := range changes {
		if change["path"] != "quarterly_data.xlsx" {
			continue
		}
		found = true
		hint, ok := change["focus_hint"].(map[string]any)
		if !ok {
			t.Fatalf("expected focus_hint for quarterly_data.xlsx")
		}
		if hint["sheet"] != "Annual" {
			t.Fatalf("expected sheet Annual, got %v", hint["sheet"])
		}
	}
	if !found {
		t.Fatalf("expected quarterly_data.xlsx change")
	}
}

func TestWorkshopRunAgentFocusHintsLastToolCallWinsForSamePath(t *testing.T) {
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
			{
				Content:      "Research metrics workbook.",
				FinishReason: "stop",
			},
			{
				Content: rpiPlanWithItems(
					"- [ ] 1. Update metrics workbook — apply all changes",
				),
				FinishReason: "stop",
			},
			{
				Content: "Applying updates.",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-hint-1",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name: "xlsx_operations",
							Arguments: `{
								"path":"metrics.xlsx",
								"operations":[
									{"op":"set_range","sheet":"Q1","start":"A1","values":[["Metric","Value"]]}
								]
							}`,
						},
					},
					{
						ID:   "tc-hint-2",
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
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Content:      "Workbook updates done.",
				FinishReason: "stop",
			},
		},
		streamResponses: []string{"Done"},
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "AgentHintOrder"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

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

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Update metrics workbook",
	})); errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}

	if _, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	})); errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}

	resp, errInfo := eng.ReviewGetChangeSet(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("changeset: %v", errInfo)
	}
	data, err := json.Marshal(resp.(map[string]any)["changes"])
	if err != nil {
		t.Fatalf("marshal changes: %v", err)
	}
	var changes []map[string]any
	if err := json.Unmarshal(data, &changes); err != nil {
		t.Fatalf("unmarshal changes: %v", err)
	}
	found := false
	for _, change := range changes {
		if change["path"] != "metrics.xlsx" {
			continue
		}
		found = true
		hint, ok := change["focus_hint"].(map[string]any)
		if !ok {
			t.Fatalf("expected focus_hint for metrics.xlsx")
		}
		if hint["sheet"] != "Annual" {
			t.Fatalf("expected last hint sheet Annual, got %v", hint["sheet"])
		}
		if _, ok := hint["row_start"]; ok {
			t.Fatalf("expected final hint to be sheet-only, got %v", hint)
		}
	}
	if !found {
		t.Fatalf("expected metrics.xlsx change")
	}
}

func TestDraftPublishAppendsCheckpointConversationEventAndNotifies(t *testing.T) {
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

	var notifications []testNotification
	eng.SetNotifier(func(method string, params any) {
		payload, _ := params.(map[string]any)
		notifications = append(notifications, testNotification{method: method, params: payload})
	})

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "PublishEvent"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "summary.md", "draft body"); err != nil {
		t.Fatalf("apply draft write: %v", err)
	}

	publishResp, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("publish: %v", errInfo)
	}
	checkpointID := publishResp.(map[string]any)["checkpoint_id"].(string)
	if checkpointID == "" {
		t.Fatalf("expected checkpoint_id")
	}

	convoResp, errInfo := eng.WorkshopGetConversation(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("conversation: %v", errInfo)
	}
	messages := convoResp.(map[string]any)["messages"].([]conversationMessage)
	foundPublishEvent := false
	for _, msg := range messages {
		if msg.EventKind != "checkpoint_publish" {
			continue
		}
		foundPublishEvent = true
		if msg.Type != "system_event" {
			t.Fatalf("expected system_event type, got %s", msg.Type)
		}
		if msg.CheckpointID != checkpointID {
			t.Fatalf("expected checkpoint_id %s, got %s", checkpointID, msg.CheckpointID)
		}
		if msg.Reason != "publish" {
			t.Fatalf("expected reason publish, got %s", msg.Reason)
		}
		if msg.CreatedAt == "" || msg.Timestamp == "" {
			t.Fatalf("expected created_at/timestamp")
		}
	}
	if !foundPublishEvent {
		t.Fatalf("expected checkpoint_publish system event")
	}

	hasCheckpointCreated := false
	hasDraftStateChanged := false
	hasWorkbenchDraftStateChanged := false
	for _, n := range notifications {
		switch n.method {
		case "CheckpointCreated":
			if n.params["checkpoint_id"] == checkpointID && n.params["reason"] == "publish" {
				hasCheckpointCreated = true
			}
		case "DraftStateChanged":
			if n.params["has_draft"] == false {
				hasDraftStateChanged = true
			}
		case "WorkbenchDraftStateChanged":
			if n.params["has_draft"] == false {
				hasWorkbenchDraftStateChanged = true
			}
		}
	}
	if !hasCheckpointCreated {
		t.Fatalf("expected publish CheckpointCreated notification")
	}
	if !hasDraftStateChanged {
		t.Fatalf("expected DraftStateChanged notification")
	}
	if !hasWorkbenchDraftStateChanged {
		t.Fatalf("expected WorkbenchDraftStateChanged notification")
	}
}

func TestCheckpointRestoreAppendsConversationEventAndNotifies(t *testing.T) {
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

	var notifications []testNotification
	eng.SetNotifier(func(method string, params any) {
		payload, _ := params.(map[string]any)
		notifications = append(notifications, testNotification{method: method, params: payload})
	})

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "RestoreEvent"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	cpResp, errInfo := eng.CheckpointCreate(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"reason":       "manual",
		"description":  "manual checkpoint",
	}))
	if errInfo != nil {
		t.Fatalf("checkpoint create: %v", errInfo)
	}
	checkpointID := cpResp.(map[string]any)["checkpoint_id"].(string)
	if checkpointID == "" {
		t.Fatalf("expected checkpoint id")
	}

	if _, errInfo := eng.CheckpointRestore(ctx, mustJSON(t, map[string]any{
		"workbench_id":  workbenchID,
		"checkpoint_id": checkpointID,
	})); errInfo != nil {
		t.Fatalf("restore: %v", errInfo)
	}

	convoResp, errInfo := eng.WorkshopGetConversation(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("conversation: %v", errInfo)
	}
	messages := convoResp.(map[string]any)["messages"].([]conversationMessage)
	foundRestoreEvent := false
	for _, msg := range messages {
		if msg.EventKind != "checkpoint_restore" {
			continue
		}
		foundRestoreEvent = true
		if msg.Type != "system_event" {
			t.Fatalf("expected system_event type, got %s", msg.Type)
		}
		if msg.CheckpointID != checkpointID {
			t.Fatalf("expected checkpoint_id %s, got %s", checkpointID, msg.CheckpointID)
		}
		if msg.Reason != "restore" {
			t.Fatalf("expected restore reason, got %s", msg.Reason)
		}
		if msg.CreatedAt == "" || msg.Timestamp == "" {
			t.Fatalf("expected created_at/timestamp")
		}
		if msg.Metadata == nil {
			t.Fatalf("expected metadata")
		}
		if preID, ok := msg.Metadata["pre_restore_checkpoint_id"].(string); !ok || preID == "" {
			t.Fatalf("expected pre_restore_checkpoint_id metadata, got %v", msg.Metadata["pre_restore_checkpoint_id"])
		}
	}
	if !foundRestoreEvent {
		t.Fatalf("expected checkpoint_restore system event")
	}

	hasRestoreNotification := false
	for _, n := range notifications {
		if n.method == "CheckpointRestored" && n.params["checkpoint_id"] == checkpointID {
			hasRestoreNotification = true
			break
		}
	}
	if !hasRestoreNotification {
		t.Fatalf("expected CheckpointRestored notification")
	}
}

func TestWorkshopUndoToMessageRestoresPublishedAcrossPublishCheckpoint(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "UndoPublished"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("v0"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	sendResp, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "before publish",
	}))
	if errInfo != nil {
		t.Fatalf("send: %v", errInfo)
	}
	targetMessageID := sendResp.(map[string]any)["message_id"].(string)

	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "notes.txt", "v1"); err != nil {
		t.Fatalf("apply draft write: %v", err)
	}
	if _, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("publish: %v", errInfo)
	}

	if content, err := eng.workbenches.ReadFile(workbenchID, "published", "notes.txt"); err != nil {
		t.Fatalf("read published: %v", err)
	} else if content != "v1" {
		t.Fatalf("expected published v1 before undo, got %q", content)
	}

	if _, errInfo := eng.WorkshopUndoToMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   targetMessageID,
	})); errInfo != nil {
		t.Fatalf("undo: %v", errInfo)
	}

	if content, err := eng.workbenches.ReadFile(workbenchID, "published", "notes.txt"); err != nil {
		t.Fatalf("read restored published: %v", err)
	} else if content != "v0" {
		t.Fatalf("expected published v0 after undo, got %q", content)
	}
}

func TestWorkshopUndoToMessageRestoresPublishedFromNearestRemovedPublish(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "UndoNearestPublish"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("v0"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "before first publish",
	})); errInfo != nil {
		t.Fatalf("send first: %v", errInfo)
	}
	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create first draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "notes.txt", "v1"); err != nil {
		t.Fatalf("apply first draft: %v", err)
	}
	if _, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("first publish: %v", errInfo)
	}

	sendResp, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "between publishes",
	}))
	if errInfo != nil {
		t.Fatalf("send target: %v", errInfo)
	}
	targetMessageID := sendResp.(map[string]any)["message_id"].(string)

	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create second draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "notes.txt", "v2"); err != nil {
		t.Fatalf("apply second draft: %v", err)
	}
	if _, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("second publish: %v", errInfo)
	}

	if _, errInfo := eng.WorkshopUndoToMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   targetMessageID,
	})); errInfo != nil {
		t.Fatalf("undo: %v", errInfo)
	}

	if content, err := eng.workbenches.ReadFile(workbenchID, "published", "notes.txt"); err != nil {
		t.Fatalf("read restored published: %v", err)
	} else if content != "v1" {
		t.Fatalf("expected published v1 after undo, got %q", content)
	}
}

func TestWorkshopUndoToMessageRestoresPublishedAcrossRestoreEvent(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "UndoRestoreEvent"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("v0"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "before first publish",
	})); errInfo != nil {
		t.Fatalf("send first: %v", errInfo)
	}
	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create first draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "notes.txt", "v1"); err != nil {
		t.Fatalf("apply first draft: %v", err)
	}
	firstPublishResp, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("first publish: %v", errInfo)
	}
	firstCheckpointID := firstPublishResp.(map[string]any)["checkpoint_id"].(string)
	if firstCheckpointID == "" {
		t.Fatalf("expected first checkpoint id")
	}

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "before second publish",
	})); errInfo != nil {
		t.Fatalf("send second: %v", errInfo)
	}
	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create second draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "notes.txt", "v2"); err != nil {
		t.Fatalf("apply second draft: %v", err)
	}
	if _, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID})); errInfo != nil {
		t.Fatalf("second publish: %v", errInfo)
	}

	sendResp, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "before restore",
	}))
	if errInfo != nil {
		t.Fatalf("send target: %v", errInfo)
	}
	targetMessageID := sendResp.(map[string]any)["message_id"].(string)

	if _, errInfo := eng.CheckpointRestore(ctx, mustJSON(t, map[string]any{
		"workbench_id":  workbenchID,
		"checkpoint_id": firstCheckpointID,
	})); errInfo != nil {
		t.Fatalf("checkpoint restore: %v", errInfo)
	}
	if content, err := eng.workbenches.ReadFile(workbenchID, "published", "notes.txt"); err != nil {
		t.Fatalf("read restored published: %v", err)
	} else if content != "v0" {
		t.Fatalf("expected published v0 after checkpoint restore, got %q", content)
	}

	if _, errInfo := eng.WorkshopUndoToMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   targetMessageID,
	})); errInfo != nil {
		t.Fatalf("undo: %v", errInfo)
	}

	if content, err := eng.workbenches.ReadFile(workbenchID, "published", "notes.txt"); err != nil {
		t.Fatalf("read undo published: %v", err)
	} else if content != "v2" {
		t.Fatalf("expected published v2 after undo, got %q", content)
	}
}

func TestWorkshopUndoToMessageFailsWhenRequiredCheckpointMissing(t *testing.T) {
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
	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "UndoMissingCheckpoint"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(src, []byte("v0"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	sendResp, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "before publish",
	}))
	if errInfo != nil {
		t.Fatalf("send: %v", errInfo)
	}
	targetMessageID := sendResp.(map[string]any)["message_id"].(string)

	if _, err := eng.workbenches.CreateDraft(workbenchID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "notes.txt", "v1"); err != nil {
		t.Fatalf("apply draft write: %v", err)
	}
	publishResp, errInfo := eng.DraftPublish(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("publish: %v", errInfo)
	}
	checkpointID := publishResp.(map[string]any)["checkpoint_id"].(string)
	if checkpointID == "" {
		t.Fatalf("expected checkpoint id")
	}

	checkpointsRoot := filepath.Join(eng.workbenchesRoot(), workbenchID, "meta", "checkpoints")
	if err := os.RemoveAll(filepath.Join(checkpointsRoot, checkpointID)); err != nil {
		t.Fatalf("remove checkpoint dir: %v", err)
	}
	if err := os.Remove(filepath.Join(checkpointsRoot, checkpointID+".json")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove checkpoint metadata: %v", err)
	}

	if _, errInfo := eng.WorkshopUndoToMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   targetMessageID,
	})); errInfo == nil {
		t.Fatalf("expected undo error with missing checkpoint")
	} else if errInfo.ErrorCode != "FILE_WRITE_FAILED" {
		t.Fatalf("expected FILE_WRITE_FAILED, got %s", errInfo.ErrorCode)
	}
	convoResp, errInfo := eng.WorkshopGetConversation(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("conversation: %v", errInfo)
	}
	messages := convoResp.(map[string]any)["messages"].([]conversationMessage)
	if len(messages) != 2 {
		t.Fatalf("expected conversation unchanged, got %d entries", len(messages))
	}
	if content, err := eng.workbenches.ReadFile(workbenchID, "published", "notes.txt"); err != nil {
		t.Fatalf("read published: %v", err)
	} else if content != "v1" {
		t.Fatalf("expected published v1 after failed undo, got %q", content)
	}
}

func TestReadConversationLegacyCompatibility(t *testing.T) {
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

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "LegacyConversation"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	conversationPath := filepath.Join(eng.workbenchesRoot(), workbenchID, "meta", "conversation.jsonl")
	if err := os.MkdirAll(filepath.Dir(conversationPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := "{\"message_id\":\"u-1\",\"role\":\"user\",\"text\":\"hello\",\"created_at\":\"2026-02-01T00:00:00Z\"}\n" +
		"{\"id\":\"a-1\",\"role\":\"assistant\",\"content\":\"world\",\"timestamp\":\"2026-02-01T00:00:01Z\"}\n" +
		"{\"id\":\"s-1\",\"role\":\"system\",\"content\":\"restored\",\"timestamp\":\"2026-02-01T00:00:02Z\",\"event\":\"checkpoint_restore\",\"checkpoint_id\":\"cp-1\",\"reason\":\"restore\",\"metadata\":{\"pre_restore_checkpoint_id\":\"cp-pre\"}}\n"
	if err := os.WriteFile(conversationPath, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write conversation: %v", err)
	}

	entries, err := eng.readConversation(workbenchID)
	if err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Type != "user_message" {
		t.Fatalf("expected inferred user_message type, got %s", entries[0].Type)
	}
	if entries[1].MessageID != "a-1" || entries[1].Text != "world" {
		t.Fatalf("expected legacy assistant fields mapped, got id=%s text=%s", entries[1].MessageID, entries[1].Text)
	}
	if entries[1].CreatedAt != "2026-02-01T00:00:01Z" || entries[1].Type != "assistant_message" {
		t.Fatalf("expected assistant timestamp/type mapping, got created_at=%s type=%s", entries[1].CreatedAt, entries[1].Type)
	}
	if entries[2].Type != "system_event" || entries[2].EventKind != "checkpoint_restore" {
		t.Fatalf("expected system_event checkpoint_restore, got type=%s kind=%s", entries[2].Type, entries[2].EventKind)
	}
	if entries[2].CheckpointID != "cp-1" || entries[2].Reason != "restore" {
		t.Fatalf("expected checkpoint metadata, got checkpoint_id=%s reason=%s", entries[2].CheckpointID, entries[2].Reason)
	}
	if preID, ok := entries[2].Metadata["pre_restore_checkpoint_id"].(string); !ok || preID != "cp-pre" {
		t.Fatalf("expected legacy metadata mapping, got %v", entries[2].Metadata["pre_restore_checkpoint_id"])
	}
}

func TestWorkshopUndoToMessageTruncatesConversationAndRestoresDraftState(t *testing.T) {
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
			{
				Content:      "Research summary file.",
				FinishReason: "stop",
			},
			{
				Content: rpiPlanWithItems(
					"- [ ] 1. Create summary.md — write v1",
				),
				FinishReason: "stop",
			},
			{
				Content: "Applying change",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-undo-1",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      "write_text_file",
							Arguments: `{"path":"summary.md","content":"v1"}`,
						},
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Content:      "Summary written.",
				FinishReason: "stop",
			},
		},
		streamResponses: []string{"Done"},
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Undo"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

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

	sendResp, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Create summary",
	}))
	if errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}
	userMessageID := sendResp.(map[string]any)["message_id"].(string)

	runResp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}
	assistantMessageID := runResp.(map[string]any)["message_id"].(string)

	if err := eng.workbenches.ApplyDraftWrite(workbenchID, "summary.md", "mutated"); err != nil {
		t.Fatalf("mutate draft: %v", err)
	}
	if _, errInfo := eng.WorkshopUndoToMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   assistantMessageID,
	})); errInfo != nil {
		t.Fatalf("undo to assistant: %v", errInfo)
	}
	content, err := eng.workbenches.ReadFile(workbenchID, "draft", "summary.md")
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if content != "v1" {
		t.Fatalf("expected restored draft content v1, got %q", content)
	}

	if _, errInfo := eng.WorkshopUndoToMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   userMessageID,
	})); errInfo != nil {
		t.Fatalf("undo to user: %v", errInfo)
	}
	draftState, errInfo := eng.DraftGetState(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("draft state: %v", errInfo)
	}
	if draftState.(map[string]any)["has_draft"] != false {
		t.Fatalf("expected draft removed after undo to user")
	}
	convoResp, errInfo := eng.WorkshopGetConversation(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("conversation: %v", errInfo)
	}
	messages := convoResp.(map[string]any)["messages"].([]conversationMessage)
	if len(messages) != 1 || messages[0].MessageID != userMessageID {
		t.Fatalf("expected conversation truncated to user message, got %#v", messages)
	}
}

func TestWorkshopRegenerateCreatesFreshAssistantMessageAfterRewind(t *testing.T) {
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
			{Content: "Research pass 1", FinishReason: "stop"},
			{
				Content:      rpiPlanWithItems("- [ ] 1. Respond — produce answer"),
				FinishReason: "stop",
			},
			{Content: "Implemented pass 1", FinishReason: "stop"},
			{Content: "Research pass 2", FinishReason: "stop"},
			{
				Content:      rpiPlanWithItems("- [ ] 1. Respond — produce answer"),
				FinishReason: "stop",
			},
			{Content: "Implemented pass 2", FinishReason: "stop"},
		},
		streamResponses: []string{"First answer", "Second answer"},
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "api_key": "sk-test"})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{"provider_id": "openai"})); errInfo != nil {
		t.Fatalf("validate: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{"provider_id": "openai", "enabled": true})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Regenerate"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

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

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "Answer me",
	})); errInfo != nil {
		t.Fatalf("send message: %v", errInfo)
	}
	firstRunResp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}
	firstAssistantID := firstRunResp.(map[string]any)["message_id"].(string)

	regenerateResp, errInfo := eng.WorkshopRegenerate(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   firstAssistantID,
	}))
	if errInfo != nil {
		t.Fatalf("regenerate: %v", errInfo)
	}
	newAssistantID := regenerateResp.(map[string]any)["message_id"].(string)
	if newAssistantID == "" || newAssistantID == firstAssistantID {
		t.Fatalf("expected fresh assistant id, got %q", newAssistantID)
	}

	convoResp, errInfo := eng.WorkshopGetConversation(ctx, mustJSON(t, map[string]any{"workbench_id": workbenchID}))
	if errInfo != nil {
		t.Fatalf("conversation: %v", errInfo)
	}
	messages := convoResp.(map[string]any)["messages"].([]conversationMessage)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after regenerate, got %d", len(messages))
	}
	if messages[1].Role != "assistant" || messages[1].MessageID != newAssistantID {
		t.Fatalf("expected regenerated assistant message, got %#v", messages[1])
	}
	if messages[1].Text != "Second answer" {
		t.Fatalf("expected regenerated text 'Second answer', got %q", messages[1].Text)
	}
}
