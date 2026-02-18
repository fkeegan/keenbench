# M4 Follow-up Fix Plan — Office Review Focus Targets (2026-02-11)

Status: Proposed  
Owner: TBD (next implementation session)  
Source issue: `docs/issues/2026-02-10-file-type-operations-issues.md` (Issue 2)

## Problem Summary

Review currently opens office previews at default positions (first slide/first sheet) instead of the changed target for some TC flows:

- TC-150 (`slides.pptx`): opens at `Slides 1/4` instead of the newly added final slide.
- TC-131 (`quarterly_data.xlsx`): opens `Q1` instead of changed `Annual` sheet.

This hurts review usability and forces manual navigation to find the actual change.

## Confirmed Evidence

### TC-150 (PPTX)

- Change operation is add-slide and content check targets final slide:
  - `<tmp-run-dir>` (`PptxApplyOps`, `ops_kinds:["add_slide"]`)
  - `<tmp-run-dir>` (`PptxExtractText`, `slide_index:3`)
- Review change set has no `focus_hint`:
  - `<tmp-run-dir>` (`ReviewGetChangeSet` result for `slides.pptx`)
- Review preview requests start at first slide:
  - `<tmp-run-dir>` (`ReviewGetPptxPreviewSlide`, `slide_index:0`, draft)
  - `<tmp-run-dir>` (`ReviewGetPptxPreviewSlide`, `slide_index:0`, published)

### TC-131 (XLSX)

- Change operation is summary-sheet generation:
  - `<tmp-run-dir>` (`XlsxApplyOps`, `ops_kinds:["summarize_by_category"]`)
- Review change set has no `focus_hint`:
  - `<tmp-run-dir>` (`ReviewGetChangeSet` result for `quarterly_data.xlsx`)
- Review preview requests with `sheet:null`:
  - `<tmp-run-dir>` (`ReviewGetXlsxPreviewGrid`, draft, `sheet:null`)
  - `<tmp-run-dir>` (`ReviewGetXlsxPreviewGrid`, published, `sheet:null`)
- Worker then defaults to first sheet:
  - `engine/tools/pyworker/worker.py:1152` (`sheet_name = params.get("sheet") or (sheets[0] if sheets else None)`)

## Root Cause Analysis

### 1) Focus hints are only persisted in proposal-apply path

- `WorkshopApplyProposal` computes and writes focus hints:
  - `engine/internal/engine/engine.go:1022`
  - `engine/internal/engine/engine.go:1037`
  - `engine/internal/engine/engine.go:1045`
  - `engine/internal/engine/engine.go:1068`
- Current user workflow uses `WorkshopRunAgent` (tool calls), which does not persist focus hints; it only persists draft summary fallback:
  - `engine/internal/engine/engine.go:1338`
- Result: `ReviewGetChangeSet` has no `focus_hint` for agent-written office files.

### 2) PPTX fallback hint logic is wrong for add-slide-without-index

- `buildPptxFocusHint` currently sets slide to 0 when it sees `add_slide` without explicit index:
  - `engine/internal/engine/engine.go:3502`
  - `engine/internal/engine/engine.go:3503`
- This points review to first slide, not the newly added final slide.

### 3) XLSX hint builder misses summary-style ops

- `buildXlsxFocusHint` only handles:
  - `set_cells` at `engine/internal/engine/engine.go:3383`
  - `set_range` at `engine/internal/engine/engine.go:3400`
- It ignores `summarize_by_category` and `ensure_sheet`, so no hint is produced for TC-131-type edits.

### 4) Preview loading is brittle when target exists only in Draft

- Review UI calls published preview with same target index/sheet as draft:
  - PPTX call path: `app/lib/screens/review_screen.dart:320`
  - XLSX call path: `app/lib/screens/review_screen.dart:442`
- If the target slide/sheet exists only in draft, published calls may fail and currently can fail the whole preview load path (especially XLSX catch path):
  - `app/lib/screens/review_screen.dart:473`

## Goals

1. Review opens on the changed PPTX slide / XLSX sheet by default for agent-driven edits.
2. `focus_hint` is persisted for office edits regardless of whether changes came from proposal apply or direct tool calls in `WorkshopRunAgent`.
3. New draft-only targets (e.g., added slide/new sheet) do not break preview loading.

## Non-goals

1. No redesign of Review UI layout.
2. No changes to diff semantics for text/docx/pptx beyond target selection behavior.
3. No changes to unrelated Issue 1 baseline behavior in this plan.

## Implementation Plan

### Phase A — Persist focus hints from agent tool path

Files:

- `engine/internal/engine/workshop_tools.go`
- `engine/internal/engine/engine.go`

Tasks:

1. Extend `ToolHandler` to collect per-path focus hint candidates for successful office write tools:
   - `xlsx_operations` -> derive from `buildXlsxFocusHint(ops)`.
   - `docx_operations` -> derive from `buildDocxFocusHint(ops)`.
   - `pptx_operations` -> derive from `buildPptxFocusHint(ops)` plus add-slide post-resolution (Phase B).
