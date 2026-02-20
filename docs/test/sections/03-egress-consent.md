# KeenBench — Test Plan: Egress Consent

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

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

Key elements for this section:
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.consentDialog`, `AppKeys.consentFileList`, `AppKeys.consentScopeHash`
- `AppKeys.consentCancelButton`, `AppKeys.consentContinueButton`
- `AppKeys.workbenchMessageList`
- `AppKeys.settingsConsentModeToggle`

## Egress Consent Behavior

- Consent is per-workbench, per-scope (file set hash), and per-provider/model.
- The consent dialog shows provider name, model name, file list with sizes, and scope hash.
- Consent persists for the same scope (same files) and same provider/model.
- Adding/removing files changes the scope hash and invalidates prior consent.
- Switching provider or model also requires new consent.
- Global consent mode is opt-in in Settings. Default remains prompt-per-scope (`ask`).

---

## Test Cases

### 4. Egress Consent

#### TC-020: Consent prompt on first model call `[AI]`
- Priority: P0
- Preconditions: Valid OpenAI key configured. Workbench has `notes.txt` and `data.csv`. No prior consent.
- Steps:
  1. Click the composer field (`AppKeys.workbenchComposerField`) and type "Summarize the files."
     Expected: The text appears in the composer.
  2. Click Send (`AppKeys.workbenchSendButton`).
     Expected: The consent dialog appears (`AppKeys.consentDialog`) with title "Consent required".
  3. Verify the dialog shows the provider name ("OpenAI") and model name in the description text.
     Expected: The text reads "KeenBench will send Workbench content to OpenAI (gpt-4o-mini) to generate responses." (model name may vary).
  4. Verify the file list (`AppKeys.consentFileList`) shows both files with sizes.
     Expected: Two entries: "notes.txt (N bytes)" and "data.csv (N bytes)" where N reflects actual file sizes.
  5. Verify the scope hash (`AppKeys.consentScopeHash`) is displayed.
     Expected: Text reads "Scope hash: <hex string>".

#### TC-021: Cancel consent blocks model call `[AI]`
- Priority: P0
- Preconditions: Consent dialog is visible (from TC-020 step 2).
- Steps:
  1. Click "Cancel" (`AppKeys.consentCancelButton`).
     Expected: The dialog closes. No assistant message appears in the message list. The composer field is still visible and enabled.
  2. Verify the message list has no assistant messages.
     Expected: The message list (`AppKeys.workbenchMessageList`) does not contain any assistant-role messages.

#### TC-022: Grant consent and receive response `[AI]`
- Priority: P0
- Preconditions: Workbench with files, no prior consent.
- Steps:
  1. Type "Summarize the staffing decisions from the roster and notes." in the composer and click Send.
     Expected: The consent dialog appears.
  2. Click "Continue" (`AppKeys.consentContinueButton`).
     Expected: The dialog closes. The user message appears in the message list. After 5-30 seconds, an assistant message begins streaming (text appearing incrementally).
  3. Wait for the assistant response to complete (streaming stops, send button re-enables).
     Expected: The assistant message is complete. It contains text (not empty). The message references staffing, employees, or project-related content (structural assertion: the response is contextually relevant to the files provided). Timeout: 60 seconds.

#### TC-023: Consent persists for same scope `[AI]`
- Priority: P1
- Preconditions: Consent was granted in TC-022.
- Steps:
  1. Type "List any open roles or gaps." in the composer and click Send.
     Expected: NO consent dialog appears. The message sends immediately. An assistant response streams back. Timeout: 60 seconds.
  2. Verify there are now at least 2 assistant messages in the conversation.
     Expected: The message list contains the responses from both TC-022 and this test.

#### TC-024: Consent invalidates on scope change `[AI]`
- Priority: P0
- Preconditions: Consent previously granted for 2 files.
- Steps:
  1. Add a new file `scope_change.txt` (a small text file) to the workbench via `WorkbenchFilesAdd`.
     Expected: The file appears in the file list. File count is now 3.
  2. Type "What changed?" in the composer and click Send.
     Expected: The consent dialog (`AppKeys.consentDialog`) reappears. The file list now shows 3 files including `scope_change.txt`.
  3. Click "Continue" to re-grant consent.
     Expected: The dialog closes. The message sends. An assistant response arrives. Timeout: 60 seconds.

#### TC-025: Consent required when switching provider/model `[AI]`
- Priority: P1
- Preconditions: Consent granted for OpenAI. Anthropic key also configured. Workbench has files.
- Steps:
  1. Change the active model to an Anthropic model via the model selector dropdown in the workbench app bar.
     Expected: The model selector updates to show the Anthropic model name.
  2. Type "Describe the files." in the composer and click Send.
     Expected: The consent dialog reappears showing "Anthropic" as the provider name and the new model name. The file list and scope hash are displayed.
  3. Click "Continue".
     Expected: The message sends. The assistant response arrives from the Anthropic model. Timeout: 90 seconds.

#### TC-026: Global consent mode skips prompt in Workshop `[AI]`
- Priority: P1
- Preconditions: Valid provider key configured. Workbench has files. No prior scoped consent.
- Steps:
  1. Open Settings and enable `AppKeys.settingsConsentModeToggle`.
     Expected: Confirmation dialog appears. Confirm enable.
  2. Return to Workbench. Type "Summarize the files." and click Send.
     Expected: No consent dialog appears. Message sends immediately and assistant response arrives.

#### TC-027: Disabling global consent mode restores prompt behavior `[AI]`
- Priority: P1
- Preconditions: TC-026 completed and global consent mode is enabled.
- Steps:
  1. Open Settings and disable `AppKeys.settingsConsentModeToggle`.
     Expected: Toggle turns off without extra confirmation.
  2. Add or remove a file to force a new scope hash.
     Expected: Workbench scope changes.
  3. Send a new message.
     Expected: Consent dialog appears again before model call.
