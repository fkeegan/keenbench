# RPI Agent Workflow — Research, Plan, Implement

## Status
Implemented — 2026-02-16

## Problem

The current workshop agent runs as a single unbounded loop: it receives the user prompt, the full file manifest, and the entire conversation history, then calls tools until it finishes or hits the turn limit. This causes:

1. **No upfront planning** — the model starts calling tools immediately, often throwing exploratory queries to discover what it needs rather than reasoning about the task first.
2. **Context bloat** — tool receipts from completed work accumulate in the message array. By turn 30+ the context is full of stale receipts from entities already finished, reducing quality for the current entity.
3. **No resumability** — if the agent hits a turn limit, timeout, or error mid-task, all progress knowledge exists only in the conversation history. There is no structured artifact the next invocation can read to pick up where it left off.
4. **No progress visibility** — the user sees tool names flash by but has no sense of overall progress ("3 of 12 entities done").

## Design

### Core Idea

Every `WorkshopRunAgent` invocation runs three **phases** in sequence, each as an independent API call chain with its own fresh message context. State transfers between phases exclusively via markdown files written to the workbench draft metadata area — never via conversation history accumulation.

```
User prompt
  │
  ▼
┌──────────────────────────────────────────────────┐
│  RESEARCH phase                                  │
│  Fresh context: system prompt + manifest + prompt │
│  Tools: read-only (read_file, table_*, get_*)    │
│  Output: _rpi/research.md                        │
└──────────────────────┬───────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────┐
│  PLAN phase                                      │
│  Fresh context: system prompt + research.md      │
│  Tools: read_file, recall_tool_result             │
│  Output: _rpi/plan.md (trackable checklist)      │
└──────────────────────┬───────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────┐
│  IMPLEMENT phase (loop)                          │
│  For each unchecked item in plan.md:             │
│    Fresh context: system prompt + plan.md + item │
│    Tools: all workshop tools                     │
│    After item: engine marks done in plan.md       │
│  Output: completed files + updated plan.md       │
└──────────────────────┬───────────────────────────┘
                       │
                       ▼
Final response to user
```

### Phase Details

#### Research Phase

**Goal:** Understand the source data and the user's intent. Produce a structured research document.

**Context:**
- System prompt (research-specific)
- File manifest with structural maps (same as today)
- User prompt
- Workbench context items (style guidelines, etc.)

**Allowed tools:** Read-only subset — `list_files`, `get_file_info`, `get_file_map`, `read_file`, `table_get_map`, `table_describe`, `table_stats`, `table_read_rows`, `table_query`, `recall_tool_result`, `xlsx_get_styles`, `docx_get_styles`, `pptx_get_styles`.

**Output:** The model's final text response is captured by the engine and written to `_rpi/research.md` in draft metadata. Contains:
- Summary of source file(s): structure, dimensions, data types, notable patterns
- Interpretation of the user's request
- Key constraints or edge cases discovered
- Data samples that inform the plan

**Turn budget:** 30 turns (research should be exploratory but bounded).

#### Plan Phase

**Goal:** Produce a concrete, trackable execution plan based on the research.

**Context:**
- System prompt (plan-specific)
- Contents of `_rpi/research.md`
- User prompt (for reference)
- File manifest (lightweight, no maps needed since research captured the relevant details)

**Allowed tools:** `read_file`, `recall_tool_result` (to re-check specifics from the research if needed). No write tools — the model's final text response IS the plan artifact.

**Output:** The engine captures the model's final text response and writes it to `_rpi/plan.md` in draft metadata. Format:

```markdown
# Execution Plan

## Task
<one-line summary of what we're doing>

## Items

- [ ] 1. Entity: Groceries — Create sheet "Groceries" with columns [Date, Description, Amount] from rows matching category
- [ ] 2. Entity: Transport — Create sheet "Transport" with columns [Date, Description, Amount] from rows matching category
- [ ] 3. Entity: Utilities — Create sheet "Utilities" with columns [Date, Description, Amount]
...
- [ ] N. Final: Add summary sheet with totals per entity

## Notes
- Source file: transactions.csv (1,204 rows)
- Target file: summary.xlsx (new, create_new=true)
- Each entity sheet should be sorted by date ascending
```

