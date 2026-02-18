package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"keenbench/engine/internal/llm"
)

type routeRT struct{}

func (r *routeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/v1/models":
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("{}")),
			Header:     make(http.Header),
		}, nil
	case "/v1/responses":
		bodyBytes, _ := io.ReadAll(req.Body)
		if strings.Contains(string(bodyBytes), "\"stream\":true") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(strings.NewReader(
					"event: response.output_text.delta\n" +
						"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n" +
						"event: response.completed\n" +
						"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hi\"}]}]}}\n\n")),
				Header: make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}`)),
			Header:     make(http.Header),
		}, nil
	default:
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}
}

type captureRT struct {
	statusCode   int
	responseBody string
	payloads     []map[string]any
}

func (r *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/v1/models":
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("{}")),
			Header:     make(http.Header),
		}, nil
	case "/v1/responses":
		bodyBytes, _ := io.ReadAll(req.Body)
		var payload map[string]any
		_ = json.Unmarshal(bodyBytes, &payload)
		r.payloads = append(r.payloads, payload)
		status := r.statusCode
		if status == 0 {
			status = 200
		}
		body := strings.TrimSpace(r.responseBody)
		if body == "" {
			body = `{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}`
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	default:
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	}
}

func TestClientChat(t *testing.T) {
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: &routeRT{}},
	}
	resp, err := client.Chat(contextBackground(), "sk-test", "gpt-5.2", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp != "Hello" {
		t.Fatalf("expected Hello, got %s", resp)
	}
}

func TestClientStreamChat(t *testing.T) {
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: &routeRT{}},
	}
	var collected strings.Builder
	resp, err := client.StreamChat(contextBackground(), "sk-test", "gpt-5.2", []llm.Message{{Role: "user", Content: "hi"}}, func(delta string) {
		collected.WriteString(delta)
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp != "Hi" {
		t.Fatalf("expected Hi, got %s", resp)
	}
	if collected.String() != "Hi" {
		t.Fatalf("expected collected Hi")
	}
}

func TestClientValidate(t *testing.T) {
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: &routeRT{}},
	}
	if err := client.ValidateKey(contextBackground(), "sk-test"); err != nil {
		t.Fatalf("validate error: %v", err)
	}
}

// toolRT handles tool call responses for testing
type toolRT struct{}

func (r *toolRT) RoundTrip(req *http.Request) (*http.Response, error) {
	bodyBytes, _ := io.ReadAll(req.Body)
	bodyStr := string(bodyBytes)

	if strings.Contains(bodyStr, "\"stream\":true") {
		// Streaming response with tool call
		if strings.Contains(bodyStr, "\"tools\"") && !strings.Contains(bodyStr, "function_call_output") {
			// Initial request with tools - return tool call
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(strings.NewReader(
					"event: response.output_text.delta\n" +
						"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Let me check\"}\n\n" +
						"event: response.completed\n" +
						"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Let me check\"}]},{\"type\":\"function_call\",\"call_id\":\"call_123\",\"name\":\"list_files\",\"arguments\":\"{}\"}]}}\n\n")),
				Header: make(http.Header),
			}, nil
		}
		// Follow-up after tool result
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(strings.NewReader(
				"event: response.output_text.delta\n" +
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Done!\"}\n\n" +
					"event: response.completed\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Done!\"}]}]}}\n\n")),
			Header: make(http.Header),
		}, nil
	}

	// Non-streaming response with tool call
	if strings.Contains(bodyStr, "\"tools\"") && !strings.Contains(bodyStr, "function_call_output") {
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(strings.NewReader(`{"output":[
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Let me check"}]},
				{"type":"function_call","call_id":"call_456","name":"read_file","arguments":"{\"path\":\"test.txt\"}"}
			]}`)),
			Header: make(http.Header),
		}, nil
	}

	// Response after tool result
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done!"}]}]}`)),
		Header:     make(http.Header),
	}, nil
}

func TestChatWithTools(t *testing.T) {
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: &toolRT{}},
	}
	tools := []llm.Tool{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}}
	messages := []llm.ChatMessage{{Role: "user", Content: "Read test.txt"}}

	resp, err := client.ChatWithTools(contextBackground(), "sk-test", "gpt-4", messages, tools)
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}
	if resp.Content != "Let me check" {
		t.Fatalf("expected 'Let me check', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("expected read_file tool call, got %s", resp.ToolCalls[0].Function.Name)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %s", resp.FinishReason)
	}
}

