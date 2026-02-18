package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

const defaultBaseURL = "https://api.openai.com"
const defaultCodexBaseURL = "https://chatgpt.com/backend-api"

const (
	defaultReasoningEffort   = "medium"
	defaultTemperature       = 0.0
	defaultTopP              = 1.0
	defaultTruncation        = "disabled"
	codexDefaultInstructions = "You are Codex."
	maxErrorBodyBytes        = 2048
)

type responseEnvelope struct {
	Output []responseItem `json:"output"`
}

type responseItem struct {
	Type      string            `json:"type"`
	Role      string            `json:"role,omitempty"`
	Content   []responseContent `json:"content,omitempty"`
	CallID    string            `json:"call_id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Arguments string            `json:"arguments,omitempty"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type streamEvent struct {
	Type     string            `json:"type"`
	Delta    string            `json:"delta,omitempty"`
	Response *responseEnvelope `json:"response,omitempty"`
}

type Client struct {
	baseURL string
	client  *http.Client
	codex   bool
}

func NewClient() *Client {
	return newClient(false)
}

func NewCodexClient() *Client {
	return newClient(true)
}

func newClient(codex bool) *Client {
	hosts := []string{"api.openai.com"}
	baseURL := defaultBaseURL
	if codex {
		hosts = []string{"chatgpt.com"}
		baseURL = defaultCodexBaseURL
	}
	transport := egress.NewAllowlistRoundTripper(http.DefaultTransport, hosts)
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout:   600 * time.Second,
			Transport: transport,
		},
		codex: codex,
	}
}

func (c *Client) applyAuthorizationHeaders(req *http.Request, token string) bool {
	req.Header.Set("Authorization", "Bearer "+token)
	if !c.codex {
		return false
	}
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "pi")
	req.Header.Set("Accept", "text/event-stream")
	if accountID := strings.TrimSpace(ExtractCodexChatGPTAccountID(token)); accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
		return true
	}
	return false
}

func (c *Client) responsesEndpoint() string {
	base := strings.TrimRight(c.baseURL, "/")
	if c.codex {
		return base + "/codex/responses"
	}
	return base + "/v1/responses"
}

func (c *Client) ValidateKey(ctx context.Context, apiKey string) error {
	if c.codex {
		if strings.TrimSpace(apiKey) == "" {
			return llm.ErrUnauthorized
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	hasAccountHeader := c.applyAuthorizationHeaders(req, apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return llm.ErrEgressBlocked
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return unauthorizedError(resp, "", c.codex, hasAccountHeader)
	}
	if resp.StatusCode >= 500 {
		return llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("validation failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) Chat(ctx context.Context, apiKey, model string, messages []llm.Message) (string, error) {
	if c.codex {
		content, err := c.StreamChat(ctx, apiKey, model, messages, nil)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(content) == "" {
			return "", errors.New("openai empty response")
		}
		return content, nil
	}

	payload := c.buildTextRequestPayload(ctx, model, messages, false)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.responsesEndpoint(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hasAccountHeader := c.applyAuthorizationHeaders(req, apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return "", llm.ErrEgressBlocked
		}
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", unauthorizedError(resp, model, c.codex, hasAccountHeader)
	}
	if resp.StatusCode >= 500 {
		return "", llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", c.requestError(resp, payload)
	}
	var response responseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	content, _ := extractTextAndToolCalls(response.Output)
	if content == "" {
		return "", errors.New("openai empty response")
	}
	return content, nil
}

func (c *Client) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
	payload := c.buildTextRequestPayload(ctx, model, messages, true)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.responsesEndpoint(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hasAccountHeader := c.applyAuthorizationHeaders(req, apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return "", llm.ErrEgressBlocked
		}
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", unauthorizedError(resp, model, c.codex, hasAccountHeader)
	}
	if resp.StatusCode >= 500 {
		return "", llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", c.requestError(resp, payload)
	}

	var builder strings.Builder
	var finalResponse *responseEnvelope
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta == "" {
				continue
			}
			builder.WriteString(event.Delta)
			if onDelta != nil {
				onDelta(event.Delta)
			}
		case "response.completed":
			if event.Response != nil {
				finalResponse = event.Response
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), err
	}
	content := builder.String()
	if finalResponse != nil {
		if finalContent, _ := extractTextAndToolCalls(finalResponse.Output); finalContent != "" {
			content = finalContent
		}
	}
	return content, nil
}

// ChatWithTools sends a chat completion request with tools and returns the response.
func (c *Client) ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	if c.codex {
		resp, err := c.StreamChatWithTools(ctx, apiKey, model, messages, tools, nil)
		if err != nil {
			return llm.ChatResponse{}, err
		}
		if strings.TrimSpace(resp.Content) == "" && len(resp.ToolCalls) == 0 {
			return llm.ChatResponse{}, errors.New("openai empty response")
		}
		return resp, nil
	}

	payload := c.buildToolRequestPayload(ctx, model, messages, tools, false)
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.responsesEndpoint(), bytes.NewReader(body))
	if err != nil {
		return llm.ChatResponse{}, err
	}
	hasAccountHeader := c.applyAuthorizationHeaders(req, apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return llm.ChatResponse{}, llm.ErrEgressBlocked
		}
		return llm.ChatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return llm.ChatResponse{}, unauthorizedError(resp, model, c.codex, hasAccountHeader)
	}
	if resp.StatusCode >= 500 {
		return llm.ChatResponse{}, llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llm.ChatResponse{}, c.requestError(resp, payload)
	}
	var response responseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return llm.ChatResponse{}, err
	}
	content, toolCalls := extractTextAndToolCalls(response.Output)
	if content == "" && len(toolCalls) == 0 {
		return llm.ChatResponse{}, errors.New("openai empty response")
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return llm.ChatResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}, nil
}

