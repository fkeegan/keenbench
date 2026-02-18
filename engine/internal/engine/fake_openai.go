package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"keenbench/engine/internal/llm"
)

const fakeNetworkMarker = "[network-error]"
const fakeDeleteMarker = "[delete]"
const fakeProposal2Marker = "[proposal2]"
const fakeOrgChartMarker = "[orgchart]"
const fakeProposalOpsMarker = "[proposal_ops]"

func newFakeOpenAI() LLMClient {
	return &fakeOpenAI{}
}

type fakeOpenAI struct{}

type fakeNetErr struct{}

func (fakeNetErr) Error() string   { return "network unavailable" }
func (fakeNetErr) Timeout() bool   { return true }
func (fakeNetErr) Temporary() bool { return true }

func (f *fakeOpenAI) ValidateKey(_ context.Context, apiKey string) error {
	if isInvalidKey(apiKey) {
		return llm.ErrUnauthorized
	}
	return nil
}

func (f *fakeOpenAI) Chat(_ context.Context, apiKey, _ string, messages []llm.Message) (string, error) {
	if isInvalidKey(apiKey) {
		return "", llm.ErrUnauthorized
	}
	prompt := joinMessages(messages)
	if strings.Contains(prompt, fakeDeleteMarker) {
		return `{"summary":"Delete request","writes":[{"path":"summary.md","content":"Cannot delete files in M0."}],"delete":[{"path":"notes.txt"}]}`,
			nil
	}
	if isOrgChartPrompt(prompt) {
		orgChart, assignments := orgChartDraftFiles()
		if strings.Contains(prompt, fakeProposal2Marker) {
			orgChart += "\n## Open questions\n- QA support remains unassigned.\n"
		}
		return fmt.Sprintf(
			`{"summary":"Project staffing draft","writes":[{"path":"org_chart.md","content":%q},{"path":"project_assignments.csv","content":%q}]}`,
			orgChart,
			assignments,
		), nil
	}
	if strings.Contains(prompt, fakeProposalOpsMarker) {
		return `{"schema_version":2,"summary":"Office ops draft","writes":[],"ops":[{"path":"report.docx","kind":"docx","summary":"Update report","ops":[{"op":"set_paragraphs","paragraphs":[{"text":"Report","style":"Heading1"},{"text":"Draft content","style":"Normal"}]}]},{"path":"metrics.xlsx","kind":"xlsx","summary":"Update metrics","ops":[{"op":"ensure_sheet","sheet":"Summary"},{"op":"set_range","sheet":"Summary","start":"A1","values":[["Metric","Value"],["Q1",120]]}]},{"path":"deck.pptx","kind":"pptx","summary":"Update deck","ops":[{"op":"add_slide","layout":"title_and_content","title":"Overview","body":"Highlights"}]}],"warnings":[]}`,
			nil
	}
	content := "Draft summary"
	if strings.Contains(prompt, fakeProposal2Marker) {
		content = "Draft summary\n\nSecond paragraph."
	}
	return fmt.Sprintf(
		`{"summary":"Draft summary","writes":[{"path":"summary.md","content":%q}]}`,
		content,
	), nil
}

func (f *fakeOpenAI) StreamChat(_ context.Context, apiKey, _ string, messages []llm.Message, onDelta func(string)) (string, error) {
	if isInvalidKey(apiKey) {
		return "", llm.ErrUnauthorized
	}
	lastUser := lastUserMessage(messages)
	if strings.Contains(lastUser, fakeNetworkMarker) {
		return "", fakeNetErr{}
	}
	response := buildAssistantResponse(lastUser)
	if onDelta != nil {
		for _, chunk := range splitChunks(response) {
			onDelta(chunk)
		}
	}
	return response, nil
}

func (f *fakeOpenAI) ChatWithTools(_ context.Context, apiKey, _ string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error) {
	if isInvalidKey(apiKey) {
		return llm.ChatResponse{}, llm.ErrUnauthorized
	}
	lastUser := lastUserMessageChat(messages)
	if hasToolResultAfterLastUser(messages) {
		response := buildAssistantResponse(lastUser)
		return llm.ChatResponse{
			Content:      response,
			FinishReason: "stop",
		}, nil
	}
	if toolCalls := fakeToolCallsForUser(lastUser); len(toolCalls) > 0 {
		return llm.ChatResponse{
			Content:      "",
			ToolCalls:    toolCalls,
			FinishReason: "tool_calls",
		}, nil
	}
	response := buildAssistantResponse(lastUser)
	return llm.ChatResponse{
		Content:      response,
		FinishReason: "stop",
	}, nil
}

