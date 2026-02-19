# KeenBench — Test Plan: Workbench Context (Manual QA)

> Extracted from the [master test plan](../test-plan.md). This section can be used independently.

## Policy

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**
No fake/mock AI clients. No `KEENBENCH_FAKE_OPENAI`. No conditional branching on fake vs real mode.
Assertions on AI output must be structural or numerical — never exact-text-match on prose.
See `CLAUDE.md` for the full testing policy.

## Test Environment (Manual)

- Desktop app build (Linux recommended; macOS/Windows acceptable if supported by the build you are testing).
- Network access to: `api.openai.com`, `api.anthropic.com`, `generativelanguage.googleapis.com`, `api.mistral.ai`.
- Valid API keys in `.env`:
  - `KEENBENCH_OPENAI_API_KEY` (required for all AI tests)
  - `KEENBENCH_ANTHROPIC_API_KEY` (required for multi-provider tests)
  - `KEENBENCH_GEMINI_API_KEY` (required for multi-provider tests)
  - `KEENBENCH_MISTRAL_API_KEY` (required for multi-provider tests)
- Optional but recommended for repeatable runs: a clean app data directory (`KEENBENCH_DATA_DIR` pointed to an empty temp dir).

## Soap Opera Scenario (Test Story)

These cases use a self-referential, fictional company so the test data is realistic and easy to reuse.

- **Company:** KeenBench (fictional startup)
- **Product:** KeenBench (this app)
- **Cast (fictional):** Mira Kwon (CEO), Ethan Park (CTO), Priya Desai (Sales Rep), Luis Alvarez (Lead Engineer)
- **Departments (fictional):**
  - Engineering: CTO + Lead Engineer
  - Ops: CEO
  - Sales/Growth: CEO + Sales Rep

## Test Data (KeenBench Fixtures)

All fixtures for this section live under:

- `docs/test/fixtures/workbench-context/`

### Text blocks (copy/paste into context)

- `docs/test/fixtures/workbench-context/keenbench_company_context_v1.txt`
- `docs/test/fixtures/workbench-context/keenbench_company_context_v2.txt`
- `docs/test/fixtures/workbench-context/keenbench_department_engineering_brief.txt`
- `docs/test/fixtures/workbench-context/keenbench_situation_alpha.txt`
- `docs/test/fixtures/workbench-context/keenbench_situation_beta.txt`
- `docs/test/fixtures/workbench-context/keenbench_document_style_v1.txt`

### File-upload fixtures (used in Upload file mode)

- `docs/test/fixtures/workbench-context/keenbench_company_overview.docx`
- `docs/test/fixtures/workbench-context/keenbench_engineering_department_brief.docx`
- `docs/test/fixtures/workbench-context/keenbench_company_metrics.xlsx`
- `docs/test/fixtures/workbench-context/keenbench_pitch_deck.pptx`
- `docs/test/fixtures/workbench-context/keenbench_security_overview.pdf`
- `docs/test/fixtures/workbench-context/keenbench_launch_plan.csv`

### Workbench files (used to create Drafts)

- `docs/test/fixtures/workbench-context/keenbench_weekly_notes.txt`

### Negative / large fixtures

Generated files (gitignored) are written to `artifacts/testdata/`:

- `artifacts/testdata/keenbench_situation_clutter_payload.md` (large Situation payload to trigger clutter warning)
- `artifacts/testdata/keenbench_oversize_roster.csv` (>25MB CSV for size-limit rejection)

Unsupported file types (committed fixtures):

- `docs/test/fixtures/workbench-context/keenbench_logo.png` (image input)
- `docs/test/fixtures/workbench-context/keenbench_unknown.bin` (unknown binary)

Fixture generation command:

```bash
engine/tools/pyworker/.venv/bin/python scripts/testdata/generate_workbench_context_fixtures.py
```

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## Native File Picker Note (Automation)

When driving the Linux native `Open File` dialog with `xdotool`, use this sequence:

```bash
xdotool windowactivate --sync <OPEN_FILE_WINDOW_ID>
xdotool key --clearmodifiers ctrl+l
xdotool key --clearmodifiers ctrl+a BackSpace
xdotool type --delay 1 '/absolute/path/to/file'
xdotool key --clearmodifiers Return
```

