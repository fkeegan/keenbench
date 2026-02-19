package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
)

type scriptedRPIToolTurn struct {
	resp llm.ChatResponse
	err  error
}

type scriptedRPIStreamTurn struct {
	text string
	err  error
}

type scriptedRPIClient struct {
	toolTurns []scriptedRPIToolTurn
	streams   []scriptedRPIStreamTurn

	toolIndex   int
	streamIndex int

	toolSets            [][]string
	streamToolDeltaHits int
	streamDeltaHits     int
	toolRequestEfforts  []string
	streamEfforts       []string
}

func (s *scriptedRPIClient) ValidateKey(ctx context.Context, apiKey string) error {
	return nil
}

func (s *scriptedRPIClient) Chat(ctx context.Context, apiKey, model string, messages []llm.Message) (string, error) {
	return "", nil
}

func (s *scriptedRPIClient) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
	s.streamEfforts = append(s.streamEfforts, requestReasoningEffort(ctx))
	turn := scriptedRPIStreamTurn{text: ""}
	if s.streamIndex < len(s.streams) {
		turn = s.streams[s.streamIndex]
	}
	s.streamIndex++
	if turn.err != nil {
		return "", turn.err
	}
	if onDelta != nil && turn.text != "" {
		s.streamDeltaHits++
		onDelta(turn.text)
	}
	return turn.text, nil
}

func (s *scriptedRPIClient) ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	return s.StreamChatWithTools(ctx, apiKey, model, messages, tools, nil)
}

func (s *scriptedRPIClient) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	s.toolRequestEfforts = append(s.toolRequestEfforts, requestReasoningEffort(ctx))
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Function.Name)
	}
	s.toolSets = append(s.toolSets, toolNames)

	turn := scriptedRPIToolTurn{resp: llm.ChatResponse{Content: "", FinishReason: "stop"}}
	if s.toolIndex < len(s.toolTurns) {
		turn = s.toolTurns[s.toolIndex]
	}
	s.toolIndex++
	if turn.err != nil {
		return llm.ChatResponse{}, turn.err
	}
	if onDelta != nil && turn.resp.Content != "" {
		s.streamToolDeltaHits++
		onDelta(turn.resp.Content)
	}
	return turn.resp, nil
}

func requestReasoningEffort(ctx context.Context) string {
	profile, ok := llm.RequestProfileFromContext(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(profile.ReasoningEffort)
}

type blockingRPIClient struct {
	started chan struct{}
}

func (b *blockingRPIClient) ValidateKey(ctx context.Context, apiKey string) error {
	return nil
}

func (b *blockingRPIClient) Chat(ctx context.Context, apiKey, model string, messages []llm.Message) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (b *blockingRPIClient) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (b *blockingRPIClient) ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	return b.StreamChatWithTools(ctx, apiKey, model, messages, tools, nil)
}

func (b *blockingRPIClient) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	if b.started != nil {
		select {
		case b.started <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}

func setupRPIWorkflowRun(t *testing.T, client LLMClient, userText string) (*Engine, string, string, string) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("KEENBENCH_DATA_DIR", dataDir)
	t.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")

	ctx := context.Background()
	eng, err := New()
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	eng.providers[ProviderOpenAI] = client

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"api_key":     "sk-test",
	})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersValidate(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
	})); errInfo != nil {
		t.Fatalf("validate provider: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"enabled":     true,
	})); errInfo != nil {
		t.Fatalf("enable provider: %v", errInfo)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "RPI Workflow"}))
	if errInfo != nil {
		t.Fatalf("create workbench: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	src := filepath.Join(dataDir, "seed.txt")
	if err := os.WriteFile(src, []byte("seed"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{src},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	consentStatus, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
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
		"text":         userText,
	}))
	if errInfo != nil {
		t.Fatalf("send user message: %v", errInfo)
	}
	messageID := sendResp.(map[string]any)["message_id"].(string)
	return eng, workbenchID, messageID, dataDir
}

