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

## Provider Configuration (BYOC)

v1 uses a **Bring Your Own Credential (BYOC)** model. Users configure providers by supplying the credential required for that provider.

### Supported Providers (v1)
- OpenAI API (`openai`) via API key
- OpenAI Codex (`openai-codex`) via OAuth
- Anthropic API (`anthropic`) via API key
- Anthropic Claude (`anthropic-claude`) via setup token
- Google (Gemini) (`google`) via API key
- Mistral (`mistral`) via API key
- OpenRouter (`openrouter`) via API key

Additional providers may be added based on demand.

**Model requirements:** The built-in catalog continues to target image/vision-capable frontier models. OpenRouter is handled differently: the engine fetches OpenRouter's live model catalog after key validation/startup and stores capability metadata locally, so capability differences across OpenRouter models are expected.

### Supported Models (v1)
v1 supports a hybrid model catalog:

**Curated built-in models**
- OpenAI API: `openai:gpt-5.4`
- OpenAI Codex: `openai-codex:gpt-5.4`
- Anthropic API: `anthropic:claude-sonnet-4-6`
- Anthropic API: `anthropic:claude-opus-4-6`
- Anthropic Claude setup token: `anthropic-claude:claude-sonnet-4-6`
- Anthropic Claude setup token: `anthropic-claude:claude-opus-4-6`
- Google: `google:gemini-3-pro`
- Mistral: `mistral:mistral-large`

**Dynamic OpenRouter catalog**
- OpenRouter models are fetched from the OpenRouter `/models` API when the provider is validated and again on engine startup if an OpenRouter key is already configured.
- OpenRouter model IDs are surfaced as `openrouter:<upstream-model-id>`.
- The fetched OpenRouter catalog is cached locally so previously discovered models can still be listed before the next refresh.

### Provider Capabilities (v1)
| Provider | File Read | File Write | Notes |
|----------|-----------|------------|-------|
| OpenAI API / OpenAI Codex | Yes | Yes | Uses the shared local tool-worker file workflow |
| Anthropic API / Anthropic Claude | Yes | Yes | Uses the shared local tool-worker file workflow |
| Google | Yes | Yes | Uses the same local tool-worker file workflow as other providers |
| Mistral | Yes | Yes | Uses the shared local tool-worker file workflow; EU-hosted inference |
| OpenRouter | Model-dependent | Model-dependent | Uses the shared local tool-worker file workflow when the fetched model metadata reports `tools` support; other OpenRouter models may be analysis-only |

**File operation model**: File operations are executed locally (Workbench + local tool worker). Built-in providers can all use this path. OpenRouter models can use it only when the fetched model metadata reports `tools` support. See `docs/prd/capabilities/file-operations.md`.

### Configuration UX

**Access:** Settings > Model Providers

**Per-provider configuration:**
| Field | Required | Notes |
|-------|----------|-------|
| Credential | Yes | API key, setup token, or OAuth connection depending on provider; stored securely (encrypted at rest) |
| Enabled | Yes | Toggle to enable/disable provider |

**Configuration flow:**
1. User opens Settings > Model Providers.
2. User sees a list of supported providers with enable/disable toggles.
3. To enable an API-key or setup-token provider, user enters the credential. OAuth-backed providers use a connect flow instead.
4. On save/connect, the app validates the credential with a lightweight provider call.
5. If validation fails, show a provider-specific error and keep the provider unavailable.
6. If validation succeeds, provider is enabled and its models appear in model selectors. For OpenRouter, successful validation also triggers a background refresh of the provider's model catalog and updates the local cache.

**Credential validation errors:**
- Invalid credential format: "Credential format is invalid for [provider]."
- Credential rejected by provider: "Credential was rejected by [provider]. Please verify it."
- Network error during validation: "Could not reach [provider] to validate key. Check your connection and try again."

**Credential management:**
- Credentials are stored locally and encrypted at rest.
- Users can update, reconnect, or remove credentials at any time.
- Removing a credential disables the provider; in-progress jobs using that provider will fail gracefully.

**No provider configured:**
- If no providers are configured, the app prompts the user to add at least one before using Workshop.
- Message: "Add a model provider to get started. Go to Settings > Model Providers."

### Model Discovery
- Built-in providers use the curated allowlist above.
- OpenRouter exposes a fetched model catalog from OpenRouter's `/models` API; users do not type arbitrary model IDs manually.
- The engine caches OpenRouter model metadata locally and refreshes it when the provider is validated and on startup when an OpenRouter key is already configured.

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
1. Users can configure multiple providers via BYOC (Bring Your Own Credential).
2. Provider configuration includes provider-appropriate credential entry or connection, validation, and enable/disable toggle.
3. Credentials are validated on save/connect; invalid credentials show clear error messages.
4. Credentials are stored locally and encrypted at rest.
5. At least one provider must be configured before using Workshop.
6. The app exposes the curated built-in model list plus cached OpenRouter-discovered models; models with missing provider credentials are disabled/unavailable.
7. User default model is set in user settings.
8. Workbench default model is inherited from user default and can be changed.
9. In Workshop, switching models is seamless: new model picks up conversation history, no confirmation.
10. Model switch persists as the new Workbench default.
11. Model/provider usage is recorded in audit logs.
12. File operations can be executed regardless of selected primary model for the built-in providers; OpenRouter file operations require a model that advertises tool support, and writes are still applied locally in Draft.
13. Workshop supports both analysis-only responses and Draft-producing edits depending on user request and selected model capabilities.

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
- Users can add, update, reconnect, and remove credentials for supported providers.
- Credentials are validated on save/connect with clear success/error feedback.
- Invalid or missing credentials prevent provider use with a clear message.
- At least one configured provider is required to use Workshop.
- Built-in providers expose the curated model list, and OpenRouter exposes its fetched cached catalog.
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

~~How do we present model capability constraints (context limits, file support) without adding config burden?~~ → **Resolved**: The built-in catalog stays curated and image-capable. OpenRouter models are fetched dynamically and may vary by capability; context limits and file-operation support come from the fetched model metadata, and context compression is still handled automatically by the model-aware Clutter Bar.
