# ADR-0010: Agentic Workshop File Operations

## Status
Proposed (2026-02-04)

## Superseded
Superseded by `docs/design/capabilities/rpi-agent-workflow.md` for active Workshop execution flow (Research → Plan → Implement orchestration).

## Context

The M1 implementation of file operations has a fundamental design flaw: it attempts to cram truncated file content into a single model context, then asks the model to propose changes in one shot. This fails when:

1. **Files are too large** - Content gets truncated, model can't see what it needs
2. **Multiple files** - Context budget is split, each file gets less
3. **Model can't explore** - No way to query specific parts of files
4. **Model gives up or hallucinates** - Returns `no_changes: true` or makes blind guesses

**Example failure**: User adds an Excel file and asks to translate it. The model sees truncated content with a hint "Ask the user for the sheet name and row/column range." The model correctly refuses to proceed (`no_changes: true`), but the user sees the assistant say "Proceeding to update the Workbench file now" - a confusing disconnect.

The root cause: the model has no agency. It can't explore files, query specific ranges, or iteratively work through the problem.

## Decision

Redesign the Workshop to use an **agentic tool-use loop** where the model has tools to explore files and apply operations iteratively until the task is complete.

### New Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        User sends message                        │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  System prompt + file manifest (metadata only) + tools schema    │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
        ┌───────────────────────────────────────────┐
        │              Model responds               │
        │  (text + optional tool calls)             │
        └───────────────────────┬───────────────────┘
                                │
              ┌─────────────────┼─────────────────┐
              │                 │                 │
              ▼                 ▼                 ▼
         Text only        Tool calls         Done (stop)
              │                 │                 │
              ▼                 ▼                 │
     Stream to user     Execute tools            │
              │                 │                 │
              │                 ▼                 │
              │         Return results           │
              │                 │                 │
              └────────►◄──────┘                 │
                        │                         │
                        ▼                         │
               Continue loop ◄────────────────────┘
```

### Tool Schema

The model gets access to these tools (OpenAI function calling format):

#### File Discovery
```json
{
  "name": "list_files",
  "description": "List all files in the workbench with their metadata",
  "parameters": {
    "type": "object",
    "properties": {},
    "required": []
  }
}
```

#### Reading Files

> **Note:** The tool schema below reflects the ADR-era design. The current canonical schema is defined in `docs/design/capabilities/file-operations.md` (Chunked Read — Tool Updates section) and adds format-specific parameters: `section` (docx), `slide_index` (pptx), `pages` (pdf), `line_start`/`line_count` (text). The `page_start`/`page_end` parameters below have been superseded.

```json
{
  "name": "read_file",
  "description": "Read content from a file. Use get_file_info first for the structural map, then read specific regions.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "File path"},
      "sheet": {"type": "string", "description": "Sheet name (xlsx only)"},
      "range": {"type": "string", "description": "Cell range like A1:E50 (xlsx only)"},
      "section": {"type": "string", "description": "Section heading or index (docx only)"},
      "slide_index": {"type": "integer", "description": "Slide index, 0-based (pptx only)"},
      "pages": {"type": "string", "description": "Page range like 1-5 (pdf only)"},
      "line_start": {"type": "integer", "description": "Starting line, 1-indexed (text only)"},
      "line_count": {"type": "integer", "description": "Number of lines to read (text only)"}
    },
    "required": ["path"]
  }
}
```

#### File Information
```json
{
  "name": "get_file_info",
  "description": "Get detailed metadata for a file (sheets list for xlsx, page count for pdf, dimensions for images, etc.)",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "File path"}
    },
    "required": ["path"]
  }
}
```

#### Writing Files
```json
{
  "name": "write_text_file",
  "description": "Write or create a text file (md, txt, csv, json, etc.)",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "File path"},
      "content": {"type": "string", "description": "File content"}
    },
    "required": ["path", "content"]
  }
}
```

#### Office Operations
```json
{
  "name": "xlsx_operations",
  "description": "Apply operations to an xlsx file",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "File path"},
      "create_if_missing": {"type": "boolean", "description": "Create file if it doesn't exist"},
      "operations": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "op": {"type": "string", "enum": ["ensure_sheet", "set_cells", "set_range", "delete_sheet"]},
            "sheet": {"type": "string"},
            "start": {"type": "string", "description": "Starting cell for set_range"},
            "values": {"type": "array", "description": "2D array for set_range"},
            "cells": {"type": "array", "description": "Array of {cell, value} for set_cells"}
          },
          "required": ["op"]
        }
      }
    },
    "required": ["path", "operations"]
  }
}
```

Similar tools for `docx_operations` and `pptx_operations`.

Contract note:
- `docx_operations.replace_text` uses canonical field `search` (legacy `find` accepted by engine normalization).
- `pptx_operations.set_slide_text` and `append_bullets` use canonical field `index` (legacy `slide_index` accepted by engine normalization).

### Key Design Principles

1. **Model drives the loop** - The model decides what to read, when to write, and when it's done
2. **Metadata first, content on demand** - Start with file manifest, model queries what it needs
3. **Multiple calls are fine** - Task completion is the goal, not minimizing API calls
4. **Writes go to draft** - All write operations target the draft sandbox
5. **Streaming continues** - Text output streams to user between tool calls
6. **Conversation history preserved** - Tool calls and results become part of the conversation

### Implementation Changes

#### 1. OpenAI Client (`engine/internal/openai/client.go`)

Add tool calling support:

```go
type Tool struct {
    Type     string       `json:"type"` // "function"
    Function FunctionDef  `json:"function"`
}

type FunctionDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
    ID       string `json:"id"`
    Type     string `json:"type"` // "function"
    Function struct {
        Name      string `json:"name"`
        Arguments string `json:"arguments"`
    } `json:"function"`
}

type ChatResponse struct {
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    FinishReason string   `json:"finish_reason"`
}

func (c *Client) ChatWithTools(ctx context.Context, apiKey, model string,
    messages []Message, tools []Tool) (ChatResponse, error)

func (c *Client) StreamChatWithTools(ctx context.Context, apiKey, model string,
    messages []Message, tools []Tool, onDelta func(string), onToolCall func(ToolCall)) (ChatResponse, error)
```

#### 2. New Workshop Flow (`engine/internal/engine/engine.go`)

Replace `WorkshopStreamAssistantReply` + `WorkshopProposeChanges` + `WorkshopApplyProposal` with a single agentic endpoint:

```go
func (e *Engine) WorkshopRunAgent(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
    // 1. Build initial messages with file manifest (not content)
    // 2. Define tools schema
    // 3. Enter agent loop:
    //    a. Call model with tools
    //    b. Stream text to client
    //    c. If tool calls: execute, add results to messages, continue
    //    d. If no tool calls (stop): exit loop
    // 4. Return final state
}
```

#### 3. Tool Handlers (`engine/internal/engine/workshop_tools.go` - new)

```go
type WorkshopToolHandler struct {
    engine      *Engine
    workbenchID string
}

func (h *WorkshopToolHandler) HandleToolCall(call ToolCall) (string, error) {
    switch call.Function.Name {
    case "list_files":
        return h.listFiles()
    case "read_file":
        return h.readFile(call.Function.Arguments)
    case "get_file_info":
        return h.getFileInfo(call.Function.Arguments)
    case "write_text_file":
        return h.writeTextFile(call.Function.Arguments)
    case "xlsx_operations":
        return h.xlsxOperations(call.Function.Arguments)
    // ... etc
    }
}
```

#### 4. IPC Updates

New notification for tool execution status:
```json
{"method": "WorkshopToolExecuting", "params": {"tool_name": "read_file", "path": "data.xlsx"}}
{"method": "WorkshopToolComplete", "params": {"tool_name": "read_file", "success": true}}
```

Stream continues to use `WorkshopAssistantStreamDelta` for text.

#### 5. UI Changes (`app/lib/`)

- Update `workbench_state.dart` to handle the new agentic flow
- Show tool execution status in the UI (subtle indicators)
- Remove the broken auto-apply flow

### Conversation Message Format

Tool calls and results are stored in the conversation history:

```jsonl
{"type":"user_message","message_id":"u-1","role":"user","text":"Translate the excel file..."}
{"type":"assistant_message","message_id":"a-1","role":"assistant","text":"I'll read the file first to understand its structure.","tool_calls":[{"id":"tc-1","function":{"name":"read_file","arguments":"{\"path\":\"data.xlsx\"}"}}]}
{"type":"tool_result","message_id":"tr-1","tool_call_id":"tc-1","content":"Sheet: Sales\nA1: Product, B1: Price..."}
{"type":"assistant_message","message_id":"a-2","role":"assistant","text":"I can see the file has a Sales sheet. Let me create the translated copy.","tool_calls":[{"id":"tc-2","function":{"name":"xlsx_operations","arguments":"..."}}]}
{"type":"tool_result","message_id":"tr-2","tool_call_id":"tc-2","content":"Successfully created data_english.xlsx"}
{"type":"assistant_message","message_id":"a-3","role":"assistant","text":"Done! I've created data_english.xlsx with all text translated to English and prices converted to USD."}
```

### Safety & Limits

1. **Max tool calls per turn**: 50 (raised from original 20; loop detection guards against runaway behavior — see `docs/design/capabilities/workshop.md` Agent Loop Intelligence section for current values and detection algorithm)
2. **Max total turns**: 50 (hard limit)
3. **Timeout per tool**: 30 seconds
4. **All writes to draft only** - same sandbox rules as before
5. **Read-only formats enforced** - PDF/image writes rejected
6. **No path traversal** - same security as before

### Migration

1. Keep proposal system for now (can be removed later)
2. New agentic flow runs alongside old flow initially
3. Feature flag to enable agentic mode per workbench
4. Old flow deprecated once agentic is stable

## Consequences

### Positive
- Model can handle any file size (queries what it needs)
- Model can handle multiple files (examines each as needed)
- Task completion rate dramatically improves
- More natural conversation flow
- Model can ask clarifying questions naturally

### Negative
- More API calls = higher cost
- More complex implementation
- Streaming UX needs refinement for tool calls
- Conversation history grows with tool results

### Neutral
- Latency may increase (more round trips) but perceived UX improves (streaming + progress)

## Alternatives Considered

### 1. Increase context limits
**Rejected**: Doesn't solve the fundamental problem. Large files still won't fit. Expensive.

### 2. Smart chunking/summarization
**Rejected**: Model still can't query specifics. Summarization loses critical details.

### 3. Multi-stage pipeline (explore → plan → execute)
**Rejected**: More complex than unified agentic loop. Agentic approach is more flexible.

## References
- [OpenAI Function Calling](https://platform.openai.com/docs/guides/function-calling)
- Claude Code architecture (similar agentic pattern)
- Original M1 plan: `docs/plans/m1-implementation-plan.md`
