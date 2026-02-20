package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"keenbench/engine/internal/llm"
)

func TestAPIKeyClientUsesXAPIKeyHeader(t *testing.T) {
	t.Helper()

	var gotAuthorization string
	var gotXAPIKey string
	var gotVersion string
	var gotBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotXAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotBeta = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		client:  server.Client(),
	}

	resp, err := client.Chat(context.Background(), "sk-ant-api-test", "claude-sonnet-4-6", []llm.Message{
		{Role: "user", Content: "Hello"},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected response %q, got %q", "ok", resp)
	}
	if gotAuthorization != "" {
		t.Fatalf("expected Authorization header to be empty, got %q", gotAuthorization)
	}
	if gotXAPIKey != "sk-ant-api-test" {
		t.Fatalf("expected x-api-key header, got %q", gotXAPIKey)
	}
	if gotVersion != defaultVersion {
		t.Fatalf("expected anthropic-version=%q, got %q", defaultVersion, gotVersion)
	}
	if gotBeta != "" {
		t.Fatalf("expected anthropic-beta to be empty for api key auth, got %q", gotBeta)
	}
}

func TestSetupTokenClientUsesOAuthHeaders(t *testing.T) {
	t.Helper()

	var gotAuthorization string
	var gotXAPIKey string
	var gotVersion string
	var gotBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotXAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotBeta = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL:        server.URL,
		client:         server.Client(),
		setupTokenAuth: true,
	}

	resp, err := client.Chat(context.Background(), "sk-ant-oat01-test-token", "claude-sonnet-4-6", []llm.Message{
		{Role: "user", Content: "Hello"},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected response %q, got %q", "ok", resp)
	}
	if gotAuthorization != "Bearer sk-ant-oat01-test-token" {
		t.Fatalf("expected bearer Authorization header, got %q", gotAuthorization)
	}
	if gotXAPIKey != "" {
		t.Fatalf("expected x-api-key header to be empty, got %q", gotXAPIKey)
	}
	if gotVersion != defaultVersion {
		t.Fatalf("expected anthropic-version=%q, got %q", defaultVersion, gotVersion)
	}
	if gotBeta != setupTokenBetaHeader {
		t.Fatalf("expected anthropic-beta=%q, got %q", setupTokenBetaHeader, gotBeta)
	}
}

func TestSetupTokenClientValidateKeyRequiresNonEmptyToken(t *testing.T) {
	t.Helper()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		baseURL:        server.URL,
		client:         server.Client(),
		setupTokenAuth: true,
	}

	if err := client.ValidateKey(context.Background(), "sk-ant-oat01-test-token"); err != nil {
		t.Fatalf("expected setup token validation to pass for non-empty token, got %v", err)
	}
	if called {
		t.Fatalf("expected setup token validation to avoid network requests")
	}

	if err := client.ValidateKey(context.Background(), "   "); err != llm.ErrUnauthorized {
		t.Fatalf("expected llm.ErrUnauthorized for empty setup token, got %v", err)
	}
	if called {
		t.Fatalf("expected setup token validation to avoid network requests")
	}
}

func TestChatUsesTopLevelSystemParameter(t *testing.T) {
	t.Helper()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		client:  server.Client(),
	}

	resp, err := client.Chat(context.Background(), "sk-test", "claude-sonnet-4-6", []llm.Message{
		{Role: "system", Content: "System instruction"},
		{Role: "user", Content: "Hello"},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected response %q, got %q", "ok", resp)
	}

	gotSystem, ok := payload["system"].(string)
	if !ok {
		t.Fatalf("expected payload.system string, got %#v", payload["system"])
	}
	if gotSystem != "System instruction" {
		t.Fatalf("expected payload.system to equal system prompt, got %q", gotSystem)
	}

	rawMessages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("expected payload.messages array, got %#v", payload["messages"])
	}
	if len(rawMessages) != 1 {
		t.Fatalf("expected 1 non-system message, got %d", len(rawMessages))
	}
	msg, ok := rawMessages[0].(map[string]any)
	if !ok {
		t.Fatalf("expected message object, got %#v", rawMessages[0])
	}
	if msg["role"] == "system" {
		t.Fatalf("did not expect system role in messages payload")
	}

	outputConfig, ok := payload["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload.output_config object, got %#v", payload["output_config"])
	}
	if got, ok := outputConfig["effort"].(string); !ok || got != "high" {
		t.Fatalf("expected output_config.effort=high by default, got %#v", outputConfig["effort"])
	}
}

func TestChatWithToolsUsesTopLevelSystemParameter(t *testing.T) {
	t.Helper()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		client:  server.Client(),
	}

	resp, err := client.ChatWithTools(context.Background(), "sk-test", "claude-opus-4-6", []llm.ChatMessage{
		{Role: "system", Content: "System A"},
		{Role: "system", Content: "System B"},
		{Role: "user", Content: "Run"},
	}, nil)
	if err != nil {
		t.Fatalf("chat with tools: %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("expected response %q, got %q", "done", resp.Content)
	}

	gotSystem, ok := payload["system"].(string)
	if !ok {
		t.Fatalf("expected payload.system string, got %#v", payload["system"])
	}
	if gotSystem != "System A\n\nSystem B" {
		t.Fatalf("expected joined system prompt, got %q", gotSystem)
	}

	rawMessages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("expected payload.messages array, got %#v", payload["messages"])
	}
	for _, raw := range rawMessages {
		msg, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected message object, got %#v", raw)
		}
		if msg["role"] == "system" {
			t.Fatalf("did not expect system role in messages payload")
		}
	}
}

