# KeenBench — Test Plan: Draft Creation and Auto-Apply (Text/CSV)

> Extracted from the [master test plan](../test-plan.md). This section can be used independently.

## Policy

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**
No fake/mock AI clients. No `KEENBENCH_FAKE_OPENAI`. No conditional branching on fake vs real mode.
Assertions on AI output must be structural or numerical — never exact-text-match on prose.
See `CLAUDE.md` for the full testing policy.

## Test Environment

- Linux desktop (X11). E2E harness targets `flutter test integration_test -d linux`.
- Network access to: `api.openai.com`, `api.anthropic.com`, `generativelanguage.googleapis.com`, `api.mistral.ai`.
- Valid API keys in `.env`:
  - `KEENBENCH_OPENAI_API_KEY` (required for all AI tests)
  - `KEENBENCH_ANTHROPIC_API_KEY` (required for multi-provider tests)
  - `KEENBENCH_GEMINI_API_KEY` (required for multi-provider tests)
  - `KEENBENCH_MISTRAL_API_KEY` (required for multi-provider tests)
- Clean app data directory per test run (`KEENBENCH_DATA_DIR` pointed to temp dir).
- Python venv with tool worker dependencies (`engine/tools/pyworker/.venv`).

## Test Data

| File | Location | Description |
|------|----------|-------------|
| `notes.txt` | `app/integration_test/support/` | Staffing meeting notes (projects Atlas/Beacon, 5 employees, decisions, open items) |
| `data.csv` | `app/integration_test/support/` | Employee roster CSV (name, role, location, availability) with 5 rows |
| `cuentas_octubre_2024_anonymized_draft.xlsx` | `engine/testdata/real/` | Anonymized Santander bank statement. Sheet "Movimientos", 103 transactions (rows 9-111), columns: FECHA OPERACIÓN, FECHA VALOR, CONCEPTO, IMPORTE EUR, SALDO. All expenses, total -12,257.08. Running balance from 31,793.85 (row 10) down to 10,856.40 (last row). 100 unique CONCEPTO values. Transaction types: Pago Movil, Compra Comercio, Compra Internet, Recibo, Transferencia, etc. |

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
- `AppKeys.reviewScreen`, `AppKeys.reviewChangeList`

## Draft Lifecycle

- The AI proposes file changes via tool calls (list_files, read_file, write_text_file, etc.).
- The engine captures the proposal and auto-applies it to a draft directory.
- Draft files live in `workbenches/<id>/draft/` alongside published files in `workbenches/<id>/published/`.
- Only one draft can exist per workbench at a time.
- While a draft exists, the workshop composer is hidden (replaced by draft banner).

---

## Test Cases

### 6. Draft Creation and Auto-Apply — Text/CSV

#### TC-040: Auto-draft from staffing prompt (text files) `[AI]`
- Priority: P0
- Preconditions: Consent granted. Workbench has `notes.txt` (staffing notes) and `data.csv` (employee roster). No existing Draft.
- Steps:
  1. Type the following in the composer and click Send:
     "Create two new files:\n1. org_chart.md — an org chart grouped by project (Atlas, Beacon) listing each employee by full name and role.\n2. project_assignments.csv — a CSV with header: name,project,role and one row per employee."
     Expected: The user message appears. The assistant begins processing.
  2. Wait for the assistant response to complete.
     Expected: The assistant acknowledges the request. Tool calls may be visible in the conversation (list_files, read_file, write_text_file). Timeout: 120 seconds.
  3. Wait for the Draft to be created (the review screen auto-opens or the draft banner appears).
     Expected: Either the review screen (`AppKeys.reviewScreen`) appears automatically, OR the draft banner (`AppKeys.workbenchDraftBanner`) appears with text "Draft in progress". Timeout: 30 seconds after assistant response.
  4. If on workbench screen, click "Open Review" (`AppKeys.workbenchReviewButton`).
     Expected: The review screen opens.
  5. Verify the change list (`AppKeys.reviewChangeList`) shows at least one file.
     Expected: At least one entry appears in the change list. Each entry shows a file path and an "ADDED" badge.
  6. Verify the published files via engine API: call `WorkbenchFilesList` and check the draft directory on disk.
     Expected: Draft directory contains at least one `.md` or `.csv` file. The files are parseable (valid UTF-8 text). If `org_chart.md` exists: it contains the text "Atlas" and "Beacon" (case-insensitive). If `project_assignments.csv` exists: it has a header row containing "name" and at least 3 data rows.

#### TC-041: Auto-draft from bank statement analysis `[AI]`
- Priority: P0
- Preconditions: Consent granted. Workbench has `cuentas_octubre_2024_anonymized_draft.xlsx`. No existing Draft.
- Steps:
  1. Type the following in the composer and click Send:
     "Analyze the bank statement and create a file called spending_summary.csv with columns: Category, TransactionCount, TotalAmount. Group transactions by their type (e.g., Pago Movil, Compra Comercio, Recibo, Transferencia, etc.) and calculate the total for each category."
     Expected: The user message appears. The assistant begins processing. Tool calls for reading the XLSX file should occur.
  2. Wait for the assistant response and Draft creation.
     Expected: The assistant response completes. A Draft is created. Timeout: 120 seconds.
  3. Navigate to the review screen.
     Expected: The change list shows `spending_summary.csv` (or similarly named CSV) with "ADDED" badge.
  4. Verify the draft file on disk.
     Expected: The CSV file exists in the draft directory. It is valid CSV (parseable). It has a header row. It has at least 3 data rows (there are multiple transaction categories). The total amounts in the file are all negative numbers (expenses). The sum of all TotalAmount values is approximately -12,257 (within +/-1000 tolerance).

#### TC-042: Auto-draft creates new XLSX sheet from bank data `[AI]`
- Priority: P1
- Preconditions: Consent granted. Workbench has `cuentas_octubre_2024_anonymized_draft.xlsx`. No existing Draft.
- Steps:
  1. Type the following in the composer and click Send:
     "Add a new sheet called 'Summary' to the bank statement file. In this sheet, put a header row with: Category, Count, Total. Then list each transaction category with its count and total amount. At the bottom, add a row with 'TOTAL' and the grand totals."
     Expected: The assistant processes the request using xlsx_operations tool calls.
  2. Wait for Draft creation.
     Expected: A Draft is created. Timeout: 120 seconds.
  3. Navigate to the review screen. Select the XLSX file in the change list.
     Expected: The change list shows the XLSX file with "MODIFIED" badge. The detail pane shows a sheet selector. A "Summary" sheet should be available in the dropdown.
  4. Verify the draft XLSX file on disk using the tool worker or openpyxl.
     Expected: The workbook has a new sheet named "Summary" (or case variation). The sheet has a header row. It has multiple data rows. There is a total row at the bottom.