// StreamChatWithTools sends a streaming chat completion with tools.
// Text content is streamed via onDelta. Tool calls are accumulated and returned in the response.
func (c *Client) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	payload := c.buildToolRequestPayload(ctx, model, messages, tools, true)
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.responsesEndpoint(), bytes.NewReader(body))
	if err != nil {
		return llm.ChatResponse{}, err
	}
	hasAccountHeader := c.applyAuthorizationHeaders(req, apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return llm.ChatResponse{}, llm.ErrEgressBlocked
		}
		return llm.ChatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return llm.ChatResponse{}, unauthorizedError(resp, model, c.codex, hasAccountHeader)
	}
	if resp.StatusCode >= 500 {
		return llm.ChatResponse{}, llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llm.ChatResponse{}, c.requestError(resp, payload)
	}

	var contentBuilder strings.Builder
	var finalResponse *responseEnvelope
	toolCallsMap := make(map[string]*llm.ToolCall)
	var toolCallOrder []string

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		eventType, _ := event["type"].(string)
		switch eventType {
		case "response.output_text.delta":
			delta, _ := event["delta"].(string)
			if delta == "" {
				continue
			}
			contentBuilder.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
		case "response.output_item.added":
			itemRaw, ok := event["item"]
			if !ok {
				continue
			}
			itemBytes, err := json.Marshal(itemRaw)
			if err != nil {
				continue
			}
			var item responseItem
			if err := json.Unmarshal(itemBytes, &item); err != nil {
				continue
			}
			if item.Type != "function_call" {
				continue
			}
			if item.CallID == "" {
				continue
			}
			tc, ok := toolCallsMap[item.CallID]
			if !ok {
				tc = &llm.ToolCall{ID: item.CallID, Type: "function"}
				toolCallsMap[item.CallID] = tc
				toolCallOrder = append(toolCallOrder, item.CallID)
			}
			if item.Name != "" {
				tc.Function.Name = item.Name
			}
			if item.Arguments != "" {
				tc.Function.Arguments = item.Arguments
			}
		case "response.function_call_arguments.delta":
			callID, _ := event["call_id"].(string)
			delta, _ := event["delta"].(string)
			if callID == "" || delta == "" {
				continue
			}
			tc, ok := toolCallsMap[callID]
			if !ok {
				tc = &llm.ToolCall{ID: callID, Type: "function"}
				toolCallsMap[callID] = tc
				toolCallOrder = append(toolCallOrder, callID)
			}
			tc.Function.Arguments += delta
		case "response.completed":
			respRaw, ok := event["response"]
			if !ok {
				continue
			}
			respBytes, err := json.Marshal(respRaw)
			if err != nil {
				continue
			}
			var respObj responseEnvelope
			if err := json.Unmarshal(respBytes, &respObj); err != nil {
				continue
			}
			finalResponse = &respObj
		}
	}
	if err := scanner.Err(); err != nil {
		return llm.ChatResponse{Content: contentBuilder.String()}, err
	}

	content := contentBuilder.String()
	var toolCalls []llm.ToolCall
	if finalResponse != nil {
		finalContent, finalToolCalls := extractTextAndToolCalls(finalResponse.Output)
		if finalContent != "" {
			content = finalContent
		}
		toolCalls = finalToolCalls
	} else if len(toolCallOrder) > 0 {
		for _, callID := range toolCallOrder {
			if tc, ok := toolCallsMap[callID]; ok {
				toolCalls = append(toolCalls, *tc)
			}
		}
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return llm.ChatResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}, nil
}

