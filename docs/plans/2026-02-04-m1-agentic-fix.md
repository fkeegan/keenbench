# M1 Agentic Fix — Implementation Plan

## Status
Draft (2026-02-04)

## Problem

The current M1 file operations design is broken. It truncates file content and asks the model to propose changes in one shot. When files are large or content is truncated, the model either refuses (`no_changes: true`) or hallucinates.

The fix: make the model agentic. Give it tools to explore files and operate on them iteratively.

## Scope

This plan covers the minimal changes needed to make Workshop file operations work via an agentic tool-use loop. It doesn't replace all of M1 — it fixes the core flow.

---

## Implementation Tasks

### Phase 1: OpenAI Tool Calling Support

**File: `engine/internal/openai/client.go`**

1. Add types for tool calling:
   ```go
   type Tool struct {
       Type     string      `json:"type"`
       Function FunctionDef `json:"function"`
   }

   type FunctionDef struct {
       Name        string          `json:"name"`
       Description string          `json:"description"`
       Parameters  json.RawMessage `json:"parameters"`
   }

   type ToolCall struct {
       ID       string `json:"id"`
       Type     string `json:"type"`
       Function struct {
           Name      string `json:"name"`
           Arguments string `json:"arguments"`
       } `json:"function"`
   }

   type MessageWithTools struct {
       Role       string     `json:"role"`
       Content    string     `json:"content,omitempty"`
       ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
       ToolCallID string     `json:"tool_call_id,omitempty"`
   }

   type ChatResponse struct {
       Content      string     `json:"content"`
       ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
       FinishReason string     `json:"finish_reason"`
   }
   ```

2. Add `ChatWithTools` method (non-streaming first for simplicity):
   ```go
   func (c *Client) ChatWithTools(ctx context.Context, apiKey, model string,
       messages []MessageWithTools, tools []Tool) (ChatResponse, error)
   ```

3. Add `StreamChatWithTools` for streaming text with tool calls:
   ```go
   func (c *Client) StreamChatWithTools(ctx context.Context, apiKey, model string,
       messages []MessageWithTools, tools []Tool,
       onDelta func(string)) (ChatResponse, error)
   ```

**Tests**: Add `TestChatWithTools` to verify tool call parsing.

---

### Phase 2: Workshop Tools Definition

**File: `engine/internal/engine/workshop_tools.go` (new)**

Define the tool schemas:

```go
var workshopTools = []openai.Tool{
    {
        Type: "function",
        Function: openai.FunctionDef{
            Name:        "list_files",
            Description: "List all files in the workbench with metadata (path, type, size)",
            Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
        },
    },
    {
        Type: "function",
        Function: openai.FunctionDef{
            Name:        "get_file_info",
            Description: "Get detailed info for a file (sheet names for xlsx, page count for pdf, etc.)",
            Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
        },
    },
    {
        Type: "function",
        Function: openai.FunctionDef{
            Name:        "read_file",
            Description: "Read file content. For xlsx: specify sheet and optional range (e.g. A1:Z100). For pdf/docx: specify page range.",
            Parameters:  json.RawMessage(`{
                "type": "object",
                "properties": {
                    "path": {"type": "string"},
                    "sheet": {"type": "string", "description": "Sheet name (xlsx only)"},
                    "range": {"type": "string", "description": "Cell range like A1:D100 (xlsx only)"},
                    "page_start": {"type": "integer", "description": "Start page, 1-indexed (pdf/docx)"},
                    "page_end": {"type": "integer", "description": "End page (pdf/docx)"}
                },
                "required": ["path"]
            }`),
        },
    },
    {
        Type: "function",
        Function: openai.FunctionDef{
            Name:        "write_text_file",
            Description: "Write or overwrite a text file",
            Parameters:  json.RawMessage(`{
                "type": "object",
                "properties": {
                    "path": {"type": "string"},
                    "content": {"type": "string"}
                },
                "required": ["path", "content"]
            }`),
        },
    },
    {
        Type: "function",
        Function: openai.FunctionDef{
            Name:        "xlsx_operations",
            Description: "Apply operations to an xlsx file (create or modify)",
            Parameters:  json.RawMessage(`{
                "type": "object",
                "properties": {
                    "path": {"type": "string"},
                    "create_new": {"type": "boolean", "description": "Create new file instead of modifying existing"},
                    "copy_from": {"type": "string", "description": "Path to copy from if creating new"},
                    "operations": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "op": {"type": "string", "enum": ["ensure_sheet", "set_range", "set_cells"]},
                                "sheet": {"type": "string"},
                                "start": {"type": "string"},
                                "values": {"type": "array"}
                            },
                            "required": ["op"]
                        }
                    }
                },
                "required": ["path", "operations"]
            }`),
        },
    },
    // docx_operations and pptx_operations similar
}
```