Important: run `type` and `key Return` as separate commands. If they are accidentally concatenated, text like `keyReturnkeyReturn` can be typed into the location field instead of submitting the path.

## UI Keys Reference (Optional)

If you are debugging using the widget tree (or building automation later), these keys are relevant:

- `AppKeys.workbenchAddContextButton`
- `AppKeys.contextOverviewScreen`
- `AppKeys.contextModeTextRadio`, `AppKeys.contextModeFileRadio`
- `AppKeys.contextTextField`, `AppKeys.contextFilePathField`, `AppKeys.contextNoteField`
- `AppKeys.contextProcessButton`, `AppKeys.contextReprocessButton`
- `AppKeys.contextDirectEditToggle`, `AppKeys.contextDirectSaveButton`
- `AppKeys.contextProcessingIndicator`
- `AppKeys.contextManualOverwriteConfirm`, `AppKeys.contextManualOverwriteCancel`
- `AppKeys.workbenchContextWarning`
- `AppKeys.contextCategoryCard(<category>)`
- `AppKeys.contextCategoryAddButton(<category>)`
- `AppKeys.contextCategoryEditButton(<category>)`
- `AppKeys.contextCategoryInspectButton(<category>)`
- `AppKeys.contextCategoryDeleteButton(<category>)`
- `AppKeys.contextArtifactField(<path>)`

## Workbench Context Behavior

- Four fixed categories, one slot per category: company-context, department-context, situation, document-style.
- Add/Reprocess uses synchronous model processing.
- Direct edit saves raw artifact files without skill-validation gate.
- Context mutations are blocked while Draft exists.
- Runtime prompt assembly always injects active context items.
- Clutter warning should appear when context share is high (`>= 0.35`).

## Common Setup (Recommended)

Use one Workbench for most cases (TC-160 through TC-170), and a separate fresh Workbench for TC-171.

1. Generate the fixtures (if not already generated):

   `engine/tools/pyworker/.venv/bin/python scripts/testdata/generate_workbench_context_fixtures.py`

2. Create a new Workbench named `WB Context - KeenBench Soap Opera`.
   Expected: The Workbench opens.
3. Add `docs/test/fixtures/workbench-context/keenbench_weekly_notes.txt` to the Workbench.
   Expected: The file appears in the file list.
4. Grant egress consent for the active provider/model by sending a harmless Workshop message and clicking "Continue" on the consent dialog.
   Expected: Consent is granted and an assistant response arrives. Timeout: 60 seconds.

   Message to send: `Reply with only: ok`
5. Ensure no Draft exists (discard or publish any Draft before running context-mutation tests).
   Expected: Composer is visible and context Add/Edit/Delete are enabled (unless the test case requires a Draft).

---

## Test Cases

### 18. Workbench Context

#### TC-160: Open context overview and verify fixed categories
- Priority: P0
- Preconditions: Workbench exists. No Draft. No context items yet.
- Steps:
  1. Open the target Workbench and click the sidebar button labeled "Add Context".
     Expected: The "Workbench Context" screen opens.
  2. Verify the four category cards are present:
     Expected: The cards are exactly: "Company-Wide Information", "Department-Specific Information", "Situation", and "Document Style & Formatting".
  3. On a fresh Workbench with no context items, verify empty-card actions.
     Expected: Each card shows an enabled "Add" button. "Inspect" and "Delete" are disabled for empty categories.

