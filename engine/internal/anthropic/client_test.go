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
