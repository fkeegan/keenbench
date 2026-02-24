# Implementation Plan: Product-Model-Fit Feedback

## Status
Planned (2026-02-22)

## Summary

Add an opt-in model-feedback capability so KeenBench can treat the model as an internal customer of the toolset.

When `KEENBENCH_MODEL_FEEDBACK=1` is enabled, `WorkshopRunAgent` records:
- objective run telemetry (phase completion, tool usage/failures, elapsed time)
- model-written feedback collected with a post-run markdown survey

Runtime records are written per workbench under metadata, and a manual export command generates curated intake docs in `docs/issues/model-feedback/`.

## Scope

- In scope:
  - Feature flag for model feedback collection
  - Post-`WorkshopRunAgent` feedback collection and telemetry persistence
  - Runtime local storage under `meta/workshop/model_feedback/`
  - Manual export command to generate docs under `docs/issues/model-feedback/`
  - PRD + design docs for the new capability
- Out of scope:
  - Automatic issue/ticket creation
  - Runtime writes directly into tracked repo docs
  - UI controls for feedback mode (v1 is env-flag driven)
  - Collection from compatibility flow (`WorkshopStreamAssistantReply`) in v1

## Deliverables

1. Engine runtime collection
   - `engine/internal/engine/model_feedback.go`
   - `WorkshopRunAgent` hook in `engine/internal/engine/engine.go`
2. Export command
   - `engine/cmd/keenbench-model-feedback-export/main.go`
   - `Makefile` target: `feedback-intake`
3. Documentation
   - `docs/prd/capabilities/model-feedback.md`
   - `docs/design/capabilities/model-feedback.md`
   - `docs/issues/model-feedback/README.md`
   - index updates in `docs/prd/README.md` and `docs/design/README.md`
   - cross-links in Workshop PRD/design docs
4. Tests
   - parser, telemetry, and export behavior unit coverage
   - manual real-model test cases in `docs/test/test-plan.md`

## Implementation Details

### 1) Feature Flag and Hook

- Gate with `KEENBENCH_MODEL_FEEDBACK`.
- Hook at end of `WorkshopRunAgent` using deferred collection so failed runs are also recorded.
- Do not fail the primary user run when feedback collection fails.

### 2) Runtime Record Format

Per run, write:
- markdown record file:
  - `workbenches/<wb>/meta/workshop/model_feedback/<record_id>.md`
- index append entry:
  - `workbenches/<wb>/meta/workshop/model_feedback/index.jsonl`

Collection status values:
- `collected`
- `model_call_failed`
- `parse_failed`
- `skipped`

### 3) Feedback Survey Prompt

Collect markdown with these exact sections:
- `## What slowed me down`
- `## Hardest tool interaction`
- `## Highest-impact tooling change`
- `## Confidence (1-5)`
- `## Additional notes`

Parser requirements:
- first three sections required
- confidence normalized to integer `1..5` (default `3`)
- bounded section lengths with truncation marker

### 4) Telemetry

Capture at minimum:
- provider/model ids
- run id and timestamps
- phase completion flags (research/plan/implement/summary)
- tool call totals, failures, elapsed ms, per-tool counts
- draft presence and summary message id
- run error code/phase/subphase/detail if run failed

### 5) Manual Export Command

`keenbench-model-feedback-export`:
- reads runtime `index.jsonl` entries
- reads corresponding runtime markdown records
- writes curated intake docs to `docs/issues/model-feedback/<YYYY-MM-DD>/`
- dedupes by `source_record_id` unless `--force`

CLI flags:
- `--out`
- `--data-dir`
- `--since YYYY-MM-DD`
- `--max N`
- `--force`

## Testing Plan

### Unit tests

- Markdown parser behavior (valid, missing headings, confidence coercion, truncation)
- Tool metrics aggregation from synthetic tool logs
- Runtime record/index writing
- Export command filtering and dedupe behavior

### Manual real-model tests

Add cases to `docs/test/test-plan.md`:
- flag off: no feedback artifacts
- flag on: runtime record and index created after `WorkshopRunAgent`
- parse/model failure still produces status-bearing runtime record
- export command writes curated docs under `docs/issues/model-feedback/`

## Acceptance Criteria

- Flag off: no behavior change
- Flag on: each `WorkshopRunAgent` invocation writes one runtime feedback record
- Feedback collection failures never block workshop completion
- Manual export command produces stable intake docs in `docs/issues/model-feedback/`
- PRD and design docs are added and linked from indexes