Contract note for operations fields:
- `docx_operations.replace_text` uses canonical `search` (legacy `find` accepted by normalization).
- `pptx_operations.set_slide_text` and `append_bullets` use canonical `index` (legacy `slide_index` accepted by normalization).

---

### Phase 3: Tool Handlers

**File: `engine/internal/engine/workshop_tools.go`**

```go
type ToolHandler struct {
    e           *Engine
    workbenchID string
}

func (h *ToolHandler) Execute(call openai.ToolCall) (string, error) {
    switch call.Function.Name {
    case "list_files":
        return h.listFiles()
    case "get_file_info":
        return h.getFileInfo(call.Function.Arguments)
    case "read_file":
        return h.readFile(call.Function.Arguments)
    case "write_text_file":
        return h.writeTextFile(call.Function.Arguments)
    case "xlsx_operations":
        return h.xlsxOperations(call.Function.Arguments)
    case "docx_operations":
        return h.docxOperations(call.Function.Arguments)
    case "pptx_operations":
        return h.pptxOperations(call.Function.Arguments)
    default:
        return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
    }
}
```

Handler implementations:
- `listFiles`: Return JSON array of `{path, kind, size_bytes, mime_type}`
- `getFileInfo`: For xlsx call toolWorker to get sheet names + dimensions; for pdf get page count
- `readFile`: Route to appropriate toolWorker extract method based on file kind
- `writeTextFile`: Ensure draft exists, write via workbenches.ApplyWriteToDraft
- `xlsxOperations`: Ensure draft, call toolWorker XlsxApplyOps (handle create_new/copy_from)

---

### Phase 4: Agentic Loop

**File: `engine/internal/engine/engine.go`**

Add new method `WorkshopRunAgent`:

```go
func (e *Engine) WorkshopRunAgent(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
    var req struct {
        WorkbenchID string `json:"workbench_id"`
        MessageID   string `json:"message_id"`  // User message ID that triggers this
    }
    // ... validation ...

    // Build initial messages
    messages := e.buildAgentMessages(ctx, req.WorkbenchID)

    handler := &ToolHandler{e: e, workbenchID: req.WorkbenchID}

    maxTurns := 50
    for turn := 0; turn < maxTurns; turn++ {
        // Call model with tools
        resp, err := e.openai.ChatWithTools(ctx, apiKey, model, messages, workshopTools)
        if err != nil {
            return nil, mapOpenAIError(errinfo.PhaseWorkshop, err)
        }

        // Stream text content to client
        if resp.Content != "" {
            e.notify("WorkshopAssistantStreamDelta", map[string]any{
                "workbench_id": req.WorkbenchID,
                "token_delta":  resp.Content,
            })
        }

        // If no tool calls, we're done
        if len(resp.ToolCalls) == 0 {
            break
        }

        // Add assistant message with tool calls
        messages = append(messages, openai.MessageWithTools{
            Role:      "assistant",
            Content:   resp.Content,
            ToolCalls: resp.ToolCalls,
        })

        // Execute each tool call
        for _, call := range resp.ToolCalls {
            e.notify("WorkshopToolExecuting", map[string]any{
                "workbench_id": req.WorkbenchID,
                "tool_name":    call.Function.Name,
            })

            result, err := handler.Execute(call)
            if err != nil {
                result = fmt.Sprintf("Error: %s", err.Error())
            }

            // Add tool result message
            messages = append(messages, openai.MessageWithTools{
                Role:       "tool",
                ToolCallID: call.ID,
                Content:    result,
            })

            e.notify("WorkshopToolComplete", map[string]any{
                "workbench_id": req.WorkbenchID,
                "tool_name":    call.Function.Name,
                "success":      err == nil,
            })
        }
    }

    // Save final assistant response to conversation
    // ...

    return map[string]any{"completed": true}, nil
}
```

Helper for building initial messages:
```go
func (e *Engine) buildAgentMessages(ctx context.Context, workbenchID string) []openai.MessageWithTools {
    // 1. System prompt (explains tools, context)
    // 2. File manifest (metadata only, no content!)
    // 3. Conversation history
    // 4. Current user message
}
```

