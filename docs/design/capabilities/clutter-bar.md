# Design: Clutter Bar (Context Health Indicator)

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/clutter-bar.md`
- `docs/prd/keenbench-prd.md` (Clutter Bar; Token & Context Management)
- Related:
  - `docs/design/capabilities/workbench.md` (Workbench chrome + always-visible UI)
  - `docs/design/capabilities/workshop.md` (conversation + context compression)
  - `docs/design/capabilities/multi-model.md` (per-model context window estimates)
  - `docs/design/capabilities/accessibility.md` (non-color indicators)

## Summary
The Clutter Bar is a model-aware meter that communicates "context pressure" without exposing tokens. It is visible across Workbench UI surfaces (Workshop, Review/Diff, Checkpoints) and helps users build intuition about when performance may degrade as:
- more/larger files enter the Workbench, and
- Workshop conversation history grows.

Key v1 choices (confirmed):
- The bar is **always visible** in Workbench chrome and is **informational only** (never blocks actions).
- It remains **simple** (not clickable; no breakdown UI).
- Calculation is **model-aware** using `context_tokens_estimate` from the supported model registry.
- Thresholds are **static in v1** (no telemetry calibration).
- When the engine performs context compression, Workshop shows a non-blocking transcript system event.

## Goals / Non-Goals

### Goals
- Make context pressure legible without jargon or configuration.
- Update in real time as scope changes (files, conversation, model switch).
- Provide a consistent, cross-platform UI element across Workbench UI surfaces.
- Support context management strategies (summarization/compression) without interrupting users.

### Non-Goals
- Displaying token counts or “remaining tokens”.
- Offering remediation suggestions or blocking actions in v1.
- User-tunable thresholds or per-model tuning in v1.
- Telemetry-driven calibration in v1 (deferred to v2+).

## User Experience

### Placement & Visibility
The Clutter Bar appears in the Workbench chrome and remains visible across:
- Workshop
- Review/Diff
- Checkpoints

(Not shown in global screens like Settings.)

### Visual Design
v1 is intentionally simple:
- A horizontal meter with fill percentage.
- A label: `Light` / `Moderate` / `Heavy` (always visible).
- Color may reinforce state (green/yellow/red), but **text labels are required** (see accessibility).

When `Heavy`:
- show a short warning near the bar:
  - “Workbench is cluttered — performance may be degraded.”

### Behavior
Updates occur when:
- files are added/removed/replaced,
- conversation grows (Workshop),
- the active model changes.

The bar never blocks sending a message or starting a job.

### Context Compression Visibility
Context compression is **transparent to users**—they see context pressure via the Clutter Bar, not compression mechanics. When the engine compresses context during Workshop:
- The bar may decrease if compression reduces estimated context usage.
- A non-blocking transcript system event is appended (e.g., “Context compressed to stay within model limits (older messages summarized).”). No modal or prompt is shown.

## Architecture

### UI Responsibilities (Flutter)
- Render the bar (meter + label + optional warning).
- Subscribe to engine updates and rerender quickly.
- Ensure accessible semantics:
  - state label announced by screen readers,
  - warning text announced when it appears.

### Engine Responsibilities (Go)
- Own the clutter score calculation so logic is consistent across platforms.
- Recompute score on relevant triggers and emit updates:
  - file changes, conversation append/rewind, model switch.
- Apply context compression strategies when approaching limits:
  - summarize older Workshop messages (and/or summarize/exclude lower-relevance file content).
- When compression occurs in Workshop, write a transcript system event and emit the conversation update event.

### IPC / API Surface
The Workbench doc already proposes a clutter event; Clutter Bar formalizes it.

**Commands**
- `WorkbenchGetClutter(workbench_id) -> {score, level, model_id}` (optional convenience)

**Events**
- `WorkbenchClutterChanged(workbench_id, {score, level, model_id})`

Notes:
- `score` is a normalized `0.0–1.0` ratio of estimated context usage.
- `level` is derived from the static thresholds (Light/Moderate/Heavy).

## Data & Storage
- Clutter score is primarily computed on demand and does not need durable persistence.
- For fast “re-open Workbench” UX, the engine may cache the last computed value in:
  - `meta/workbench.json` (optional), or
  - keep it purely in memory and compute on open.

Workshop context compression artifacts are already defined in `docs/design/capabilities/workshop.md`:
- `meta/conversation.jsonl` includes `summary_message` entries and a `system_event` when compression occurs.

## Algorithms / Logic

### Inputs
- File manifest:
  - file count
  - file sizes and types (supported vs opaque)
- Workshop conversation history (up to `conversation_head_id`)
- Prompt-injected context payload estimate (situation + injected Agent Skills + injected format style skills, including merged style skills when present)
- Active model `context_tokens_estimate` (from `docs/design/capabilities/multi-model.md`)

### Token Estimation (Best Effort)
v1 uses a coarse token estimator to avoid heavyweight tokenization dependencies:
- **Text heuristic**: `estimated_tokens ≈ bytes / 4` (UTF-8 typical)
- **Conversation heuristic**: `estimated_tokens ≈ chars / 4` (conservative)

The point is stable relative measurement, not exact token accounting.

### Weighting Model (v1)
Compute an estimated “context usage”:
```
usage =
  file_count_weight +
  sum(file_weight(file_i)) +
  conversation_weight +
  context_items_weight