func buildTextInput(messages []llm.Message, codex bool) []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if codex && (role == "system" || role == "developer") {
			continue
		}
		input = append(input, map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	return input
}

func buildToolInput(messages []llm.ChatMessage, codex bool) []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if codex && (role == "system" || role == "developer") {
			continue
		}
		switch role {
		case "tool":
			if msg.ToolCallID == "" {
				continue
			}
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})
		default:
			if msg.Content != "" {
				input = append(input, map[string]any{
					"role":    msg.Role,
					"content": msg.Content,
				})
			}
			if len(msg.ToolCalls) == 0 {
				continue
			}
			for _, call := range msg.ToolCalls {
				callItem := map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Function.Name,
					"arguments": call.Function.Arguments,
				}
				input = append(input, callItem)
			}
		}
	}
	return input
}

func (c *Client) buildTextRequestPayload(ctx context.Context, model string, messages []llm.Message, stream bool) map[string]any {
	payload := map[string]any{
		"model": model,
		"input": buildTextInput(messages, c.codex),
	}
	if c.codex {
		payload["instructions"] = codexInstructionsFromTextMessages(messages)
		payload["store"] = false
	}
	if stream {
		payload["stream"] = true
	}
	applyRequestDefaults(ctx, payload, model, c.codex)
	return payload
}

func (c *Client) buildToolRequestPayload(ctx context.Context, model string, messages []llm.ChatMessage, tools []llm.Tool, stream bool) map[string]any {
	payload := map[string]any{
		"model": model,
		"input": buildToolInput(messages, c.codex),
	}
	if c.codex {
		payload["instructions"] = codexInstructionsFromToolMessages(messages)
		payload["store"] = false
	}
	if stream {
		payload["stream"] = true
	}
	applyRequestDefaults(ctx, payload, model, c.codex)
	if len(tools) > 0 {
		payload["tools"] = buildToolPayload(tools)
		payload["tool_choice"] = toolChoiceForTurn(messages)
		payload["parallel_tool_calls"] = false
	}
	return payload
}

func codexInstructionsFromTextMessages(messages []llm.Message) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "system" && role != "developer" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}
	return joinCodexInstructions(parts)
}

func codexInstructionsFromToolMessages(messages []llm.ChatMessage) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "system" && role != "developer" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}
	return joinCodexInstructions(parts)
}

func joinCodexInstructions(parts []string) string {
	if len(parts) == 0 {
		return codexDefaultInstructions
	}
	return strings.Join(parts, "\n\n")
}

func (c *Client) requestError(resp *http.Response, payload map[string]any) error {
	return fmt.Errorf(
		"openai error: %s endpoint=%s codex=%t diag={%s} - %s",
		resp.Status,
		c.responsesEndpoint(),
		c.codex,
		summarizeRequestPayload(payload),
		readErrorBody(resp),
	)
}

