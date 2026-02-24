# Implementation Plan: M4 — File Operations Intelligence + Agentic Reliability

## Status
In Progress (partially implemented, updated 2026-02-16)

## Goal
Implement the file-operations improvements defined in `docs/design/capabilities/file-operations.md`, aligned with `docs/design/design.md` and `docs/design/adr/ADR-0010-agentic-workshop-file-operations.md`, so Workshop can reliably handle large structured files with map-first context, chunked reads, deterministic local edits, and Review fallback paths for DOCX/PPTX when raster preview renderers are unavailable.

---

## Key References
- `docs/design/capabilities/file-operations.md`
- `docs/design/capabilities/workshop.md`
- `docs/design/design.md`
- `docs/design/adr/ADR-0010-agentic-workshop-file-operations.md`
- `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md`
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`
- `docs/test/test-plan.md`
- `AGENTS.md`
- `CLAUDE.md`

---

## Current State Audit (Codebase)

| Area | Current State | Gap vs Target |
|---|---|---|
| Workshop initial context | `buildAgentMessages()` now builds a map-first manifest and includes `*GetMap` output for structured files (`getFileMapForContext`). | No explicit token-budget cap for manifest/map payload size in the agentic path. |
| Legacy prompt path | `buildWorkshopFileContext()` still powers compatibility flow (`WorkshopStreamAssistantReply` / `WorkshopProposeChanges`) with truncation-based content dumps. | Legacy and agentic context strategies diverge; deprecation/removal plan needed once compatibility flow is retired. |
| Tool read contract | `read_file` now accepts `section`, `slide_index`, `pages`, `line_start`, and `line_count`; worker returns `chunk_info` for scoped reads. | `xlsx` `range` is only routed via `XlsxReadRange` when `sheet` is provided; `range`-only calls currently fall back to broader extraction. |
| Tool worker map RPCs | `engine/tools/pyworker/worker.py` implements and dispatches `XlsxGetMap`, `DocxGetMap`, `PptxGetMap`, `PdfGetMap`; engine consumes them via `get_file_map` and `buildAgentMessages()`. | Method availability/wiring is largely closed; remaining work is deterministic/performance hardening on large real-world files. |
| RPI orchestration safeguards | Workshop now runs as Research → Plan → Implement → Summary with per-phase turn limits and implement retry/failure marking. | Continue tuning phase prompts and retry heuristics on real workloads. |
| Conversation persistence | Agent loop now persists assistant `tool_calls` plus `tool_result` entries to `conversation.jsonl`. | Strengthen replay/regression coverage for long tool-heavy conversations. |
| Provider parity | Google model now advertises `supports_file_write=true`; `WorkshopRunAgent` uses the same tool-capable workflow across providers. | Keep parity tests in place to prevent provider-specific regressions. |
| OpenAI request profile | OpenAI Responses calls now set `truncation=disabled`, `reasoning.effort=medium`, enforce `parallel_tool_calls=false`, set function tools `strict=true`, normalize function schemas to strict-mode JSON Schema requirements, and use `tool_choice=required` on first tool turn then `auto`; sampling fields are model-aware (`temperature`/`top_p` omitted for GPT-5 family compatibility, kept deterministic for non-GPT-5 models). | Future enhancement: make these settings user-tunable per workbench/run without weakening safe defaults. |
| UI feedback | Workshop state handles `WorkshopToolExecuting` / `WorkshopToolComplete`; Workshop screen renders active tool status. | UI currently shows only a single active tool label (limited per-call detail/progress). |
| DOCX/PPTX focus hints | `WorkshopApplyProposal` only computes `buildXlsxFocusHint()`. | Missing focus hints for changed DOCX sections and PPTX slides. |
| DOCX/PPTX review path | Review relies on renderer-driven `ReviewGetDocxPreviewPage` / `ReviewGetPptxPreviewSlide` for visual comparison. | Missing structured content fallback when preview renderer is unavailable; user sees preview error pane instead of useful content. |
| AI tests policy | Several tests still branch on fake mode or use fake AI (`KEENBENCH_FAKE_OPENAI`, fake clients) for AI-path behavior. | Violates repo policy: AI interaction tests must use real model calls. |

---

## Delivery Snapshot (2026-02-10)

### Implemented in Codebase
1. Map-first Workshop context assembly (`buildAgentMessages`) with structural map retrieval.
2. `get_file_map` tool path and map retrieval wiring for xlsx/docx/pptx/pdf/text.
3. Expanded `read_file` selectors (`section`, `slide_index`, `pages`, `line_start`, `line_count`) with chunk metadata support in worker responses.
4. RPI phase controls (`rpiResearchMaxTurns`, `rpiPlanMaxTurns`, `rpiItemMaxTurns`) with implement retry/failure progression.
5. Conversation persistence for tool-capable runs (`assistant_message` with `tool_calls`, plus `tool_result` entries).
6. Provider parity in agentic path (Google included in the same tool-capable workflow and model registry write capability).
7. Workshop UI handling/rendering of `WorkshopToolExecuting` and `WorkshopToolComplete`.
8. OpenAI request hardening for faithful tool use and large-file workflows (GPT-5-compatible sampling policy, explicit reasoning effort, sequential tool calls, first-turn required tool grounding, and strict function schemas).

### Remaining to Close M4
1. Add DOCX/PPTX focus hints in `WorkshopApplyProposal` (`buildDocxFocusHint`, `buildPptxFocusHint`).
2. Add structured DOCX/PPTX review diff RPCs and worker payload methods (`ReviewGetDocxContentDiff`, `ReviewGetPptxContentDiff`, `DocxGetSectionContent`, `PptxGetSlideContent` layout-lite).
3. Add Review fallback behavior so DOCX/PPTX remain usable without renderer-driven previews.
4. Align AI-path tests with real-model-only policy and reduce fake-mode coverage to plumbing-only cases.
5. Harden map/agent prompt sizing and deterministic behavior on large real-world files.

---

## M4 Scope

### In Scope (Remaining Work)
1. DOCX/PPTX focus hints persisted from proposal-apply for Review navigation.
2. Structured DOCX/PPTX Review content diff RPCs backed by worker section/slide extraction.
3. DOCX/PPTX review fallback path that remains usable when raster preview renderer is missing.
4. AI test suite alignment with real-model-only policy.
5. Determinism/performance hardening for map-first context and chunked reads on large files.

### Out of Scope
1. Remote tool containers or provider-side file editing APIs.
2. Workbench nested directories.
3. Real-time collaborative editing.
4. Non-local review/diff processing.
5. Style/asset preservation APIs (`*GetStyles`, `*CopyAssets`) and related derivative-file fidelity work (moved to M5).
6. Full positioned PPTX canvas rendering (deferred to M5; M4 ships text/layout-lite slide payloads).

---

## Decisions Locked for M4
1. `get_file_map` is the canonical map retrieval tool and the required first-step for structured-file browsing.
2. `get_file_info` remains lightweight metadata and does not include full structural maps.
3. Structured files (xlsx/docx/pptx/pdf) are represented in initial context by map data, not full extracted content.
4. `read_file` returns `{text, chunk_info?}`; `chunk_info` is required for chunked/partial reads.
5. Agent loop controls follow Workshop design constants: max turns 50, max tool calls/turn 50, with loop warning/hard-stop detection.
6. Tool calls/results are persisted to conversation history as first-class records (additive format, backward-compatible reader).
7. Google model path uses the same tool-capable loop as other providers; no special no-write fallback.
8. AI behavior tests use real model APIs only; fake AI remains only for pure plumbing tests where AI content is irrelevant.
9. DOCX/PPTX structured review diff compares Draft against a frozen Review Reference captured at draft creation (legacy wire/internal name: "baseline"), not mutable live Published content.
10. Review UX/API must distinguish the diff reference side from preview rendering: `Reference (Draft start)` vs `Published (current preview)`. User-facing copy must avoid leaking internal "baseline" terminology.
11. DOCX/PPTX preview failures do not block review; UI falls back to structured content diff, then existing text diff.
12. If the Draft-start Review Reference is unavailable, structured diff must degrade gracefully: compare against current Published when available and surface explicit source/warning metadata instead of a dead-end empty state.
13. PPTX structured payload in M4 is text/layout-lite (shape blocks + text runs); full positioned slide rendering is deferred to M5.

---

## API / Contract Changes

### Workshop Tool Contracts
1. Implemented: `get_file_info(path)` returns lightweight metadata, and `get_file_map(path)` returns structural map payloads.
2. Implemented: `read_file(path, sheet?, range?, section?, slide_index?, pages?, line_start?, line_count?)` contract is available.
3. Implemented: chunk-capable worker read paths return `text` plus optional `chunk_info`.
4. Remaining: tighten selector consistency for `xlsx` range-only reads and codify deterministic response limits.

### Engine Internal RPC to Pyworker
1. Implemented methods:
   - `XlsxGetMap`
   - `DocxGetMap`
   - `PptxGetMap`
   - `PdfGetMap`
2. Implemented chunk-capable extraction/read methods for docx/pptx/pdf/text/xlsx browsing.
3. Remaining review-structured extraction methods:
   - `DocxGetSectionContent`
   - `PptxGetSlideContent` (layout-lite payload in M4)

### Review Contracts
1. Remaining: persist new focus hints at proposal-apply time:
   - `buildDocxFocusHint(ops)` for changed sections
   - `buildPptxFocusHint(ops)` for changed slides
2. Remaining: add `ReviewGetDocxContentDiff(workbench_id, path, section_index?) -> {baseline, draft, section_count, baseline_missing, reference_source, reference_warning?}`.
3. Remaining: add `ReviewGetPptxContentDiff(workbench_id, path, slide_index?) -> {baseline, draft, slide_count, baseline_missing, reference_source, reference_warning?}`.
4. Keep existing preview endpoints (`ReviewGetDocxPreviewPage`, `ReviewGetPptxPreviewSlide`) as optional visual aids.
5. Structured diff endpoints must honor draft-start reference semantics used by `ReviewGetTextDiff`.
6. `baseline` remains the legacy wire field name for compatibility; semantically it is the reference side.
7. `reference_source` contract:
   - `draft_start_snapshot`: authoritative diff reference.
   - `published_current_fallback`: degraded fallback when draft-start reference is unavailable.
   - `none`: no reference available.

### Conversation Record Extensions
Implemented additive record support in `meta/conversation.jsonl`:
1. `assistant_message` with `tool_calls[]`.
2. `tool_result` entries with `tool_call_id`, `tool_name`, `success`, `content`.

---

## Implementation Plan (By Area)

### Phase 1: Pyworker File Maps + Chunked Reads
Files:
- `engine/tools/pyworker/worker.py`

Status: Partially complete.

Completed:
1. `XlsxGetMap`, `DocxGetMap`, `PptxGetMap`, `PdfGetMap`, and `TextGetMap` are implemented and dispatched.
2. Read methods support selector-driven chunked browsing (`section`, `slide_index`, `pages`, `line_start`, `line_count`) with `chunk_info` responses where applicable.

Remaining:
1. Implement `DocxGetSectionContent` returning structured paragraphs/runs, table blocks, and inline image refs.
2. Implement `PptxGetSlideContent` returning layout-lite shape/text payloads (stable ordering, basic style metadata, optional coarse bounds).
3. Add deterministic/performance hardening against large real-world files (response-size and latency stability).

Acceptance:
1. Each `*GetMap` method returns deterministic JSON for the same input file.
2. Chunk selectors produce stable ranges with explicit `chunk_info`.
3. `DocxGetSectionContent` and `PptxGetSlideContent` return deterministic JSON keyed by section/slide index.

### Phase 2: Engine Tool Handler and Prompt Assembly
Files:
- `engine/internal/engine/workshop_tools.go`
- `engine/internal/engine/engine.go`

Status: Mostly complete.

Completed:
1. `WorkshopTools` map-first schemas and descriptions are in place.
2. `ToolHandler.getFileInfo()`, `get_file_map`, and expanded `ToolHandler.readFile()` routing are implemented.
3. `buildAgentMessages()` uses map-first context and fails closed when manifest assembly fails.
4. Legacy `buildWorkshopFileContext()` remains available for compatibility flow.

Remaining:
1. Define and enforce explicit prompt-size budgeting for manifest/map assembly in agentic runs.
2. Plan and execute compatibility-flow deprecation to remove long-term context divergence.
3. Harden map extraction fallback behavior under heavy files and partial worker failures.

Acceptance:
1. Agent system prompt contains manifest + maps, not truncated structured content dumps.
2. Agent run fails closed when full manifest cannot be assembled.

### Phase 3: Agent Loop Reliability and Persistence
Files:
- `engine/internal/engine/engine.go`
- `engine/internal/engine/models_registry.go`
- `engine/internal/engine/providers_helpers.go`
- `engine/internal/gemini/client.go`

Status: Mostly complete.

Completed:
1. Tool-call cap is raised to 50 with loop detection warning/hard-stop logic.
2. Tool calls/results are persisted into conversation history.
3. Google runs through the same tool-capable workflow as other providers and advertises write support.
4. Gemini tool-call IDs are generated uniquely per call.

Remaining:
1. Add stronger regression tests for replay stability and repeated-call loop thresholds.
2. Tune loop-detection thresholds with production-like corpus to reduce false positives.

Acceptance:
1. Repeated identical tool calls trigger warning then hard stop with clear error.
2. Conversation replay contains complete tool-use trace.
3. All providers run through the same tool-capable workflow.

### Phase 4: Review DOCX/PPTX Structured Diff + Fallback
Files:
- `engine/internal/engine/engine.go`
- `engine/tools/pyworker/worker.py`
- `app/lib/screens/review_screen.dart`
- `app/lib/models/models.dart`

Status: Not started.

Tasks:
1. Add `buildDocxFocusHint()` and `buildPptxFocusHint()` in `WorkshopApplyProposal` and persist them alongside existing XLSX focus hints.
2. Add `ReviewGetDocxContentDiff` and `ReviewGetPptxContentDiff` engine RPCs that return baseline + draft structured payloads for selected section/slide.
3. Keep reference semantics aligned with existing review flow: use the draft-start Review Reference snapshot as primary comparison source.
4. Add explicit source metadata (`reference_source`, optional `reference_warning`) so UI can show whether comparison uses draft-start reference or published-current fallback.
5. Update Review UI so DOCX/PPTX use structured content diff as the primary fallback when renderer-driven preview is unavailable, and label panes clearly as `Reference (Draft start)` or `Published (current preview)` as appropriate.
6. Preserve existing renderer-based preview behavior when available (no regression for environments with renderer installed).
7. If draft-start reference is unavailable but current Published is available, return structured comparison using published-current fallback instead of blocking with `baseline_missing` empty state.

Acceptance:
1. DOCX/PPTX review remains usable when `ReviewGetDocxPreviewPage` / `ReviewGetPptxPreviewSlide` fail with renderer-unavailable errors.
2. Initial DOCX/PPTX review view opens near changed section/slide based on focus hint.
3. Structured diff payloads for reference and draft are aligned by section/slide index and are deterministic.
4. Review UI clearly distinguishes diff reference panes from preview panes (`Reference (Draft start)` vs `Published (current preview)`), with no user-facing "baseline" jargon.
5. Missing draft-start reference does not produce a dead-end for modified files when current Published is available; review falls back to published-current comparison and shows explicit warning/source state.

### Phase 5: Workshop UI Tool Progress
Files:
- `app/lib/state/workbench_state.dart`
- `app/lib/models/models.dart`
- `app/lib/screens/workbench_screen.dart`

Status: Complete for M4 baseline.

Completed:
1. `WorkshopToolExecuting` and `WorkshopToolComplete` notifications are handled.
2. Active tool execution state is tracked.
3. Workshop message area renders non-blocking tool activity status.

Remaining (optional polish, not M4-critical):
1. Expand single-label status into richer per-call progress/details.

Acceptance:
1. User sees non-blocking tool activity feedback during agent runs.
2. Existing streaming text UX remains intact.

### Phase 6: Tests and Policy Alignment
Files (non-exhaustive):
- `engine/internal/engine/*_test.go`
- `engine/internal/openai/*_test.go`
- `engine/internal/toolworker/*_test.go`
- `app/integration_test/*.dart`
- `app/integration_test/support/e2e_utils.dart`

Status: Partially complete.

Completed:
1. Core engine behavior for map-first prompt/context and loop detection is covered by existing unit tests.

Remaining:
1. Replace fake-AI branches in AI-path tests with real-model execution path.
2. Keep fake clients only in pure plumbing tests where AI response semantics are irrelevant.
3. Expand worker tests for map determinism and chunk-selector edge cases.
4. Add integration/E2E assertions for:
   - map-driven reads,
   - large xlsx chunked processing completion,
   - docx/pptx focus hints and structured review fallback behavior,
   - tool execution notifications,
   - provider parity for tool-based writes.

Acceptance:
1. AI-path tests satisfy real-model-only policy.
2. Structural/numeric assertions validate behavior without brittle prose matching.

---

## Acceptance Criteria (M4)
| Criterion | Status (2026-02-10) | Notes |
|---|---|---|
| Every structured file in Workbench context includes a structural map for Workshop runs. | Implemented | Delivered via map-first `buildAgentMessages()` and `getFileMapForContext`. |
| `read_file` supports format-specific selectors and chunk metadata. | Implemented (with caveat) | Core selectors are implemented; `xlsx` range-only routing consistency remains to be tightened. |
| Large spreadsheets/documents can be processed without silent truncation. | Partially implemented | Map-first + chunked reads are in place; explicit prompt-size budgeting still needs hardening. |
| Workshop executes as resumable RPI phases with internal artifacts and summary-only visible output. | Implemented | `WorkshopRunAgent` orchestrates Research/Plan/Implement/Summary with `_rpi` state and phase notifications. |
| Conversation history captures assistant tool calls and tool results. | Implemented | Stored in `conversation.jsonl` as additive metadata/types. |
| OpenAI, Anthropic, and Google all support the same local file-write workflow. | Implemented | Shared tool-capable workflow is active. |
| DOCX/PPTX review remains functional without a local preview renderer by using structured content fallback. | Pending | Phase 4. |
| DOCX/PPTX focus hints are stored and consumed for initial review navigation. | Pending | Phase 4. |
| DOCX/PPTX structured review endpoints honor draft-start reference semantics and include explicit `reference_source` metadata. | Pending | Phase 4. |
| Review UI labels and copy distinguish `Reference (Draft start)` from `Published (current preview)` and avoid user-facing baseline jargon. | Pending | Phase 4. |
| Missing draft-start reference degrades to published-current comparison (when available) with an explicit warning instead of a dead-end state. | Pending | Phase 4. |
| AI interaction tests run with real model APIs only. | Pending | Phase 6 cleanup is still required. |

---

## Rollout / Migration Notes
1. Conversation format changes are additive; legacy entries remain readable.
2. Existing proposal/apply compatibility endpoints remain available during M4.
3. Map and chunk behavior is internal to engine/tool contracts; no destructive data migration required.
4. Focus hint payload is additive; existing drafts without DOCX/PPTX hints continue to function with default view selection.

---

## Risks and Mitigations
1. Risk: map payloads bloat prompt size.
   - Mitigation: include metadata-only maps; cap verbose fields; omit heavy optional sections unless requested.
2. Risk: loop detection false positives on legitimate chunk sweeps.
   - Mitigation: hash by arguments and tune thresholds with real-file test corpus.
3. Risk: pyworker performance regression on very large files.
   - Mitigation: cache lightweight map computation within a single request cycle and profile representative fixtures.
4. Risk: test instability with real models.
   - Mitigation: structural assertions, generous timeouts, and deterministic fixtures in `engine/testdata/`.
5. Risk: structured DOCX/PPTX payloads diverge from visible layout.
   - Mitigation: keep M4 scope explicitly text/layout-lite, surface summary + diff fallback, and defer full positioned rendering to M5.