#### TC-161: Process company context from text and verify skill artifact contract `[AI]`
- Priority: P0
- Preconditions: Valid provider key configured and enabled. Egress consent already granted for the active provider/model. No Draft.
- Steps:
  1. Open "Add Context". On "Company-Wide Information", click "Add".
     Expected: A modal opens with "Write text" selected and a multi-line text field.
  2. Paste the full contents of `docs/test/fixtures/workbench-context/keenbench_company_context_v1.txt` into the text field.
     Expected: The pasted text includes `QA marker: KEENBENCH_COMPANY_CONTEXT=V1`.
  3. Click "Process".
     Expected: Processing completes within 120 seconds. The modal closes and "Company-Wide Information" becomes active with a non-empty summary (the card shows "Edit" and "Inspect" enabled).
  4. Click "Inspect" on "Company-Wide Information".
     Expected: The artifact includes at least `SKILL.md` and `references/summary.md`, both non-empty.
  5. In the `SKILL.md` section, validate the frontmatter is present.
     Expected: The content begins with YAML frontmatter and contains `name: company-context` plus a non-empty `description:`.
  6. Click "Close". Click "Edit" for "Company-Wide Information".
     Expected: The modal opens in "Write text" mode with the original source text pre-filled.
  7. Replace the text field contents with the full contents of `docs/test/fixtures/workbench-context/keenbench_company_context_v2.txt`, then click "Reprocess".
     Expected: Processing completes within 120 seconds, the card remains a single slot, and the summary/artifact updates (the Edit dialog now pre-fills with `QA marker: KEENBENCH_COMPANY_CONTEXT=V2`).

#### TC-162: Process department context from file mode with note persistence `[AI]`
- Priority: P0
- Preconditions: Valid provider configured and enabled. Egress consent already granted for the active provider/model. No Draft. Fixture file exists: `docs/test/fixtures/workbench-context/keenbench_engineering_department_brief.docx`.
- Steps:
  1. Open "Add Context". On "Department-Specific Information", click "Add".
     Expected: The processing modal opens.
  2. Switch to "Upload file", click "Choose file", and select `docs/test/fixtures/workbench-context/keenbench_engineering_department_brief.docx`.
     Expected: The selected path appears in the file path field.
  3. Paste this note exactly into the note field: `QA note: KEENBENCH_DEPT_NOTE=ENG-2026-02-13. Use this as Engineering department context; preserve terminology.`
     Expected: The note field contains the text exactly as entered.
  4. Click "Process".
     Expected: Processing completes within 120 seconds and the "Department-Specific Information" card becomes active with a non-empty summary.
  5. Click "Inspect" on "Department-Specific Information".
     Expected: The artifact includes at least `SKILL.md` and `references/summary.md`, both non-empty.
  6. Click "Close". Click "Edit" on "Department-Specific Information".
     Expected: The modal opens with "Upload file" selected and the note field pre-filled with `QA note: KEENBENCH_DEPT_NOTE=ENG-2026-02-13. Use this as Engineering department context; preserve terminology.` (the file path may show "No file selected" and is not required to be persisted).

#### TC-163: Process situation context and verify injection artifact shape `[AI]`
- Priority: P0
- Preconditions: Valid provider configured and enabled. Egress consent already granted for the active provider/model. No Draft.
- Steps:
  1. Open "Add Context". On "Situation", click "Add".
     Expected: The processing modal opens in "Write text" mode.
  2. Paste the full contents of `docs/test/fixtures/workbench-context/keenbench_situation_alpha.txt` into the text field.
     Expected: The pasted text includes `QA marker: KEENBENCH_SITUATION_TAG=HNDY-LAUNCH-ALPHA-4187`.
  3. Click "Process".
     Expected: Processing completes within 120 seconds and the "Situation" card becomes active with a non-empty summary.
  4. Click "Inspect" on "Situation".
     Expected: The artifact file list contains `context.md` (and does not require `SKILL.md`). `context.md` is non-empty.

#### TC-164: Process document-style context and verify style artifact shape `[AI]`
- Priority: P0
- Preconditions: Valid provider configured and enabled. Egress consent already granted for the active provider/model. No Draft.
- Steps:
  1. Open "Add Context". On "Document Style & Formatting", click "Add".
     Expected: The processing modal opens in "Write text" mode.
  2. Paste the full contents of `docs/test/fixtures/workbench-context/keenbench_document_style_v1.txt` into the text field.
     Expected: The pasted text includes `QA marker: KEENBENCH_DOC_STYLE=V1`.
  3. Click "Process".
     Expected: Processing completes within 120 seconds and the "Document Style & Formatting" card becomes active with a non-empty summary.
  4. Click "Inspect" on "Document Style & Formatting".
     Expected: The artifact includes `SKILL.md` and `references/style-rules.md`, both non-empty.
  5. In the `SKILL.md` section, validate the frontmatter is present.
     Expected: The content contains `name: document-style` plus a non-empty `description:`.

