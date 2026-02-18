# KeenBench — Test Plan: Review Screen

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

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

Key elements for this section:
- `AppKeys.reviewScreen`, `AppKeys.reviewChangeList`
- `AppKeys.reviewPublishButton`, `AppKeys.reviewDiscardButton`
- `AppKeys.reviewDiffList`
- `AppKeys.workbenchReviewButton`

## Review Screen Behavior

- The review screen shows a change list (left) and detail pane (right).
- Each change entry shows: file path, change type badge ("ADDED" or "MODIFIED"), file type badge.
- Detail pane varies by file type:
  - **Text files (txt, csv, md, etc.):** Summary + line-by-line diff (green for added, red for removed).
  - **DOCX:** Summary + text extraction diff + side-by-side page preview (Published left, Draft right).
  - **XLSX:** Summary + text extraction diff + grid preview with sheet selector dropdown.
  - **PPTX:** Summary + slide preview with navigation (Previous/Next, slide counter).
- Review works entirely offline — no model calls during review.

---

## Test Cases

### 8. Review Screen

#### TC-060: Review shows change list with correct badges
- Priority: P0
- Preconditions: Draft exists with at least one ADDED text file and one MODIFIED office file.
- Steps:
  1. Open the review screen (auto-opened or via `AppKeys.workbenchReviewButton`).
     Expected: The review screen (`AppKeys.reviewScreen`) is visible with title "Review Draft".
  2. Verify the change list (`AppKeys.reviewChangeList`) is populated.
     Expected: At least one entry in the list. Each entry shows: file path, change type badge ("ADDED" or "MODIFIED"), file type badge (e.g., "TXT", "DOCX", "XLSX").
  3. Verify the Publish button (`AppKeys.reviewPublishButton`) and Discard button (`AppKeys.reviewDiscardButton`) are visible in the app bar.
     Expected: Both buttons are present and enabled.

#### TC-061: Review text diff for added text file
- Priority: P0
- Preconditions: Draft contains an ADDED `.md` or `.txt` file.
- Steps:
  1. Click on the added text file in the change list.
     Expected: The detail pane updates. A "Summary" section appears with markdown-rendered text. A diff panel (`AppKeys.reviewDiffList`) appears below.
  2. Verify the diff panel.
     Expected: All lines are shown as added (green background with `+` prefix). The content is the full text of the new file. Line numbers are displayed.

#### TC-062: Review text extraction diff for DOCX
- Priority: P1
- Preconditions: Draft contains a MODIFIED `.docx` file.
- Steps:
  1. Click on the DOCX file in the change list.
     Expected: The detail pane shows: Summary section, text extraction diff, and page preview.
  2. Verify the text extraction diff.
     Expected: The diff shows removed lines (red, from published version) and added lines (green, from draft version). The diff reflects the actual text content changes.
  3. Verify the page preview.
     Expected: Side-by-side view with "Published" label on the left and "Draft" on the right. Page images are rendered. Navigation controls show page count.

#### TC-063: Review XLSX grid preview
- Priority: P1
- Preconditions: Draft contains a MODIFIED `.xlsx` file.
- Steps:
  1. Click on the XLSX file in the change list.
     Expected: The detail pane shows: Summary, text extraction diff, and grid preview.
  2. Verify the grid preview has a sheet selector dropdown.
     Expected: A dropdown is visible showing available sheet names.
  3. Select a sheet from the dropdown.
     Expected: The grid table updates to show the selected sheet's data. Cells with values are displayed.
  4. Click the navigation buttons (next row / previous row).
     Expected: The grid scrolls to show different row ranges.

#### TC-064: Review works offline
- Priority: P1
- Preconditions: Draft exists with changes.
- Steps:
  1. Disable the OpenAI provider via `ProvidersSetEnabled` API call (this simulates no network without actually disconnecting).
     Expected: Provider is disabled.
  2. Open the review screen.
     Expected: The review screen loads. The change list is populated. Diffs render correctly. Previews load. No error about network or provider.
  3. Select various files and verify diffs and previews all load.
     Expected: All review content is available offline. No model calls are made during review.
  4. Re-enable the provider.
     Expected: Provider is re-enabled.
