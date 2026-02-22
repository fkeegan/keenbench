# Design: Model Feedback (Product-Model-Fit)

## Status
Draft

## Version
v0.1

## Last Updated
2026-02-22

## PRD References
- `docs/prd/capabilities/model-feedback.md`
- Related:
  - `docs/prd/capabilities/workshop.md`
  - `docs/design/capabilities/workshop.md`
  - `docs/design/capabilities/security-egress.md`

## Summary
Model Feedback is an opt-in engine capability that captures post-run model feedback and objective execution telemetry for the RPI workshop flow.

The feature runs behind `KEENBENCH_MODEL_FEEDBACK=1`, writes runtime records under workbench metadata, and exposes a manual export command that generates triage-ready markdown docs in `docs/issues/model-feedback/`.

## Architecture

### Trigger Point
- Hook: `Engine.WorkshopRunAgent`.
- Mechanism: deferred collection block so failed runs still produce runtime artifacts.
- Scope: RPI workflow only (`WorkshopRunAgent`).

### Collection Pipeline
1. Build base record context (run id, model/provider, timestamps, phase completion state).
2. Aggregate tool metrics from tool log sequence window (`tool_log.jsonl`).
3. If run reached Summary successfully, request model survey response.
4. Parse markdown survey sections with tolerant normalization.
5. Persist markdown record + append index entry regardless of collection status.

### Status Model
- `collected`: survey call and parse succeeded.
- `model_call_failed`: survey model call failed.
- `parse_failed`: survey call succeeded but markdown parse failed.
- `skipped`: no survey attempted (for example run failed before summary).

## Data Model

### Runtime Record Path
`workbenches/<workbench_id>/meta/workshop/model_feedback/<record_id>.md`

### Runtime Index Path
`workbenches/<workbench_id>/meta/workshop/model_feedback/index.jsonl`

### Index Fields (minimum)
- `record_id`, `record_path`, `run_id`, `workbench_id`
- `provider_id`, `model_id`
- `collection_status`, `collected_at`
- run timing and tool metrics
- phase completion map
- optional run/model/parse error strings

## Survey Prompt Contract

The engine requests markdown with exact H2 headings:
- `## What slowed me down`
- `## Hardest tool interaction`
- `## Highest-impact tooling change`
- `## Confidence (1-5)`
- `## Additional notes`

Parser behavior:
- required: first three sections
- optional: additional notes
- confidence coerced to integer `1..5`, default `3`
- section length bounded with truncation marker

## Export Command

### Binary
`engine/cmd/keenbench-model-feedback-export`

### Inputs
- `--out` target directory (required)
- `--data-dir` optional override
- `--since` optional date filter
- `--max` optional count limit
- `--force` bypass dedupe

### Output
Curated docs in:
`docs/issues/model-feedback/<YYYY-MM-DD>/`

Each exported doc includes:
- source runtime record ids and metadata
- intake candidate summary
- embedded runtime record content

Dedupe strategy:
- by `source_record_id` found in existing exported docs unless `--force`.

## Safety / Privacy

- Uses existing provider egress path only.
- Stores artifacts locally in workbench metadata.
- Avoids writing secrets or API keys.
- Never blocks user-facing workshop run on feedback collection failures.

## Testing Strategy

### Unit
- Markdown parser (required sections, confidence normalization, truncation)
- Tool metrics aggregation from synthetic tool log lines
- Runtime record/index writing
- Export command filtering and dedupe

### Manual Real-Model
- Validate feature-flag behavior and runtime record creation with live provider keys.
- Validate export command output into `docs/issues/model-feedback/`.