#### TC-165: Direct edit lifecycle and reprocess overwrite warning `[AI]`
- Priority: P0
- Preconditions: "Company-Wide Information" is active (TC-161 completed). No Draft.
- Steps:
  1. Open "Add Context". On "Company-Wide Information", click "Inspect".
     Expected: The inspect modal opens with a "Direct Edit" toggle and a `SKILL.md` section.
  2. Toggle "Direct Edit" ON.
     Expected: The text fields become editable and the "Save Direct Edit" button becomes enabled.
  3. In the `SKILL.md` field, append this exact line at the very end of the file (add a new line if needed): `MANUAL_EDIT_MARKER=KEENBENCH_CONTEXT_EDIT_1`
     Expected: The marker line is present in the field.
  4. Click "Save Direct Edit".
     Expected: Save succeeds, the modal closes, and the "Company-Wide Information" card shows a "Manually edited" indicator.
  5. Click "Edit" for "Company-Wide Information", then click "Reprocess".
     Expected: A confirmation dialog appears warning that reprocessing will overwrite manual edits, with "Cancel" and "Proceed".
  6. Click "Cancel".
     Expected: Reprocessing does not run. The "Manually edited" indicator remains.
  7. Click "Inspect" again and verify the marker still exists.
     Expected: `SKILL.md` still contains `MANUAL_EDIT_MARKER=KEENBENCH_CONTEXT_EDIT_1`.
  8. Click "Close". Click "Edit" again and click "Reprocess" again, then click "Proceed".
     Expected: Processing completes within 120 seconds. The "Manually edited" indicator clears. `SKILL.md` no longer contains `MANUAL_EDIT_MARKER=KEENBENCH_CONTEXT_EDIT_1` (manual edits were overwritten).

#### TC-166: Always-inject behavior and forward-only context updates in Workshop `[AI]`
- Priority: P0
- Preconditions: Valid provider configured and enabled. Egress consent already granted for the active provider/model. No Draft.
- Steps:
  1. Ensure "Situation" is active and up to date by reprocessing it with `docs/test/fixtures/workbench-context/keenbench_situation_alpha.txt`.
     Expected: Processing completes within 120 seconds.
  2. In Workshop, send this exact message: `Reply with only the full value of KEENBENCH_SITUATION_TAG from Workbench Context. Output only the value.`
     Expected: The assistant response contains `HNDY-LAUNCH-ALPHA-4187`. Timeout: 60 seconds.
  3. Reprocess "Situation" with `docs/test/fixtures/workbench-context/keenbench_situation_beta.txt`.
     Expected: Processing completes within 120 seconds.
  4. Send the same message again.
     Expected: The assistant response contains `HNDY-LAUNCH-BETA-9026`. Timeout: 60 seconds.
  5. Verify forward-only behavior in history.
     Expected: The earlier assistant response from step 2 still shows `HNDY-LAUNCH-ALPHA-4187` (it is not rewritten), while the latest response shows `HNDY-LAUNCH-BETA-9026`.

#### TC-167: Context mutations blocked while Draft exists `[AI]`
- Priority: P0
- Preconditions: At least one active context item exists. Valid provider configured and enabled. An editable workbench file is available to create a Draft.
- Steps:
  1. Add `docs/test/fixtures/workbench-context/keenbench_weekly_notes.txt` to the Workbench file list.
     Expected: The file appears in the file list as an editable text file.
  2. In Workshop, send this exact message to force a Draft: `Open keenbench_weekly_notes.txt and append a new bullet under Updates: "- QA marker: DRAFT_BLOCK_TEST=1". Save the file.`
     Expected: A Draft is created (Draft banner appears and the composer is no longer available). Timeout: 120 seconds.
  3. Click "Add Context" in the sidebar.
     Expected: The context overview opens, but "Add"/"Edit" and "Delete" buttons are disabled (tooltip indicates Draft must be published or discarded).
  4. Click "Inspect" on an active context item (for example, "Company-Wide Information").
     Expected: In the inspect modal, "Direct Edit" toggle is disabled, "Reprocess" is disabled, and "Save Direct Edit" is disabled.

