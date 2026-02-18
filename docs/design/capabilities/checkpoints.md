# Design: Checkpoints (Hidden Versioning)

## Status
Draft (v1)

## PRD References
- `docs/prd/keenbench-prd.md` (FR4; “Hidden Versioning” concept)
- Related:
  - `docs/prd/capabilities/draft-publish.md` (checkpoint + restore requirements)
  - `docs/prd/capabilities/failure-modes.md`
  - `docs/design/capabilities/failure-modes.md`

## Summary
Checkpoints are the user-facing representation of “hidden versioning”: restorable snapshots of the Workbench’s **Published** state. In v1, restores rewind both:
- Published files, and
- Workbench “history” that affects future context (conversation + job records),
to avoid mismatches between what the UI/model sees and what the filesystem contains.

v1 supports **auto checkpoints** and a minimal **manual “Create checkpoint”** action (optional but included per PRD recommendation). The UI avoids Git concepts; the engine implements correctness-first snapshot + restore with clear audit trail entries.
Publish checkpoints are also surfaced inline in Workshop chat as convenience cards, while the Checkpoints screen remains the authoritative full-history/manual-action surface.

This doc defines checkpoint UX, metadata, restore semantics, and retention. The underlying storage mechanism choice (snapshot store vs Git-backed) is captured in an ADR.

## Goals / Non-Goals
### Goals
- Make restore reliable, understandable, and safe for non-technical users.
- Auto-checkpoint at required moments (publish; restore; destructive Published operations) to reduce fear.
- Preserve auditability: checkpoint creation/restores are recorded.
- Provide reasonable storage growth controls (retention policy).

### Non-Goals
- Exposing branching/merging semantics in v1 UI.
- Cross-Workbench restore or export/archive.
- Remote backup/sync (future).

## User Experience

### Checkpoint Timeline
Inside a Workbench, users can open “Checkpoints” (canonical full history) and see a chronological list:
- Timestamp
- Short description
  - Manual checkpoints: user-entered (optional)
  - Auto/publish/restore checkpoints: engine-generated
- Reason badge (Auto / Manual / Publish / Restore)
Description guidelines (v1):
- Must be meaningful without internal IDs.
- Examples:
  - Auto: "Before publish"
  - Publish: "Before publish"
  - Restore: "Before restore to <timestamp>"

### Auto-Checkpoint Moments (v1)
Note: **Checkpoints** snapshot **Published** state and are distinct from **Draft Revisions** (Workshop-internal snapshots of Draft state for message-level undo). Workshop is interactive; approving draft changes does not mutate Published and already has message-level undo + Draft discard, so it does not require automatic checkpointing in v1.

Minimum set (per PRDs and safety invariants):
- **On publish**: create a checkpoint of the current Published **before** applying Draft changes (so "undo publish" and rollback are always possible)
- **Before restore**: create a "pre-restore" checkpoint of the current state (so restore is undoable)

Recommended additional safety checkpoints:
- Before destructive user actions on Published (e.g., removing a file when no Draft exists)
- (Optional) Before Workshop publish when the user wants persistent, long-lived restore points (use manual checkpoint in v1)

### Manual Checkpoint (v1 Decision)
Included in v1 (minimal) to match PRD guidance (“optional in v1, recommended”):
- Entry points:
  - Checkpoints screen: **Create checkpoint**
  - (Optional) Workbench overflow menu: **Create checkpoint**
- Behavior:
  - Prompts for an optional description (single line; e.g., “Before trying a risky change”)
  - Disabled while a Draft exists (to avoid confusion about Draft vs Published)
  - Creates a snapshot of Published + selected Workbench metadata (same snapshot set as auto checkpoints)

### Restore Flow
1. User selects a checkpoint.
2. UI shows a confirmation modal:
   - “Restoring will revert files and Workbench history to <timestamp>.”
   - Explicit note: “Current Draft (if any) must be discarded first.”
3. User confirms restore.
4. UI shows progress and returns user to Workbench with a “Restored” banner.

### Workshop Inline Publish Checkpoints
- After publish, Workshop chat shows a system card for that publish checkpoint with timestamp/description and a **Restore** action.
- These cards are a shortcut for recent publish checkpoints only; they do not replace the Checkpoints screen timeline.
- Restore from chat uses the same confirmation and restore pipeline as Checkpoints screen restore.

### When a Draft Exists
Restore (and manual checkpoint creation) should be blocked while a Draft exists (v1), because Published vs Draft semantics become ambiguous.
- UI behavior: disable Restore with a tooltip “Discard or Publish your Draft to restore a checkpoint.” This applies to both Checkpoints screen and Workshop chat cards.

### Accessibility
- Checkpoint list is keyboard navigable; each checkpoint row has a readable label (timestamp + description).
- Restore confirmation uses explicit text describing impact; focus management returns to the restored checkpoint row.

## Architecture

### UI Responsibilities (Flutter)
- Render checkpoint list + details panel as the full-history checkpoint surface.
- Provide “Create checkpoint” UI for manual checkpoints (optional description prompt).
- Provide restore confirmations and progress UI.
- Disable restore/create actions when Draft exists; guide user to publish/discard first.
- Render Workshop inline publish checkpoint cards (from conversation system events) with the same restore gating.

### Engine Responsibilities (Go)
- Create checkpoint snapshots of Published + context-relevant metadata at required moments.
- Maintain checkpoint metadata and retention policy.
- Restore Published + context-relevant metadata from checkpoint safely and recoverably.
- Record audit trail entries for checkpoint creation/restores.
- Validate checkpoint integrity before restore (best effort).
- Emit checkpoint metadata into Workshop conversation `system_event` entries for publish and restore visibility in chat.

### IPC / API Surface