score = clamp(usage / model_context_estimate, 0, 1)
```

Defaults (align with PRD’s simplified formula):
- `file_count_weight`: `500 * file_count`
- `conversation_weight`: estimated tokens from conversation (summaries + recent messages)
- `context_items_weight`: estimated tokens for prompt-injected context artifacts (see `docs/design/capabilities/workbench-context.md`, Clutter Weight Calculation)

`file_weight(file)` depends on type and size class:
- Text (including code): size-tiered weighting:
  - Small (<10 KB): `bytes/4` (low overhead)
  - Medium (10–100 KB): `bytes/4` (moderate overhead)
  - Large (>100 KB): `min(large_text_tokens_cap, bytes/4)` (capped to avoid dominating context)
- Docx/PDF: `base_doc_tokens + bytes/6` (extraction overhead)
- XLSX/PPTX: `base_office_tokens + bytes/8` (higher overhead, capped)
- Images: `base_image_tokens` (fixed overhead; content depends on vision prompting)
- Opaque/binary: `base_opaque_tokens` (metadata-only)

Suggested v1 constants (best-guess starting points):
- `large_text_tokens_cap = 25_000` per file (~100 KB)
- `base_doc_tokens = 4_000`
- `base_office_tokens = 6_000`
- `base_image_tokens = 4_000` (Medium: comparable to document overhead)
- `base_opaque_tokens = 500`

These are intentionally adjustable in v1 without UX changes.

### Thresholds (v1)
Static thresholds per PRD:
- Light: `< 0.40`
- Moderate: `0.40–0.70`
- Heavy: `> 0.70`

### Model Context Estimate Fallback
If `context_tokens_estimate` is missing for the active model:
- use a conservative default (e.g., `32_000`) and mark the meter as "approximate" internally.

Note: This fallback is unlikely to be exercised in practice since v1 supports only known flagship models with documented context windows. The 32K default exists purely as a safety net for edge cases.

### Context Compression Trigger
When the engine predicts the next turn’s context would exceed safe limits:
- perform context compression, then recompute `score`.

v1 policy:
- Trigger compression when `score > 0.85` (or when a turn’s assembled prompt would exceed the model estimate).
- Prefer summarizing older conversation first; keep the most recent user/assistant turns verbatim.

## Error Handling & Recovery
- Clutter calculation fails: show the bar in an `Unavailable` state (or hide it) and continue; do not block.
- Model context estimate unavailable: use conservative default.
- Conversation log corruption: treat as `conversation_weight=0` and surface a recoverable warning; do not block file operations.

## Security & Privacy
- Clutter calculation is local only.
- No file contents are transmitted for clutter estimation.
- Compression summaries are stored locally in Workbench metadata and are covered by Workbench sandbox rules.

## Telemetry (If Any)
v1: none.

Future (v2+):
- opt-in calibration based on “clutter heavy” correlation with degraded job outcomes.

## Open Questions
- None currently.

## Self-Review (Design Sanity)
- Matches PRD scope: visible across Workbench UI surfaces; model-aware, informational, static thresholds, transparent compression.
- Keeps v1 UX simple (no breakdowns) while still providing a stable engine-side model for future tuning.
- Uses conservative estimation and graceful fallbacks so the bar never becomes a blocker.
