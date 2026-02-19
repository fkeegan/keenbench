# KeenBench — Test Plan: Setup, Smoke, and Settings

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

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

All testable elements use AppKeys from `app/lib/app_keys.dart`. Key elements for this section:
- `AppKeys.homeScreen`, `AppKeys.homeEmptyState`, `AppKeys.homeWorkbenchGrid`
- `AppKeys.homeSettingsButton`, `AppKeys.homeNewWorkbenchButton`
- `AppKeys.settingsScreen`, `AppKeys.settingsProviderStatus`
- `AppKeys.settingsApiKeyField`, `AppKeys.settingsSaveButton`
- `AppKeys.settingsProviderToggle`
- `AppKeys.workbenchScreen`, `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.workbenchSettingsButton`
- `AppKeys.providerRequiredDialog`, `AppKeys.providerRequiredOpenSettings`, `AppKeys.providerRequiredCancel`

---

## Test Cases

### 1. Setup and Smoke

#### TC-001: Launch app and engine handshake
- Priority: P0
- Preconditions: Fresh app data dir. Engine binary built.
- Steps:
  1. Launch the app via `make run` or `flutter run -d linux`.
     Expected: The home screen appears (`AppKeys.homeScreen` is visible). No error dialogs or crash logs.
  2. Verify the home screen shows "Create a Workbench to begin." (`AppKeys.homeEmptyState`).
     Expected: The empty state message is displayed. The workbench grid (`AppKeys.homeWorkbenchGrid`) is empty.
  3. Verify the Settings button (`AppKeys.homeSettingsButton`) is visible and enabled.
     Expected: The settings icon button is rendered in the app bar.

#### TC-002: Open Settings and view initial provider status
- Priority: P0
- Preconditions: App launched, home screen visible.
- Steps:
  1. Click the Settings button (`AppKeys.homeSettingsButton`).
     Expected: The Settings screen (`AppKeys.settingsScreen`) opens with title "Settings".
  2. Locate the OpenAI provider card.
     Expected: The card displays "OpenAI" as the provider name.
  3. Read the provider status text (`AppKeys.settingsProviderStatus`).
     Expected: The status reads "Not configured" (displayed in orange/warning color).
  4. Verify the API key field (`AppKeys.settingsApiKeyField`) is empty.
     Expected: The text field shows the placeholder "Keys are stored locally and encrypted at rest" with no value.

---

### 2. Settings and Provider Key

#### TC-003: Save invalid OpenAI key
- Priority: P0
- Preconditions: Settings screen open. No key previously configured.
- Steps:
  1. Click on the API key field (`AppKeys.settingsApiKeyField`).
     Expected: The text field gains focus and shows a cursor.
  2. Type `sk-invalid-test-key-12345` into the field.
     Expected: The text appears in the field as typed (obscured).
  3. Click the "Save & Validate" button (`AppKeys.settingsSaveButton`).
     Expected: A loading indicator appears on the button. After 2-10 seconds, a SnackBar appears with an error message containing "invalid" or "auth" or "failed".
  4. Read the provider status text (`AppKeys.settingsProviderStatus`).
     Expected: The status still reads "Not configured".

#### TC-004: Save valid OpenAI key
- Priority: P0
- Preconditions: Settings screen open. Valid `KEENBENCH_OPENAI_API_KEY` available in `.env`.
- Steps:
  1. Clear the API key field (`AppKeys.settingsApiKeyField`) and type the valid key from `KEENBENCH_OPENAI_API_KEY`.
     Expected: The key text appears in the field (obscured).
  2. Click "Save & Validate" (`AppKeys.settingsSaveButton`).
     Expected: A loading indicator appears. After 2-15 seconds, a SnackBar appears with text "Key saved and validated." or "OpenAI key saved."
  3. Read the provider status text (`AppKeys.settingsProviderStatus`).
     Expected: The status reads "Configured" (displayed in green/success color).
  4. Verify the provider toggle (`AppKeys.settingsProviderToggle`) is in the ON position.
     Expected: The switch widget is toggled ON.

#### TC-005: Provider enable/disable gating
- Priority: P1
- Preconditions: Valid OpenAI key configured. A workbench exists with at least one file.
- Steps:
  1. From the workbench screen, click the Settings button (`AppKeys.workbenchSettingsButton`) in the bottom-left of the sidebar.
     Expected: The Settings screen (`AppKeys.settingsScreen`) opens.
  2. Locate the OpenAI provider toggle (`AppKeys.settingsProviderToggle`) and click it to switch it OFF.
     Expected: The toggle animates to the OFF position. The provider status may update.
  3. Click the back button to return to the workbench screen.
     Expected: The workbench screen (`AppKeys.workbenchScreen`) is displayed with the file list and composer visible.
  4. Click the composer field (`AppKeys.workbenchComposerField`) and type "Hello".
     Expected: The text "Hello" appears in the composer field.
  5. Click the Send button (`AppKeys.workbenchSendButton`).
     Expected: A dialog appears (`AppKeys.providerRequiredDialog`) with title containing "key required" and text indicating the provider needs to be configured. The dialog has an "Open Settings" button (`AppKeys.providerRequiredOpenSettings`).
  6. Click "Cancel" (`AppKeys.providerRequiredCancel`) to dismiss the dialog.
     Expected: The dialog closes. The composer still contains "Hello". No assistant message was generated.
  7. Click the Settings button, re-enable the OpenAI toggle, and return to the workbench.
     Expected: The provider is re-enabled. The workbench screen is displayed.
  8. Click Send again (the composer should still have "Hello" or re-type it).
     Expected: The consent dialog appears (if first message) or the message sends and an assistant response begins streaming.

#### TC-006: Key persistence across restart
- Priority: P0
- Preconditions: Valid OpenAI key saved and validated.
- Steps:
  1. Note the current provider status ("Configured").
     Expected: Status is "Configured".
  2. Quit the app completely (close the window or terminate the process).
     Expected: The app window closes.
  3. Relaunch the app.
     Expected: The home screen appears.
  4. Click Settings (`AppKeys.homeSettingsButton`).
     Expected: The Settings screen opens.
  5. Read the provider status text (`AppKeys.settingsProviderStatus`).
     Expected: The status reads "Configured". The key was persisted (encrypted at rest) and does not need re-entry.