func TestStreamChatWithTools(t *testing.T) {
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: &toolRT{}},
	}
	tools := []llm.Tool{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "list_files",
			Description: "List files",
			Parameters:  []byte(`{"type":"object","properties":{}}`),
		},
	}}
	messages := []llm.ChatMessage{{Role: "user", Content: "List files"}}

	var collected strings.Builder
	resp, err := client.StreamChatWithTools(contextBackground(), "sk-test", "gpt-4", messages, tools, func(delta string) {
		collected.WriteString(delta)
	})
	if err != nil {
		t.Fatalf("StreamChatWithTools error: %v", err)
	}
	if collected.String() != "Let me check" {
		t.Fatalf("expected streamed 'Let me check', got %q", collected.String())
	}
	if resp.Content != "Let me check" {
		t.Fatalf("expected content 'Let me check', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_123" {
		t.Fatalf("expected tool call id call_123, got %s", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Function.Name != "list_files" {
		t.Fatalf("expected list_files, got %s", resp.ToolCalls[0].Function.Name)
	}
	if resp.ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("expected {}, got %s", resp.ToolCalls[0].Function.Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %s", resp.FinishReason)
	}
}

func TestChatWithToolsNoTools(t *testing.T) {
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: &routeRT{}},
	}
	messages := []llm.ChatMessage{{Role: "user", Content: "hi"}}

	resp, err := client.ChatWithTools(contextBackground(), "sk-test", "gpt-4", messages, nil)
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Fatalf("expected 'Hello', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
}

func TestChatPayloadAppliesFaithfulDefaults(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	_, err := client.Chat(contextBackground(), "sk-test", "gpt-5.2", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	if _, ok := payload["temperature"]; ok {
		t.Fatalf("expected temperature to be omitted for gpt-5.2 at medium reasoning, got %#v", payload["temperature"])
	}
	if _, ok := payload["top_p"]; ok {
		t.Fatalf("expected top_p to be omitted for gpt-5.2 at medium reasoning, got %#v", payload["top_p"])
	}
	if got, ok := payload["truncation"].(string); !ok || got != "disabled" {
		t.Fatalf("expected truncation=disabled, got %#v", payload["truncation"])
	}
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object, got %#v", payload["reasoning"])
	}
	if got, ok := reasoning["effort"].(string); !ok || got != "medium" {
		t.Fatalf("expected reasoning.effort=medium, got %#v", reasoning["effort"])
	}
}

func TestChatPayloadReasoningEffortOverrideNone(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	ctx := llm.WithRequestProfile(contextBackground(), llm.RequestProfile{
		ReasoningEffort: "none",
	})
	_, err := client.Chat(ctx, "sk-test", "gpt-5.2", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object, got %#v", payload["reasoning"])
	}
	if got, ok := reasoning["effort"].(string); !ok || got != "none" {
		t.Fatalf("expected reasoning.effort=none, got %#v", reasoning["effort"])
	}
	if got, ok := payload["temperature"].(float64); !ok || got != 0 {
		t.Fatalf("expected temperature=0 for gpt-5.2 with reasoning none, got %#v", payload["temperature"])
	}
	if got, ok := payload["top_p"].(float64); !ok || got != 1 {
		t.Fatalf("expected top_p=1 for gpt-5.2 with reasoning none, got %#v", payload["top_p"])
	}
}

func TestChatPayloadReasoningEffortOverrideXHigh(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	ctx := llm.WithRequestProfile(contextBackground(), llm.RequestProfile{
		ReasoningEffort: "xhigh",
	})
	_, err := client.Chat(ctx, "sk-test", "gpt-5.2", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object, got %#v", payload["reasoning"])
	}
	if got, ok := reasoning["effort"].(string); !ok || got != "xhigh" {
		t.Fatalf("expected reasoning.effort=xhigh, got %#v", reasoning["effort"])
	}
	if _, ok := payload["temperature"]; ok {
		t.Fatalf("expected temperature to be omitted for gpt-5.2 at xhigh reasoning, got %#v", payload["temperature"])
	}
	if _, ok := payload["top_p"]; ok {
		t.Fatalf("expected top_p to be omitted for gpt-5.2 at xhigh reasoning, got %#v", payload["top_p"])
	}
}

func TestChatPayloadReasoningEffortInvalidFallsBackToMedium(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	ctx := llm.WithRequestProfile(contextBackground(), llm.RequestProfile{
		ReasoningEffort: "not-a-valid-effort",
	})
	_, err := client.Chat(ctx, "sk-test", "gpt-5.2", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object, got %#v", payload["reasoning"])
	}
	if got, ok := reasoning["effort"].(string); !ok || got != "medium" {
		t.Fatalf("expected reasoning.effort fallback to medium, got %#v", reasoning["effort"])
	}
	if _, ok := payload["temperature"]; ok {
		t.Fatalf("expected temperature to be omitted for gpt-5.2 at medium reasoning, got %#v", payload["temperature"])
	}
	if _, ok := payload["top_p"]; ok {
		t.Fatalf("expected top_p to be omitted for gpt-5.2 at medium reasoning, got %#v", payload["top_p"])
	}
}

func TestChatPayloadKeepsSamplingForNonGPT5Models(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	_, err := client.Chat(contextBackground(), "sk-test", "gpt-4o", []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	if got, ok := payload["temperature"].(float64); !ok || got != 0 {
		t.Fatalf("expected temperature=0, got %#v", payload["temperature"])
	}
	if got, ok := payload["top_p"].(float64); !ok || got != 1 {
		t.Fatalf("expected top_p=1, got %#v", payload["top_p"])
	}
}

func TestChatWithToolsInitialTurnUsesRequiredAndSequentialTools(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	tools := []llm.Tool{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "list_files",
			Description: "List files",
			Parameters:  []byte(`{"type":"object","properties":{}}`),
		},
	}}
	messages := []llm.ChatMessage{{Role: "user", Content: "List files"}}
	_, err := client.ChatWithTools(contextBackground(), "sk-test", "gpt-5.2", messages, tools)
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	if got, ok := payload["tool_choice"].(string); !ok || got != "required" {
		t.Fatalf("expected tool_choice=required, got %#v", payload["tool_choice"])
	}
	if got, ok := payload["parallel_tool_calls"].(bool); !ok || got {
		t.Fatalf("expected parallel_tool_calls=false, got %#v", payload["parallel_tool_calls"])
	}
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object, got %#v", payload["reasoning"])
	}
	if got, ok := reasoning["effort"].(string); !ok || got != "medium" {
		t.Fatalf("expected reasoning.effort=medium, got %#v", reasoning["effort"])
	}
	toolsPayload, ok := payload["tools"].([]any)
	if !ok || len(toolsPayload) != 1 {
		t.Fatalf("expected one tool payload, got %#v", payload["tools"])
	}
	firstTool, ok := toolsPayload[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object, got %#v", toolsPayload[0])
	}
	if got, ok := firstTool["strict"].(bool); !ok || !got {
		t.Fatalf("expected function strict=true, got %#v", firstTool["strict"])
	}
}

