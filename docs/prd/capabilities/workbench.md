# PRD: Workbench

## Status
Draft

## Version
v0.3

## Last Updated
2026-02-13

## Purpose
Provide a bounded, sandboxed container for user files where AI can safely read and write without affecting original files or the broader filesystem.

## Scope
- In scope (v1): create/open Workbench, explicit file add (copy-based), file limits enforcement, sandboxed AI access, scope messaging, single Workbench focus, delete Workbench, and extract Published files to a user-selected local folder (copy-only; no archive format).
- Out of scope (v1): multiple concurrent Workbenches, Workbench archive export (.zip), cloud sync, connectors (Drive/Notion/etc.), automatic file ingestion.

## Key Concepts
- **Workbench**: A persistent, named container for user files. Users can rename Workbenches after creation.
- **Add files**: User explicitly selects files to add; files are copied into the Workbench.
- **Sandbox**: AI can only read/write files inside the Workbench; no access to originals or other OS paths.
- **Scope boundary**: The Workbench defines the AI's entire world for a given session.
- **Flat structure**: Workbench has a flat file list. Users cannot create subfolders.

## User Experience

### Create a Workbench
- User initiates "New Workbench" from the app.
- User provides a name for the Workbench.
- Workbench is created and ready to add files.

### Delete a Workbench
- User selects “Delete Workbench” from the Workbench list.
- System confirms the deletion with clear copy about local-only impact.
- If a Draft exists, deletion is blocked until the Draft is published or discarded.

### Extract Files
- User clicks a per-file **Extract** action in the Workbench file row and selects a destination folder.
- App copies the selected Published file to that folder (no Draft export in v1).
- Existing destination files are not overwritten; those files are skipped with a clear result summary.
- If a Draft exists, extraction is blocked until the Draft is published or discarded.

### Add Files
- User adds files via drag-and-drop or file picker.
- Files are copied into the Workbench; originals remain untouched.
- On successful add, show confirmation: "Added [filename] to Workbench."
- Display scope notice: "Workbench contains copies of your files; originals are untouched."

### File Limits Enforcement (v1)
| Limit | Value | Enforcement |
|-------|-------|-------------|
| Max files per Workbench | 10 | Block add beyond limit |
| Max file size | 25 MB per file | Block oversized files |

**UX for limit violations:**

- **Too many files (exceeds 10)**:
  - If user drags/selects multiple files that would exceed the limit:
    - Show error: "Workbench can hold up to 10 files. You have [X] files and tried to add [Y] more."
    - Offer action: "Remove files to make room" or "Cancel add."
  - Do not partially add; reject the entire batch if it exceeds the limit.

- **File too large (exceeds 25 MB)**:
  - Show error: "[filename] is too large ([size] MB). Maximum file size is 25 MB."
  - If adding multiple files, skip the oversized file and add the rest.
  - List skipped files clearly: "Skipped: [filename1], [filename2] (exceeded size limit)."

- **Unsupported file type**:
  - Allow adding but show notice: "[filename] will be added but cannot be read by AI (unsupported type)."
  - Unsupported files are treated as opaque binaries (no content extraction).

- **Symlinks, shortcuts, reparse points**:
  - Not supported. Only regular files can be added.
  - Show error: "Links aren't supported. Add the original file instead."

- **Duplicate file (same path)**:
  - Block with error: "[filename] already exists in this Workbench."
  - User must rename the file externally before adding.

### View Workbench Contents
- File list shows all files in the Workbench with:
  - File name
  - File type indicator (icon or label)
  - Supported/unsupported status
  - Optional compact badges (type/read-only/opaque)
- Each file row includes actions for Extract and Remove.
- Users can remove files from the Workbench with explicit confirmation (does not affect originals).
- File add/remove/extract actions are disabled while a Draft exists; users must publish or discard first.
- Context item add/edit/delete actions are also disabled while a Draft exists (see `docs/prd/capabilities/workbench-context.md`).
- Users cannot rename files within the Workbench. To rename, remove and re-add with the new name.

### Scope Messaging
- The UI always indicates the active Workbench name.
- Scope notice is visible during onboarding and accessible from Help.
- AI operations clearly reference "Workbench" scope.

## Functional Requirements

### v1
1. Users can create a named Workbench. Users can rename Workbenches.
2. Users can add files via drag-and-drop or file picker.
3. Added files are copied into the Workbench; originals are never modified.
4. File count limit (10) is enforced when adding files, with clear error messaging.
5. File size limit (25 MB) is enforced when adding files, with clear error messaging.
6. Unsupported file types can be added but are marked as opaque (no AI content access).
7. Symlinks, shortcuts, and reparse points cannot be added. Only regular files are supported.
8. Duplicate files (same filename) cannot be added. User must rename externally first.
9. Users can view all files in the Workbench with name, type/status badges, and clear filename truncation behavior.
10. Users can remove files from the Workbench with explicit confirmation. Users cannot rename files within the Workbench.
11. Users can delete a Workbench with explicit confirmation; deletion is blocked while a Draft exists.
12. Users can extract Published Workbench files to a user-selected local folder; extraction is blocked while a Draft exists.
13. AI read/write operations are restricted to Workbench contents only.
14. The app never accesses filesystem paths outside user-selected files.
15. Workbench name and scope are always visible in the UI.
16. Workbench has a flat structure. Users cannot create subfolders.
17. Workshop requires at least 1 file. Empty Workbenches show: "Add files to start working."

## Supported File Types (v1)
| Category | Formats |
|----------|---------|
| Text | .md, .txt, .csv, .json, .xml, .yaml, .html |
| Code | .js, .ts, .py, .java, .go, .rb, .rs, .c, .cpp, .h, .css, .sql |
| Documents | .docx, .odt, .pdf |
| Presentations | .pptx |
| Spreadsheets | .xlsx |
| Images | .png, .jpg, .jpeg, .gif, .svg, .webp |

Archives (.zip) and unlisted types are treated as unsupported (opaque binaries).

CSV (`.csv`) is treated as first-class tabular text. Tabular schema/stats/query/export behavior is defined in `docs/prd/capabilities/file-operations.md`.

## Storage
- Workbenches are stored locally on the user's machine.
- Storage location is app-managed (not user-configurable in v1).
- Each Workbench is a self-contained directory with added files and metadata.

## Failure Modes & Recovery
- Add files fails (disk full, permission error): show error with the specific file(s) that failed; allow retry.
- Workbench metadata corrupted: prompt user to restore from checkpoint or discard.
- File read fails during AI operation: list affected files and continue if possible.

## Security & Privacy
- No implicit filesystem scanning.
- No access to paths outside the Workbench.
- Import is always explicit and user-initiated.
- Workbench contents are stored locally by default.

## Acceptance Criteria
- Users can create and rename a Workbench.
- Users can add files via drag-drop or picker. Files are copied; originals are never modified.
- Adding more than 10 files shows a clear error and blocks the operation.
- Adding a file over 25 MB shows a clear error and skips that file.
- Adding a duplicate filename shows a clear error and blocks it.
- Adding symlinks/shortcuts shows a clear error and blocks them.
- Unsupported files are added with a notice and marked as opaque.
- Users can view and remove files. Users cannot rename files within the Workbench.
- Users can extract Published files to a selected folder (copy-only), with collision-safe auto-rename (`file.ext`, `file(1).ext`, `file(2).ext`, ...).
- File add/remove/extract actions are blocked while a Draft exists.
- AI operations are visibly scoped to the Workbench.
- The app never accesses filesystem paths the user didn't explicitly select.
- Workshop cannot start with an empty Workbench.

## Open Questions
None currently.
