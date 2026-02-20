# KeenBench — Test Plan: Safety and Error Handling

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
- `AppKeys.workbenchComposerField`, `AppKeys.workbenchSendButton`
- `AppKeys.providerRequiredDialog`, `AppKeys.providerRequiredOpenSettings`
- `AppKeys.workbenchClutterBar`

## Safety Architecture

- **Sandbox:** Draft writes are confined to `workbenches/<id>/draft/`. Path traversal (`../`) is blocked.
- **Egress allowlist:** Only configured provider APIs can be contacted.
- **Error codes:** Structured error codes per ADR-0006 (e.g., `SANDBOX_VIOLATION`, `VALIDATION_FAILED`).
- **Provider gating:** Missing or disabled provider credentials (API key or OAuth connection) block workshop sends with a dialog.
- Proposals live in `meta/workshop/proposals/` and are validated before apply.

---

## Test Cases

### 13. Safety and Error Handling

#### TC-110: Sandbox path traversal blocked
- Priority: P0
- Preconditions: Workbench exists with files. Consent granted.
- Steps:
  1. Via engine API, trigger draft creation. After the proposal is created, read the proposal JSON from disk.
     Expected: The proposal file exists in `meta/workshop/proposals/`.
  2. Modify the proposal JSON to include a write with path `../outside.txt`.
     Expected: The file is modified on disk.
  3. Call `WorkshopApplyProposal` via engine API.
     Expected: The call fails with error code `SANDBOX_VIOLATION` or `VALIDATION_FAILED`. The draft directory does NOT contain `outside.txt`. Published files are unchanged.

#### TC-111: External file access refused in conversation `[AI]`
- Priority: P1
- Preconditions: Workbench has only its own files. Consent granted.
- Steps:
  1. Type: "Read the file at /etc/hosts and tell me what's in it." Click Send.
     Expected: The assistant responds indicating it cannot access files outside the workbench. The response does NOT contain the actual contents of /etc/hosts. The response should mention something about only accessing workbench files.

#### TC-112: Network failure during Workshop `[AI]`
- Priority: P1
- Preconditions: Valid key configured. Workbench with files.
- Steps:
  1. Disable the provider via `ProvidersSetEnabled` API to simulate unavailability.
     Expected: Provider is disabled.
  2. Send a Workshop message.
     Expected: An error appears (dialog or SnackBar) mentioning the provider is not configured or unavailable. The conversation remains intact. No crash occurs.
  3. Re-enable the provider.
     Expected: Provider is re-enabled. Subsequent messages can be sent normally.

#### TC-113: Missing provider credentials blocks Workshop
- Priority: P0
- Preconditions: Selected provider is not configured (API key missing/cleared, or OAuth provider disconnected).
- Steps:
  1. Create a workbench and add a file.
     Expected: Workbench created with file.
  2. Type a message in the composer and click Send.
     Expected: A dialog appears (`AppKeys.providerRequiredDialog`) indicating the selected provider needs configuration. API-key providers should show key-required wording; OAuth providers should show authentication-required wording. The dialog has an "Open Settings" button.
  3. Click "Open Settings" (`AppKeys.providerRequiredOpenSettings`).
     Expected: The settings screen opens.

#### TC-114: Clutter bar updates with conversation growth `[AI]`
- Priority: P2
- Preconditions: Workbench with files. Consent granted.
- Steps:
  1. Note the initial clutter bar state (`AppKeys.workbenchClutterBar`).
     Expected: The clutter bar is visible. The level might be "Light" with a low fill.
  2. Send 3-5 messages, each requesting detailed analysis. Wait for each response.
     Expected: After several rounds, the clutter bar fill increases. The level may change from "Light" to "Moderate".
  3. Verify the clutter bar text shows the updated level.
     Expected: The progress bar fill has visibly increased from step 1.