#### TC-168: Context file-input variants (supported and blocked) `[AI]`
- Priority: P1
- Preconditions: Valid provider configured and enabled. Egress consent already granted for the active provider/model. No Draft. Fixture generator has been run (`scripts/testdata/generate_workbench_context_fixtures.py`) so the required files exist.
- Steps:
  1. Open "Add Context" to reach the "Workbench Context" screen.
     Expected: The four category cards are visible.
  2. Process Company-Wide from a DOCX file:
     Expected: Processing succeeds within 120 seconds and "Company-Wide Information" becomes active. "Inspect" shows `SKILL.md` and `references/summary.md` (both non-empty).

     - Click "Add" (or "Edit") on "Company-Wide Information"
     - Select "Upload file"
     - Choose `docs/test/fixtures/workbench-context/keenbench_company_overview.docx`
     - Click "Process"/"Reprocess"
  3. Repeat step 2 for Department from a PDF file:
     Expected: Processing succeeds within 120 seconds and "Department-Specific Information" becomes active. "Inspect" shows `SKILL.md` and `references/summary.md` (both non-empty).

     - "Department-Specific Information" -> "Add"/"Edit" -> "Upload file"
     - Choose `docs/test/fixtures/workbench-context/keenbench_security_overview.pdf`
     - Click "Process"/"Reprocess"
  4. Repeat step 2 for Situation from a CSV file:
     Expected: Processing succeeds within 120 seconds and "Situation" becomes active. "Inspect" shows `context.md` (non-empty).

     - "Situation" -> "Add"/"Edit" -> "Upload file"
     - Choose `docs/test/fixtures/workbench-context/keenbench_launch_plan.csv`
     - Click "Process"/"Reprocess"
  5. Repeat step 2 for Document Style from a PPTX file:
     Expected: Processing succeeds within 120 seconds and "Document Style & Formatting" becomes active. "Inspect" shows `SKILL.md` and `references/style-rules.md` (both non-empty).

     - "Document Style & Formatting" -> "Add"/"Edit" -> "Upload file"
     - Choose `docs/test/fixtures/workbench-context/keenbench_pitch_deck.pptx`
     - Note: `Focus on layout and heading rules; preserve hex colors.`
     - Click "Process"/"Reprocess"
  6. Repeat step 2 for Company-Wide from an XLSX file:
     Expected: Processing succeeds within 120 seconds. Company-Wide remains active and "Inspect" shows `SKILL.md` and `references/summary.md` (both non-empty).

     - "Company-Wide Information" -> "Edit" -> "Upload file"
     - Choose `docs/test/fixtures/workbench-context/keenbench_company_metrics.xlsx`
     - Click "Reprocess"
  7. Attempt to reprocess Company-Wide with an image file (should be blocked):
     Expected: A "Processing failed" dialog appears (mentions unsupported image inputs or unsupported file type). After clicking "Cancel", Company-Wide remains active and unchanged (Inspect still shows the last successful artifact files).

     - "Company-Wide Information" -> "Edit" -> "Upload file"
     - Choose `docs/test/fixtures/workbench-context/keenbench_logo.png`
     - Click "Reprocess"
  8. Repeat step 7 with an unknown binary file:
     Expected: A "Processing failed" dialog appears (mentions unsupported file type). After clicking "Cancel", Company-Wide remains active and unchanged.

     - Choose `docs/test/fixtures/workbench-context/keenbench_unknown.bin`
  9. Repeat step 7 with a >25MB CSV file:
     Expected: A "Processing failed" dialog appears (mentions size limit, 25MB). After clicking "Cancel", Company-Wide remains active and unchanged.

     - Choose `artifacts/testdata/keenbench_oversize_roster.csv`
  10. Create a symlink and repeat step 7 with the symlink path:
      Expected: A "Processing failed" dialog appears (mentions symlinks are not allowed). After clicking "Cancel", Company-Wide remains active and unchanged.

      - Create symlink (Linux/macOS): `ln -s "$(pwd)/docs/test/fixtures/workbench-context/keenbench_company_context_v1.txt" /tmp/keenbench_company_context_link.txt`
      - Choose `/tmp/keenbench_company_context_link.txt`

