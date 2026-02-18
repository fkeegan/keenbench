# Implementation Plan: RPI Agent Workflow (Research → Plan → Implement)

## Context

The current workshop agent runs as a single unbounded loop: the model receives the full conversation history + file manifest and calls tools until it finishes or hits the turn limit. This causes context bloat, no upfront planning, no resumability, and no progress visibility. The RPI workflow replaces this with three distinct phases, each running with fresh API context. State transfers between phases via markdown files in workbench metadata.

Design doc: `docs/design/capabilities/rpi-agent-workflow.md`

---

## Phase 1: RPI State Management and Storage

**Goal:** `_rpi/` directory lifecycle, state detection, plan-file parsing. No agent loop changes yet.

**File: `engine/internal/engine/engine.go`**

1. Add constants near line 1085:
   - `rpiResearchMaxTurns = 30`, `rpiPlanMaxTurns = 10`, `rpiItemMaxTurns = 30`, `rpiMaxPlanInflation = 2`

2. Add types:
   - `rpiState` struct: `HasResearch`, `HasPlan`, `PlanItems []rpiPlanItem`, `OriginalCount`, `AllDone`
   - `rpiPlanItem` struct: `Index`, `Label`, `Status` ("pending"/"done"/"failed"), `RawLine`

3. Add engine methods:
   - `rpiDir(workbenchID) string` — returns `meta/workshop/_rpi/` path
   - `readRPIState(workbenchID) rpiState` — checks for research.md/plan.md, parses plan items via regex `^- \[([ x!])\]\s*(\d+)\.\s*(.+)$`
   - `clearRPIState(workbenchID) error` — removes `_rpi/` directory
   - `writeRPIArtifact(workbenchID, filename, content) error` — writes to `_rpi/<filename>`
   - `readRPIArtifact(workbenchID, filename) (string, error)` — reads from `_rpi/<filename>`
   - `markPlanItem(workbenchID, itemIndex, status, reason) error` — updates the checkbox character in place: `- [ ]` → `- [x]` (done) or `- [!]` (failed). On failure, appends reason to end of line: `- [!] 3. Entity: Transport — ... [Failed: <reason>]`. The numbered prefix and label are always preserved so the parser regex matches all three states.
   - `currentToolLogSeq(workbenchID) int` — reads max ID from tool_log.jsonl

4. Clear `_rpi/` state in all flows that invalidate prior agent output:
   - `WorkshopSendUserMessage` (line 738): call `clearRPIState` after appending the user message — new prompt clears prior RPI state.
   - `WorkshopRegenerate` (line 691): call `clearRPIState` before calling `WorkshopRunAgent` — regenerate replays from a prior user message, stale `_rpi/` from the original run must not be reused.
   - `WorkshopUndoToMessage` (line 648): call `clearRPIState` after `undoWorkshopToMessage` — rewind invalidates any RPI artifacts from the undone run.

5. Plan metadata: inject `<!-- original_count: N -->` comment in plan.md to enforce 2x inflation guard. If metadata is missing or unparseable when checked, default `OriginalCount` to the current item count (i.e., no cap enforcement rather than blocking all amendments).

6. Add `appendPlanItems(workbenchID, newItems []string) error` — appends new `- [ ]` items to plan.md after the last existing item. Before appending, reads `<!-- original_count: N -->` and current item count; if appending would exceed `N * rpiMaxPlanInflation`, silently drops the excess items and logs a warning.

7. Add `extractNewPlanItems(text string) []string` — scans text for lines matching the plan item regex `^- \[ \]\s*\d+\.\s*.+$` and returns them. Used by `runImplementPhase` to detect plan amendments in the model's final text response.

8. Malformed plan handling: `readRPIState` must handle edge cases:
   - If plan.md exists but the regex matches zero items → set `HasPlan = true`, `AllDone = true`, `PlanItems = []`. The summary phase will report that no actionable items were found.
   - If items have duplicate indices → accept them as-is (indices are labels, not unique keys; the engine iterates the slice in order).
   - If items have missing/non-sequential numbering → accept them; the regex captures the label from group 3 regardless of numbering.

**New file: `engine/internal/engine/rpi_state_test.go`**
- Tests for state detection, plan parsing, markPlanItem, clearRPIState, inflation guard, appendPlanItems, malformed plan edge cases (pure engine plumbing, no real AI needed)

