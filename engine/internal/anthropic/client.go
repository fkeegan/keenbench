package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

const defaultBaseURL = "https://api.anthropic.com"
const defaultVersion = "2023-06-01"
const defaultReasoningEffort = "high"

// Client implements Anthropic Messages API (minimal v1 support).
type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient() *Client {
	transport := egress.NewAllowlistRoundTripper(http.DefaultTransport, []string{"api.anthropic.com"})
	return &Client{
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout:   120 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) ValidateKey(ctx context.Context, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", defaultVersion)
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return llm.ErrEgressBlocked
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return llm.ErrUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return llm.ErrRateLimited
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
	anthropicMessages, systemPrompt, err := toAnthropicMessages(messages, nil)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   anthropicMessages,
		"output_config": map[string]any{
			"effort": resolveReasoningEffort(ctx, model),
		},
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	respBody, err := c.post(ctx, apiKey, body)
	if err != nil {
		return "", err
	}
	var response anthropicResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", err
	}
	content := extractText(response.Content)
	if content == "" {
		return "", errors.New("anthropic empty response")
	}
	return content, nil
}

func (c *Client) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
	// For now, use non-streaming API and emit once.
	content, err := c.Chat(ctx, apiKey, model, messages)
	if err != nil {
		return "", err
	}
	if onDelta != nil {
		onDelta(content)
	}
	return content, nil
}

func (c *Client) ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	anthropicMessages, systemPrompt, err := toAnthropicMessages(nil, messages)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	payload := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   anthropicMessages,
		"output_config": map[string]any{
			"effort": resolveReasoningEffort(ctx, model),
		},
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}
	if len(tools) > 0 {
		payload["tools"] = toAnthropicTools(tools)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	respBody, err := c.post(ctx, apiKey, body)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	var response anthropicResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return llm.ChatResponse{}, err
	}
	text, toolCalls := extractTools(response.Content)
	if text == "" && len(toolCalls) == 0 {
		return llm.ChatResponse{}, errors.New("anthropic empty response")
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return llm.ChatResponse{Content: text, ToolCalls: toolCalls, FinishReason: finishReason}, nil
}

func (c *Client) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	resp, err := c.ChatWithTools(ctx, apiKey, model, messages, tools)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	if resp.Content != "" && onDelta != nil {
		onDelta(resp.Content)
	}
	return resp, nil
}

func (c *Client) post(ctx context.Context, apiKey string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", defaultVersion)
	req.Header.Set("content-type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return nil, llm.ErrEgressBlocked
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, llm.ErrUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, llm.ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return nil, llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic error: %s - %s", resp.Status, string(errorBody))
	}
	return io.ReadAll(resp.Body)
}

type anthropicRequestMessage struct {
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}

type anthropicContent struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
}

func toAnthropicMessages(simple []llm.Message, chat []llm.ChatMessage) ([]anthropicRequestMessage, string, error) {
	var messages []anthropicRequestMessage
	systemParts := make([]string, 0)
	if len(chat) == 0 {
		for _, msg := range simple {
			role := strings.ToLower(strings.TrimSpace(msg.Role))
			if role == "system" {
				text := strings.TrimSpace(msg.Content)
				if text != "" {
					systemParts = append(systemParts, text)
				}
				continue
			}
			messages = append(messages, anthropicRequestMessage{
				Role: normalizeAnthropicRole(role),
				Content: []map[string]any{{
					"type": "text",
					"text": msg.Content,
				}},
			})
		}
		if len(messages) == 0 {
			return nil, "", errors.New("anthropic requires at least one non-system message")
		}
		return messages, strings.Join(systemParts, "\n\n"), nil
	}
	for _, msg := range chat {
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			text := strings.TrimSpace(msg.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}
		case "tool":
			toolUseID := strings.TrimSpace(msg.ToolCallID)
			if toolUseID == "" {
				return nil, "", errors.New("anthropic tool_result missing tool_use_id")
			}
			messages = append(messages, anthropicRequestMessage{
				Role: "user",
				Content: []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": toolUseID,
					"content":     msg.Content,
				}},
			})
		default:
			content := make([]map[string]any, 0, 1+len(msg.ToolCalls))
			if msg.Content != "" {
				content = append(content, map[string]any{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, call := range msg.ToolCalls {
				callID := strings.TrimSpace(call.ID)
				if callID == "" {
					return nil, "", errors.New("anthropic tool_use missing id")
				}
				callName := strings.TrimSpace(call.Function.Name)
				if callName == "" {
					return nil, "", errors.New("anthropic tool_use missing name")
				}
				input := map[string]any{}
				if call.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
						// Keep payload valid for Anthropic even if upstream args are malformed.
						input = map[string]any{}
					}
				}
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    callID,
					"name":  callName,
					"input": input,
				})
			}
			if len(content) == 0 {
				continue
			}
			messages = append(messages, anthropicRequestMessage{
				Role:    normalizeAnthropicRole(msg.Role),
				Content: content,
			})
		}
	}
	if len(messages) == 0 {
		return nil, "", errors.New("anthropic requires at least one non-system message")
	}
	return messages, strings.Join(systemParts, "\n\n"), nil
}

func normalizeAnthropicRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "model":
		return "assistant"
	default:
		return "user"
	}
}

func toAnthropicTools(tools []llm.Tool) []anthropicTool {
	result := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		schema := bytes.TrimSpace(tool.Function.Parameters)
		if len(schema) == 0 || !json.Valid(schema) {
			schema = []byte(`{"type":"object","properties":{}}`)
		}
		result = append(result, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: json.RawMessage(schema),
		})
	}
	return result
}

func resolveReasoningEffort(ctx context.Context, model string) string {
	profile, ok := llm.RequestProfileFromContext(ctx)
	if !ok {
		return defaultReasoningEffort
	}
	effort := normalizeReasoningEffort(profile.ReasoningEffort)
	if effort == "max" && !supportsMaxReasoningEffort(model) {
		return "high"
	}
	return effort
}

func normalizeReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "max":
		return "max"
	default:
		return defaultReasoningEffort
	}
}

func supportsMaxReasoningEffort(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(name, "claude-opus-4-6")
}

func extractText(contents []anthropicContent) string {
	var buf bytes.Buffer
	for _, item := range contents {
		if item.Type == "text" {
			buf.WriteString(item.Text)
		}
	}
	return buf.String()
}

func extractTools(contents []anthropicContent) (string, []llm.ToolCall) {
	var buf bytes.Buffer
	var calls []llm.ToolCall
	for _, item := range contents {
		switch item.Type {
		case "text":
			buf.WriteString(item.Text)
		case "tool_use":
			input := item.Input
			if input == nil {
				input = map[string]any{}
			}
			args, _ := json.Marshal(input)
			calls = append(calls, llm.ToolCall{
				ID:   item.ID,
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      item.Name,
					Arguments: string(args),
				},
			})
		}
	}
	return buf.String(), calls
}
