# KeenBench — Test Plan: Checkpoints and Multi-Provider

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
- `AppKeys.workbenchCheckpointsButton`, `AppKeys.checkpointsScreen`
- `AppKeys.checkpointsList`, `AppKeys.checkpointsCreateButton`
- `AppKeys.settingsScreen`, `AppKeys.settingsApiKeyField`, `AppKeys.settingsSaveButton`
- `AppKeys.settingsProviderStatus`, `AppKeys.settingsProviderToggle`
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.consentDialog`, `AppKeys.consentContinueButton`

## Checkpoint Behavior

- Checkpoints capture the current state of published files.
- Creating a checkpoint: manual via UI button, or automatic on publish.
- Restoring a checkpoint: reverts published files to the captured state.
- Restore is blocked while a draft exists.

## Multi-Provider Behavior

- Supported providers: OpenAI, Anthropic, Google Gemini, Mistral.
- Each provider requires its own API key, configured in Settings.
- The model selector in the workbench app bar allows switching between configured providers/models.
- Consent is per-provider/model — switching requires new consent.

---

## Test Cases

### 10. Checkpoints

#### TC-080: Create manual checkpoint
- Priority: P1
- Preconditions: Workbench with published files, no Draft.
- Steps:
  1. Click the Checkpoints button (`AppKeys.workbenchCheckpointsButton`).
     Expected: Checkpoints screen opens.
  2. Click "Create checkpoint" (`AppKeys.checkpointsCreateButton`).
     Expected: A dialog appears with a text field for description and Create/Cancel buttons.
  3. Type "Before analysis changes" in the description field and click Create.
     Expected: The dialog closes. A new checkpoint appears in the list with description "Before analysis changes" and a timestamp.

#### TC-081: Restore checkpoint
- Priority: P1
- Preconditions: At least one checkpoint exists. Workbench has published files that differ from the checkpoint state.
- Steps:
  1. Open the checkpoints screen.
     Expected: At least one checkpoint is listed.
  2. Click "Restore" on the earliest checkpoint.
     Expected: A confirmation dialog appears asking to confirm the restore.
  3. Click "Restore" in the confirmation dialog.
     Expected: The dialog closes. A SnackBar or system event message confirms the restore. The workbench returns to the state captured in that checkpoint.
  4. Verify the published files match the checkpoint state.
     Expected: Files that were added after the checkpoint are removed. Files that existed at the checkpoint time are present with their original content.

#### TC-082: Restore blocked while Draft exists
- Priority: P1
- Preconditions: Draft exists. Checkpoints exist.
- Steps:
  1. Open the checkpoints screen.
     Expected: Checkpoints are listed.
  2. Verify the "Create checkpoint" button is disabled.
     Expected: The button is grayed out with tooltip "Publish or discard Draft to create a checkpoint."
  3. Verify the "Restore" button on each checkpoint is disabled.
     Expected: Each restore button is disabled with tooltip "Publish or discard Draft to restore."

---

### 11. Multi-Provider

#### TC-090: Configure Anthropic key
- Priority: P1
- Preconditions: Settings screen open. Valid `KEENBENCH_ANTHROPIC_API_KEY` available.
- Steps:
  1. Locate the Anthropic provider card in Settings.
     Expected: The card is visible with provider name "Anthropic".
  2. Enter the Anthropic API key in the key field and click Save & Validate.
     Expected: Validation succeeds. Provider status shows "Configured".

#### TC-091: Configure Gemini key
- Priority: P1
- Preconditions: Settings screen open. Valid `KEENBENCH_GEMINI_API_KEY` available.
- Steps:
  1. Locate the Google Gemini provider card in Settings.
     Expected: The card is visible.
  2. Enter the Gemini API key and save.
     Expected: Validation succeeds. Provider status shows "Configured".

#### TC-092: Switch model during conversation `[AI]`
- Priority: P1
- Preconditions: OpenAI and Anthropic both configured. Workbench with files.
- Steps:
  1. Send a message with OpenAI selected (the default). Grant consent.
     Expected: Assistant response arrives from OpenAI.
  2. Switch the model selector to an Anthropic model.
     Expected: Model selector shows the new model name.
  3. Send another message.
     Expected: Consent dialog appears for Anthropic (new provider). Grant consent. The assistant response arrives. Timeout: 90 seconds.
  4. Verify both responses are in the conversation history.
     Expected: Two assistant messages are visible, from different providers.