func TestRPIOrchestratorFullCycle(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{resp: llm.ChatResponse{Content: "Research findings.", FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: strings.Join([]string{
				"# Execution Plan",
				"",
				"## Task",
				"Create output files.",
				"",
				"## Items",
				"- [ ] 1. First item — write one.txt",
				"- [ ] 2. Second item — write two.txt",
			}, "\n"), FinishReason: "stop"}},
			{resp: llm.ChatResponse{
				Content: "Writing first item.",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      "write_text_file",
							Arguments: `{"path":"one.txt","content":"one"}`,
						},
					},
				},
				FinishReason: "tool_calls",
			}},
			{resp: llm.ChatResponse{Content: "First item done.", FinishReason: "stop"}},
			{resp: llm.ChatResponse{
				Content: "Writing second item.",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-2",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      "write_text_file",
							Arguments: `{"path":"two.txt","content":"two"}`,
						},
					},
				},
				FinishReason: "tool_calls",
			}},
			{resp: llm.ChatResponse{Content: "Second item done.", FinishReason: "stop"}},
		},
		streams: []scriptedRPIStreamTurn{
			{text: "Final summary."},
		},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Please process files.")
	ctx := context.Background()

	var notifications []testNotification
	eng.SetNotifier(func(method string, params any) {
		payload, _ := params.(map[string]any)
		notifications = append(notifications, testNotification{method: method, params: payload})
	})

	resp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}
	result := resp.(map[string]any)
	if result["message_id"] == "" {
		t.Fatalf("expected assistant message id")
	}
	if result["has_draft"] != true {
		t.Fatalf("expected has_draft=true, got %v", result["has_draft"])
	}

	research, err := eng.readRPIArtifact(workbenchID, rpiResearchFile)
	if err != nil {
		t.Fatalf("read research: %v", err)
	}
	if !strings.Contains(research, "Research findings.") {
		t.Fatalf("unexpected research artifact: %q", research)
	}
	plan, err := eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if !strings.Contains(plan, "<!-- original_count: 2 -->") {
		t.Fatalf("missing original_count metadata: %q", plan)
	}
	if !strings.Contains(plan, "- [x] 1. First item — write one.txt") || !strings.Contains(plan, "- [x] 2. Second item — write two.txt") {
		t.Fatalf("expected completed plan items, got:\n%s", plan)
	}

	conversation, err := eng.readConversation(workbenchID)
	if err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	if len(conversation) != 2 {
		t.Fatalf("expected only user + summary assistant in conversation, got %d entries", len(conversation))
	}
	if conversation[0].Role != "user" || conversation[1].Role != "assistant" {
		t.Fatalf("unexpected conversation roles: %#v", conversation)
	}
	if conversation[1].Metadata == nil {
		t.Fatalf("expected summary metadata")
	}
	elapsedRaw, ok := conversation[1].Metadata["job_elapsed_ms"]
	if !ok {
		t.Fatalf("expected job_elapsed_ms metadata, got %v", conversation[1].Metadata)
	}
	elapsedMS, ok := elapsedRaw.(float64)
	if !ok {
		t.Fatalf("expected numeric job_elapsed_ms, got %T", elapsedRaw)
	}
	if elapsedMS < 0 {
		t.Fatalf("expected non-negative job_elapsed_ms, got %v", elapsedMS)
	}

	phaseStarts := map[string]bool{}
	phaseCompletes := map[string]bool{}
	streamDeltaCount := 0
	progressCount := 0
	for _, n := range notifications {
		switch n.method {
		case "WorkshopPhaseStarted":
			phaseStarts[n.params["phase"].(string)] = true
		case "WorkshopPhaseCompleted":
			phaseCompletes[n.params["phase"].(string)] = true
		case "WorkshopAssistantStreamDelta":
			streamDeltaCount++
		case "WorkshopImplementProgress":
			progressCount++
		}
	}
	for _, phase := range []string{"research", "plan", "implement", "summary"} {
		if !phaseStarts[phase] {
			t.Fatalf("missing WorkshopPhaseStarted for %s", phase)
		}
		if !phaseCompletes[phase] {
			t.Fatalf("missing WorkshopPhaseCompleted for %s", phase)
		}
	}
	if progressCount != 2 {
		t.Fatalf("expected implement progress for 2 items, got %d", progressCount)
	}
	if streamDeltaCount != 1 {
		t.Fatalf("expected summary-only streaming delta (1), got %d", streamDeltaCount)
	}
	if client.streamToolDeltaHits != 0 {
		t.Fatalf("expected no StreamChatWithTools delta callbacks in R/P/I phases, got %d", client.streamToolDeltaHits)
	}

	if len(client.toolSets) < 6 {
		t.Fatalf("expected >=6 tool turns, got %d", len(client.toolSets))
	}
	researchToolSet := client.toolSets[0]
	planToolSet := client.toolSets[1]
	if slices.Contains(researchToolSet, "write_text_file") {
		t.Fatalf("research tools must be read-only, got %+v", researchToolSet)
	}
	if !slices.Contains(researchToolSet, "table_query") {
		t.Fatalf("expected research tool subset, got %+v", researchToolSet)
	}
	if len(planToolSet) != 2 || !slices.Contains(planToolSet, "read_file") || !slices.Contains(planToolSet, "recall_tool_result") {
		t.Fatalf("expected plan tool subset, got %+v", planToolSet)
	}
}