func summarizeRequestPayload(payload map[string]any) string {
	inputItems := 0
	switch items := payload["input"].(type) {
	case []map[string]any:
		inputItems = len(items)
	case []any:
		inputItems = len(items)
	}
	instructions, _ := payload["instructions"].(string)
	instructions = strings.TrimSpace(instructions)
	hasTools := false
	if tools, ok := payload["tools"].([]map[string]any); ok {
		hasTools = len(tools) > 0
	} else if tools, ok := payload["tools"].([]any); ok {
		hasTools = len(tools) > 0
	} else if _, ok := payload["tools"]; ok {
		hasTools = true
	}
	stream, _ := payload["stream"].(bool)
	return fmt.Sprintf(
		"has_instructions=%t instructions_len=%d input_items=%d has_tools=%t stream=%t",
		instructions != "",
		len(instructions),
		inputItems,
		hasTools,
		stream,
	)
}

func buildToolPayload(tools []llm.Tool) []map[string]any {
	payload := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		entry := map[string]any{
			"type": tool.Type,
		}
		if tool.Type == "function" {
			if tool.Function.Name != "" {
				entry["name"] = tool.Function.Name
			}
			if tool.Function.Description != "" {
				entry["description"] = tool.Function.Description
			}
			if len(tool.Function.Parameters) > 0 {
				parameters := json.RawMessage(tool.Function.Parameters)
				strictParameters, err := strictifyFunctionParameters(parameters)
				if err == nil {
					parameters = strictParameters
				}
				entry["parameters"] = parameters
			}
			entry["strict"] = true
		}
		payload = append(payload, entry)
	}
	return payload
}

func strictifyFunctionParameters(parameters json.RawMessage) (json.RawMessage, error) {
	if len(parameters) == 0 {
		return parameters, nil
	}
	var schema any
	if err := json.Unmarshal(parameters, &schema); err != nil {
		return nil, err
	}
	strictSchema := strictifySchemaNode(schema)
	out, err := json.Marshal(strictSchema)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

func strictifySchemaNode(node any) any {
	switch v := node.(type) {
	case map[string]any:
		// Recurse through known schema-bearing keys.
		if propertiesAny, ok := v["properties"]; ok {
			if properties, ok := propertiesAny.(map[string]any); ok {
				for name, propertySchema := range properties {
					properties[name] = strictifySchemaNode(propertySchema)
				}
				// In strict mode, all properties must be required.
				requiredSet := make(map[string]struct{})
				if requiredAny, ok := v["required"].([]any); ok {
					for _, item := range requiredAny {
						name, _ := item.(string)
						if name != "" {
							requiredSet[name] = struct{}{}
						}
					}
				}
				required := make([]string, 0, len(properties))
				for name, propertySchema := range properties {
					required = append(required, name)
					if _, ok := requiredSet[name]; ok {
						continue
					}
					properties[name] = makeSchemaNullable(propertySchema)
				}
				sort.Strings(required)
				requiredAny := make([]any, 0, len(required))
				for _, name := range required {
					requiredAny = append(requiredAny, name)
				}
				v["required"] = requiredAny
			}
		}
		if itemsAny, ok := v["items"]; ok {
			v["items"] = strictifySchemaNode(itemsAny)
		}
		for _, key := range []string{"anyOf", "allOf", "oneOf"} {
			if variantsAny, ok := v[key]; ok {
				variants, ok := variantsAny.([]any)
				if !ok {
					continue
				}
				for i, variant := range variants {
					variants[i] = strictifySchemaNode(variant)
				}
				v[key] = variants
			}
		}
		// Strict mode requires additionalProperties=false for every object.
		if schemaType, hasType := v["type"]; hasType {
			switch t := schemaType.(type) {
			case string:
				if t == "object" {
					v["additionalProperties"] = false
				}
			case []any:
				for _, item := range t {
					typeName, _ := item.(string)
					if typeName == "object" {
						v["additionalProperties"] = false
						break
					}
				}
			}
		} else if _, hasProperties := v["properties"]; hasProperties {
			v["additionalProperties"] = false
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = strictifySchemaNode(item)
		}
		return v
	default:
		return v
	}
}

func makeSchemaNullable(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok {
		return schema
	}
	if schemaType, hasType := m["type"]; hasType {
		switch t := schemaType.(type) {
		case string:
			if t == "null" {
				return m
			}
			m["type"] = []any{t, "null"}
			return m
		case []any:
			for _, item := range t {
				typeName, _ := item.(string)
				if typeName == "null" {
					return m
				}
			}
			m["type"] = append(t, "null")
			return m
		}
	}
	if anyOfAny, ok := m["anyOf"]; ok {
		if anyOf, ok := anyOfAny.([]any); ok {
			for _, variant := range anyOf {
				if variantMap, ok := variant.(map[string]any); ok {
					if typeName, ok := variantMap["type"].(string); ok && typeName == "null" {
						return m
					}
				}
			}
			m["anyOf"] = append(anyOf, map[string]any{"type": "null"})
			return m
		}
	}
	snapshot, err := json.Marshal(m)
	if err != nil {
		return map[string]any{
			"anyOf": []any{
				m,
				map[string]any{"type": "null"},
			},
		}
	}
	var clone map[string]any
	if err := json.Unmarshal(snapshot, &clone); err != nil {
		return map[string]any{
			"anyOf": []any{
				m,
				map[string]any{"type": "null"},
			},
		}
	}
	return map[string]any{
		"anyOf": []any{
			clone,
			map[string]any{"type": "null"},
		},
	}
}

func applyRequestDefaults(ctx context.Context, payload map[string]any, model string, codex bool) {
	if !codex {
		payload["truncation"] = defaultTruncation
	}
	reasoningEffort := resolveReasoningEffort(ctx)
	payload["reasoning"] = map[string]any{"effort": reasoningEffort}
	if !codex && supportsSamplingParams(model, reasoningEffort) {
		payload["temperature"] = defaultTemperature
		payload["top_p"] = defaultTopP
	}
}

func resolveReasoningEffort(ctx context.Context) string {
	profile, ok := llm.RequestProfileFromContext(ctx)
	if !ok {
		return defaultReasoningEffort
	}
	return normalizeReasoningEffort(profile.ReasoningEffort)
}

func normalizeReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none":
		return "none"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	default:
		return defaultReasoningEffort
	}
}