**Turn budget:** 10 turns (planning should be concise).

#### Implement Phase

**Goal:** Execute the plan item by item, updating progress after each.

**Orchestration:** The engine (not the model) drives the outer loop:
1. Engine reads `_rpi/plan.md`, finds the first unchecked item (`- [ ]`)
2. Engine starts a fresh API call chain with:
   - System prompt (implement-specific)
   - Contents of `_rpi/plan.md` (current state with checkmarks)
   - The specific item to work on
   - File manifest (current state — reflects files created by prior items)
3. Model works the item using all available tools
4. Engine marks the item done in `_rpi/plan.md` (`- [x]`) — the model does not update the plan file
5. Engine refreshes the manifest and loops to step 1

**Context per item:** System prompt + plan + current item instructions. No accumulated receipts from prior items. The tool log is available via `recall_tool_result` if the model needs to reference earlier work.

**Turn budget per item:** 30 turns. If an item exceeds this, mark it as failed in the plan and move on.

**Completion:** When all items are checked (or marked failed), the engine runs one final API call to generate the user-facing summary response.

### Resumability

If the agent is interrupted at any point:
- **During Research:** Next invocation detects no `_rpi/research.md` → restarts research.
- **During Plan:** Next invocation detects `_rpi/research.md` but no `_rpi/plan.md` → restarts plan phase.
- **During Implement:** Next invocation detects `_rpi/plan.md` with unchecked items → resumes from first unchecked item. This is safe because the engine rebuilds the file manifest before each item, so the model sees the current file state (including any files partially created by the interrupted prior run) and acts accordingly.
- **All done:** Next invocation detects all items checked → generates summary response.

The `_rpi/` directory acts as a state machine. The engine inspects it to determine which phase to enter.

### Progress Notifications

New notification events for the UI:

| Event | Payload | UI Display |
|-------|---------|------------|
| `WorkshopPhaseStarted` | `{phase: "research"\|"plan"\|"implement"\|"summary", workbench_id}` | "Analyzing files...", "Planning approach...", "Implementing changes...", "Summarizing results..." |
| `WorkshopImplementProgress` | `{current_item: 3, total_items: 12, item_label: "Transport", workbench_id}` | "Working on item 3 of 12: Transport" |
| `WorkshopPhaseCompleted` | `{phase: "research"\|"plan"\|"implement"\|"summary", workbench_id}` | Clears phase indicator |

The UI already handles `WorkshopToolExecuting`/`WorkshopToolComplete` for tool-level status. Phase-level status is a new layer above that.

### Artifact Storage

RPI artifacts live in the workbench draft metadata, not in user-visible draft files:

```
workbenches/<id>/meta/workshop/_rpi/
  research.md
  plan.md
```

These are internal engine state. They are not surfaced in the file list or included in published output. They persist across agent invocations within the same workbench until the user sends a new prompt (which clears the `_rpi/` directory and starts fresh).

### Tool Log as History

The existing tool log (`meta/workshop/tool_log.jsonl`) already stores full tool results with sequential IDs. In the RPI model, this becomes the definitive history:

- The model receives only receipts (compact summaries) during execution
- `recall_tool_result(entry_id)` retrieves full results when needed
- The plan file references describe what was done; the tool log has the raw evidence
- No conversation history accumulates between items — the plan + tool log is the "git log"

### System Prompts

Each phase gets a tailored system prompt:

**Research prompt** emphasizes exploration: read files, run queries, understand the data. Produce a structured research summary as your final text response (the engine captures it and writes `_rpi/research.md`). Do NOT make any modifications.

**Plan prompt** emphasizes structure: read the research, design a step-by-step plan with trackable items. Produce the plan as your final text response (the engine captures it and writes `_rpi/plan.md`). Each item should be atomic (one entity, one sheet, one section). Items should be ordered so that early items can be started and saved before later items are planned in detail.

**Implement prompt** emphasizes execution: you are working on one specific item from the plan. Complete it and verify your work (e.g., read back what you wrote, check formulas). Do not work on other items. The engine updates the plan checkboxes — the model does not modify plan.md. The tool log has history from prior items if you need to reference earlier work.