func TestRPIOrchestratorPropagatesPerPhaseReasoningEffortProfile(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{resp: llm.ChatResponse{Content: "Research findings.", FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: strings.Join([]string{
				"# Execution Plan",
				"",
				"## Task",
				"Do one thing.",
				"",
				"## Items",
				"- [ ] 1. Mark complete",
			}, "\n"), FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: "Implemented.", FinishReason: "stop"}},
		},
		streams: []scriptedRPIStreamTurn{
			{text: "Summary"},
		},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Run with phase profiles.")
	ctx := context.Background()

	if _, errInfo := eng.ProvidersSetReasoningEffort(ctx, mustJSON(t, map[string]any{
		"provider_id":      ProviderOpenAI,
		"research_effort":  "none",
		"plan_effort":      "low",
		"implement_effort": "high",
	})); errInfo != nil {
		t.Fatalf("set reasoning effort: %v", errInfo)
	}

	if _, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	})); errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}

	if len(client.toolRequestEfforts) != 3 {
		t.Fatalf("expected 3 R/P/I tool requests, got %d (%v)", len(client.toolRequestEfforts), client.toolRequestEfforts)
	}
	if client.toolRequestEfforts[0] != "none" || client.toolRequestEfforts[1] != "low" || client.toolRequestEfforts[2] != "high" {
		t.Fatalf("unexpected R/P/I request profile efforts: %v", client.toolRequestEfforts)
	}
	if len(client.streamEfforts) != 1 {
		t.Fatalf("expected one summary stream request, got %d", len(client.streamEfforts))
	}
	if client.streamEfforts[0] != "" {
		t.Fatalf("expected summary phase to have no request profile override, got %q", client.streamEfforts[0])
	}
}

func TestRPIOrchestratorResearchFails(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{err: llm.ErrUnavailable},
		},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Do work")
	ctx := context.Background()

	_, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	}))
	if errInfo == nil {
		t.Fatalf("expected error")
	}
	if errInfo.Subphase != errinfo.SubphaseRPIResearch {
		t.Fatalf("expected subphase %s, got %s", errinfo.SubphaseRPIResearch, errInfo.Subphase)
	}

	conversation, err := eng.readConversation(workbenchID)
	if err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	if len(conversation) != 1 || conversation[0].Role != "user" {
		t.Fatalf("expected only user message persisted, got %#v", conversation)
	}
}

