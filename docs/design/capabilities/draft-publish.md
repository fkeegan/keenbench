# Design: Draft / Publish

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/draft-publish.md`
- `docs/prd/keenbench-prd.md` (FR2, SR4, SR5; failure modes)
- Related:
  - `docs/prd/capabilities/review-diff.md`
  - `docs/prd/capabilities/workshop.md`
  - `docs/design/capabilities/failure-modes.md`

## Summary
Draft/Publish provides the product’s core safety invariant: **AI never edits Published directly**. All AI-driven file mutations occur in an isolated Draft; users review diffs/summaries and then either publish atomically or discard entirely. v1 enforces a single Draft at a time.

This doc focuses on Draft lifecycle + publish/discard semantics. Checkpoint storage and restore mechanics are covered in `docs/design/capabilities/checkpoints.md`.
Review diff semantics are linked: Review compares Draft against a frozen Draft-start reference so the "what changed" view stays stable throughout the Draft lifecycle.

## Goals / Non-Goals
### Goals
- Preserve a clean separation between **Published (approved)** and **Draft (mutable)**.
- Ensure publish is atomic and crash-safe (or crash-recoverable).
- Make Draft state and actions obvious in the UI.
- Keep v1 simple: one Draft at a time; no forking.

### Non-Goals
- Concurrent Drafts (v1.5).
- Automatic merge/conflict resolution.
- Cross-Workbench merge or publish gates (team approvals are v2+).

## User Experience

### Draft State Indicator
When a Draft exists, the user always sees:
- A banner/status pill: “Draft in progress”
- Review auto-opens when the Draft appears (including Workbench reopen with an existing Draft).
- Actions: **Publish**, **Discard** (with an optional return-to-Review affordance)
- Optional: "Draft created by: Workshop" + timestamp

Workshop input is blocked while a Draft exists (v1: one Draft total). Users must publish or discard before continuing Workshop edits.

### File Operations Blocked While Draft Exists
Users cannot add, remove, or extract files from the Workbench while a Draft is in progress:
- "Add files", "Remove", and "Extract files" actions are disabled with tooltip: "Publish or discard your Draft to modify files."
- Rationale: Prevents confusion about which state (Draft or Published) the files belong to.

### Draft Age Reminder
If a Draft is older than 24 hours, show a gentle reminder on Workbench open:
- "You have an open Draft from [date]. Review and publish or discard?"
- Actions: **Review** / **Discard** / **Dismiss**

### Stale Draft Auto-Cleanup
On Workbench open, if a Draft is older than 7 days with no activity:
- Prompt: "A stale Draft was found (last modified [date]). Discard it?"
- Actions: **Discard** / **Keep**

### Review → Publish/Discard Loop
1. User creates/receives a Draft (Workshop auto-apply).
2. App auto-opens Review to show diffs/summaries.
3. User chooses Publish or Discard.

### Publish Checkpoint Visibility
- After successful publish, Workshop chat shows an inline publish-checkpoint card with a **Restore** action.
- Restore is disabled while a Draft exists; user guidance points to publish/discard first.
- Full checkpoint history/manual checkpoint actions remain in the Checkpoints screen.

### Accessibility
- Draft banner must be reachable by keyboard and announced by screen readers.
- Publish/Discard confirmations must not rely on color only; require explicit button labels.
- After publish/discard, focus returns to a sensible anchor (e.g., file list header).

## Architecture

### UI Responsibilities (Flutter)
- Display Draft state and actions consistently.
- Auto-open Review when Draft appears and when reopening with an existing Draft.
- Provide publish/discard confirmations, including deletion confirmations from review flow.
- Surface publish checkpoint cards in Workshop chat and apply restore-disabled gating while Draft exists.
- Render progress during publish/discard when operations take noticeable time.

### Engine Responsibilities (Go)
- Implement Draft state machine and enforce “Draft-first” rule for AI writes.
- Provide crash-safe publish/discard operations.
- Record audit trail entries for draft creation, publish, discard, and publish failures.
- Coordinate with checkpoint creation (e.g., checkpoint on publish; checkpoint before execution).
- Emit publish checkpoint metadata compatible with Workshop chat timeline rendering.

### IPC / API Surface

**Commands**
- `DraftGetState(workbench_id) -> {has_draft, draft_id, created_at, source, stale_reason?}`
- `DraftCreate(workbench_id, reason) -> {draft_id}` (usually implicit, created on first write)
- `DraftPublish(workbench_id) -> {checkpoint_id, published_at}` (`checkpoint_id` is the publish checkpoint created during the publish sequence)
- `DraftDiscard(workbench_id) -> {}`

**Events**
- `DraftStateChanged(workbench_id, {has_draft, draft_id})`
- `DraftPublishProgress(workbench_id, {phase, percent?})`
- `DraftPublishFailed(workbench_id, {error, recovery_action})`
- `CheckpointCreated(workbench_id, {checkpoint_id, reason=publish, description?})` (consumed by Workshop chat/checkpoint UI)

#### `DraftPublishProgress` phases
The engine emits `DraftPublishProgress` with a stable `phase` enum so the UI can show meaningful (and accessible) progress messaging.

Phases (in order):
- `validating`: acquire lock; validate Draft exists; validate review preconditions; check disk space.
- `conflict_check`: ensure `published/` has not changed since Draft creation (via `published_generation`).
- `checkpoint_pre_publish`: create the **Publish** checkpoint of current Published (rollback/undo safety; “undo publish”).
- `swap_directories`: atomically swap `published/` and `draft/` (via rename).
- `finalizing`: cleanup (`meta/draft.json`, `published.prev/` as applicable) and emit completion events.
- `rolling_back`: best-effort rollback to a consistent Published state after a failure (Draft preserved by default).

## Data & Storage

### State Model
v1 supports a single Draft with three effective states:
- `NO_DRAFT`
- `DRAFT_READY` (draft exists; mutable)
- `PUBLISHING` (transient; used for UI progress and crash recovery)

### On-Disk Representation
- Published files: `published/`
- Draft files (when present): `draft/`
- Draft metadata: `meta/draft.json` (recommended)
  - `draft_id`
  - `created_at`
  - `base_checkpoint_id` (optional; see checkpoints design)
  - `source`: `{kind: workshop|system}`
  - `published_generation` (monotonic integer or hash) for conflict detection
- Review metadata: `meta/review/<draft_id>/...` (recommended)
  - Includes extracted Draft-start diff reference artifacts for office/PDF under `baseline/` (legacy path name retained for compatibility).

## Algorithms / Logic

### Draft Creation
Draft is created lazily on first AI write operation (Workshop auto-apply).

Creation algorithm (conceptual):
1. Ensure no Draft exists (v1).
2. Create `draft/` as a snapshot of `published/` (implementation shared with checkpoint snapshotting).
3. Capture Draft-start review reference artifacts for review diff (for office/PDF extraction paths, store under `meta/review/<draft_id>/baseline/`; legacy directory name retained).
4. Write `meta/draft.json`.
5. Emit `DraftStateChanged`.

Review reference rules:
- Draft-start review reference is immutable for the lifetime of a Draft.
- Preview panes may still render current `published/` as a visual aid; this must be labeled distinctly from the Draft-start reference in Review UI.

### Writes Routing
- AI write operations must target the Draft root only.
- If no Draft exists, engine must create Draft first, then apply writes.
- UI file list uses the “active view” convention: Draft if present, otherwise Published.

### Publish (Atomic + Recoverable)
Publish must ensure “all-or-nothing” from the user’s perspective.

Recommended publish sequence:
1. Validate: Draft exists; review deletion confirmations captured (from review layer); sufficient disk space.
2. Validate conflict: ensure `published/` has not changed since Draft was created (see below).
3. Create a checkpoint of current Published (“pre-publish”) for rollback and history.
4. Atomically swap directories:
   - Rename `published/` → `published.prev/` (temporary)
   - Rename `draft/` → `published/`
5. Finalize:
   - Remove `published.prev/` (or keep briefly until checkpoint is confirmed)
   - Delete `meta/draft.json`
   - Record “publish” in audit trail with new checkpoint id
6. Emit `DraftStateChanged(has_draft=false)` and `CheckpointCreated`.
7. Append/persist a Workshop conversation `system_event` for the publish checkpoint so the chat timeline can render the inline checkpoint card.

If any step fails:
- Roll back to a known-safe state using the pre-publish checkpoint and/or `published.prev/`.
- Keep Draft intact unless the swap succeeded and cleanup is complete.

### Discard
Discard removes Draft without affecting Published:
1. Validate Draft exists.
2. Delete `draft/` (recursive).
3. Delete `meta/draft.json`.
4. Record discard in audit trail (who, when, Draft source/job_id).
5. Emit `DraftStateChanged(has_draft=false)`.

No checkpoint is created on discard since Published is not modified.

### Conflict Detection (Published Changed Outside Draft)
Conflicts should be rare if the Workbench root is app-managed, but must be handled.

v1 strategy:
- When Draft is created, record a lightweight `published_generation` value (e.g., hash of `meta/files.json` + mtimes) in `meta/draft.json`.
- At publish time, recompute and compare.
- If mismatch: block publish with an error and offer:
  - "Discard Draft" (safe)
  - "Restore Published from checkpoint" (if user wants to undo external edits)
- Fork option deferred to v1.5.

## Error Handling & Recovery
- **Publish fails mid-write**: restore Published to pre-publish checkpoint; keep Draft unless swap completed and integrity is assured.
- **Draft missing/corrupt**: block publish; allow discard; suggest restore from checkpoint if published is also suspect.
- **Checkpoint creation fails**: block publish until resolved; show actionable error.
- **Disk full**: pause operations; keep Draft; surface required free space estimate if possible.
- **Crash during publish**: Draft is preserved. On next app launch, user sees Draft still exists and can retry Publish or Discard. No automatic recovery in v1.

## Security & Privacy
- Engine enforces that AI can only write under `draft/` and never under `published/` directly.
- Publish/discard require explicit user initiation from UI.
- All operations remain within the Workbench sandbox boundary.

## Telemetry (If Any)
Local-only by default:
- Count of publishes vs discards.
- Publish failure reasons (disk, conflict, permission).
- Time-to-publish from draft creation.

## Open Questions
~~Should v1 allow user file add/remove while a Draft exists?~~ → **Resolved**: No. File operations (add/remove/extract) blocked while Draft exists.

~~Do we want "auto-cleanup stale draft on open" behaviors?~~ → **Resolved**: Yes. Drafts older than 7 days prompt for cleanup on Workbench open.

## Self-Review (Design Sanity)
- Preserves the core invariant (AI writes only to Draft).
- Publish path is designed to be atomic and recoverable (checkpoint + directory swap).
- Flags the only major v1 complexity (conflict detection) without inventing a merge system.
