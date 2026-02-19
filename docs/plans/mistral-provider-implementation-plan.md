# Implementation Plan: Mistral API-Key Provider

## Status
Implemented (2026-02-19)

## Summary
Add Mistral as a first-class API-key provider in KeenBench with one supported model ID, full provider status/validation integration, encrypted key storage, allowlisted egress, and updated test/docs fixtures.

This implementation keeps the existing architecture intact:
- Provider ID: `mistral`
- Model ID (registry/UI/consent): `mistral:mistral-large`
- Provider API model alias (at call time): `mistral-large-latest`
- Auth mode: `api_key`

## Scope

### In Scope
1. Engine provider/model registry update.
2. New Mistral client implementing `LLMClient`.
3. Encrypted secrets + settings backfill for Mistral key/provider settings.
4. Provider status and validation wiring through existing JSON-RPC methods.
5. Unit/widget test fixture updates for provider/model lists.
6. `.env.example`, test policy docs, and test-plan docs updates.

### Out of Scope
1. OAuth for Mistral.
2. Dynamic model discovery from provider APIs.
3. Additional Mistral model variants beyond `mistral:mistral-large`.

## Design Decisions
1. Keep app-level model ID stable (`mistral:mistral-large`) to match existing PRD/design docs.
2. Route Mistral API calls to `mistral-large-latest` in `providerModelName()` for quality-first behavior.
3. Reuse the existing chat/tool abstraction (`LLMClient`) without introducing provider-specific execution paths.
4. Enforce network policy with `api.mistral.ai` host allowlist and existing HTTPS/IP-literal restrictions.
5. Keep RPI reasoning-effort controls OpenAI-only; Mistral has no reasoning-effort settings.

## Implementation Breakdown

### 1) Engine Model and Provider Registry
- `engine/internal/engine/models_registry.go`
  - Added `ProviderMistral`.
  - Added `ModelMistralID = "mistral:mistral-large"`.
  - Added model metadata entry and inclusion in `listSupportedModels()`.
  - Added `providerModelName()` mapping from `mistral:mistral-large` -> `mistral-large-latest`.

- `engine/internal/engine/engine.go`
  - Registered `mistral.NewClient()` in `providers`.
  - Added Mistral entry to `ProvidersGetStatus()` provider list with `auth_mode="api_key"`.

### 2) Provider Key and Settings Wiring
- `engine/internal/engine/providers_helpers.go`
  - Added Mistral branches in `providerKey()` and `setProviderKey()`.

- `engine/internal/secrets/secrets.go`
  - Added encrypted `MistralKey` field.
  - Added `GetMistralKey()` / `SetMistralKey()`.
  - Added `ClearProviderKey("mistral")`.

- `engine/internal/settings/settings.go`
  - Added `providerMistral` constant.
  - Included Mistral in defaults and backfill.
  - Left reasoning-effort normalization unchanged (OpenAI/OpenAI-Codex only).

### 3) New Mistral Client
- `engine/internal/mistral/client.go` (new)
  - Implemented `ValidateKey`, `Chat`, `StreamChat`, `ChatWithTools`, `StreamChatWithTools`.
  - Implemented request/response mapping for Mistral chat completions.
  - Added robust tool-call argument normalization (string/object).
  - Added `api.mistral.ai` allowlist transport.
  - Reused standard llm error mappings (`ErrUnauthorized`, `ErrRateLimited`, `ErrUnavailable`, `ErrEgressBlocked`).

### 4) Tests and Fixtures
- Go tests:
  - `engine/internal/mistral/client_test.go` (new)
  - `engine/internal/secrets/secrets_test.go` (Mistral key roundtrip + clear)
  - `engine/internal/settings/settings_test.go` (default + backfill coverage)
  - `engine/internal/engine/engine_rpc_test.go` (provider status includes Mistral)

- Flutter widget fixtures:
  - `app/test/settings_screen_test.dart` (Mistral provider/model fixture + API key save test)
  - `app/test/workbench_review_checkpoint_test.dart` (default fixtures include Mistral)
  - `app/test/widget_test.dart` (provider/model fixtures include Mistral)

### 5) Docs and Env
- `.env.example`
  - Added `KEENBENCH_MISTRAL_API_KEY=`.

- Policy and test docs:
  - `CLAUDE.md` multi-provider key requirements include Mistral.
  - `docs/test/test-plan.md` and `docs/test/sections/*.md` include:
    - `api.mistral.ai` in network requirements.
    - `KEENBENCH_MISTRAL_API_KEY` in multi-provider key requirements.

## Public Interface Impact
No new RPC methods were added. Existing methods now include Mistral data:
1. `ProvidersGetStatus` returns a `mistral` provider entry.
2. `ModelsListSupported` returns `mistral:mistral-large`.
3. Existing key methods (`ProvidersSetApiKey`, `ProvidersValidate`, `ProvidersClearApiKey`, `ProvidersSetEnabled`) now accept `provider_id="mistral"`.

## Validation Plan
1. Run Go unit tests for affected packages:
   - `engine/internal/mistral`
   - `engine/internal/secrets`
   - `engine/internal/settings`
   - `engine/internal/engine` (focused provider RPC tests)
2. Run Flutter widget tests that use updated fixtures:
   - `app/test/settings_screen_test.dart`
   - `app/test/widget_test.dart`
   - `app/test/workbench_review_checkpoint_test.dart`

## Acceptance Criteria
1. Mistral appears in Settings provider list and model registry.
2. Mistral API key can be saved, validated, enabled/disabled, and cleared.
3. Workshop provider readiness and consent logic work with Mistral via existing code paths.
4. Egress for Mistral is restricted to `api.mistral.ai`.
5. Existing providers remain unaffected.
6. Updated tests pass.

## Risks and Mitigations
1. Risk: Mistral response shape differences (especially tool-call arguments).
   - Mitigation: normalize arguments from string/object payloads and keep parser tolerant.
2. Risk: alias drift for `mistral-large-latest`.
   - Mitigation: maintain stable app-level model ID and central alias mapping in one place.
3. Risk: brittle UI tests with hardcoded provider sets.
   - Mitigation: add Mistral to shared test fixtures and keep assertions membership-oriented.