**Commands**
- `CheckpointsList(workbench_id) -> {checkpoints[]}`
- `CheckpointCreate(workbench_id, {description?, reason}) -> {checkpoint_id}`
- `CheckpointRestore(workbench_id, checkpoint_id) -> {}`
- `CheckpointGet(workbench_id, checkpoint_id) -> {metadata}`

`CheckpointCreate.reason` (v1):
- UI may request: `manual`
- Engine emits/records as needed: `auto|publish|pre_restore`

**Events**
- `CheckpointCreated(workbench_id, {checkpoint_id, reason})`
- `CheckpointRestoreProgress(workbench_id, {phase, percent?})`
- `CheckpointRestored(workbench_id, {checkpoint_id})`
- Workshop conversation log may include checkpoint `system_event` entries (`publish_checkpoint`, `checkpoint_restored`) for inline chat rendering.

## Data & Storage

### Metadata
`meta/checkpoints/<checkpoint_id>.json` (illustrative fields):
- `checkpoint_id`
- `created_at`
- `reason`: `auto|manual|publish|pre_restore`
- `description` (optional; user-entered for manual; engine-generated otherwise)
- `job_id` (optional; reserved for future job linkage)
- `published_generation` (optional integrity hint)
- `stats`: `{files, total_bytes}` (best effort; used for retention decisions and UI)

### Snapshot Data
`meta/checkpoints/<checkpoint_id>/published_snapshot/...`
- Stores a snapshot of the Published tree as of `created_at`.

`meta/checkpoints/<checkpoint_id>/meta_snapshot/...`
- Stores a snapshot of context-relevant metadata, excluding the checkpoint store itself.
- Recommended contents:
  - `workbench.json`
  - `files.json`
  - `conversation.jsonl`
  - `jobs/` (job specs/plans/reports up to the checkpoint time)

Implementation details (copy vs hardlink vs reflink) are in ADR.

### Retention Policy (v1 Recommendation)
Because v1 supports up to 10 files × 25MB, storage can still grow quickly.

Recommended retention:
- Always keep the last:
  - 1 “Publish” checkpoint
  - 1 “Pre-restore” checkpoint
  - N “Manual” checkpoints (default N=50)
  - N “Auto” checkpoints (default N=200)

Pruning guidance (best effort, in order):
1. Prune oldest **Auto** checkpoints beyond the limit.
2. If still under pressure, prune oldest **Manual** checkpoints beyond the limit.
3. Never auto-prune the most recent **Publish** and **Pre-restore** checkpoints.

If a checkpoint still cannot be created after pruning, the engine must fail safely and block the action that required the checkpoint.

## Algorithms / Logic

### Snapshot Creation (Conceptual)
1. Acquire a workbench-level lock (prevents concurrent publish/restore/job writes).
2. Validate Published exists and is readable.
3. Create snapshot directories:
   - `meta/checkpoints/<id>/published_snapshot/`
   - `meta/checkpoints/<id>/meta_snapshot/`
4. Materialize snapshots (copy/hardlink/reflink based on platform; see ADR):
   - Copy/link `published/` into `published_snapshot/`
   - Copy/link the selected `meta/` files into `meta_snapshot/` (excluding `meta/checkpoints/`)
5. Write metadata JSON (atomically: write temp + rename).
6. Emit `CheckpointCreated`.

### Restore (Conceptual)
Preconditions:
- No Draft exists (v1).
- Engine can acquire the workbench-level lock.

Restore sequence:
1. Create a “pre-restore” checkpoint of current state (so restore is undoable).
2. Materialize the checkpoint snapshot into staging directories:
   - `published.restore_tmp/`
   - `meta.restore_tmp/` (only the selected meta subset)
3. Write a restore transaction marker (e.g., `meta/restore.json`) describing the target checkpoint and staging paths.
4. Swap directories:
   - Rename `published/` → `published.prev/`
   - Rename `published.restore_tmp/` → `published/`
   - Apply meta restore:
     - Move current meta files aside (e.g., `conversation.jsonl.prev`, `jobs.prev/`)
     - Move `meta.restore_tmp/` contents into place
5. Finalize:
   - Delete temporary `.prev` artifacts after verification.
   - Delete the restore transaction marker.
   - Record restore event in audit trail (checkpoint id, timestamp, operator=user).
6. Emit `CheckpointRestored`.

Crash recovery:
- If the engine starts and finds a restore transaction marker or `*.prev`/`*.restore_tmp` artifacts, it must reconcile to a consistent state using the latest “pre-restore” checkpoint.

## Error Handling & Recovery
- **Checkpoint creation fails**: block publish/restore that required it; surface actionable error (disk full, permission).
- **Checkpoint metadata missing/corrupt**: hide that checkpoint from UI and log; never crash the Workbench.
- **Restore fails mid-way**: revert to `published.prev/` if possible; otherwise restore from “pre-restore” checkpoint; keep an audit record of failure.

## Security & Privacy
- Checkpoints contain only Workbench state (Published + selected local metadata). No external data is pulled during snapshot/restore.
- Restore never expands sandbox scope; all paths are validated within the Workbench root.

## Telemetry (If Any)
Local-only by default:
- Count of restores; restore success/failure reasons.
- Checkpoint creation failures (disk/permission).
- Storage growth (bytes in checkpoints) to tune retention defaults.

## Open Questions
- How should “disk pressure” pruning be surfaced to users (silent best-effort prune vs explicit warning vs hard failure)?

## Self-Review (Design Sanity)
- Treats restore as a first-class safety mechanism with “undo restore” via pre-restore checkpoint.
- Keeps Draft semantics simple (restore blocked while Draft exists).
- Identifies storage mechanism as an ADR-level one-way decision.