func TestRPIOrchestratorImplementRetrySucceeds(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{resp: llm.ChatResponse{Content: "Research", FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: strings.Join([]string{
				"# Execution Plan",
				"",
				"## Task",
				"Test retry",
				"",
				"## Items",
				"- [ ] 1. Retry item — write retry.txt",
			}, "\n"), FinishReason: "stop"}},
			{err: llm.ErrUnavailable}, // first implement attempt fails
			{resp: llm.ChatResponse{
				Content: "Retry write",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-retry",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      "write_text_file",
							Arguments: `{"path":"retry.txt","content":"ok"}`,
						},
					},
				},
				FinishReason: "tool_calls",
			}},
			{resp: llm.ChatResponse{Content: "Retry success", FinishReason: "stop"}},
		},
		streams: []scriptedRPIStreamTurn{{text: "Summary"}},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Retry test")
	ctx := context.Background()

	if _, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	})); errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}

	plan, err := eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if !strings.Contains(plan, "- [x] 1. Retry item — write retry.txt") {
		t.Fatalf("expected retried item marked done, got:\n%s", plan)
	}
	if strings.Contains(plan, "- [!]") {
		t.Fatalf("did not expect failed item marker, got:\n%s", plan)
	}
}

func TestRPIOrchestratorImplementRetryFailsThenContinues(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{resp: llm.ChatResponse{Content: "Research", FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: strings.Join([]string{
				"# Execution Plan",
				"",
				"## Task",
				"Continue after failure",
				"",
				"## Items",
				"- [ ] 1. Failing item — this one fails",
				"- [ ] 2. Working item — write ok.txt",
			}, "\n"), FinishReason: "stop"}},
			{err: llm.ErrUnavailable}, // item 1 first attempt
			{err: llm.ErrUnavailable}, // item 1 retry fails
			{resp: llm.ChatResponse{
				Content: "write",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tc-ok",
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      "write_text_file",
							Arguments: `{"path":"ok.txt","content":"ok"}`,
						},
					},
				},
				FinishReason: "tool_calls",
			}},
			{resp: llm.ChatResponse{Content: "done", FinishReason: "stop"}},
		},
		streams: []scriptedRPIStreamTurn{{text: "Summary"}},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Continue test")
	ctx := context.Background()

	if _, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	})); errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}

	plan, err := eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if !strings.Contains(plan, "- [!] 1. Failing item — this one fails [Failed:") {
		t.Fatalf("expected first item marked failed with reason, got:\n%s", plan)
	}
	if !strings.Contains(plan, "- [x] 2. Working item — write ok.txt") {
		t.Fatalf("expected second item to continue and complete, got:\n%s", plan)
	}
}

func TestRPIOrchestratorImplementRateLimitStopsRunWithBackoff(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{resp: llm.ChatResponse{Content: "Research", FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: strings.Join([]string{
				"# Execution Plan",
				"",
				"## Task",
				"Stop on rate limits",
				"",
				"## Items",
				"- [ ] 1. Rate-limited item",
				"- [ ] 2. Should never run",
			}, "\n"), FinishReason: "stop"}},
			{err: llm.ErrRateLimited},
			{err: llm.ErrRateLimited},
			{err: llm.ErrRateLimited},
			{err: llm.ErrRateLimited},
			{err: llm.ErrRateLimited},
			{err: llm.ErrRateLimited},
		},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Rate-limit test")
	ctx := context.Background()

	var waits []time.Duration
	eng.sleep = func(ctx context.Context, wait time.Duration) error {
		waits = append(waits, wait)
		return nil
	}

	_, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	}))
	if errInfo == nil {
		t.Fatalf("expected rate-limit error")
	}
	if errInfo.Subphase != errinfo.SubphaseRPIImplement {
		t.Fatalf("expected subphase %s, got %s", errinfo.SubphaseRPIImplement, errInfo.Subphase)
	}
	if errInfo.ErrorCode != errinfo.CodeProviderUnavailable {
		t.Fatalf("expected error code %s, got %s", errinfo.CodeProviderUnavailable, errInfo.ErrorCode)
	}
	if len(waits) != 5 {
		t.Fatalf("expected 5 backoff waits, got %d (%v)", len(waits), waits)
	}
	if waits[0] != 10*time.Second ||
		waits[1] != 20*time.Second ||
		waits[2] != 40*time.Second ||
		waits[3] != 80*time.Second ||
		waits[4] != 160*time.Second {
		t.Fatalf("unexpected backoff sequence: %v", waits)
	}

	plan, err := eng.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if !strings.Contains(plan, "- [ ] 1. Rate-limited item") {
		t.Fatalf("expected first item to remain pending, got:\n%s", plan)
	}
	if !strings.Contains(plan, "- [ ] 2. Should never run") {
		t.Fatalf("expected second item to remain pending, got:\n%s", plan)
	}

	conversation, err := eng.readConversation(workbenchID)
	if err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	if len(conversation) != 1 || conversation[0].Role != "user" {
		t.Fatalf("expected only user message (no summary on hard stop), got %#v", conversation)
	}
}

