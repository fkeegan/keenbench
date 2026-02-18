# PRD: Multi-Model

## Status
Draft

## Purpose
Let users choose the best model(s) for their work without losing Workbench context or safety guarantees.

## Scope
- In scope (v1): per-provider API configuration, user default model, Workbench default model for Workshop, seamless model switching in Workshop, and model visibility in logs.
- In scope (v1.5): "Try with another model" forks for Workshop responses (requires concurrent Drafts).
- Out of scope: automatic provider selection, hidden fallback to unconfigured providers.

## Model Hierarchy

1. **User default model**: Set in user settings. Applies to new Workbenches.
2. **Workbench default model**: Inherited from user default when Workbench is created. Can be changed per Workbench and persists.
3. **Active model (Workshop)**: User can switch models during a Workshop session; the switch is immediate and persists.

## Provider Configuration (BYOK)

v1 uses a **Bring Your Own Key (BYOK)** model. Users configure providers by supplying their own API keys.

### Supported Providers (v1)
- OpenAI
- Anthropic
- Google (Gemini)
- Mistral

Additional providers may be added based on demand.

**Model requirements:** Only models with image/vision support are included. This ensures all Workbench file types (including images) can be processed by any selected model.

### Supported Models (v1)
v1 supports a **curated allowlist** of frontier models (no arbitrary model selection):
- OpenAI: `openai:gpt-5.2`
- Anthropic: `anthropic:claude-opus-4.5`
- Google: `google:gemini-3-pro`
- Mistral: `mistral:mistral-large`

### Provider Capabilities (v1)
| Provider | File Read | File Write | Notes |
|----------|-----------|------------|-------|
| OpenAI | Yes | Yes | Uses the shared local tool-worker file workflow |
| Anthropic | Yes | Yes | Uses the shared local tool-worker file workflow |
| Google | Yes | Yes | Uses the same local tool-worker file workflow as other providers |
| Mistral | Yes | Yes | Uses the shared local tool-worker file workflow; EU-hosted inference |

**File operation model**: File operations are executed locally (Workbench + local tool worker). Provider selection affects reasoning/response behavior, not whether file edits are possible. See `docs/prd/capabilities/file-operations.md`.

### Configuration UX

**Access:** Settings > Model Providers

**Per-provider configuration:**
| Field | Required | Notes |
|-------|----------|-------|
| API Key | Yes | Stored securely (encrypted at rest) |
| Enabled | Yes | Toggle to enable/disable provider |

**Configuration flow:**
1. User opens Settings > Model Providers.
2. User sees a list of supported providers with enable/disable toggles.
3. To enable a provider, user enters their API key.
4. On save, the app validates the key with a lightweight API call (e.g., list models).
5. If validation fails, show error: "Invalid API key. Please check and try again."
6. If validation succeeds, provider is enabled and its models appear in model selectors.

**Key validation errors:**
- Invalid key format: "API key format is invalid for [provider]."
- Key rejected by provider: "API key was rejected by [provider]. Please verify your key."
- Network error during validation: "Could not reach [provider] to validate key. Check your connection and try again."

**Key management:**
- Keys are stored locally and encrypted at rest.
- Users can update or remove keys at any time.
- Removing a key disables the provider; in-progress jobs using that provider will fail gracefully.

**No provider configured:**
- If no providers are configured, the app prompts the user to add at least one before using Workshop.
- Message: "Add a model provider to get started. Go to Settings > Model Providers."

### Model Discovery
- v1 uses a curated allowlist of supported models (above).
- Provider API calls may be used for key validation and health checks, but the UI does not expose arbitrary provider-discovered models in v1.

## Workshop Behavior

### Workshop Mode (v1)
- User can switch models at any time.
- **No confirmation dialog** — switch is immediate.
- New model picks up the conversation history and continues from there.
- Switching does not branch or fork; conversation is linear.
- Model choice persists as the Workbench default.

### v1.5 Additions
- "Try with another model" in Workshop: creates a parallel response branch, user can compare and choose.

## User Experience
- Model selector visible in Workshop header.
- Current model always displayed.
- Switching is one-click, no confirmation.
- "Try with another model" button (v1.5) forks a Workshop response.

## Functional Requirements

### v1
1. Users can configure multiple providers via BYOK (Bring Your Own Key).
2. Provider configuration includes API key entry, validation, and enable/disable toggle.
3. API keys are validated on save; invalid keys show clear error messages.
4. API keys are stored locally and encrypted at rest.
5. At least one provider must be configured before using Workshop.
6. The app exposes only the curated supported model list in v1; models with missing provider keys are disabled/unavailable.
7. User default model is set in user settings.
8. Workbench default model is inherited from user default and can be changed.
9. In Workshop, switching models is seamless: new model picks up conversation history, no confirmation.
10. Model switch persists as the new Workbench default.
11. Model/provider usage is recorded in audit logs.
12. File operations can be executed regardless of selected primary model; writes are applied locally in Draft.
13. Workshop supports both analysis-only responses and Draft-producing edits depending on user request.

### v1.5
17. "Try with another model" creates a forked Workshop response.
18. Concurrent Drafts are supported and clearly labeled (Workshop).

## Failure Modes & Recovery
- Model unavailable: surface error, allow retry or switch model.
- Model capability mismatch (file size/tooling): warn via Clutter Bar; proceed with degraded performance or block if critical.
- Provider rate limit: pause and resume or allow model switch.
- File operation tooling failure: preserve Draft state, surface clear error, allow retry.

## Security & Privacy
- Model calls only to configured providers.
- Scope remains bounded to Workbench content.

## Acceptance Criteria

### v1
- Users can add, update, and remove API keys for supported providers.
- API keys are validated on save with clear success/error feedback.
- Invalid or missing keys prevent provider use with a clear message.
- At least one configured provider is required to use Workshop.
- Only the supported model list is available in v1.
- Users can configure multiple providers and switch between them.
- Switching models in Workshop is seamless; conversation continues with new model.
- Model switch persists as Workbench default.
- Current model is always visible in the UI.
- Audit logs record model/provider used.

### v1.5
- "Try with another model" creates labeled parallel outputs.
- Forked runs do not overwrite each other's Drafts (Workshop).
- "Try with another model" is limited to Workshop responses.

## Open Questions
~~Should there be a default model per Workbench or per user?~~ → **Resolved**: Both. User default applies to new Workbenches; Workbench default can be changed and persists.

~~What happens to in-progress Workshop conversation when switching models mid-session?~~ → **Resolved**: New model picks up conversation history and continues. No branching in v1.

~~How do we present model capability constraints (context limits, file support) without adding config burden?~~ → **Resolved**: Only image-capable models are supported. Context limits are handled by the model-aware Clutter Bar. Context compression is applied automatically when limits are approached. No additional configuration needed.
