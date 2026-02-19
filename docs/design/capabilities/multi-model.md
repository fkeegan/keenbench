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
- Users can configure **provider API keys** (BYOK) for OpenAI, Anthropic, Google, and Mistral.
- Users can pick a **single active model** for Workshop, switchable at any time.

Key v1 choices (confirmed):
- Supported models (v1): OpenAI `openai:gpt-5.2`, Anthropic `anthropic:claude-sonnet-4-6`, Anthropic `anthropic:claude-opus-4-6`, Google `google:gemini-3-pro`, Mistral `mistral:mistral-large`.
- Workshop uses **one active model at a time** (switchable mid-conversation).
- Provider endpoints are **fixed** to official public APIs (no custom endpoints in v1).
- Provider API keys are stored in an **encrypted local file** (see ADR-0004).

## Goals / Non-Goals

### Goals
- Make it obvious which model is active and where data is going (provider + model).
- Keep configuration minimal: four providers, one key each, simple status.
- Provide deterministic model selection rules (user default, Workbench default, Workshop active).
- Ensure all model calls go through the same egress/consent controls (see `docs/design/capabilities/security-egress.md`).

### Non-Goals
- Automatic “best model” selection or smart routing in v1.
- Arbitrary model discovery and selection beyond the supported list.
- Workshop multi-model / parallel responses (v1.5+).
- Local/offline model support (cloud-only for v1).
- Enterprise endpoints (Azure OpenAI, Mistral Azure, private gateways) in v1.

## User Experience

### Settings: Model Providers (BYOK)
Settings shows four providers with consistent affordances:
- Provider name + status badge: `Not configured` | `Configured` | `Needs attention`
- API key input + Save
- Enabled toggle (optional but recommended): disabling a provider prevents model calls without deleting the key
- Optional: “Test connection” button (or auto-test on Save)

Validation UX:
- On Save, the engine performs a lightweight validation call.
- If validation fails, show a provider-specific, actionable error.
- If validation succeeds, mark the provider as configured.

Recommended copy (v1):
> "Your key is stored locally in encrypted form. KeenBench sends selected Workbench content to the model provider when you run Workshop."

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
- Provide Settings UI for entering/updating provider keys (never logs keys).
- Show provider configuration status and validation feedback.
- Show model selector in Workshop.
- Keep provider + model visible during Workshop and during consent prompts.

### Engine Responsibilities (Go)
- Store provider keys securely (encrypted at rest) and expose provider status.
- Implement provider clients with consistent streaming interfaces.
- Maintain a canonical registry of supported models and their metadata:
  - context window estimate (for Clutter Bar)
  - "supports vision" (v1 assumption: all supported models do)
  - `supports_file_read`: true for all v1 models
  - `supports_file_ops`: true for all v1 models (file edits execute locally via the tool worker)
- Resolve model selection per the hierarchy and enforce "configured + enabled provider only".
- Record model usage in job/workbench audit artifacts.

### Model Capability Matrix

| Capability | OpenAI (`openai:gpt-5.2`) | Anthropic (`anthropic:claude-sonnet-4-6` / `anthropic:claude-opus-4-6`) | Google (`google:gemini-3-pro`) | Mistral (`mistral:mistral-large`) |
|------------|------------------|-----------------------------|-----------------------|--------------------------|
| Vision | Yes | Yes | Yes | Yes |
| Context Tokens | ~200k | ~200k | ~1M | ~128k |
| Local File Read Access | Yes | Yes | Yes | Yes |
| Local File Write Access | Yes | Yes | Yes | Yes |
| File Operation Path | Local tool worker | Local tool worker | Local tool worker | Local tool worker |

### File Operation Execution

All providers use the same local file-operation path:
1. Model reasons about the task and issues file/tool operations.
2. Engine validates operations and enforces sandbox boundaries.
3. Local tool worker applies reads/writes in Workbench scope (writes go to Draft).
4. User reviews Draft diffs and chooses publish/discard.

See: `docs/design/capabilities/file-operations.md` for full file operations design.

### IPC / API Surface
API names are illustrative (JSON-RPC per ADR-0003).

**Provider configuration**
- `ProvidersGetStatus() -> {providers[]}`
  - `providers[]`: `{provider_id, configured, enabled, last_validated_at?, last_error?}`
- `ProvidersSetApiKey({provider_id, api_key}) -> {configured}`
- `ProvidersClearApiKey({provider_id}) -> {}`
- `ProvidersSetEnabled({provider_id, enabled}) -> {}`
- `ProvidersValidate({provider_id}) -> {ok, error?}`

**Model registry + selection**
- `ModelsListSupported() -> {models[]}`
  - `models[]`: `{model_id, provider_id, display_name, context_tokens_estimate, pricing?, requires_key=true}`
- `UserSetDefaultModel({model_id}) -> {}`
- `UserGetDefaultModel() -> {model_id}`
- `WorkbenchSetDefaultModel(workbench_id, {model_id}) -> {}`
- `WorkbenchGetDefaultModel(workbench_id) -> {model_id}`

**Model capabilities**
- `ModelsGetCapabilities(model_id) -> {capabilities}`
  - `capabilities`: `{supports_file_read, supports_file_ops, supported_formats[], context_tokens}`

Workshop-specific RPCs are defined in their capability docs; they should reference `model_id` values from this registry.

## Data & Storage

### Model IDs (v1)
Use stable, namespaced IDs:
- `openai:gpt-5.2`
- `anthropic:claude-sonnet-4-6`
- `anthropic:claude-opus-4-6`
- `google:gemini-3-pro`
- `mistral:mistral-large`

### Global Settings (Conceptual)
Store global settings outside any Workbench (platform app data dir):
- `settings.json`:
  - `schema_version`
  - `user_default_model_id`
  - `providers`: `{provider_id: {enabled}}`
  - `provider_status_cache?` (optional; avoid on if redundant)
- `secrets.enc`:
  - encrypted provider keys (see ADR-0004)

### Workbench Metadata
Workbench stores:
- `meta/workbench.json.default_model_id` (Workbench default model)
- Workshop stores:
  - `meta/workshop_state.json.active_model_id`

## Algorithms / Logic

### Provider Key Validation
Validation should be cheap and provider-specific:
- Perform a minimal authenticated request that fails fast on invalid credentials.
- Store `last_validated_at` and `last_error` for UX.
- Treat transient network errors as “needs attention” but do not erase keys.

### Model Metadata for Clutter Bar
The engine maintains per-model metadata:
- `context_tokens_estimate` for Clutter Bar calculations.

All metadata is local (no dynamic discovery in v1), and should be easy to update between releases.

## Error Handling & Recovery
- **Provider key missing**: block selecting that model; guide user to Settings.
- **Provider key invalid**: mark as "needs attention"; block model calls; surface retry + Settings link.
- **Provider/model unavailable mid-run**: retry with backoff; offer model switch in Workshop.
- **Model switch during streaming** (Workshop): complete/terminate current stream cleanly; next turn uses the new model.

**Large/complex file handling**: See `docs/design/capabilities/file-operations.md` for map-first chunked reading and on-demand loading via local file tools.

## Security & Privacy
- Keys stored encrypted at rest (ADR-0004); never written to logs.
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
- Aligns with the stated v1 product constraint: exactly four supported models and fixed provider endpoints.
- Keeps Workshop single-model (switchable mid-conversation) without additional UX complexity.
- Avoids adding dynamic model discovery and reduces UX surface area for early versions.
- Makes key storage a one-way decision captured in an ADR and avoids leaking secrets into Workbench artifacts.
