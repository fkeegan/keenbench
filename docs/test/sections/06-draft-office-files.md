# KeenBench — Test Plan: Draft Creation and Auto-Apply (Office Files)

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

| File | Location | Description |
|------|----------|-------------|
| `data.csv` | `app/integration_test/support/` | Employee roster CSV (name, role, location, availability) with 5 rows |
| `simple.docx` | `engine/testdata/office/` | Simple Word document with paragraphs and headings |
| `multi-sheet.xlsx` | `engine/testdata/office/` | Excel workbook with multiple sheets |
| `slides.pptx` | `engine/testdata/office/` | PowerPoint with 2-3 slides |

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

Key elements for this section:
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.workbenchDraftBanner`, `AppKeys.workbenchReviewButton`
- `AppKeys.reviewScreen`, `AppKeys.reviewChangeList`, `AppKeys.reviewDiffList`

## Office File Operations

The engine uses a Python tool worker (`engine/tools/pyworker/`) for office file operations:
- `docx_operations`: Create and modify Word documents (headings, paragraphs, tables)
- `xlsx_operations`: Create and modify Excel workbooks (sheets, cells, formulas)
- `pptx_operations`: Create and modify PowerPoint presentations (slides, text, layouts)

---

## Test Cases

### 7. Draft Creation and Auto-Apply — Office Files

#### TC-050: Create a DOCX report from CSV data `[AI]`
- Priority: P0
- Preconditions: Consent granted. Workbench has `data.csv` (employee roster). No existing Draft.
- Steps:
  1. Type the following in the composer and click Send:
     "Create a Word document called team_report.docx with a heading 'Team Overview', a paragraph describing the team composition, and a summary of each employee's name and role."
     Expected: The assistant processes the request. Tool calls for `read_file` (to read the CSV) and `docx_operations` (to create the document) should occur.
  2. Wait for Draft creation.
     Expected: A Draft is created with `team_report.docx`. Timeout: 120 seconds.
  3. Navigate to review. Select `team_report.docx` in the change list.
     Expected: The file shows "ADDED" badge and "DOCX" type badge. The detail pane shows a page preview (side-by-side, with "Draft" on the right).
  4. Verify the draft DOCX on disk.
     Expected: The file is a valid DOCX (can be opened by python-docx without error). It contains at least one heading element. It contains at least one paragraph with text. It references at least 2 employee names from `data.csv`.

#### TC-051: Modify existing DOCX `[AI]`
- Priority: P1
- Preconditions: Consent granted. Workbench has `simple.docx`. No existing Draft.
- Steps:
  1. Type: "Read the document and add a new section at the end with heading 'Conclusion' and a paragraph summarizing the document's content." Click Send.
     Expected: The assistant reads the file and proposes modifications.
  2. Wait for Draft creation.
     Expected: Draft created with modified `simple.docx`. Timeout: 120 seconds.
  3. Navigate to review. Select `simple.docx`.
     Expected: "MODIFIED" badge. The text extraction diff shows added lines containing "Conclusion". The page preview shows Published (left) and Draft (right) side by side.
  4. Verify the diff panel (`AppKeys.reviewDiffList`).
     Expected: Green-highlighted (added) lines are visible containing the word "Conclusion".

#### TC-052: Create PPTX presentation from data `[AI]`
- Priority: P1
- Preconditions: Consent granted. Workbench has `data.csv`. No existing Draft.
- Steps:
  1. Type: "Create a PowerPoint called team_deck.pptx with 3 slides: 1) Title slide with 'Team Overview'. 2) A slide listing all team members with their roles. 3) A conclusion slide." Click Send.
     Expected: The assistant uses `pptx_operations` to create the presentation.
  2. Wait for Draft creation.
     Expected: Draft created with `team_deck.pptx`. Timeout: 120 seconds.
  3. Navigate to review. Select the PPTX file.
     Expected: "ADDED" badge. The detail pane shows slide preview with navigation (Previous/Next buttons, "Slides" label, "1 / N" counter).
  4. Navigate through slides using the Next button.
     Expected: Each click advances to the next slide. The slide counter updates. At least 2 slides are present.
  5. Verify the draft PPTX on disk.
     Expected: Valid PPTX file. Contains at least 2 slides. At least one slide contains text referencing team members.

#### TC-053: Modify existing XLSX with multi-sheet operations `[AI]`
- Priority: P0
- Preconditions: Consent granted. Workbench has `multi-sheet.xlsx`. No existing Draft.
- Steps:
  1. Type: "Read the spreadsheet. Add a new sheet called 'Summary' with a single cell A1 containing the text 'Summary of all sheets'. In cell A2, list the names of all other sheets separated by commas." Click Send.
     Expected: The assistant reads the file via tool calls, then uses `xlsx_operations` to modify it.
  2. Wait for Draft creation.
     Expected: Draft created with modified `multi-sheet.xlsx`. Timeout: 120 seconds.
  3. Navigate to review. Select the XLSX file.
     Expected: "MODIFIED" badge. The detail pane shows a grid preview with a sheet selector dropdown.
  4. Select "Summary" from the sheet dropdown.
     Expected: The grid preview shows data in the Summary sheet. Cell A1 area is visible.
  5. Verify the draft XLSX on disk.
     Expected: The workbook has a "Summary" sheet. Cell A1 contains "Summary of all sheets" (or close text). Cell A2 is not empty.
