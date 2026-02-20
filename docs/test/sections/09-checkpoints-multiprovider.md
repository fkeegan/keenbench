# KeenBench — Test Plan: Checkpoints and Multi-Provider

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
- `AppKeys.workbenchCheckpointsButton`, `AppKeys.checkpointsScreen`
- `AppKeys.checkpointsList`, `AppKeys.checkpointsCreateButton`
- `AppKeys.settingsScreen`, `AppKeys.settingsApiKeyField`, `AppKeys.settingsSaveButton`
- `AppKeys.settingsProviderStatus`, `AppKeys.settingsProviderToggle`
- `AppKeys.settingsOAuthStatusText('openai-codex')`, `AppKeys.settingsOAuthConnectButton('openai-codex')`
- `AppKeys.settingsOAuthDisconnectButton('openai-codex')`, `AppKeys.settingsOAuthRedirectField('openai-codex')`
- `AppKeys.settingsOAuthCompleteButton('openai-codex')`
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.consentDialog`, `AppKeys.consentContinueButton`
- `AppKeys.providerRequiredDialog`, `AppKeys.providerRequiredOpenSettings`

## Checkpoint Behavior

- Checkpoints capture the current state of published files.
- Creating a checkpoint: manual via UI button, or automatic on publish.
- Restoring a checkpoint: reverts published files to the captured state.
- Restore is blocked while a draft exists.

## Multi-Provider Behavior

- Supported providers: OpenAI, OpenAI Codex, Anthropic, Google Gemini, Mistral.
- OpenAI, Anthropic, Google Gemini, and Mistral use API keys. OpenAI Codex uses OAuth browser authentication.
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

#### TC-093: OpenAI Codex OAuth connect via browser callback `[MANUAL ONLY]`
- Priority: P1
- Runner: Human only
- Preconditions: Settings screen open. OpenAI Codex shows "Not connected".
- Steps:
  1. Click Connect on OpenAI Codex (`AppKeys.settingsOAuthConnectButton('openai-codex')`).
     Expected: Browser authorization flow starts.
  2. Complete authentication and consent in the browser.
     Expected: The app captures callback automatically when available.
  3. Return to Settings and verify OpenAI Codex status text (`AppKeys.settingsOAuthStatusText('openai-codex')`).
     Expected: Status shows connected state ("Connected" or "Connected as <account>").

#### TC-094: OpenAI Codex OAuth manual redirect fallback `[MANUAL ONLY]`
- Priority: P1
- Runner: Human only
- Preconditions: Settings screen open. OpenAI Codex disconnected.
- Steps:
  1. Start OpenAI Codex Connect flow and force manual completion path (for example, when callback capture is unavailable or times out).
     Expected: A dialog asks for redirect URL paste.
  2. Complete browser auth and paste the full redirect URL into the app dialog (`AppKeys.settingsOAuthRedirectField('openai-codex')`), then submit (`AppKeys.settingsOAuthCompleteButton('openai-codex')`).
     Expected: Connection completes successfully.
  3. Verify status text.
     Expected: OpenAI Codex status shows connected.

#### TC-095: OpenAI Codex OAuth disconnect `[MANUAL ONLY]`
- Priority: P1
- Runner: Human only
- Preconditions: OpenAI Codex is connected.
- Steps:
  1. Click Disconnect (`AppKeys.settingsOAuthDisconnectButton('openai-codex')`).
     Expected: Disconnect completes without error.
  2. Verify status text.
     Expected: OpenAI Codex shows "Not connected".

#### TC-096: OAuth provider-required dialog path
- Priority: P1
- Preconditions: OpenAI Codex is selected as active model in Workbench, but OpenAI Codex is not connected.
- Steps:
  1. Type any message and click Send.
     Expected: Provider-required dialog appears (`AppKeys.providerRequiredDialog`).
  2. Inspect dialog title and body.
     Expected: Dialog indicates OAuth auth is required for OpenAI Codex (authentication required / connect in Settings), not API key entry.
  3. Click "Open Settings" (`AppKeys.providerRequiredOpenSettings`).
     Expected: Settings screen opens.