2. Add a method on `ToolHandler` to expose collected hints.
3. In `WorkshopRunAgent`, after draft state is available, persist tool-path hints with existing `writeProposalFocusHints(...)` into the same review metadata location used by `ReviewGetChangeSet`.
4. Ensure merge behavior is deterministic when multiple tool calls touch same path (last successful tool call wins).

Acceptance:

1. `ReviewGetChangeSet` for agent-created office edits includes `focus_hint`.
2. Existing `WorkshopApplyProposal` hint behavior is unchanged.

### Phase B — Correct PPTX add-slide default target

Files:

- `engine/internal/engine/engine.go`
- `engine/internal/engine/workshop_tools.go`

Tasks:

1. Change `buildPptxFocusHint` so `add_slide` without explicit `index`/`slide_index` does not default to 0.
2. In agent tool flow (`pptx_operations`), after successful `PptxApplyOps`, resolve final slide index for add-slide cases from draft file state (e.g., via `PptxGetMap` or equivalent) and record `focus_hint.slide_index = last_slide_index`.
3. Preserve behavior for explicit index operations (`set_slide_text`, `append_bullets`) using canonical/normalized index fields.

Acceptance:

1. TC-150-like edit opens on final added slide by default.
2. No regression for edits targeting existing explicit slide indices.

### Phase C — Add XLSX hint support for summary/sheet-level ops

Files:

- `engine/internal/engine/engine.go`

Tasks:

1. Extend `buildXlsxFocusHint` to recognize sheet-only operations that imply a target sheet even when cell bounds are not explicit:
   - `summarize_by_category`
   - `ensure_sheet`
2. When exact bounds are unknown, still emit minimal hint with `sheet` (and optional row/col defaults if needed by UI).
3. Keep existing bound inference for `set_cells`/`set_range`.

Acceptance:

1. TC-131-like edit opens on `Annual` sheet by default.
2. Existing `set_range` / `set_cells` hint behavior remains unchanged.

### Phase D — Harden review preview behavior for draft-only targets

Files:

- `app/lib/screens/review_screen.dart`

Tasks:

1. `_loadSlidePreview`:
   - Treat published slide fetch failure for target index as non-fatal.
   - Keep draft preview visible; set published preview pane to empty state if unavailable.
2. `_loadXlsxPreview`:
   - Avoid failing entire load when published target sheet is missing.
   - Keep draft grid visible; published grid can be empty with soft state.
3. Keep fallback structured-diff behavior for renderer failures unchanged.

Acceptance:

1. Added-slide / draft-only sheet scenarios do not show hard preview error.
2. User can review draft target even when published target is absent.

## Test Plan

### Engine tests (Go)

Files likely touched:

- `engine/internal/engine/review_content_diff_test.go`
- `engine/internal/engine/engine_m1_test.go`
- `engine/internal/engine/engine_m21_test.go` (or new targeted test file)

Add/update tests:

1. Update PPTX hint expectation:
   - Remove/replace old assumption that add-slide fallback is index 0 (`review_content_diff_test.go` currently expects this).
2. Add XLSX focus-hint test for `summarize_by_category` producing sheet target.
3. Add `WorkshopRunAgent` path test:
   - Scripted tool-call response performs office operation.
   - Assert `ReviewGetChangeSet` includes persisted `focus_hint` for changed file.
4. Add test for multi-tool same-path overwrite to validate deterministic hint selection.

### Flutter tests

Files likely touched:

- `app/test/workbench_review_checkpoint_test.dart`

Add/update tests:

1. For xlsx review with provided focus hint sheet, assert first `ReviewGetXlsxPreviewGrid` request uses that sheet.
2. For pptx review with focus slide, assert first `ReviewGetPptxPreviewSlide` request uses that index.
3. Add cases where published target fails but draft preview still renders without global preview error.

### Manual verification

Use:

- `docs/test/sections/12-file-type-operations.md` (TC-131, TC-150)

Verify:

1. TC-150 opens on newly added slide by default.
2. TC-131 opens on `Annual` sheet by default.
3. No hard preview break when published target is missing.

## Risks and Mitigations

1. Risk: storing hints from tool calls could overwrite useful proposal-path hints.
   - Mitigation: only persist in run context that executed those tool calls; deterministic last-write policy by file.
2. Risk: published-target soft failures might hide real errors.
   - Mitigation: only soften known target-missing validation/read failures; keep unexpected errors surfaced.
3. Risk: index conventions (`index` vs `slide_index`) drift.
   - Mitigation: continue alias normalization in tool handler (`normalizePptxOperationAliases`), and centralize hint derivation logic.

## Exit Criteria

1. `ReviewGetChangeSet` returns `focus_hint` for agent-driven PPTX/XLSX edits in affected scenarios.
2. TC-150 and TC-131 default to changed target in Review without manual navigation.
3. Unit/widget tests cover the new behavior and pass.
4. No regressions in existing review fallback tests for DOCX/PPTX.