func (f *fakeOpenAI) StreamChatWithTools(_ context.Context, apiKey, _ string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error) {
	if isInvalidKey(apiKey) {
		return llm.ChatResponse{}, llm.ErrUnauthorized
	}
	lastUser := lastUserMessageChat(messages)
	if strings.Contains(lastUser, fakeNetworkMarker) {
		return llm.ChatResponse{}, fakeNetErr{}
	}
	if hasToolResultAfterLastUser(messages) {
		response := buildAssistantResponse(lastUser)
		if onDelta != nil {
			for _, chunk := range splitChunks(response) {
				onDelta(chunk)
			}
		}
		return llm.ChatResponse{
			Content:      response,
			FinishReason: "stop",
		}, nil
	}
	if toolCalls := fakeToolCallsForUser(lastUser); len(toolCalls) > 0 {
		return llm.ChatResponse{
			Content:      "",
			ToolCalls:    toolCalls,
			FinishReason: "tool_calls",
		}, nil
	}
	response := buildAssistantResponse(lastUser)
	if onDelta != nil {
		for _, chunk := range splitChunks(response) {
			onDelta(chunk)
		}
	}
	return llm.ChatResponse{
		Content:      response,
		FinishReason: "stop",
	}, nil
}

func lastUserMessageChat(messages []llm.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func hasToolResultAfterLastUser(messages []llm.ChatMessage) bool {
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return false
	}
	for i := lastUserIdx + 1; i < len(messages); i++ {
		if messages[i].Role == "tool" {
			return true
		}
	}
	return false
}

func fakeToolCallsForUser(lastUser string) []llm.ToolCall {
	lower := strings.ToLower(lastUser)
	if strings.Contains(lower, fakeDeleteMarker) {
		return nil
	}
	if strings.Contains(lower, fakeProposalOpsMarker) {
		return []llm.ToolCall{
			buildToolCall("call-docx", "docx_operations", map[string]any{
				"path":       "report.docx",
				"create_new": true,
				"operations": []map[string]any{
					{
						"op": "set_paragraphs",
						"paragraphs": []map[string]any{
							{"text": "Report", "style": "Heading1"},
							{"text": "Draft content", "style": "Normal"},
						},
					},
				},
			}),
			buildToolCall("call-xlsx", "xlsx_operations", map[string]any{
				"path":       "metrics.xlsx",
				"create_new": true,
				"operations": []map[string]any{
					{"op": "ensure_sheet", "sheet": "Summary"},
					{
						"op":    "set_range",
						"sheet": "Summary",
						"start": "A1",
						"values": [][]any{
							{"Metric", "Value"},
							{"Q1", 120},
						},
					},
				},
			}),
			buildToolCall("call-pptx", "pptx_operations", map[string]any{
				"path":       "deck.pptx",
				"create_new": true,
				"operations": []map[string]any{
					{
						"op":     "add_slide",
						"layout": "title_and_content",
						"title":  "Overview",
						"body":   "Highlights",
					},
				},
			}),
		}
	}
	if isOrgChartPrompt(lastUser) {
		orgChart, assignments := orgChartDraftFiles()
		return []llm.ToolCall{
			buildToolCall("call-org-chart", "write_text_file", map[string]any{
				"path":    "org_chart.md",
				"content": orgChart,
			}),
			buildToolCall("call-assignments", "write_text_file", map[string]any{
				"path":    "project_assignments.csv",
				"content": assignments,
			}),
		}
	}
	return nil
}

func buildToolCall(id, name string, args map[string]any) llm.ToolCall {
	encoded, err := json.Marshal(args)
	if err != nil {
		encoded = []byte("{}")
	}
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.ToolCallFunction{
			Name:      name,
			Arguments: string(encoded),
		},
	}
}

func buildAssistantResponse(lastUser string) string {
	lower := strings.ToLower(lastUser)
	if strings.Contains(lower, "/etc/hosts") || strings.Contains(lower, "external file") {
		return "I can only access files that were added to the Workbench."
	}
	if strings.TrimSpace(lastUser) == "" {
		return "Hello from the workshop."
	}
	return fmt.Sprintf("Assistant response: %s", lastUser)
}

func splitChunks(text string) []string {
	words := strings.Fields(text)
	if len(words) <= 1 {
		return []string{text}
	}
	mid := len(words) / 2
	first := strings.Join(words[:mid], " ") + " "
	second := strings.Join(words[mid:], " ")
	return []string{first, second}
}

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func joinMessages(messages []llm.Message) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, msg.Content)
	}
	return strings.Join(parts, "\n")
}

func isOrgChartPrompt(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, fakeOrgChartMarker) ||
		strings.Contains(lower, "org chart") ||
		strings.Contains(lower, "org_chart") ||
		strings.Contains(lower, "organization chart") ||
		strings.Contains(lower, "project assignments") ||
		strings.Contains(lower, "project_assignments")
}

func orgChartDraftFiles() (string, string) {
	orgChart := `# Project Org Chart

## Atlas
- Alice Kim — PM Lead
- Chloe Nguyen — Backend Engineer
- Diego Patel — Design (shared)

## Beacon
- Bruno Silva — Data Analyst
- Elena Rossi — Frontend Engineer
- Diego Patel — Design (shared)
`
	assignments := `name,project,role
Alice Kim,Atlas,PM Lead
Chloe Nguyen,Atlas,Backend Engineer
Diego Patel,Atlas/Beacon,Designer
Bruno Silva,Beacon,Data Analyst
Elena Rossi,Beacon,Frontend Engineer
`
	return orgChart, assignments
}

func isInvalidKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "invalid") || strings.Contains(lower, "bad")
}

var _ net.Error = fakeNetErr{}
