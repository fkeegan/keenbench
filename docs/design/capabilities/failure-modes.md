# Design: Failure Modes & Recovery

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/failure-modes.md`
- `docs/prd/keenbench-prd.md` (Failure Modes & Recovery; SR4/SR5)
- `docs/prd/milestones/v1.md` (network interruption + resume)
- Related:
  - `docs/design/capabilities/workshop.md`
  - `docs/design/capabilities/workbench.md`
  - `docs/design/capabilities/draft-publish.md`
  - `docs/design/capabilities/checkpoints.md`
  - `docs/design/capabilities/security-egress.md`
  - `docs/design/adr/ADR-0005-file-operation-batches-as-atomic-unit.md`
  - `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`

## Summary
Failure Modes & Recovery defines the cross-cutting reliability contract of KeenBench:
- failures are **classified**, **logged**, and **actionable**,
- **Published is never modified on failure**,
- **Draft is preserved by default** (unless the user discards it),
- long-running operations stop/resume at **safe boundaries**, and
- users always receive a **status report** with partial results when available.

Key v1 choices (confirmed):
- Auto-retry transient provider errors **3 times with exponential backoff**, then pause and require explicit user action (retry / switch model / settings / restore).
- Checkpoint creation is a hard precondition for actions that would otherwise risk irrecoverable state (Publish, Restore).
- Error classification uses stable **error codes + phase** (captured in ADR-0006) so the UI never relies on string matching.

## Goals / Non-Goals

### Goals
- Make failures predictable: users should know what happened and what to do next.
- Preserve safety invariants: Published is never corrupted; Draft remains reviewable.
- Provide recoverability: retry, resume, restore checkpoint, or safely stop with partial results.
- Provide auditable artifacts: phase, models, steps/batches, files touched, checkpoints, estimate vs actual usage.
- Fail closed on safety boundaries (scope, consent, sandbox), with clear user messaging.

### Non-Goals
- Full OS crash recovery guarantees (power loss, OS kill at any time) beyond best-effort reconciliation using transaction markers (see Checkpoints/Draft-Publish docs).
- Data recovery after disk failure.
- Automatic provider/model switching without explicit user action (v1).
- Remote support log upload, background telemetry, or prompt/content capture by default.

## User Experience

### Global Error Surface
All workbench-scoped screens should share a consistent error surface:
- A non-blocking error banner/toast for recoverable issues (e.g., transient provider error after retries).
- A blocking modal only when continuing would violate safety invariants (e.g., disk full prevents Draft writes; consent required before egress).

Error UI includes:
- **Short title** (human-readable)
- **What happened** (1–2 sentences)
- **Primary action** (most likely next step)
- **Secondary actions** (switch model, restore checkpoint, view report)
- A “Details” disclosure that shows:
  - phase (`workbench`, `workshop`, `publish`, `restore`, …)
  - error code (stable)
  - provider/model involved (if any)
  - timestamp

### Workshop Failures
Workshop failures are inline and conversational:
- Streaming failure: show an inline “Message failed” state with actions:
  - **Retry** (re-issue the same request from the same conversation head)
  - **Switch model** (opens model selector; then retry)
  - **Cancel** (dismisses the failed attempt; conversation remains unchanged)
- Draft generation/apply failure: no Draft is created. The assistant responds with a clear limitation and suggested alternative.

When context compression occurs during a Workshop turn, append a non-blocking system event in the transcript:
> “Context compressed to stay within model limits (older messages summarized).”

### Publish / Restore Failures
- Publish failure: UI shows “Publish failed” with:
  - confirmation that Published remains unchanged (or was restored),
  - Draft preserved,
  - actions: **Retry publish**, **Review Draft**, **Restore checkpoint** (if relevant), **Discard Draft**.
- Restore failure: UI shows “Restore failed” with:
  - best-effort rollback result,
  - action: **Retry restore** or restore to the latest “pre-restore” checkpoint.

### Cancellation & Partial Results
Cancellation is always safe:
- Workshop: cancel stops streaming or draft generation; no file mutations occur unless a Draft was already created.

## Architecture

### UI Responsibilities (Flutter)
- Present consistent error states and actions across Workshop, Review, Publish, Restore.
- Keep failure actions "shallow" and obvious:
  - Retry, Switch model/provider, Settings, Restore checkpoint, View report.
- Provide accessible error UX (focus management, screen reader announcements).

### Engine Responsibilities (Go)
- Classify failures with stable codes and phases (ADR-0006).
- Implement retry policy and pause behavior after exhaustion.
- Enforce invariants:
  - never mutate Published on job/execution failure,
  - keep Draft consistent via atomic writes and batch boundaries.
- Persist sufficient state to resume safely:
  - Workshop: conversation head + stored draft changes artifacts (proposal files) + draft revisions (for undo).
- Produce reports and audit artifacts that can be rendered offline (no model calls needed to view failure details).

### IPC / API Surface
Failure handling is surfaced via two layers:
- **Structured errors** in JSON-RPC responses/notifications (ADR-0006).
- **Explicit workbench status APIs** for UI-driven resume/retry.

Illustrative additions (beyond per-capability APIs):

**Commands**
- `EngineGetLastError(workbench_id) -> {error?}` (optional convenience)

Notes:
- "Retry" is modeled as a UI-driven operation from the last safe point, not a hidden background loop.

## Data & Storage

### Failure Artifacts (Workshop)
Workshop failures are recorded as structured system events:
- `meta/conversation.jsonl`:
  - `system_event` entries for:
    - `workshop_stream_failed`
    - `proposal_failed` (internal event name for draft generation failures)
    - `apply_failed`
    - `context_compressed`
  - These events include stable error codes and are rendered in the transcript.

For deterministic "apply" diagnostics:
- `meta/workshop/proposals/<proposal_id>/apply_attempts.jsonl`
  - append-only attempts with timestamps and outcomes

### Workbench-Level Failures
Some failures are not job-scoped (add files, manifest corruption, disk pressure):
- Record a short, user-visible event in a workbench-local log:
  - `meta/workbench_events.jsonl`

## Algorithms / Logic

### Retry Policy (v1)
Auto-retry only when:
- the error is classified as transient (`retryable=true`), and
- repeating the exact call is safe (idempotent at the API boundary).

Default: 3 attempts with exponential backoff:
- attempt 1: immediate retry after a short delay
- attempt 2: ~1–2 seconds
- attempt 3: ~4–8 seconds
- respect provider `Retry-After` headers when present

After retries are exhausted:
- stop further progress,
- persist state and a failure event,
- return control to the user (pause with explicit next actions).

### Resume Semantics
Resume always starts from the last safe boundary, not "mid-request".

Resume is provider-agnostic:
- switching provider/model changes *future* calls but does not change cursor semantics,
- the engine resumes from the same boundary regardless of which provider is used next (subject to consent and model availability).

Workshop:
- Retry regenerates from the same `conversation_head_id`.
- Auto-apply retry re-validates the changeset against the current active view; if mismatch is detected, the engine blocks apply and asks the user to re-generate draft changes.

### Disk Pressure Handling
Write-path failures (disk full, permission) must:
- stop further Draft writes immediately,
- preserve Draft consistency (no partially-written files),
- surface a single actionable error (“Free disk space to continue”).

Best-effort preflight:
- check available free space before large operations (publish, checkpoint, large apply),
- but treat preflight as advisory (don’t rely on it exclusively).

## Error Handling & Recovery (Selected Cases)

### Provider / Model Errors
- Invalid key: fail fast, no retries; action: Settings.
- Rate limit / timeout: retry with backoff; if exhausted, pause; action: Retry / Switch model.
- Provider outage: retry; if exhausted, pause; action: Retry / Switch provider/model.

### Network Interruption
- Preserve conversation/job state up to last completed request/batch.
- Offer Resume; do not attempt background reconnection loops that could surprise users.

### Context Exhaustion
- Workshop: apply context compression (summarize older messages into `summary_message` entries) and emit a non-blocking `context_compressed` system event in the transcript. If the request still exceeds context limits after compression, fail the turn with a structured `CONTEXT_EXHAUSTED` error and guide the user to reduce scope (e.g., select fewer files, ask for a narrower change, or restart from a summary).

### File I/O Errors
- File read/parse failure: list affected files; continue when safe (e.g., skip a preview) and log `file_read_failed`.
- Manifest corruption: attempt rebuild; if not possible, recommend restore from checkpoint.

## Security & Privacy
- In production, errors and reports exclude raw file contents and raw prompts by default. In debug mode, raw prompt/file excerpts may be logged for development triage and must be clearly labeled and off by default.
- Sensitive values are always redacted (API keys, auth headers).
- Failures never expand file scope or bypass egress/consent controls; “recovery” cannot introduce new providers/scope without re-consent (see `docs/design/capabilities/security-egress.md`).

## Telemetry (If Any)
v1: none.

Local-only debug logs (optional):
- retry counts by provider
- common failure codes
- disk-full incidence (without paths/contents)

## Open Questions
- None currently.

## Self-Review (Design Sanity)
- Aligns with PRD guarantees: Published never modified on failure; Draft preserved by default; failures always produce a status report.
- Keeps user control explicit: auto-retry is bounded, and post-retry behavior is “pause + ask,” not auto-switching providers/models.
- Reuses existing one-way decisions (ADR-0005 atomic batches; checkpoint store) instead of inventing new stop/resume mechanics.
- Ensures offline reviewability: reports and error details are stored locally and don’t require new model calls to understand what happened.
