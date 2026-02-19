# KeenBench — Test Plan: Bank Statement Analysis Scenarios

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
| `cuentas_octubre_2024_anonymized_draft.xlsx` | `engine/testdata/real/` | Anonymized Santander bank statement. Sheet "Movimientos", 103 transactions (rows 9-111), columns: FECHA OPERACIÓN, FECHA VALOR, CONCEPTO, IMPORTE EUR, SALDO. All expenses, total -12,257.08. Running balance from 31,793.85 (row 10) down to 10,856.40 (last row). 100 unique CONCEPTO values. Transaction types: Pago Movil, Compra Comercio, Compra Internet, Recibo, Transferencia, etc. |

### Additional files for cross-reference test

| File | Purpose | Specification |
|------|---------|--------------|
| `budget_notes.txt` | Budget cross-reference | Text file with content: "Monthly budget: Entertainment 500 EUR, Groceries 800 EUR, Transport 200 EUR, Bills 2000 EUR." Created as part of test setup. |

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

## Bank Statement Details

The bank statement (`cuentas_octubre_2024_anonymized_draft.xlsx`) contains:
- **Sheet:** "Movimientos"
- **Structure:** Headers in rows 1-8, data in rows 9-111
- **Columns:** FECHA OPERACIÓN, FECHA VALOR, CONCEPTO, IMPORTE EUR, SALDO
- **Key facts:** 103 transactions, all expenses (negative IMPORTE EUR), total -12,257.08
- **Balance range:** 31,793.85 (row 10) down to 10,856.40 (last row)
- **Transaction categories:** Pago Movil, Compra Comercio, Compra Internet, Recibo, Transferencia, and others

---

## Test Cases

### 12. Bank Statement Analysis Scenarios `[AI]`

#### TC-100: Extract transactions above threshold to CSV `[AI]`
- Priority: P1
- Preconditions: Consent granted. Workbench has bank statement XLSX. No Draft.
- Steps:
  1. Type: "Find all transactions in the bank statement where the absolute value of IMPORTE EUR is greater than 200. Create a CSV file called large_transactions.csv with columns: Date, Concept, Amount." Click Send.
     Expected: The assistant reads the XLSX, filters transactions, and creates the CSV.
  2. Wait for Draft creation.
     Expected: Draft created. Timeout: 120 seconds.
  3. Verify the draft CSV on disk.
     Expected: The file is valid CSV. It has a header row with at least Date and Amount columns. Every Amount value has absolute value > 200 (tolerance: allow the AI to interpret > vs >=, but no values with |amount| < 150 should appear). At least 5 rows of data (there are many transactions over 200 in the dataset). All amounts are negative (they are all expenses).

#### TC-101: Create monthly summary in markdown `[AI]`
- Priority: P1
- Preconditions: Consent granted. Workbench has bank statement XLSX. No Draft.
- Steps:
  1. Type: "Create a file called october_summary.md that contains: a heading 'October 2024 Bank Statement Summary', the total number of transactions, the total amount spent, and the beginning and ending balance from the SALDO column." Click Send.
     Expected: The assistant processes the XLSX and creates the markdown file.
  2. Wait for Draft creation.
     Expected: Draft created. Timeout: 120 seconds.
  3. Verify the draft file.
     Expected: The file contains "October" and "2024" (or "october" case-insensitive). It mentions a number of transactions in the range 90-120. It mentions a total amount (absolute value) in the range 10,000-15,000. It mentions a beginning balance around 31,000-32,000 and an ending balance around 10,000-11,000.

#### TC-102: Cross-reference bank statement with text notes `[AI]`
- Priority: P2
- Preconditions: Consent granted. Workbench has bank statement XLSX AND a `budget_notes.txt` file with text: "Monthly budget: Entertainment 500 EUR, Groceries 800 EUR, Transport 200 EUR, Bills 2000 EUR."
- Steps:
  1. Type: "Compare the bank statement spending against the budget in budget_notes.txt. Create a file called budget_vs_actual.csv with columns: Category, Budget, Actual, Difference. Try to map transaction concepts to budget categories." Click Send.
     Expected: The assistant reads both files and creates the comparison.
  2. Wait for Draft creation.
     Expected: Draft created. Timeout: 120 seconds.
  3. Verify the draft CSV.
     Expected: The file is valid CSV with at least a header row and 2+ data rows. Each row has 4 columns. The Budget column contains values from the notes (500, 800, 200, 2000 or similar). The Actual column contains negative numbers derived from the bank statement.