---

## Phase 2: Tool Filtering

**Goal:** Read-only tool subsets for Research and Plan phases.

**File: `engine/internal/engine/workshop_tools.go`** (after `WorkshopTools` var, line 657)

1. Add `ResearchTools []llm.Tool` and `PlanTools []llm.Tool` package-level vars
2. Populate via `init()`:
   - `ResearchTools`: `list_files`, `get_file_info`, `get_file_map`, `read_file`, `table_get_map`, `table_describe`, `table_stats`, `table_read_rows`, `table_query`, `recall_tool_result`, `xlsx_get_styles`, `docx_get_styles`, `pptx_get_styles`
   - `PlanTools`: `read_file`, `recall_tool_result` (plan artifact is captured from model's final text, not a tool call)
   - Implement uses full `WorkshopTools`

**Tests in `rpi_state_test.go`:** verify ResearchTools has no write tools, PlanTools is minimal.

---

## Phase 3: Phase-Specific System Prompts

**Goal:** Four new prompt constants for R, P, I, and Summary.

**File: `engine/internal/engine/workshop_tools.go`** (near line 2027)

1. `RPIResearchSystemPrompt` — emphasizes exploration, read-only, produce research summary as final text response. Includes MAP-FIRST workflow, receipt instructions.
2. `RPIPlanSystemPrompt` — produce markdown checklist plan as final text response. Exact format specified (# Execution Plan, ## Items with `- [ ]` items). Rules: atomic items, ordered, no execution.
3. `RPIImplementSystemPrompt` — template with `%s` for current item and plan state. Complete ONE item, verify, do NOT work on other items. Full tool set described.
4. `RPISummarySystemPrompt` — template with `%s` for completed plan and `%s` for current file manifest. Summarize outcomes, note failures, list created/modified files (using the manifest for accurate file names/sizes).

No tests needed — static strings, tested implicitly by Phase 4 integration tests.

---

## Phase 4: RPI Orchestrator (Core Engine Refactor)

**Goal:** Replace monolithic `WorkshopRunAgent` with three-phase orchestrator.

**File: `engine/internal/engine/engine.go`**

### 4a. Extract reusable agent loop

Extract current loop (lines 1342-1527) into a parameterized function:

```go
type agentLoopConfig struct {
    workbenchID           string
    client                LLMClient
    apiKey, modelID       string
    messages              []llm.ChatMessage
    tools                 []llm.Tool
    maxTurns              int
    handler               *ToolHandler
    phaseName             string    // "research"/"plan"/"implement" for logging
    persistToConversation bool      // false for RPI internal phases
    emitStreamDeltas      bool      // false for R/P/I, true for Summary (controls WorkshopAssistantStreamDelta)
    toolLogSeqStart       int       // continue tool log IDs from prior phase
}

type agentLoopResult struct {
    finalText     string
    toolLogSeqEnd int
    turnCount     int
    toolCallCount int
    err           *errinfo.ErrorInfo
}

func (e *Engine) runAgentLoop(ctx context.Context, cfg agentLoopConfig) agentLoopResult
```

Key differences from current loop:
- Uses `cfg.tools` instead of hardcoded `WorkshopTools`
- Uses `cfg.maxTurns` instead of `maxAgentTurns`
- Conditional conversation persistence via `cfg.persistToConversation`
- Conditional stream deltas via `cfg.emitStreamDeltas` (false for R/P/I internal phases, true for Summary only). When false, pass `nil` as the `onDelta` callback to `StreamChatWithTools` — the model's text is still captured in `fullResponse` but not streamed to the UI.
- Phase-prefixed logging: `workshop.{phaseName}.agent_*`
- Tool-level notifications (WorkshopToolExecuting/Complete) still emitted in all phases (they feed the spinner/status display, not chat content)

### 4b. Refactor `WorkshopRunAgent` into orchestrator

```
WorkshopRunAgent:
  1. Existing validation (lines 1287-1321, unchanged)
  2. Read RPI state
  3. If !HasResearch → runResearchPhase → notifyPhaseComplete
  4. If HasResearch && !HasPlan → runPlanPhase → notifyPhaseComplete
  5. If HasPlan && !AllDone → runImplementPhase → notifyPhaseComplete
  6. runSummaryPhase → persist summary to conversation.jsonl
  7. Return response
```

### 4c. `runResearchPhase`

- Builds messages: `RPIResearchSystemPrompt` + file manifest + context items + user's latest message
- Calls `runAgentLoop` with `ResearchTools`, `maxTurns=30`, `persistToConversation=false`
- Captures model's final text → `writeRPIArtifact("research.md", finalText)`

### 4d. `runPlanPhase`

- Reads `_rpi/research.md`
- Builds messages: `RPIPlanSystemPrompt` + research content + user prompt + lightweight manifest + context items
- Calls `runAgentLoop` with `PlanTools`, `maxTurns=10`, `persistToConversation=false`
- Captures final text → injects `<!-- original_count: N -->` → `writeRPIArtifact("plan.md", ...)`

### 4e. `runImplementPhase` (engine-driven outer loop)

```
for {
    state = readRPIState()
    if AllDone: break
    item = first pending item
    notifyImplementProgress(current, total, label)

    Rebuild file manifest (reflects files from prior items)
    messages = RPIImplementSystemPrompt(item, plan) + manifest + context items

    result = runAgentLoop(WorkshopTools, maxTurns=30, persistToConversation=false)

    if error:
        retry once with failure context injected
        if retry fails: markPlanItem(item, "failed", reason)  // produces - [!] N. Label [Failed: reason]
        continue

    markPlanItem(item, "done", "")

    // Plan amendments: scan model's finalText for new sub-items
    newItems = extractNewPlanItems(result.finalText)  // looks for lines matching "- [ ] N. ..."
    if len(newItems) > 0:
        appendPlanItems(workbenchID, newItems)  // respects 2x inflation cap, silently drops excess
}
```

**Plan amendment detection:** The `RPIImplementSystemPrompt` instructs the model: "If you discover additional work items needed (e.g., sub-entities not anticipated by the plan), include them at the end of your final text response as new checklist items in the format `- [ ] N. Label — Description`. Do NOT modify existing plan items." The engine's `extractNewPlanItems(text)` scans the model's final text for lines matching the checklist regex pattern and returns them as strings for `appendPlanItems` to append.

### 4f. `runSummaryPhase`

- Emits `WorkshopPhaseStarted` with `phase: "summary"`
- Reads completed plan.md AND current file manifest (so the model can accurately report created/modified files)
- Non-tool API call with `RPISummarySystemPrompt` — receives plan + file manifest as context
- This is the ONLY phase that emits `WorkshopAssistantStreamDelta` — the summary text streams into the chat as the user-visible assistant response
- Returns summary text (persisted to conversation.jsonl as the only user-visible output)
- Emits `WorkshopPhaseCompleted` with `phase: "summary"`
- R/P/I phases do NOT stream deltas — their model output is captured silently for engine use only

### 4g. Message builder helpers

- `buildRPIResearchMessages(ctx, workbenchID)` — system + manifest + context + user prompt
- `buildRPIPlanMessages(ctx, workbenchID, research)` — system + research + user prompt + lightweight manifest + context
- `buildRPIImplementMessages(ctx, workbenchID, item)` — system(item, plan) + manifest + context
- `buildRPIImplementRetryMessages(ctx, workbenchID, item, failureReason)` — same as above + failure context
- `buildRPISummaryMessages(ctx, workbenchID)` — system + completed plan + current file manifest

### 4h. Notification helpers

- `notifyPhase(workbenchID, phase)` → emits `WorkshopPhaseStarted`
- `notifyPhaseComplete(workbenchID, phase)` → emits `WorkshopPhaseCompleted`
- `notifyImplementProgress(workbenchID, current, total, label)` → emits `WorkshopImplementProgress`

**New file: `engine/internal/engine/engine_rpi_test.go`**
- `TestRPIOrchestrator_FullCycle` — scripted responses through all phases, verify artifacts + notifications + conversation
- `TestRPIOrchestrator_ResearchFails` — error propagation
- `TestRPIOrchestrator_ImplementRetrySucceeds` — first attempt fails, retry works
- `TestRPIOrchestrator_ImplementRetryFails` — both fail, item marked `[!]`, next item proceeds
- `TestRPIOrchestrator_NewMessageClearsState` — full cycle then new message resets

Uses `scriptedToolOpenAI` pattern (plumbing tests, not AI behavior).

---

## Phase 5: UI Phase Status Display

**Goal:** Show phase-level progress to users.

**File: `app/lib/state/workbench_state.dart`**
- Add fields: `currentPhase`, `implementCurrentItem`, `implementTotalItems`, `implementItemLabel`
- Handle notifications in switch block:
  - `WorkshopPhaseStarted` → set `currentPhase`
  - `WorkshopPhaseCompleted` → clear phase fields
  - `WorkshopImplementProgress` → set item progress fields

**File: `app/lib/screens/workbench_screen.dart`**
- Add phase status widget above existing tool status (line ~1284):
  - Spinner + `_phaseStatusLabel(state)`
  - "Analyzing files..." / "Planning approach..." / "Working on step 3 of 12: Transport" / "Summarizing results..."
- Add `_phaseStatusLabel` helper near `_toolStatusLabel`

**File: `app/lib/app_keys.dart`**
- Add `workbenchPhaseStatus` key

---

## Phase 6: Error Codes, Doc Updates, E2E Testing

### 6a. Error codes

**File: `engine/internal/errinfo/errinfo.go`** (line ~57)
- Add: `SubphaseRPIResearch`, `SubphaseRPIPlan`, `SubphaseRPIImplement`, `SubphaseRPISummary`

**Wiring:** Each `run*Phase` method wraps errors with its corresponding subphase:
- `runResearchPhase`: LLM errors and file I/O errors → `SubphaseRPIResearch`
- `runPlanPhase`: LLM errors, malformed plan (zero items parsed) → `SubphaseRPIPlan`
- `runImplementPhase`: per-item LLM errors and retry failures → `SubphaseRPIImplement` (item failures are logged but do not halt the outer loop; the error is only propagated if the engine itself fails, not if individual items fail)
- `runSummaryPhase`: LLM errors → `SubphaseRPISummary`

### 6b. Doc updates

| File | Change |
|------|--------|
| `docs/design/capabilities/rpi-agent-workflow.md` | Status → "Implemented" |
| `docs/design/capabilities/workshop.md` | Replace "Agent Loop Intelligence" section with RPI phase descriptions |
| `docs/design/adr/ADR-0010-agentic-workshop-file-operations.md` | Add "Superseded by RPI" note, reference rpi-agent-workflow.md |
| `docs/prd/capabilities/workshop.md` | Update "Agent Loop Intelligence" requirements for RPI semantics |
| `docs/plans/m4-implementation-plan.md` | Update agent loop references to reflect RPI |
| `docs/design/capabilities/workbench-context.md` | Note context injection in all three RPI phases |
| `docs/test/test-plan.md` | Add TC-RPI-01 through TC-RPI-08 test cases |

### 6c. E2E test (real model, per CLAUDE.md policy)

**New file: `app/integration_test/e2e_rpi_workflow_test.dart`**
- Load bank statement test data
- Send prompt: "Break down expenses by category into separate sheets in a new Excel file"
- Assert: draft .xlsx exists with multiple sheets, no RPI artifacts in file list, conversation has summary only, expenses sum to -12,257.08
- Timeout: 300s

---

## Dependency Order

```
Phase 1 (State Management)
  ↓
Phase 2 (Tool Filtering)  ──┐
  ↓                          │
Phase 3 (System Prompts)     │
  ↓                          │
Phase 4 (Orchestrator) ◄─────┘
  ↓
Phase 5 (UI) — can parallel with Phase 4 testing
  ↓
Phase 6 (Docs + E2E)
```

---

## Verification

1. **Unit tests:** `go test ./internal/engine/ -run TestRPI -count=1 -v` — state, parsing, filtering, orchestrator plumbing
2. **Build:** `make engine && make fmt` — compiles clean
3. **Flutter build:** `cd app && flutter build linux` — UI compiles
4. **Manual test:** `make run`, send a complex prompt (e.g., "Break this CSV into sheets by category"), observe:
   - Phase status appears in UI ("Analyzing files..." → "Planning approach..." → "Working on step N of M...")
   - Draft file(s) created
   - Conversation shows clean summary, not internal R/P/I chatter
5. **E2E:** `scripts/e2e/run_e2e.sh e2e_rpi_workflow_test.dart` — real model, structural assertions
