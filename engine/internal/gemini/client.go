package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com"

// Client implements a minimal Gemini generateContent API wrapper.
type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient() *Client {
	transport := egress.NewAllowlistRoundTripper(http.DefaultTransport, []string{"generativelanguage.googleapis.com"})
	return &Client{
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout:   120 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) ValidateKey(ctx context.Context, apiKey string) error {
	u, err := url.Parse(c.baseURL + "/v1beta/models")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("key", apiKey)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
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
	payload := geminiRequest{Contents: toGeminiContentsSimple(messages)}
	return c.send(ctx, apiKey, model, payload, nil)
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
	payload := geminiRequest{Contents: toGeminiContents(messages)}
	if len(tools) > 0 {
		payload.Tools = []geminiTool{{FunctionDeclarations: toGeminiFunctions(tools)}}
	}
	text, toolCalls, err := c.sendWithTools(ctx, apiKey, model, payload)
	if err != nil {
		return llm.ChatResponse{}, err
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

func (c *Client) send(ctx context.Context, apiKey, model string, payload geminiRequest, onDelta func(string)) (string, error) {
	text, _, err := c.sendWithTools(ctx, apiKey, model, payload)
	if err != nil {
		return "", err
	}
	if onDelta != nil {
		onDelta(text)
	}
	return text, nil
}

func (c *Client) sendWithTools(ctx context.Context, apiKey, model string, payload geminiRequest) (string, []llm.ToolCall, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, model, url.QueryEscape(apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("content-type", "application/json")
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
		errorBody, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("gemini error: %s - %s", resp.Status, string(errorBody))
	}
	var response geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", nil, err
	}
	if len(response.Candidates) == 0 {
		return "", nil, errors.New("gemini empty response")
	}
	text, calls := extractGeminiParts(response.Candidates[0].Content.Parts)
	return text, calls, nil
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
	Tools    []geminiTool    `json:"tools,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResult `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResult struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

func toGeminiFunctions(tools []llm.Tool) []geminiFunctionDeclaration {
	result := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		result = append(result, geminiFunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	return result
}

func toGeminiContentsSimple(messages []llm.Message) []geminiContent {
	var result []geminiContent
	for _, msg := range messages {
		role := mapRole(msg.Role)
		result = append(result, geminiContent{Role: role, Parts: []geminiPart{{Text: msg.Content}}})
	}
	return result
}

func toGeminiContents(messages []llm.ChatMessage) []geminiContent {
	var result []geminiContent
	toolNameByID := make(map[string]string)
	for _, msg := range messages {
		switch msg.Role {
		case "tool":
			name := toolNameByID[msg.ToolCallID]
			result = append(result, geminiContent{Role: "user", Parts: []geminiPart{{
				FunctionResponse: &geminiFunctionResult{
					Name: name,
					Response: map[string]any{
						"result": msg.Content,
					},
				},
			}}})
		default:
			parts := []geminiPart{}
			if msg.Content != "" {
				parts = append(parts, geminiPart{Text: msg.Content})
			}
			for _, call := range msg.ToolCalls {
				toolNameByID[call.ID] = call.Function.Name
				args := map[string]any{}
				if call.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
				}
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{
					Name: call.Function.Name,
					Args: args,
				}})
			}
			result = append(result, geminiContent{Role: mapRole(msg.Role), Parts: parts})
		}
	}
	return result
}

func mapRole(role string) string {
	switch role {
	case "assistant", "model":
		return "model"
	default:
		return "user"
	}
}

func extractGeminiParts(parts []geminiPart) (string, []llm.ToolCall) {
	var buf bytes.Buffer
	var calls []llm.ToolCall
	callIndex := 0
	for _, part := range parts {
		if part.Text != "" {
			buf.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			// Generate unique ID per call using index + timestamp-based suffix
			callID := fmt.Sprintf("call-%s-%d-%d", part.FunctionCall.Name, time.Now().UnixNano(), callIndex)
			callIndex++
			calls = append(calls, llm.ToolCall{
				ID:   callID,
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}
	return buf.String(), calls
}
