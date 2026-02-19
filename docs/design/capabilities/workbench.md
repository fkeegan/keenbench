# Design: Workbench

## Status
Draft (v1)

## Version
v0.3

## Last Updated
2026-02-13

## PRD References
- `docs/prd/capabilities/workbench.md`
- `docs/prd/keenbench-prd.md` (FR1, SR1; file limits; Workbench definition)
- Related:
  - `docs/prd/capabilities/file-operations.md`
  - `docs/prd/capabilities/clutter-bar.md`
  - `docs/prd/capabilities/draft-publish.md`
  - `docs/design/capabilities/file-operations-tabular-text.md`
  - `docs/design/capabilities/clutter-bar.md`

## Summary
The Workbench is the product’s safety boundary: a local, app-managed directory that contains **copies** of user-added files. The engine enforces that all AI read/write happens strictly inside this boundary, and the UI makes the boundary continuously legible.

CSV is treated as first-class tabular text in the Workbench scope. Tabular schema/stats/query/export behavior is defined in file-operations docs.

## Goals / Non-Goals
### Goals
- Provide an explicit, copy-based file add flow (no implicit scanning).
- Enforce v1 file limits when adding files (10 files, 25 MB per file).
- Maintain a flat file list that is always within the Workbench boundary.
- Make "scope" and "copies; originals untouched" obvious in the UI.
- Provide the storage foundation for Draft/Publish and Checkpoints.

### Non-Goals
- Multiple concurrently active Workbenches (v1 is single-workbench focused).
- Automatic ingestion (connectors, background sync, folder watchers).
- Workbench archive export (.zip) (v1).
- ZIP/archive extraction (v1).

## User Experience

### Primary Flow: Create → Add → Work
1. User creates a new Workbench (name).
2. Workbench opens to an empty file list state with clear scope messaging.
3. User adds files via drag/drop or file picker.
4. UI shows added files with type/status; user proceeds into Workshop.

### Key UI Elements
- **Workbench header**: workbench name (editable via context menu) + scope badge ("AI can only access files in this Workbench").
- **Scope notice**: "Workbench contains copies; originals are untouched." (persistent access via Help).
- **File list**: filename-first rows with type/status badges (extension, read-only, opaque) and (when Draft exists) change status. File-size text is not shown in the Workbench list.
- **Workbench chrome actions**: top-bar history icon for Checkpoints and bottom-left gear icon for Settings.
- **Extract action**: each file row has an extract icon that copies that Published file to a user-selected local destination folder.
- **Primary empty state**: "Add files to start working" + supported types + limit reminders. Workshop button disabled until at least 1 file added.
- **Clutter Bar**: visible across Workbench UI surfaces (Workshop, Review/Diff, Checkpoints).

### Flat Structure
Workbench has a flat file structure. Users cannot create subfolders. All files exist at the root level.

### Add File Semantics (Matches PRD)
- **Batch would exceed max file count (10)**: reject the entire batch (no partial add).
- **Oversize file (>25MB)**: skip oversize file(s) and add the rest; list skipped files.
- **Unsupported type**: allow add, mark as "opaque" (metadata-only access; no content extraction).
- **Symlink/shortcut/reparse point**: reject with an explanation ("Links aren't supported. Add the original file instead.").
- **Duplicate filename**: reject with error ("[filename] already exists in this Workbench."). User must rename externally.

### File Operations
- **Add files**: Via drag-drop or file picker. Files are copied into Workbench.
- **Extract files**: Per-file extraction from the file row; copies selected Published file to a user-selected local folder; destination collisions are auto-renamed with numeric suffixes (`file.xlsx`, `file(1).xlsx`, `file(2).xlsx`, ...).
- **Remove files**: Users can remove files with explicit confirmation. Does not affect originals.
- **Rename files**: Not supported. To rename, remove and re-add with the new name.
- **Delete Workbench**: Users can delete a Workbench with explicit confirmation. Originals remain untouched. Block deletion while a Draft exists.
- **File operations blocked during Draft**: When a Draft exists, add/remove/extract/delete are disabled. User must Publish or Discard first (see `docs/design/capabilities/draft-publish.md`).

### When a Draft Exists
The Workbench UI should make Draft state obvious:
- File list shows the Draft view with status chips vs Published: Added/Modified/Deleted.
- **File add/remove/extract operations are disabled** while a Draft exists. User must Publish or Discard first.
- Tooltip on disabled buttons: "Publish or discard your Draft to modify files."

### Accessibility
- Import affordances must be keyboard accessible (e.g., “Add files” button is always reachable).
- File list supports keyboard navigation, multi-select, and screen reader-friendly row summaries.
- Status chips (supported/opaque, A/M/D) must not rely on color alone.

## Architecture

### UI Responsibilities (Flutter)
- Create/open Workbench flows.
- Drag/drop + file picker integration.
- Directory picker integration for extracting Published files.
- Render file list and add results (added vs skipped with reasons).
- Render per-file extraction result summaries (extracted/skipped/failed).
- Keep Workbench scope messaging visible and consistent across screens.
- Display Draft state banner/actions when Draft exists (Review, Publish, Discard).
- Display Clutter Bar and warnings based on engine-provided clutter score (all Workbench surfaces).

### Engine Responsibilities (Go)
- Create and manage Workbench directories and metadata.
- Enforce the Workbench sandbox boundary for all file access.
- Copy added files into the Workbench active view (Published or Draft).
- Copy Published files to user-selected external destinations for extraction.
- Enforce file limits and add semantics.
- Classify file types and “opaque vs supported” status.
- Maintain a Workbench file manifest with stable IDs and per-file metadata.

