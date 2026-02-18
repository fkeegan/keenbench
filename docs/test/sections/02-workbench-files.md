# KeenBench — Test Plan: Workbench Creation and File Management

> Extracted from the [master test plan](../test-plan.md). This section can be used independently.

## Policy

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**
No fake/mock AI clients. No `KEENBENCH_FAKE_OPENAI`. No conditional branching on fake vs real mode.
Assertions on AI output must be structural or numerical — never exact-text-match on prose.
See `CLAUDE.md` for the full testing policy.

## Test Environment

- Linux desktop (X11). E2E harness targets `flutter test integration_test -d linux`.
- Network access to: `api.openai.com`, `api.anthropic.com`, `generativelanguage.googleapis.com`.
- Valid API keys in `.env`:
  - `KEENBENCH_OPENAI_API_KEY` (required for all AI tests)
  - `KEENBENCH_ANTHROPIC_API_KEY` (required for multi-provider tests)
  - `KEENBENCH_GEMINI_API_KEY` (required for multi-provider tests)
- Clean app data directory per test run (`KEENBENCH_DATA_DIR` pointed to temp dir).
- Python venv with tool worker dependencies (`engine/tools/pyworker/.venv`).

## Test Data

### Bundled fixtures (in repo)

| File | Location | Description |
|------|----------|-------------|
| `notes.txt` | `app/integration_test/support/` | Staffing meeting notes (projects Atlas/Beacon, 5 employees, decisions, open items) |
| `data.csv` | `app/integration_test/support/` | Employee roster CSV (name, role, location, availability) with 5 rows |
| `simple.docx` | `engine/testdata/office/` | Simple Word document with paragraphs and headings |
| `multi-sheet.xlsx` | `engine/testdata/office/` | Excel workbook with multiple sheets |
| `slides.pptx` | `engine/testdata/office/` | PowerPoint with 2-3 slides |
| `report.pdf` | `engine/testdata/office/` | PDF report (read-only) |
| `notes.odt` | `engine/testdata/office/` | OpenDocument text (read-only) |
| `chart.png` | `engine/testdata/office/` | PNG image (read-only) |
| `logo.svg` | `engine/testdata/office/` | SVG image (read-only) |
| `unknown.bin` | `engine/testdata/office/` | Unknown binary format (opaque) |

### Synthetic data (generated for tests)

| File | Purpose | Specification |
|------|---------|--------------|
| `big.csv` | Oversize rejection test | >25 MB, repeated rows |

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

