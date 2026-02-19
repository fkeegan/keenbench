package mistral

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

const defaultBaseURL = "https://api.mistral.ai"
const maxErrorBodyBytes = 2048

// Client implements a minimal Mistral chat-completions API wrapper.
type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient() *Client {
	transport := egress.NewAllowlistRoundTripper(http.DefaultTransport, []string{"api.mistral.ai"})
	return &Client{
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout:   120 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) ValidateKey(ctx context.Context, apiKey string) error {
	if strings.TrimSpace(apiKey) == "" {
		return llm.ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
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
	payload := chatCompletionRequest{
		Model:    model,
		Messages: toMistralMessagesSimple(messages),
	}
	content, _, err := c.sendChatCompletion(ctx, apiKey, payload)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("mistral empty response")
	}
	return content, nil
}

func (c *Client) StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error) {
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
	payload := chatCompletionRequest{
		Model:    model,
		Messages: toMistralMessages(messages),
	}
	if len(tools) > 0 {
		payload.Tools = toMistralTools(tools)
	}
	content, toolCalls, err := c.sendChatCompletion(ctx, apiKey, payload)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	if strings.TrimSpace(content) == "" && len(toolCalls) == 0 {
		return llm.ChatResponse{}, errors.New("mistral empty response")
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return llm.ChatResponse{Content: content, ToolCalls: toolCalls, FinishReason: finishReason}, nil
}

func (c *Client) StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	resp, err := c.ChatWithTools(ctx, apiKey, model, messages, tools)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	if strings.TrimSpace(resp.Content) != "" && onDelta != nil {
		onDelta(resp.Content)
	}
	return resp, nil
}

func (c *Client) sendChatCompletion(ctx context.Context, apiKey string, payload chatCompletionRequest) (string, []llm.ToolCall, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, llm.ErrEgressBlocked) {
			return "", nil, llm.ErrEgressBlocked
		}
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", nil, llm.ErrUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", nil, llm.ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return "", nil, llm.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return "", nil, fmt.Errorf("mistral error: %s - %s", resp.Status, string(errorBody))
	}
	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return "", nil, err
	}
	if len(completion.Choices) == 0 {
		return "", nil, errors.New("mistral empty response")
	}
	msg := completion.Choices[0].Message
	content := extractContent(msg.Content)
	toolCalls := toLLMToolCalls(msg.ToolCalls)
	return content, toolCalls, nil
}

type chatCompletionRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Tools    []chatToolSpec `json:"tools,omitempty"`
}

type chatToolSpec struct {
	Type     string          `json:"type"`
	Function chatToolDetails `json:"function"`
}

type chatToolDetails struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	Name       string            `json:"name,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCallReq `json:"tool_calls,omitempty"`
}

type chatToolCallReq struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function chatToolFunctionReq `json:"function"`
}

type chatToolFunctionReq struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatResponseMessage `json:"message"`
}

type chatResponseMessage struct {
	Content   json.RawMessage    `json:"content"`
	ToolCalls []chatToolCallResp `json:"tool_calls"`
}

type chatToolCallResp struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function chatToolFunctionRaw `json:"function"`
}

type chatToolFunctionRaw struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentBlock struct {
	Text string `json:"text"`
}

func toMistralMessagesSimple(messages []llm.Message) []chatMessage {
	result := make([]chatMessage, 0, len(messages))
	for _, msg := range messages {
		result = append(result, chatMessage{
			Role:    normalizeRole(msg.Role),
			Content: msg.Content,
		})
	}
	return result
}

func toMistralMessages(messages []llm.ChatMessage) []chatMessage {
	result := make([]chatMessage, 0, len(messages))
	toolNameByID := make(map[string]string)
	for _, msg := range messages {
		switch msg.Role {
		case "tool":
			entry := chatMessage{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			}
			if name := strings.TrimSpace(toolNameByID[msg.ToolCallID]); name != "" {
				entry.Name = name
			}
			result = append(result, entry)
		default:
			entry := chatMessage{
				Role:    normalizeRole(msg.Role),
				Content: msg.Content,
			}
			if len(msg.ToolCalls) > 0 {
				entry.ToolCalls = make([]chatToolCallReq, 0, len(msg.ToolCalls))
				for idx, call := range msg.ToolCalls {
					callID := strings.TrimSpace(call.ID)
					if callID == "" {
						callID = fmt.Sprintf("call-%s-%d", call.Function.Name, idx)
					}
					entry.ToolCalls = append(entry.ToolCalls, chatToolCallReq{
						ID:   callID,
						Type: "function",
						Function: chatToolFunctionReq{
							Name:      call.Function.Name,
							Arguments: call.Function.Arguments,
						},
					})
					toolNameByID[callID] = call.Function.Name
				}
			}
			result = append(result, entry)
		}
	}
	return result
}

func toMistralTools(tools []llm.Tool) []chatToolSpec {
	result := make([]chatToolSpec, 0, len(tools))
	for _, tool := range tools {
		result = append(result, chatToolSpec{
			Type: "function",
			Function: chatToolDetails{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return result
}

func normalizeRole(role string) string {
	switch strings.TrimSpace(role) {
	case "assistant", "user", "system", "tool":
		return strings.TrimSpace(role)
	default:
		return "user"
	}
}

func extractContent(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var builder strings.Builder
		for _, block := range blocks {
			builder.WriteString(block.Text)
		}
		return builder.String()
	}
	return ""
}

func toLLMToolCalls(calls []chatToolCallResp) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]llm.ToolCall, 0, len(calls))
	for idx, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			callID = fmt.Sprintf("call-%s-%d", name, idx)
		}
		result = append(result, llm.ToolCall{
			ID:   callID,
			Type: "function",
			Function: llm.ToolCallFunction{
				Name:      name,
				Arguments: normalizeArguments(call.Function.Arguments),
			},
		})
	}
	return result
}

func normalizeArguments(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "{}"
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if strings.TrimSpace(asString) == "" {
			return "{}"
		}
		return asString
	}
	return trimmed
}