### IPC / API Surface
API names are illustrative; protocol (JSON-RPC over stdio) is handled in ADR-0003.

**Commands (request/response)**
- `WorkbenchCreate(name) -> {workbench_id}`
- `WorkbenchRename(workbench_id, new_name) -> {}`
- `WorkbenchOpen(workbench_id) -> {workbench}`
- `WorkbenchList() -> {workbenches}` (optional in v1; useful for "recent workbenches")
- `WorkbenchFilesList(workbench_id) -> {files[]}`
- `WorkbenchFilesAdd(workbench_id, source_paths[]) -> {add_results}` (blocked if Draft exists)
- `WorkbenchFilesRemove(workbench_id, workbench_paths[]) -> {remove_results}` (blocked if Draft exists)
- `WorkbenchFilesExtract(workbench_id, destination_dir, workbench_paths[]?) -> {extract_results}` (blocked if Draft exists; source is Published; successful results may include `final_path` for collision-safe naming)
- `WorkbenchDelete(workbench_id) -> {}` (blocked if Draft exists)
- `WorkbenchGetScope(workbench_id) -> {limits, supported_types, sandbox_root}`

**Events**
- `WorkbenchFilesChanged(workbench_id, {added[], removed[], updated[]})`
- `WorkbenchDraftStateChanged(workbench_id, {has_draft, draft_id})`
- `WorkbenchClutterChanged(workbench_id, {score, level, model_id})`

## Data & Storage

### On-Disk Layout (v1)
```
workbenches/<workbench_id>/
  published/               # approved files (read-only for AI)
  draft/                   # present only when Draft exists
  meta/
    workbench.json
    files.json             # file manifest
    conversation.jsonl
    checkpoints/...
```

### Workbench Metadata (Conceptual)
`meta/workbench.json` (illustrative fields)
- `id`, `name`
- `created_at`, `updated_at`
- `limits`: `{max_files: 10, max_file_bytes: 26214400}`
- `default_model_id`
- `has_draft` (or derived from directory existence)

`meta/files.json` stores a stable manifest that supports rename/collision handling without losing identity:
- Stable `file_id` (UUID)
- `display_name` (what user sees)
- `path` (relative path inside the active view)
- `bytes`, `ext`, `mime_guess`
- `is_supported` / `is_opaque`
- Optional: `sha256` (computed lazily; used for integrity and diff acceleration)

### Filename Collisions
Duplicate filenames are blocked at add time:
- Error: "[filename] already exists in this Workbench."
- User must rename the file externally before adding.

## Algorithms / Logic

### Add Files Algorithm (High-Level)
1. Validate: no Draft exists (block if Draft present).
2. Validate source paths are user-selected and exist.
3. Validate: no symlinks/shortcuts/reparse points (reject with error).
4. Compute batch impact:
   - If `current_count + batch_count > max_files`: reject whole batch.
5. Validate no filename collisions:
   - If any filename already exists in the Workbench: reject the whole batch (no partial add).
6. For each file:
   - If size > max: mark "skipped (too large)".
   - Else: copy into Workbench `published/` root.
   - Classify extension/mime; mark supported vs opaque.
   - Add manifest entry; emit `WorkbenchFilesChanged`.
7. Update `updated_at`, recalc clutter score, emit `WorkbenchClutterChanged`.

### Extract Files Algorithm (High-Level)
1. Validate: no Draft exists (block if Draft present).
2. Validate destination path exists and is a writable directory.
3. Resolve export set:
   - if `workbench_paths` provided, use those;
   - otherwise use all manifest files.
4. For each file:
   - Validate flat workbench path.
   - Source from `published/<path>`.
   - Resolve a unique destination filename; if destination exists, append `(N)` before the extension until a free name is found.
   - Copy file to destination on success.
5. Return per-file extract results (`extracted|skipped|failed`) with reasons; successful results include `final_path`.

### Supported vs Opaque Handling
- Supported types: engine may extract text/preview as needed for jobs/review.
- Opaque types: engine exposes metadata + best-effort in-app before/after previews during review; summaries are required during review.

## Error Handling & Recovery
- **Disk full / write error**: fail the add with actionable message; do not leave partial/corrupt manifest entries.
- **Permission error reading source**: mark that file as failed and continue with remaining eligible files.
- **Manifest corruption**: rebuild manifest from on-disk files where possible; otherwise prompt user to restore from checkpoint.

## Security & Privacy
- Enforce sandbox boundary by rejecting any workbench path that escapes the root (including `..`, absolute paths, and symlink traversal).
- Never allow symlinks to exist inside the Workbench tree (prevents “symlink escape” sandbox bypasses).
  - v1 decision: reject importing symlinks/shortcuts/reparse points; only regular files are allowed.
- No background scanning of the filesystem; only user-selected imports are read.

## Telemetry (If Any)
Local-only by default (v1):
- Import success/failure counts (by reason: too many, too large, permission, disk full).
- Average workbench file count/bytes (to validate limits).

## Open Questions
~~Workbench location (v1 decision)~~ → **Resolved**: App-managed base directory only (not user-configurable). UI may show the on-disk location as read-only for transparency.

~~Folder add~~ → **Resolved**: Out of scope for v1. Flat file structure only.

~~Symlink policy~~ → **Resolved**: Forbid symlinks/shortcuts/reparse points. Only regular files supported.

## Self-Review (Design Sanity)
- Matches PRD add semantics (batch reject on count overflow; per-file skip on oversize; allow opaque types).
- Keeps Draft interactions explicit (active view concept avoids hidden merges).
- Leaves one-way-door choices (snapshot/Git, IPC protocol) to ADRs.
