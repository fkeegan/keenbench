# OpenAI Subscription OAuth (OpenAI First) Implementation Plan

## Summary
This plan adds OpenAI subscription authentication to KeenBench alongside existing API-key auth.

The first implementation uses a separate provider namespace, `openai-codex`, and keeps API-key OpenAI (`openai`) unchanged. OAuth is surfaced in the Settings "Model Providers" area with browser-based PKCE auth and manual redirect paste fallback.

## Scope
- Add engine support for `openai-codex` provider and model `openai-codex:gpt-5.3-codex`.
- Add encrypted storage for OpenAI Codex OAuth credentials.
- Add OAuth RPC methods for start/status/complete/disconnect.
- Add token refresh on use for `openai-codex`.
- Add Settings UI for OAuth connect/disconnect and status display.
- Add unit tests for engine OAuth behavior and Flutter Settings OAuth behavior.
- Keep existing OpenAI API-key flow intact.

## Public Interfaces

### Engine RPC
- `ProvidersOAuthStart({provider_id}) -> {provider_id, flow_id, authorize_url, status, expires_at, callback_listening}`
- `ProvidersOAuthStatus({provider_id, flow_id}) -> {provider_id, flow_id, status, expires_at, authorize_url, code_captured?, error?}`
- `ProvidersOAuthComplete({provider_id, flow_id, redirect_url}) -> {provider_id, oauth_connected, oauth_account_label, oauth_expires_at}`
- `ProvidersOAuthDisconnect({provider_id}) -> {}`

### Existing RPC Extension
`ProvidersGetStatus` provider entries include optional OAuth fields:
- `auth_mode`
- `oauth_connected`
- `oauth_account_label`
- `oauth_expires_at`
- `oauth_expired`

### Model and Provider Registry
- Provider: `openai-codex`
- Model: `openai-codex:gpt-5.3-codex`

## Engine Design

### OAuth Flow
- PKCE with state/verifier/challenge.
- Authorize endpoint: `https://auth.openai.com/oauth/authorize`
- Token endpoint: `https://auth.openai.com/oauth/token`
- Redirect URI: `http://localhost:1455/auth/callback`
- Client ID: `app_EMoamEEZ73f0CkXaXp7hrann`
- Scope: `openid profile email offline_access`
- Additional authorize params:
  - `id_token_add_organizations=true`
  - `codex_cli_simplified_flow=true`
  - `originator=pi`

### Credential Storage
Store OpenAI Codex OAuth credentials in encrypted secrets:
- `access_token`
- `refresh_token`
- `id_token`
- `account_label`
- `expires_at`

### Refresh Policy
- Refresh if access token is missing or expiring soon.
- Persist refreshed tokens back to encrypted secrets.
- Map refresh/exchange failures to structured provider errors.

### Provider Separation
- Keep `openai` and `openai-codex` separate for consent/provider gating and model selection.
- `openai` remains API-key-only.
- `openai-codex` remains OAuth-only.

## Flutter Settings UX
- Detect provider auth mode via `ProviderStatus.authMode`.
- API-key providers keep existing API key textfield + save/validate controls.
- OAuth providers render:
  - connection status text
  - optional account/expiry hint
  - connect button (`ProvidersOAuthStart` + browser launch + manual redirect dialog + `ProvidersOAuthComplete`)
  - disconnect button (`ProvidersOAuthDisconnect`)

## Test Plan

### Go
- `engine/internal/openai/oauth_codex_test.go`
  - PKCE generation
  - authorize URL construction
  - redirect parsing
  - code exchange and refresh request behavior
  - account id extraction
- `engine/internal/engine/providers_oauth_test.go`
  - OAuth start/status/complete/disconnect lifecycle
  - state mismatch validation
  - refresh-on-read behavior
  - error mapping for unauthorized/unavailable
  - status expiration field
- `engine/internal/secrets/secrets_test.go`
  - OAuth credential roundtrip and clear behavior
- `engine/internal/settings/settings_test.go`
  - openai-codex provider default/backfill
- `engine/internal/engine/engine_rpc_test.go`
  - provider status includes openai-codex with `auth_mode=oauth`

### Flutter
- `app/test/settings_screen_test.dart`
  - OAuth provider card disconnected render
  - connect flow calls start then complete
  - disconnect flow calls disconnect RPC
  - OpenAI API-key controls still render and save/validate

## Acceptance Criteria
- OpenAI API-key path still works with no behavior change.
- OpenAI Codex OAuth provider can connect, report status, disconnect, and be treated as configured.
- Engine test suite passes with total coverage above 50%.
- Flutter unit test suite passes, including new settings OAuth tests.

## Assumptions
- OpenAI Codex OAuth endpoints and client id are stable.
- Desktop runtime has a best-effort browser opener; manual redirect paste always remains available.
- OAuth account label may be absent; when present it is derived from token claims.
