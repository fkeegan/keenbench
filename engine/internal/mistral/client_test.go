package mistral

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

type mockRT struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestAllowlistRoundTripper(t *testing.T) {
	called := false
	rt := egress.NewAllowlistRoundTripper(&mockRT{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			called = true
			return response(http.StatusOK, "{}"), nil
		},
	}, []string{"api.mistral.ai"})

	req, _ := http.NewRequest(http.MethodGet, "https://api.mistral.ai/v1/models", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	if !called {
		t.Fatalf("expected allowlisted request to reach base transport")
	}

	blockedReq, _ := http.NewRequest(http.MethodGet, "https://example.com/v1/models", nil)
	if _, err := rt.RoundTrip(blockedReq); err != llm.ErrEgressBlocked {
		t.Fatalf("expected egress blocked error, got %v", err)
	}
}

func TestValidateKey(t *testing.T) {
	client := &Client{
		baseURL: "https://api.mistral.ai",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/models" {
					t.Fatalf("expected /v1/models, got %s", req.URL.Path)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
					t.Fatalf("unexpected authorization header: %q", got)
				}
				return response(http.StatusOK, "{}"), nil
			},
		}},
	}
	if err := client.ValidateKey(context.Background(), "sk-test"); err != nil {
		t.Fatalf("validate key failed: %v", err)
	}
}

func TestValidateKeyUnauthorized(t *testing.T) {
	client := &Client{
		baseURL: "https://api.mistral.ai",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return response(http.StatusUnauthorized, `{"message":"unauthorized"}`), nil
			},
		}},
	}
	if err := client.ValidateKey(context.Background(), "sk-test"); err != llm.ErrUnauthorized {
		t.Fatalf("expected llm.ErrUnauthorized, got %v", err)
	}
}

func TestChat(t *testing.T) {
	client := &Client{
		baseURL: "https://api.mistral.ai",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/chat/completions" {
					t.Fatalf("expected /v1/chat/completions, got %s", req.URL.Path)
				}
				return response(http.StatusOK, `{"choices":[{"message":{"content":"Hello"}}]}`), nil
			},
		}},
	}
	got, err := client.Chat(context.Background(), "sk-test", "mistral-large-latest", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if got != "Hello" {
		t.Fatalf("expected Hello, got %q", got)
	}
}

func TestChatWithToolsParsesToolCallWithObjectArgs(t *testing.T) {
	client := &Client{
		baseURL: "https://api.mistral.ai",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return response(http.StatusOK, `{"choices":[{"message":{"content":"Checking...","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":{"path":"notes.txt"}}}]}}]}`), nil
			},
		}},
	}
	resp, err := client.ChatWithTools(
		context.Background(),
		"sk-test",
		"mistral-large-latest",
		[]llm.ChatMessage{{Role: "user", Content: "Read notes.txt"}},
		[]llm.Tool{{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		}},
	)
	if err != nil {
		t.Fatalf("chat with tools failed: %v", err)
	}
	if resp.Content != "Checking..." {
		t.Fatalf("expected content Checking..., got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	call := resp.ToolCalls[0]
	if call.ID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %q", call.ID)
	}
	if call.Function.Name != "read_file" {
		t.Fatalf("expected tool function read_file, got %q", call.Function.Name)
	}
	if call.Function.Arguments != `{"path":"notes.txt"}` {
		t.Fatalf("expected object args, got %q", call.Function.Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("expected finish reason tool_calls, got %q", resp.FinishReason)
	}
}

func TestToMistralMessagesTracksToolNameForToolResult(t *testing.T) {
	messages := toMistralMessages([]llm.ChatMessage{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_99",
					Type: "function",
					Function: llm.ToolCallFunction{
						Name:      "list_files",
						Arguments: "{}",
					},
				},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_99",
			Content:    "[]",
		},
	})
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[1].Role != "tool" {
		t.Fatalf("expected tool role, got %q", messages[1].Role)
	}
	if messages[1].Name != "list_files" {
		t.Fatalf("expected tool name list_files, got %q", messages[1].Name)
	}
}
