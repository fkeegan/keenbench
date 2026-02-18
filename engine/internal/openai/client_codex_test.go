package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"keenbench/engine/internal/llm"
)

type captureRoundTripper struct {
	lastHeaders     http.Header
	lastURL         string
	lastBody        string
	statusCode      int
	responseBody    string
	responseHeaders http.Header
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.lastHeaders = req.Header.Clone()
	c.lastURL = req.URL.String()
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		c.lastBody = string(body)
	}
	status := c.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	body := strings.TrimSpace(c.responseBody)
	if body == "" {
		if strings.Contains(c.lastBody, `"stream":true`) {
			body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n" +
				"data: [DONE]\n\n"
		} else {
			body = `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`
		}
	}
	headers := make(http.Header)
	for key, values := range c.responseHeaders {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestCodexClientAddsCodexHeaders(t *testing.T) {
	rt := &captureRoundTripper{}
	client := &Client{
		baseURL: defaultCodexBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   true,
	}
	token := buildAccessTokenWithAccountID("acct_123")

	_, err := client.Chat(context.Background(), token, "gpt-5.3-codex", []llm.Message{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if got := rt.lastHeaders.Get("Authorization"); got != "Bearer "+token {
		t.Fatalf("expected Authorization Bearer token, got %q", got)
	}
	if got := rt.lastHeaders.Get("chatgpt-account-id"); got != "acct_123" {
		t.Fatalf("expected chatgpt-account-id=acct_123, got %q", got)
	}
	if got := rt.lastHeaders.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("expected OpenAI-Beta responses=experimental, got %q", got)
	}
	if got := rt.lastHeaders.Get("originator"); got != "pi" {
		t.Fatalf("expected originator=pi, got %q", got)
	}
	if !strings.Contains(rt.lastURL, "/codex/responses") {
		t.Fatalf("expected codex endpoint in URL, got %q", rt.lastURL)
	}
}

func TestCodexClientPayloadIncludesInstructions(t *testing.T) {
	rt := &captureRoundTripper{}
	client := &Client{
		baseURL: defaultCodexBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   true,
	}
	token := buildAccessTokenWithAccountID("acct_123")

	_, err := client.Chat(context.Background(), token, "gpt-5.3-codex", []llm.Message{
		{Role: "system", Content: "Follow the repo conventions."},
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rt.lastBody), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, _ := payload["instructions"].(string); got != "Follow the repo conventions." {
		t.Fatalf("expected instructions from system message, got %q", got)
	}
	input, ok := payload["input"].([]any)
	if !ok {
		t.Fatalf("expected input array, got %#v", payload["input"])
	}
	if len(input) != 1 {
		t.Fatalf("expected only user message in input, got %d items", len(input))
	}
	first, _ := input[0].(map[string]any)
	if got, _ := first["role"].(string); got != "user" {
		t.Fatalf("expected user role, got %q", got)
	}
	if got, _ := payload["store"].(bool); got {
		t.Fatalf("expected store=false for codex payload, got %#v", payload["store"])
	}
	if got, ok := payload["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true for codex payload, got %#v", payload["stream"])
	}
}

func TestCodexClientPayloadUsesFallbackInstructions(t *testing.T) {
	rt := &captureRoundTripper{}
	client := &Client{
		baseURL: defaultCodexBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   true,
	}
	token := buildAccessTokenWithAccountID("acct_123")

	_, err := client.Chat(context.Background(), token, "gpt-5.3-codex", []llm.Message{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rt.lastBody), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, _ := payload["instructions"].(string); got != codexDefaultInstructions {
		t.Fatalf("expected fallback instructions %q, got %q", codexDefaultInstructions, got)
	}
}

func TestCodexClientPayloadOmitsUnsupportedParams(t *testing.T) {
	rt := &captureRoundTripper{}
	client := &Client{
		baseURL: defaultCodexBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   true,
	}
	token := buildAccessTokenWithAccountID("acct_123")

	_, err := client.Chat(context.Background(), token, "gpt-5.3-codex", []llm.Message{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rt.lastBody), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if _, ok := payload["temperature"]; ok {
		t.Fatalf("expected temperature to be omitted for codex payload, got %#v", payload["temperature"])
	}
	if _, ok := payload["top_p"]; ok {
		t.Fatalf("expected top_p to be omitted for codex payload, got %#v", payload["top_p"])
	}
	if _, ok := payload["truncation"]; ok {
		t.Fatalf("expected truncation to be omitted for codex payload, got %#v", payload["truncation"])
	}
}

func TestStandardOpenAIClientDoesNotSetCodexHeaders(t *testing.T) {
	rt := &captureRoundTripper{}
	client := &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   false,
	}
	apiKey := "sk-test-123"

	_, err := client.Chat(context.Background(), apiKey, "gpt-5.2", []llm.Message{
		{Role: "system", Content: "System instructions stay in input for API-key mode."},
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if got := rt.lastHeaders.Get("chatgpt-account-id"); got != "" {
		t.Fatalf("expected no chatgpt-account-id header, got %q", got)
	}
	if got := rt.lastHeaders.Get("OpenAI-Beta"); got != "" {
		t.Fatalf("expected no OpenAI-Beta header, got %q", got)
	}
	if got := rt.lastHeaders.Get("originator"); got != "" {
		t.Fatalf("expected no originator header, got %q", got)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rt.lastBody), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if _, exists := payload["instructions"]; exists {
		t.Fatalf("expected no instructions field for standard OpenAI payload, got %#v", payload["instructions"])
	}
	input, ok := payload["input"].([]any)
	if !ok {
		t.Fatalf("expected input array, got %#v", payload["input"])
	}
	if len(input) != 2 {
		t.Fatalf("expected 2 input items including system message, got %d", len(input))
	}
	first, _ := input[0].(map[string]any)
	if got, _ := first["role"].(string); got != "system" {
		t.Fatalf("expected first role=system, got %q", got)
	}
}

func TestCodexToolPayloadIncludesInstructionsAndSkipsSystemMessages(t *testing.T) {
	rt := &captureRoundTripper{}
	client := &Client{
		baseURL: defaultCodexBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   true,
	}
	token := buildAccessTokenWithAccountID("acct_123")
	messages := []llm.ChatMessage{
		{Role: "system", Content: "Use tools only when needed."},
		{Role: "user", Content: "List files"},
	}

	_, err := client.ChatWithTools(context.Background(), token, "gpt-5.3-codex", messages, nil)
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rt.lastBody), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, _ := payload["instructions"].(string); got != "Use tools only when needed." {
		t.Fatalf("expected instructions from system message, got %q", got)
	}
	input, ok := payload["input"].([]any)
	if !ok {
		t.Fatalf("expected input array, got %#v", payload["input"])
	}
	if len(input) != 1 {
		t.Fatalf("expected one non-system input item, got %d", len(input))
	}
	first, _ := input[0].(map[string]any)
	if got, _ := first["role"].(string); got != "user" {
		t.Fatalf("expected user role, got %q", got)
	}
	if got, ok := payload["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true for codex tool payload, got %#v", payload["stream"])
	}
}

func TestCodexUnauthorizedErrorIncludesDiagnostics(t *testing.T) {
	rt := &captureRoundTripper{
		statusCode:   http.StatusUnauthorized,
		responseBody: `{"error":{"message":"Unauthorized"}}`,
		responseHeaders: http.Header{
			"X-Request-Id": []string{"req_123"},
		},
	}
	client := &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Transport: rt},
		codex:   true,
	}
	token := buildAccessTokenWithAccountID("acct_123")

	_, err := client.Chat(context.Background(), token, "gpt-5.3-codex", []llm.Message{
		{Role: "user", Content: "hello"},
	})
	if err == nil {
		t.Fatalf("expected unauthorized error")
	}
	if !errors.Is(err, llm.ErrUnauthorized) {
		t.Fatalf("expected wrapped llm.ErrUnauthorized, got %v", err)
	}
	errText := err.Error()
	if !strings.Contains(errText, "request_id=req_123") {
		t.Fatalf("expected request id in error text, got %q", errText)
	}
	if !strings.Contains(errText, "codex=true") {
		t.Fatalf("expected codex=true in error text, got %q", errText)
	}
	if !strings.Contains(errText, "chatgpt_account_header=true") {
		t.Fatalf("expected account header flag in error text, got %q", errText)
	}
}

func buildAccessTokenWithAccountID(accountID string) string {
	payload := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
		},
	}
	encodedPayload, _ := json.Marshal(payload)
	return strings.Join([]string{
		"header",
		base64.RawURLEncoding.EncodeToString(encodedPayload),
		"signature",
	}, ".")
}