func supportsSamplingParams(model, reasoningEffort string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	effort := strings.ToLower(strings.TrimSpace(reasoningEffort))
	if name == "" {
		return true
	}
	// OpenAI GPT-5.2 migration guidance:
	// temperature/top_p/logprobs are only compatible with GPT-5.2 and GPT-5.1
	// when reasoning.effort is none, and are otherwise rejected for GPT-5 family models.
	if strings.HasPrefix(name, "gpt-5.2") || strings.HasPrefix(name, "gpt-5.1") {
		return effort == "none"
	}
	if strings.HasPrefix(name, "gpt-5") {
		return false
	}
	return true
}

// Require tool usage on a fresh tool turn, then allow autonomous finish
// decisions once at least one tool result has been fed back.
func toolChoiceForTurn(messages []llm.ChatMessage) string {
	for _, msg := range messages {
		if msg.Role == "tool" {
			return "auto"
		}
	}
	return "required"
}

func extractTextAndToolCalls(output []responseItem) (string, []llm.ToolCall) {
	var contentBuilder strings.Builder
	var toolCalls []llm.ToolCall
	for _, item := range output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" || part.Type == "text" {
					contentBuilder.WriteString(part.Text)
				}
			}
		case "function_call":
			callID := item.CallID
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   callID,
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}
	return contentBuilder.String(), toolCalls
}

func readErrorBody(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	return strings.TrimSpace(string(body))
}

func unauthorizedError(resp *http.Response, model string, codex, hasAccountHeader bool) error {
	if resp == nil {
		return llm.ErrUnauthorized
	}
	requestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	body := readErrorBody(resp)
	return fmt.Errorf(
		"%w: status=%s model=%s codex=%t chatgpt_account_header=%t request_id=%s body=%q",
		llm.ErrUnauthorized,
		resp.Status,
		strings.TrimSpace(model),
		codex,
		hasAccountHeader,
		requestID,
		body,
	)
}

func (c *Client) BaseURL() (*url.URL, error) {
	return url.Parse(c.baseURL)
}