func TestWorkshopRunAgentCancelStopsInFlightRun(t *testing.T) {
	client := &blockingRPIClient{started: make(chan struct{}, 1)}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "Cancel run")
	ctx := context.Background()

	done := make(chan *errinfo.ErrorInfo, 1)
	go func() {
		_, runErr := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
			"workbench_id": workbenchID,
			"message_id":   messageID,
		}))
		done <- runErr
	}()

	select {
	case <-client.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("run did not reach model call in time")
	}

	cancelResp, cancelErr := eng.WorkshopCancelRun(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if cancelErr != nil {
		t.Fatalf("cancel run: %v", cancelErr)
	}
	if cancelResp.(map[string]any)["cancel_requested"] != true {
		t.Fatalf("expected cancel_requested=true, got %#v", cancelResp)
	}

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Fatalf("expected canceled run error")
		}
		if runErr.ErrorCode != errinfo.CodeUserCanceled {
			t.Fatalf("expected USER_CANCELED, got %s", runErr.ErrorCode)
		}
		if runErr.Subphase != errinfo.SubphaseRPIResearch {
			t.Fatalf("expected subphase %s, got %s", errinfo.SubphaseRPIResearch, runErr.Subphase)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for canceled run")
	}
}

func TestRPIOrchestratorNewMessageClearsState(t *testing.T) {
	client := &scriptedRPIClient{
		toolTurns: []scriptedRPIToolTurn{
			{resp: llm.ChatResponse{Content: "Research", FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: strings.Join([]string{
				"# Execution Plan",
				"",
				"## Task",
				"One item",
				"",
				"## Items",
				"- [ ] 1. Mark done",
			}, "\n"), FinishReason: "stop"}},
			{resp: llm.ChatResponse{Content: "Done without tools", FinishReason: "stop"}},
		},
		streams: []scriptedRPIStreamTurn{{text: "Summary"}},
	}
	eng, workbenchID, messageID, _ := setupRPIWorkflowRun(t, client, "First run")
	ctx := context.Background()

	runResp, errInfo := eng.WorkshopRunAgent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"message_id":   messageID,
	}))
	if errInfo != nil {
		t.Fatalf("run agent: %v", errInfo)
	}
	if runResp.(map[string]any)["has_draft"] != false {
		t.Fatalf("expected has_draft=false for no-op run, got %v", runResp.(map[string]any)["has_draft"])
	}
	conversation, err := eng.readConversation(workbenchID)
	if err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	if len(conversation) != 2 || conversation[1].Role != "assistant" {
		t.Fatalf("expected user + summary assistant, got %#v", conversation)
	}
	if conversation[1].Metadata != nil {
		if _, ok := conversation[1].Metadata["job_elapsed_ms"]; ok {
			t.Fatalf("did not expect job_elapsed_ms metadata for no-draft run")
		}
	}
	state := eng.readRPIState(workbenchID)
	if !state.HasResearch || !state.HasPlan {
		t.Fatalf("expected RPI artifacts after first run, got %#v", state)
	}

	if _, errInfo := eng.WorkshopSendUserMessage(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"text":         "new prompt",
	})); errInfo != nil {
		t.Fatalf("send second message: %v", errInfo)
	}
	state = eng.readRPIState(workbenchID)
	if state.HasResearch || state.HasPlan {
		t.Fatalf("expected RPI state cleared after new user message, got %#v", state)
	}

	_, err = os.Stat(filepath.Join(eng.rpiDir(workbenchID), rpiPlanFile))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected plan artifact cleared, stat err=%v", err)
	}
}