## Engine Changes

### `WorkshopRunAgent` Refactor

The current monolithic agent loop becomes an orchestrator:

```go
func (e *Engine) WorkshopRunAgent(ctx context.Context, req WorkshopRunAgentRequest) (any, error) {
    // Determine current RPI state
    state := e.readRPIState(req.WorkbenchID)

    switch {
    case state.NeedsResearch():
        e.notifyPhase(req.WorkbenchID, "research")
        err := e.runResearchPhase(ctx, req)
        // fall through to plan

    case state.NeedsPlan():
        e.notifyPhase(req.WorkbenchID, "plan")
        err := e.runPlanPhase(ctx, req)
        // fall through to implement

    case state.NeedsImplement():
        e.notifyPhase(req.WorkbenchID, "implement")
        err := e.runImplementPhase(ctx, req) // loops over items internally
    }

    return e.buildFinalResponse(req.WorkbenchID)
}
```

Each `run*Phase` method builds its own fresh message array, runs its own agent loop (with phase-appropriate tools and turn budget), and writes its artifact.

### Tool Filtering

Each phase restricts the tool set passed to `ChatWithTools`:

- **Research:** read-only tools only (model's final text response is captured by the engine as the research artifact)
- **Plan:** `read_file` + `recall_tool_result` only (model's final text response is captured by the engine as the plan artifact)
- **Implement:** all tools

### New Prompt Flow

Every new user message clears any existing `_rpi/` state so the new prompt gets a fresh RPI cycle. There is no distinction between "new prompt" and "follow-up" — every message triggers a clean Research → Plan → Implement sequence. This avoids a fragile heuristic for detecting plan relevance and is consistent with the decision that every prompt goes through full RPI.

## UI Changes

### Phase Status Display

The `WorkbenchState` in the UI gains:
- `currentPhase: String?` — "research", "plan", "implement", "summary", or null
- `implementProgress: (current: int, total: int, label: String)?`

The workshop screen shows a subtle status line:
- During research: "Analyzing files..."
- During plan: "Planning approach..."
- During implement: "Working on item 3 of 12: Transport"

This replaces or augments the existing tool-name display.

## Migration / Compatibility

- The existing single-loop agent behavior is replaced entirely. No feature flag — RPI is the new default.
- Existing conversation history in workbenches is unaffected (it's still stored and shown in the UI chat). RPI artifacts are separate metadata.
- The `_rpi/` directory is cleared in all flows that invalidate prior agent output:
  - **New user message** (`WorkshopSendUserMessage`): every prompt triggers a fresh RPI cycle.
  - **Regenerate** (`WorkshopRegenerate`): replays from a prior user message; stale `_rpi/` from the original run must not be reused.
  - **Undo/rewind** (`WorkshopUndoToMessage`): rewinds conversation; orphaned `_rpi/` artifacts from the undone run are cleared.

## Design Decisions

1. **Item granularity:** Markdown checklist convention. The engine parses `- [ ]` / `- [x]` / `- [!]` patterns to extract items and track progress. No structured format (YAML/JSON) — the model writes natural markdown, the engine uses a simple regex. This avoids a formatting failure mode.

2. **Failed items:** Retry once, then skip. On first failure, the engine retries the item with a fresh context that includes the failure reason. If the retry also fails, the checkbox is changed to `[!]` and the failure reason is appended to the end of the line (e.g., `- [!] 3. Entity: Transport — ... [Failed: timeout]`). The numbered prefix and label are preserved so the parser regex matches all three states. The engine moves to the next item. All failures are reported in the final summary. A user who asked for 12 sheets gets 11 rather than 0.

3. **Plan amendments:** Add only, with cap. If the model's final text response for an implement item includes new sub-items (e.g., "discovered sub-entities: X, Y, Z"), the engine appends them as new unchecked items to `plan.md`. The model cannot remove or reorder existing items. The engine re-reads the plan after each item, so new unchecked items are picked up naturally. A guard caps total items at 2x the original plan size to prevent runaway inflation.

4. **Context item injection:** All three phases. Workbench context items (style guidelines, etc.) are injected into research, plan, and implement phases. The token cost is negligible and ensures no phase misses relevant constraints.