func TestReasoningEffortMaxClampsToHighForSonnet(t *testing.T) {
	t.Helper()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		client:  server.Client(),
	}
	ctx := llm.WithRequestProfile(context.Background(), llm.RequestProfile{
		ReasoningEffort: "max",
	})
	if _, err := client.Chat(ctx, "sk-test", "claude-sonnet-4-6", []llm.Message{
		{Role: "user", Content: "Hello"},
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	outputConfig, ok := payload["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload.output_config object, got %#v", payload["output_config"])
	}
	if got := outputConfig["effort"]; got != "high" {
		t.Fatalf("expected clamped effort=high for sonnet, got %#v", got)
	}
}

func TestReasoningEffortMaxAllowedForOpus(t *testing.T) {
	t.Helper()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		client:  server.Client(),
	}
	ctx := llm.WithRequestProfile(context.Background(), llm.RequestProfile{
		ReasoningEffort: "max",
	})
	if _, err := client.Chat(ctx, "sk-test", "claude-opus-4-6", []llm.Message{
		{Role: "user", Content: "Hello"},
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	outputConfig, ok := payload["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload.output_config object, got %#v", payload["output_config"])
	}
	if got := outputConfig["effort"]; got != "max" {
		t.Fatalf("expected effort=max for opus, got %#v", got)
	}
}

func TestToAnthropicMessagesToolResultOmitsName(t *testing.T) {
	t.Helper()

	messages, _, err := toAnthropicMessages(nil, []llm.ChatMessage{
		{Role: "user", Content: "Run tool"},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "toolu_1",
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
		{Role: "tool", ToolCallID: "toolu_1", Content: "file body"},
	})
	if err != nil {
		t.Fatalf("toAnthropicMessages: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	raw, err := json.Marshal(messages[2])
	if err != nil {
		t.Fatalf("marshal tool_result message: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal tool_result message: %v", err)
	}

	content, ok := decoded["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("expected single content block, got %#v", decoded["content"])
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("expected content block object, got %#v", content[0])
	}
	if got := block["type"]; got != "tool_result" {
		t.Fatalf("expected tool_result block, got %#v", got)
	}
	if got := block["tool_use_id"]; got != "toolu_1" {
		t.Fatalf("expected tool_use_id=toolu_1, got %#v", got)
	}
	if _, exists := block["name"]; exists {
		t.Fatalf("tool_result must not include name, got %#v", block["name"])
	}
}

func TestToAnthropicMessagesToolUseIncludesInputWhenArgumentsEmpty(t *testing.T) {
	t.Helper()

	messages, _, err := toAnthropicMessages(nil, []llm.ChatMessage{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "toolu_1",
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      "read_file",
					Arguments: "",
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("toAnthropicMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	raw, err := json.Marshal(messages[0])
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	content, ok := decoded["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("expected single content block, got %#v", decoded["content"])
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("expected content block object, got %#v", content[0])
	}
	if got := block["type"]; got != "tool_use" {
		t.Fatalf("expected tool_use block, got %#v", got)
	}
	if _, exists := block["input"]; !exists {
		t.Fatalf("tool_use must include input")
	}
	input, ok := block["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected input object, got %#v", block["input"])
	}
	if len(input) != 0 {
		t.Fatalf("expected empty input object, got %#v", input)
	}
}

func TestToAnthropicMessagesRejectsToolResultWithoutToolUseID(t *testing.T) {
	t.Helper()

	if _, _, err := toAnthropicMessages(nil, []llm.ChatMessage{
		{Role: "tool", Content: "missing id"},
	}); err == nil {
		t.Fatalf("expected tool_result validation error")
	}
}

func TestToAnthropicMessagesRejectsAllSystemInput(t *testing.T) {
	t.Helper()

	if _, _, err := toAnthropicMessages([]llm.Message{
		{Role: "system", Content: "A"},
		{Role: "system", Content: "B"},
	}, nil); err == nil {
		t.Fatalf("expected missing non-system message validation error")
	}
}

func TestToAnthropicToolsBackfillsEmptyInputSchema(t *testing.T) {
	t.Helper()

	tools := toAnthropicTools([]llm.Tool{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "read_file",
			Description: "Read file",
		},
	}})
	if len(tools) != 1 {
		t.Fatalf("expected one tool, got %d", len(tools))
	}
	if !json.Valid(tools[0].InputSchema) {
		t.Fatalf("expected valid json schema, got %q", string(tools[0].InputSchema))
	}
	var schema map[string]any
	if err := json.Unmarshal(tools[0].InputSchema, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("expected object schema type, got %#v", schema["type"])
	}
}