#### TC-169: Clutter warning reflects large context share
- Priority: P1
- Preconditions: Active situation context exists. No Draft.
- Steps:
  1. Open "Add Context". On "Situation", click "Inspect".
     Expected: The inspect modal opens.
  2. Toggle "Direct Edit" ON.
     Expected: The `context.md` field becomes editable and "Save Direct Edit" becomes enabled.
  3. Replace the entire `context.md` content with the full contents of `artifacts/testdata/keenbench_situation_clutter_payload.md`.
     Expected: The text field contains a very large payload (much larger than a typical context item).
  4. Click "Save Direct Edit".
     Expected: Save succeeds. The "Situation" card shows a "Manually edited" indicator.
  5. Return to the Workbench screen.
     Expected: A warning appears: "Context is using a large share of the prompt window. Consider shortening context items."

#### TC-170: Processing failure recovery (Retry/Cancel) with no partial save `[AI]`
- Priority: P1
- Preconditions: Category is empty. Valid provider configured and enabled. Egress consent already granted for the active provider/model. No Draft.
- Steps:
  1. Open "Add Context". On "Company-Wide Information", click "Add". Paste `docs/test/fixtures/workbench-context/keenbench_company_context_v1.txt` into the text field.
     Expected: The dialog is ready to process (no Draft exists).
  2. Disconnect the machine from the network (for example: turn off WiFi or unplug the network).
     Expected: The OS indicates the machine is offline.
  3. Click "Process".
     Expected: A "Processing failed" dialog appears within 30 seconds, with error text indicating a network/provider failure. The dialog offers "Retry" and "Cancel".
  4. Click "Retry" once (while still offline).
     Expected: The request fails again and the failure dialog reappears.
  5. Click "Cancel" on the failure dialog.
     Expected: No context item is saved; "Company-Wide Information" remains empty (card still shows "Add", and "Inspect"/"Delete" are disabled).
  6. Reconnect the machine to the network.
     Expected: The OS indicates the machine is online.
  7. In the same processing dialog, click "Process" again.
     Expected: Processing succeeds within 120 seconds and "Company-Wide Information" becomes active.

#### TC-171: Context processing enforces egress consent scope `[AI]`
- Priority: P1
- Preconditions: Valid provider key configured and enabled. Consent mode is `ask` (Settings toggle off). Fresh Workbench with no granted Workshop consent for the active provider/model.
- Steps:
  1. Create a brand-new Workbench (new name) so there is no prior consent for this scope.
     Expected: The new Workbench opens with an empty conversation.
  2. Add `docs/test/fixtures/workbench-context/keenbench_weekly_notes.txt` to the Workbench file list.
     Expected: The file appears in the file list.
  3. Without sending any Workshop message yet, attempt to process Company-Wide context:
     Expected: Processing fails with an error that includes `EGRESS_CONSENT_REQUIRED` (or equivalent consent-required wording), and the Company-Wide category remains empty.

     - Open "Add Context" -> "Company-Wide Information" -> "Add"
     - Paste `docs/test/fixtures/workbench-context/keenbench_company_context_v1.txt`
     - Click "Process"
  4. Close the context dialog(s). Trigger the consent flow from Workshop by sending a harmless message: `Reply with only: ok`
     Expected: A "Consent required" dialog appears listing the workbench file(s). Click "Continue" to grant consent.
  5. Return to "Add Context" and retry the Company-Wide processing from step 3.
     Expected: Processing succeeds within 120 seconds and the Company-Wide context item becomes active.

#### TC-172: Context processing skips prompt in global consent mode `[AI]`
- Priority: P2
- Preconditions: Valid provider key configured and enabled. Enable Settings toggle `AppKeys.settingsConsentModeToggle` (global consent mode on). Fresh Workbench with files.
- Steps:
  1. Open "Add Context" -> "Company-Wide Information" -> "Add".
     Expected: Processing modal opens.
  2. Paste `docs/test/fixtures/workbench-context/keenbench_company_context_v1.txt` and click "Process".
     Expected: No consent dialog appears. Processing completes successfully and the Company-Wide context item becomes active.