Key elements for this section:
- `AppKeys.homeScreen`, `AppKeys.homeNewWorkbenchButton`
- `AppKeys.newWorkbenchDialog`, `AppKeys.newWorkbenchNameField`, `AppKeys.newWorkbenchCreateButton`
- `AppKeys.workbenchScreen`, `AppKeys.workbenchFileList`, `AppKeys.workbenchScopeLimits`
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchScopeBadge`
- `AppKeys.workbenchFileRow(path)`, `AppKeys.workbenchFileExtractButton(path)`, `AppKeys.workbenchFileRemoveButton(path)`
- `AppKeys.workbenchAddFilesButton`
- `AppKeys.workbenchRemoveFileDialog`, `AppKeys.workbenchRemoveFileCancel`, `AppKeys.workbenchRemoveFileConfirm`
- `AppKeys.workbenchDraftBanner`, `AppKeys.workbenchReviewButton`, `AppKeys.workbenchDiscardButton`
- `AppKeys.workbenchTileMenu(id)`, `AppKeys.workbenchTileDelete(id)`
- `AppKeys.homeDeleteWorkbenchDialog`, `AppKeys.homeDeleteWorkbenchCancel`, `AppKeys.homeDeleteWorkbenchConfirm`
- `AppKeys.homeWorkbenchGrid`

## File Type Semantics

- **Read+Write:** txt, csv, md, json, xml, yaml, html, code files, docx, xlsx, pptx
- **Read-only:** pdf, odt, images (png, jpg, gif, webp, svg)
- **Opaque:** any other type (metadata only)

---

## Test Cases

### 3. Workbench Creation and File Management

#### TC-007: Create a new Workbench
- Priority: P0
- Preconditions: App on home screen.
- Steps:
  1. Click "New Workbench" (`AppKeys.homeNewWorkbenchButton`).
     Expected: A dialog appears (`AppKeys.newWorkbenchDialog`) with a text field (`AppKeys.newWorkbenchNameField`) and "Create" / "Cancel" buttons.
  2. Type "Financial Analysis" into the name field.
     Expected: The text "Financial Analysis" appears in the field.
  3. Click "Create" (`AppKeys.newWorkbenchCreateButton`).
     Expected: The dialog closes. The workbench screen (`AppKeys.workbenchScreen`) opens with title "Financial Analysis". The file list (`AppKeys.workbenchFileList`) is empty. The scope description text is visible (`AppKeys.workbenchScopeLimits`).
  4. Verify the composer field (`AppKeys.workbenchComposerField`) is visible with placeholder "Ask the Workshop...".
     Expected: The composer is present and enabled.

#### TC-008: Add text and CSV files
- Priority: P0
- Preconditions: Workbench "Financial Analysis" open, no Draft, no files.
- Steps:
  1. Programmatically add `notes.txt` and `data.csv` via the engine API `WorkbenchFilesAdd` (or click "Add files" and select them).
     Expected: Both files appear in the file list. `notes.txt` shows a file row (`AppKeys.workbenchFileRow('notes.txt')`) with a "TXT" badge. `data.csv` shows a row with a "CSV" badge.
  2. Verify both file rows have an extract button (`AppKeys.workbenchFileExtractButton('notes.txt')`) and a remove button (`AppKeys.workbenchFileRemoveButton('notes.txt')`).
     Expected: Both action buttons are visible and enabled (no draft exists).
  3. Verify the scope badge (`AppKeys.workbenchScopeBadge`) shows "Scoped" or the scope limits text updates to reflect the file count.
     Expected: The file count in the scope area reflects 2 files.

#### TC-009: Add office files (DOCX, XLSX, PPTX, PDF, images)
- Priority: P0
- Preconditions: Workbench open, no Draft.
- Steps:
  1. Add `simple.docx`, `multi-sheet.xlsx`, `slides.pptx`, `report.pdf`, `chart.png`, `logo.svg` via `WorkbenchFilesAdd`.
     Expected: All 6 files appear in the file list with appropriate badges: "DOCX", "XLSX", "PPTX", "PDF", "PNG", "SVG".
  2. Verify `report.pdf`, `chart.png`, `logo.svg` show a "Read-only" badge on their file rows.
     Expected: Read-only files are visually distinguished.
  3. Verify the file count in scope limits reflects the correct total.
     Expected: File count shows the correct number (2 previous + 6 = 8, or per the test setup).

#### TC-010: Add opaque file
- Priority: P1
- Preconditions: Workbench open, no Draft.
- Steps:
  1. Add `unknown.bin` via `WorkbenchFilesAdd`.
     Expected: The file appears in the list with an "Opaque" badge. No error occurs.
  2. Verify the opaque flag is set on the file by checking `WorkbenchState.files`.
     Expected: The file's `isOpaque` property is `true`.

#### TC-011: File count limit (batch reject at 10)
- Priority: P0
- Preconditions: Workbench has 9 files already.
- Steps:
  1. Create two new temp files: `extra1.txt` and `extra2.txt`.
     Expected: Files exist on disk.
  2. Attempt to add both files via `WorkbenchFilesAdd` in a single call.
     Expected: The add operation fails with an error. The error message references the file count limit (10). No new files are added to the workbench. The file list still shows 9 files.

#### TC-012: Oversize file skip (>25 MB)
- Priority: P0
- Preconditions: Workbench open, no Draft, fewer than 10 files.
- Steps:
  1. Create `big.csv` (>25 MB, e.g., a repeated row to exceed the limit) and a small `small.txt` (a few bytes).
     Expected: Both files exist on disk.
  2. Attempt to add both `big.csv` and `small.txt` in a single `WorkbenchFilesAdd` call.
     Expected: `small.txt` is added successfully (appears in file list). `big.csv` is skipped. The add result includes a skip reason mentioning the size limit (25 MB). The file list does NOT contain `big.csv`.

#### TC-013: Duplicate filename rejection
- Priority: P0
- Preconditions: Workbench has `notes.txt` already added.
- Steps:
  1. Create a different file at a different path but also named `notes.txt` (e.g., `/tmp/test/notes.txt`).
     Expected: File exists on disk with different content than the workbench copy.
  2. Attempt to add this `notes.txt` via `WorkbenchFilesAdd`.
     Expected: The entire add batch is rejected with a duplicate-name error. No new files are added.

#### TC-014: Symlink rejection
- Priority: P1
- Preconditions: Workbench open, `notes.txt` exists on disk.
- Steps:
  1. Create a symlink `link_to_notes` pointing to `notes.txt`.
     Expected: Symlink exists.
  2. Attempt to add `link_to_notes` via `WorkbenchFilesAdd`.
     Expected: The add fails with an error mentioning "links not supported" or "symlink". No file is added.

#### TC-015: Remove file from workbench
- Priority: P0
- Preconditions: Workbench has `notes.txt` and `data.csv`, no Draft.
- Steps:
  1. Click the remove button (`AppKeys.workbenchFileRemoveButton('notes.txt')`).
     Expected: A confirmation dialog appears (`AppKeys.workbenchRemoveFileDialog`) with text 'Remove "notes.txt" from this Workbench? Originals remain untouched.'
  2. Click "Cancel" (`AppKeys.workbenchRemoveFileCancel`).
     Expected: The dialog closes. `notes.txt` is still in the file list.
  3. Click the remove button again.
     Expected: The confirmation dialog reappears.
  4. Click the red confirm button (`AppKeys.workbenchRemoveFileConfirm`).
     Expected: The dialog closes. `notes.txt` is removed from the file list. `data.csv` remains.

#### TC-016: Add/remove blocked while Draft exists
- Priority: P0
- Preconditions: Workbench has files, Draft exists.
- Steps:
  1. Verify the "Add files" button (`AppKeys.workbenchAddFilesButton`) is disabled.
     Expected: The button is grayed out or visually disabled. A tooltip says "Publish or discard the Draft" or similar.
  2. Verify the remove buttons on each file row are disabled.
     Expected: Each file's remove button (`AppKeys.workbenchFileRemoveButton(path)`) is disabled with a tooltip.
  3. Verify the extract buttons on each file row are disabled.
     Expected: Each file's extract button (`AppKeys.workbenchFileExtractButton(path)`) is disabled with a tooltip.

#### TC-017: Delete workbench from home screen
- Priority: P1
- Preconditions: Home screen with at least one workbench tile.
- Steps:
  1. Click the three-dot menu (`AppKeys.workbenchTileMenu(workbenchId)`) on a workbench tile.
     Expected: A popup menu appears with a "Delete Workbench" option.
  2. Click "Delete Workbench" (`AppKeys.workbenchTileDelete(workbenchId)`).
     Expected: A confirmation dialog appears (`AppKeys.homeDeleteWorkbenchDialog`) asking to confirm deletion.
  3. Click "Cancel" (`AppKeys.homeDeleteWorkbenchCancel`).
     Expected: The dialog closes. The workbench tile is still visible.
  4. Repeat steps 1-2, then click the red confirm button (`AppKeys.homeDeleteWorkbenchConfirm`).
     Expected: The dialog closes. The workbench tile is removed from the grid.
