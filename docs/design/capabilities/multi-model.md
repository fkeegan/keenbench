# Design: Multi-Model (Providers + Model Selection)

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/multi-model.md`
- `docs/prd/keenbench-prd.md` (FR7; SR2/SR3)
- Related:
  - `docs/prd/capabilities/workshop.md`
  - `docs/prd/capabilities/security-egress.md`
  - `docs/prd/capabilities/clutter-bar.md`

## Summary
Multi-model support in v1 means:
- Users can configure **provider credentials** for OpenAI, OpenAI Codex, Anthropic, Anthropic Claude, Google, Mistral, and OpenRouter.
- Users can pick a **single active model** for Workshop, switchable at any time.

Key v1 choices (confirmed):
- Supported models (v1): curated built-in models plus cached OpenRouter-discovered models.
- Built-in models (v1): OpenAI `openai:gpt-5.4`, OpenAI Codex `openai-codex:gpt-5.4`, Anthropic `anthropic:claude-sonnet-4-6`, Anthropic `anthropic:claude-opus-4-6`, Anthropic Claude `anthropic-claude:claude-sonnet-4-6`, Anthropic Claude `anthropic-claude:claude-opus-4-6`, Google `google:gemini-3-pro`, Mistral `mistral:mistral-large`.
- OpenRouter models are fetched from OpenRouter's `/models` API and surfaced as `openrouter:<upstream-model-id>`.
- Workshop uses **one active model at a time** (switchable mid-conversation).
- Provider endpoints are **fixed** to official public APIs for the configured providers (including OpenRouter; no custom endpoints in v1).
- Provider credentials are stored in an **encrypted local file** (see ADR-0004).

## Goals / Non-Goals

### Goals
- Make it obvious which model is active and where data is going (provider + model).
- Keep configuration minimal: a small engine-owned provider set, provider-specific auth controls, and simple status.
- Provide deterministic model selection rules (user default, Workbench default, Workshop active).
- Ensure all model calls go through the same egress/consent controls (see `docs/design/capabilities/security-egress.md`).

### Non-Goals
- Automatic “best model” selection or smart routing in v1.
- Arbitrary manual model entry or custom endpoint configuration beyond the built-in list and OpenRouter's fetched catalog.
- Workshop multi-model / parallel responses (v1.5+).
- Local/offline model support (cloud-only for v1).
- Enterprise endpoints (Azure OpenAI, Mistral Azure, private gateways) in v1.

## User Experience

### Settings: Model Providers (BYOC)
Settings shows one card per engine-supported provider:
- Provider name + status badge: `Not configured` | `Configured` | `Needs attention`
- Credential control appropriate to the provider:
  - API key + Save (`openai`, `anthropic`, `google`, `mistral`, `openrouter`)
  - Setup token + Save (`anthropic-claude`)
  - OAuth connect/disconnect (`openai-codex`)
- Enabled toggle (optional but recommended): disabling a provider prevents model calls without deleting the key
- Optional: “Test connection” button (or auto-test on Save)

Validation UX:
- On Save/connect, the engine performs a lightweight provider validation call.
- If validation fails, show a provider-specific, actionable error.
- If validation succeeds, mark the provider as configured.
- For OpenRouter, successful validation also triggers a background catalog refresh so its fetched models can appear in selectors without a restart.

Recommended copy (v1):
> "Your credential is stored locally in encrypted form. KeenBench sends selected Workbench content to the model provider when you run Workshop."

### Model Selection Hierarchy
Model choice is resolved in this order:
1. **User default model** (global setting): applies when creating new Workbenches.
2. **Workbench default model**: stored per Workbench; used as the Workshop starting model.
3. **Workshop active model**: can be switched at any time; switching updates Workbench default.

### Workshop Model Switching (Single Model)
- The Workbench header always shows the active model.
- Switching is immediate and inserts a “switched model” system event in the transcript (see `docs/design/capabilities/workshop.md`).
- If the selected model’s provider is not configured (no key) or is disabled, it is disabled in the selector with a short hint (“Enable provider / add API key in Settings”).

## Architecture

### UI Responsibilities (Flutter)
- Provide Settings UI for entering/updating provider credentials (never logs secrets).
- Show provider configuration status and validation feedback.
- Show model selector in Workshop.
- Keep provider + model visible during Workshop and during consent prompts.

### Engine Responsibilities (Go)
- Store provider credentials securely (encrypted at rest) and expose provider status.
- Implement provider clients with consistent streaming interfaces.
- Maintain a canonical registry of built-in models plus a cached OpenRouter model catalog and their metadata:
  - context window estimate (for Clutter Bar)
  - "supports vision" (v1 assumption: the curated built-in models do; OpenRouter varies by model)
  - `supports_file_read`
  - `supports_file_write`
- Resolve model selection per the hierarchy and enforce "configured + enabled provider only".
- Record model usage in job/workbench audit artifacts.

### Provider / Model Matrix

| Provider | Auth Mode | Model Source | File Read / Write | Notes |
|----------|-----------|--------------|-------------------|-------|
| OpenAI (`openai`) | API key | Built-in registry | Yes / Yes | Default OpenAI API model |
| OpenAI Codex (`openai-codex`) | OAuth | Built-in registry | Yes / Yes | OAuth-backed OpenAI subscription path |
| Anthropic (`anthropic`) | API key | Built-in registry | Yes / Yes | Standard Anthropic API path |
| Anthropic Claude (`anthropic-claude`) | Setup token | Built-in registry | Yes / Yes | Setup-token credential flow |
| Google (`google`) | API key | Built-in registry | Yes / Yes | Gemini path |
| Mistral (`mistral`) | API key | Built-in registry | Yes / Yes | Maps to provider-managed latest model name internally |
| OpenRouter (`openrouter`) | API key | Provider `/models` catalog, cached locally | Per model | `supports_file_read` / `supports_file_write` derived from provider metadata |

### File Operation Execution

All built-in providers and tool-capable OpenRouter models use the same local file-operation path:
1. Model reasons about the task and issues file/tool operations.
2. Engine validates operations and enforces sandbox boundaries.
3. Local tool worker applies reads/writes in Workbench scope (writes go to Draft).
4. User reviews Draft diffs and chooses publish/discard.

See: `docs/design/capabilities/file-operations.md` for full file operations design.

### IPC / API Surface
API names are illustrative (JSON-RPC per ADR-0003).

**Provider configuration**
- `ProvidersGetStatus() -> {providers[]}`
  - `providers[]`: `{provider_id, display_name, configured, enabled, models[], auth_mode, rpi_reasoning?, oauth_connected?, oauth_account_label?, oauth_expires_at?, oauth_expired?, token_connected?, token_account_label?}`
- `ProvidersSetApiKey({provider_id, api_key}) -> {configured}` for `auth_mode=api_key|setup_token`
- `ProvidersClearApiKey({provider_id}) -> {}`
- `ProvidersSetEnabled({provider_id, enabled}) -> {}`
- `ProvidersValidate({provider_id}) -> {ok, error?}`
- `ProvidersRefreshModels({provider_id}) -> {ok, model_count}`
  - OpenRouter only; refreshes the cached provider catalog from OpenRouter's `/models` API.
- `ProvidersOAuthStart({provider_id="openai-codex"}) -> {provider_id, flow_id, authorize_url, status, expires_at, callback_listening}`
- `ProvidersOAuthStatus({provider_id, flow_id}) -> {provider_id, flow_id, status, expires_at, authorize_url, code_captured?, error?}`
- `ProvidersOAuthComplete({provider_id, flow_id, redirect_url?}) -> {provider_id, oauth_connected, oauth_account_label, oauth_expires_at}`
- `ProvidersOAuthDisconnect({provider_id="openai-codex"}) -> {}`

**Model registry + selection**
- `ModelsListSupported() -> {models[]}`
  - `models[]`: `{model_id, provider_id, display_name, context_tokens_estimate, supports_file_read, supports_file_write, requires_key=true}`
- `UserSetDefaultModel({model_id}) -> {}`
- `UserGetDefaultModel() -> {model_id}`
- `WorkbenchSetDefaultModel(workbench_id, {model_id}) -> {}`
- `WorkbenchGetDefaultModel(workbench_id) -> {model_id}`

**Model capabilities**
- `ModelsGetCapabilities(model_id) -> {capabilities}`
  - `capabilities`: `{supports_file_read, supports_file_write, context_tokens_estimate}`

Workshop-specific RPCs are defined in their capability docs; they should reference `model_id` values from this registry.

## Data & Storage

### Model IDs (v1)
Use stable, namespaced IDs:
- `openai:gpt-5.4`
- `openai-codex:gpt-5.4`
- `anthropic:claude-sonnet-4-6`
- `anthropic:claude-opus-4-6`
- `anthropic-claude:claude-sonnet-4-6`
- `anthropic-claude:claude-opus-4-6`
- `google:gemini-3-pro`
- `mistral:mistral-large`
- `openrouter:<upstream-model-id>`

The first eight are curated built-ins. OpenRouter IDs are fetched dynamically and cached locally.

### Global Settings (Conceptual)
Store global settings outside any Workbench (platform app data dir):
- `settings.json`:
  - `schema_version`
  - `user_default_model_id`
  - `providers`: `{provider_id: {enabled}}`
  - `provider_status_cache?` (optional; avoid on if redundant)
- `secrets.enc`:
  - encrypted provider credentials (see ADR-0004)
- `openrouter_models.json`:
  - cached OpenRouter model metadata used by `ModelsListSupported`, capability checks, and Clutter Bar calculations until the next refresh

### Workbench Metadata
Workbench stores:
- `meta/workbench.json.default_model_id` (Workbench default model)
- Workshop stores:
  - `meta/workshop_state.json.active_model_id`

## Algorithms / Logic

### Provider Credential Validation
Validation should be cheap and provider-specific:
- Perform a minimal authenticated request that fails fast on invalid credentials.
- Store `last_validated_at` and `last_error` for UX.
- Treat transient network errors as “needs attention” but do not erase keys.
- For OpenRouter, the same validation pass may also kick off a catalog refresh in the background.

### Model Metadata for Clutter Bar
The engine maintains per-model metadata:
- `context_tokens_estimate` for Clutter Bar calculations.

Built-in metadata is local and release-managed. OpenRouter metadata is fetched from the provider, cached locally, and reused until the next refresh.

## Error Handling & Recovery
- **Provider key missing**: block selecting that model; guide user to Settings.
- **Provider key invalid**: mark as "needs attention"; block model calls; surface retry + Settings link.
- **Provider/model unavailable mid-run**: retry with backoff; offer model switch in Workshop.
- **Model switch during streaming** (Workshop): complete/terminate current stream cleanly; next turn uses the new model.

**Large/complex file handling**: See `docs/design/capabilities/file-operations.md` for map-first chunked reading and on-demand loading via local file tools.

## Security & Privacy
- Credentials stored encrypted at rest (ADR-0004); never written to logs.
- Engine enforces: no calls to unconfigured providers, no “silent fallback”.
- Provider + model are always visible in the UI during model use and consent.
- Model calls and providers used are recorded in job artifacts and audit trails.

## Telemetry (If Any)
v1 user-facing telemetry: none.

Local-only debug metrics (optional):
- Provider validation failures by provider.
- Model switch frequency.

## Open Questions
None currently.

## Self-Review (Design Sanity)
- Aligns with the current v1 product constraint: curated built-in models, fixed provider endpoints, and scoped dynamic discovery for OpenRouter only.
- Keeps Workshop single-model (switchable mid-conversation) without additional UX complexity.
- Keeps dynamic discovery narrow enough to avoid broad endpoint/configuration sprawl.
- Makes key storage a one-way decision captured in an ADR and avoids leaking secrets into Workbench artifacts.
