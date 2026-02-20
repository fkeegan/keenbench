# KeenBench — Test Plan: Workshop Chat

> Extracted from the [master test plan](../test-plan.md). This section can be used independently.

## Policy

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**
No fake/mock AI clients. No `KEENBENCH_FAKE_OPENAI`. No conditional branching on fake vs real mode.
Assertions on AI output must be structural or numerical — never exact-text-match on prose.
See `CLAUDE.md` for the full testing policy.

## Test Environment

- Linux desktop (X11). E2E harness targets `flutter test integration_test -d linux`.
- Network access to: `api.openai.com`, `auth.openai.com`, `api.anthropic.com`, `generativelanguage.googleapis.com`, `api.mistral.ai`.
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
| `cuentas_octubre_2024_anonymized_draft.xlsx` | `engine/testdata/real/` | Anonymized Santander bank statement. Sheet "Movimientos", 103 transactions (rows 9-111), columns: FECHA OPERACIÓN, FECHA VALOR, CONCEPTO, IMPORTE EUR, SALDO. All expenses, total -12,257.08. Running balance from 31,793.85 (row 10) down to 10,856.40 (last row). |

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Manual-only tests:** Marked with `[MANUAL ONLY]` and `Runner: Human only`.
- **Manual-only skip rule for AI agents:** Skip these cases with reason `Skipped: manual browser OAuth required (OpenAI Codex auth callback flow).`
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

Key elements for this section:
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.workbenchChatModeToggle`
- `AppKeys.workbenchMessageList`
- `AppKeys.workbenchDraftBanner`, `AppKeys.workbenchReviewButton`, `AppKeys.workbenchDiscardButton`

---

## Test Cases

### 5. Workshop Chat

#### TC-030: Streaming assistant response with text files `[AI]`
- Priority: P0
- Preconditions: Consent granted. Workbench has `notes.txt` (staffing notes) and `data.csv` (employee roster).
- Steps:
  1. Type "Who is assigned to Project Atlas? List their names and roles." in the composer and click Send.
     Expected: The user message bubble appears in the message list.
  2. Wait for the assistant response to stream.
     Expected: Text appears incrementally in the assistant message area. The response completes within 60 seconds.
  3. Read the completed assistant message.
     Expected: The response contains at least 2 of the following names from the roster: Alice Kim, Chloe Nguyen, Diego Patel. The response mentions roles or project assignments. The response is not empty.

#### TC-031: Streaming assistant response with bank statement XLSX `[AI]`
- Priority: P0
- Preconditions: Consent granted. Workbench has `cuentas_octubre_2024_anonymized_draft.xlsx`.
- Steps:
  1. Type "How many transactions are in this bank statement? What is the total of all IMPORTE EUR values?" in the composer and click Send.
     Expected: The user message appears. The assistant begins reading the file via tool calls.
  2. Wait for the assistant response to complete.
     Expected: The response completes within 120 seconds.
  3. Read the completed assistant message.
     Expected: The response mentions a transaction count in the range of 95-115 (the actual count is 103; AI may count differently depending on which rows it considers headers/blank). The response mentions a total expense amount — the absolute value should be approximately 12,257 (within +/-500 tolerance, as the AI may include or exclude edge rows). The key assertion is that the AI read and processed the XLSX data, not that it produced an exact number.

#### TC-032: Conversation persistence across app restart `[AI]`
- Priority: P0
- Preconditions: At least 2 conversation turns (user + assistant) exist in the workbench.
- Steps:
  1. Note the number of messages in the conversation and the text of the last assistant message.
     Expected: At least 2 user messages and 2 assistant messages are visible.
  2. Navigate back to home screen and note the workbench name.
     Expected: Home screen shows the workbench tile.
  3. Quit the app completely and relaunch.
     Expected: Home screen appears with the workbench tile.
  4. Click on the workbench tile to reopen it.
     Expected: The workbench screen opens. The message list contains the same number of messages as before. The last assistant message text matches what was noted in step 1.

#### TC-033: Workshop input blocked while Draft exists
- Priority: P0
- Preconditions: A Draft exists for the current workbench.
- Steps:
  1. Verify the composer field (`AppKeys.workbenchComposerField`) is NOT visible.
     Expected: Instead of the composer, the draft status area is shown with text "Draft in progress" and buttons for "Open review" (`AppKeys.workbenchReviewButton`) and "Discard" (`AppKeys.workbenchDiscardButton`).
  2. Verify there is no send button visible.
     Expected: `AppKeys.workbenchSendButton` is not found in the widget tree.

#### TC-034: Chat mode toggle changes composer intent
- Priority: P1
- Preconditions: Workbench open, no Draft, no active run.
- Steps:
  1. Verify chat mode toggle (`AppKeys.workbenchChatModeToggle`) is visible with options "Ask" and "Agent".
     Expected: Toggle renders both options and one is selected.
  2. Select "Agent".
     Expected: Composer placeholder reads "Describe a task...".
  3. Select "Ask".
     Expected: Composer placeholder reads "Ask a question...".

#### TC-035: In-flight run can be canceled from Send button `[AI]`
- Priority: P1
- Preconditions: Consent granted. Workbench has at least one file. No Draft.
- Steps:
  1. Send a prompt that takes long enough to observe an active run (for example: "Analyze all files step by step and explain your approach before the final answer.").
     Expected: Run starts and the Send button changes to "Cancel".
  2. Click the "Cancel" button (`AppKeys.workbenchSendButton`) while the run is in progress.
     Expected: A cancellation notice appears (for example, "Run canceled." or "Cancel requested. Stopping run..."), and the app remains responsive.
  3. Wait for completion of cancellation.
     Expected: The button returns to "Send". No crash occurs. Conversation state remains intact.