System prompt:
```
You are a helpful assistant working with files in a Workbench. You have tools to:
- list_files: See all available files
- get_file_info: Get detailed info about a file (sheets, pages, etc.)
- read_file: Read file content (can specify sheet/range for xlsx, page range for pdf)
- write_text_file: Write text files
- xlsx_operations: Create or modify Excel files
- docx_operations: Create or modify Word documents
- pptx_operations: Create or modify PowerPoint files

IMPORTANT:
- Always explore files first before making changes
- For large files, read specific sections rather than the entire file
- All changes are saved to a draft that the user will review
- PDF and images are read-only

Current workbench files:
[manifest here]
```

---

### Phase 5: UI Updates

**File: `app/lib/state/workbench_state.dart`**

Replace the current flow:
```dart
// Old: submitMessage -> streamReply -> proposeChanges -> applyProposal
// New: submitMessage -> runAgent (handles everything)
```

Add handler for new notifications:
```dart
case 'WorkshopToolExecuting':
  // Show subtle indicator
case 'WorkshopToolComplete':
  // Clear indicator
```

**File: `app/lib/screens/workbench_screen.dart`**

- Show tool execution status in the message area
- Remove the old "Applying changes..." flow

---

### Phase 6: Wire Up IPC

**File: `engine/internal/engine/engine.go` (router)**

```go
"WorkshopRunAgent": e.WorkshopRunAgent,
```

**File: `app/lib/services/engine_service.dart`**

Add method to call `WorkshopRunAgent`.

---

## Testing

1. **Unit: OpenAI tool calling** - Verify tool call parsing, multiple tool calls
2. **Unit: Tool handlers** - Each handler with mock toolWorker
3. **Integration: Agent loop** - Full loop with real files
4. **E2E: Excel translation** - The failing scenario that started this

---

## Migration

1. Add feature flag `agentic_workshop` (default: true for new workbenches)
2. Old workbenches continue with old flow
3. After validation, remove old flow and flag

---

## Sequence Diagram

```
User                    UI                     Engine                  Model
 │                      │                        │                       │
 │ "Translate excel"    │                        │                       │
 │─────────────────────>│ WorkshopRunAgent       │                       │
 │                      │───────────────────────>│                       │
 │                      │                        │ [system + manifest]   │
 │                      │                        │──────────────────────>│
 │                      │                        │                       │
 │                      │                        │<──────────────────────│
 │                      │                        │ "Let me check..."     │
 │                      │                        │ + tool_call(read_file)│
 │<─ stream delta ──────│<───────────────────────│                       │
 │                      │                        │                       │
 │                      │                        │ [execute read_file]   │
 │                      │<─ ToolExecuting ───────│                       │
 │                      │<─ ToolComplete ────────│                       │
 │                      │                        │                       │
 │                      │                        │ [result + continue]   │
 │                      │                        │──────────────────────>│
 │                      │                        │<──────────────────────│
 │                      │                        │ "Creating copy..."    │
 │                      │                        │ + tool_call(xlsx_ops) │
 │<─ stream delta ──────│<───────────────────────│                       │
 │                      │                        │                       │
 │                      │                        │ [execute xlsx_ops]    │
 │                      │<─ ToolExecuting ───────│                       │
 │                      │<─ ToolComplete ────────│                       │
 │                      │                        │                       │
 │                      │                        │ [result + continue]   │
 │                      │                        │──────────────────────>│
 │                      │                        │<──────────────────────│
 │                      │                        │ "Done! Created..."    │
 │<─ stream delta ──────│<───────────────────────│ (no tool calls)       │
 │                      │                        │                       │
 │                      │<─ complete ────────────│                       │
```

---

## Files Changed Summary

| File | Change |
|------|--------|
| `engine/internal/openai/client.go` | Add `ChatWithTools`, types |
| `engine/internal/openai/client_test.go` | Add tool calling tests |
| `engine/internal/engine/workshop_tools.go` | New file: tools schema + handlers |
| `engine/internal/engine/engine.go` | Add `WorkshopRunAgent`, router entry |
| `app/lib/state/workbench_state.dart` | Replace flow with agentic call |
| `app/lib/screens/workbench_screen.dart` | Tool execution UI |
| `app/lib/services/engine_service.dart` | Add `runAgent` method |

---

## Estimated Order of Implementation

1. OpenAI tool calling (required foundation)
2. Tool schemas and handlers
3. Agent loop in engine
4. UI integration
5. Testing and polish