func TestChatWithToolsFollowUpUsesAutoToolChoice(t *testing.T) {
	rt := &captureRT{}
	client := &Client{
		baseURL: "https://api.openai.com",
		client:  &http.Client{Transport: rt},
	}
	tools := []llm.Tool{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "read_file",
			Description: "Read file",
			Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}}
	messages := []llm.ChatMessage{
		{Role: "user", Content: "Read report.csv"},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      "read_file",
					Arguments: `{"path":"report.csv"}`,
				},
			}},
		},
		{Role: "tool", ToolCallID: "call_1", Content: `{"ok":true}`},
	}
	_, err := client.ChatWithTools(contextBackground(), "sk-test", "gpt-5.2", messages, tools)
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}
	if len(rt.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(rt.payloads))
	}
	payload := rt.payloads[0]
	if got, ok := payload["tool_choice"].(string); !ok || got != "auto" {
		t.Fatalf("expected tool_choice=auto, got %#v", payload["tool_choice"])
	}
}

func TestSupportsSamplingParamsCompatibility(t *testing.T) {
	cases := []struct {
		model  string
		effort string
		want   bool
	}{
		{model: "gpt-5.2", effort: "medium", want: false},
		{model: "gpt-5.2", effort: "none", want: true},
		{model: "gpt-5.1", effort: "medium", want: false},
		{model: "gpt-5.1", effort: "none", want: true},
		{model: "gpt-5.3-codex", effort: "medium", want: false},
		{model: "gpt-5.3-codex", effort: "none", want: false},
		{model: "gpt-5", effort: "none", want: false},
		{model: "gpt-5-mini", effort: "none", want: false},
		{model: "gpt-4o", effort: "medium", want: true},
	}
	for _, tc := range cases {
		if got := supportsSamplingParams(tc.model, tc.effort); got != tc.want {
			t.Fatalf("supportsSamplingParams(%q, %q)=%v, want %v", tc.model, tc.effort, got, tc.want)
		}
	}
}

func TestBuildToolPayloadStrictifiesFunctionSchema(t *testing.T) {
	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name: "read_file",
				Parameters: []byte(`{
					"type":"object",
					"properties":{
						"path":{"type":"string"},
						"sheet":{"type":"string"},
						"style":{
							"type":"object",
							"properties":{
								"font_name":{"type":"string"}
							},
							"required":[]
						}
					},
					"required":["path"]
				}`),
			},
		},
	}

	payload := buildToolPayload(tools)
	if len(payload) != 1 {
		t.Fatalf("expected one tool payload, got %d", len(payload))
	}
	first := payload[0]
	if got, ok := first["strict"].(bool); !ok || !got {
		t.Fatalf("expected strict=true, got %#v", first["strict"])
	}
	raw, ok := first["parameters"].(json.RawMessage)
	if !ok {
		t.Fatalf("expected parameters raw json, got %#v", first["parameters"])
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("invalid schema json: %v", err)
	}
	if got, ok := schema["additionalProperties"].(bool); !ok || got {
		t.Fatalf("expected root additionalProperties=false, got %#v", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("expected required array, got %#v", schema["required"])
	}
	requiredSet := make(map[string]struct{}, len(required))
	for _, item := range required {
		if name, ok := item.(string); ok {
			requiredSet[name] = struct{}{}
		}
	}
	for _, name := range []string{"path", "sheet", "style"} {
		if _, ok := requiredSet[name]; !ok {
			t.Fatalf("expected %q in required, got %#v", name, schema["required"])
		}
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object, got %#v", schema["properties"])
	}
	sheetSchema, ok := properties["sheet"].(map[string]any)
	if !ok {
		t.Fatalf("expected sheet schema, got %#v", properties["sheet"])
	}
	sheetTypes, ok := sheetSchema["type"].([]any)
	if !ok {
		t.Fatalf("expected nullable sheet type array, got %#v", sheetSchema["type"])
	}
	hasString := false
	hasNull := false
	for _, item := range sheetTypes {
		typeName, _ := item.(string)
		if typeName == "string" {
			hasString = true
		}
		if typeName == "null" {
			hasNull = true
		}
	}
	if !hasString || !hasNull {
		t.Fatalf("expected sheet type to include string and null, got %#v", sheetSchema["type"])
	}

	styleSchema, ok := properties["style"].(map[string]any)
	if !ok {
		t.Fatalf("expected style schema, got %#v", properties["style"])
	}
	switch styleType := styleSchema["type"].(type) {
	case []any:
		hasObject := false
		hasNull := false
		for _, item := range styleType {
			typeName, _ := item.(string)
			if typeName == "object" {
				hasObject = true
			}
			if typeName == "null" {
				hasNull = true
			}
		}
		if !hasObject || !hasNull {
			t.Fatalf("expected style type to include object and null, got %#v", styleSchema["type"])
		}
	case string:
		t.Fatalf("expected style to be nullable, got non-nullable type %q", styleType)
	default:
		styleAnyOf, ok := styleSchema["anyOf"].([]any)
		if !ok || len(styleAnyOf) < 2 {
			t.Fatalf("expected style to be nullable, got %#v", styleSchema)
		}
	}
}

func TestStrictifyFunctionParametersForEmptyObject(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
	out, err := strictifyFunctionParameters(raw)
	if err != nil {
		t.Fatalf("strictifyFunctionParameters error: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("invalid strictified json: %v", err)
	}
	if got, ok := schema["additionalProperties"].(bool); !ok || got {
		t.Fatalf("expected additionalProperties=false, got %#v", schema["additionalProperties"])
	}
}

func contextBackground() context.Context {
	return context.Background()
}
