# Implementation Plan: M2 — Multi‑Model + Egress Safety + Failure Modes + Checkpoints + Clutter Bar

## Status
Implemented (2026-02-05)

## Goal (M2)
Deliver the remaining M2 capabilities around **multi‑model support**, **egress security**, **failure‑mode reporting**, **checkpoints**, and a **clutter bar**—without breaking M0 safety/consent rules or Draft/Publish invariants.

This milestone focuses on:
- **Multi‑provider model registry** with per‑user default, per‑workbench default, and per‑session active model selection.
- **Egress security hardening** (allowlist + consent scoped to provider/model).
- **Failure‑mode taxonomy** mapped into ADR‑0006 error codes and surfaced to the UI.
- **Checkpoints** (create, list, restore) for published state safety.
- **Clutter bar** signal that reflects context pressure for the active model.

---

## Key References (Impacted Design)
- `docs/design/capabilities/security-egress.md` — consent and egress boundary requirements.
- `docs/design/capabilities/failure-modes.md` — structured error mapping expectations.
- `docs/design/capabilities/multi-model.md` — provider + model selection design.
- `docs/design/capabilities/checkpoints.md` — checkpoint lifecycle and restore semantics.
- `docs/design/capabilities/clutter-bar.md` — clutter scoring + UI signal.
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md` — error codes and actions.

---

## Scope

### In Scope (M2)
1. **Model registry + provider support**
2. **Provider settings and validation** (per provider key, enable/disable, validate)
3. **Egress consent bound to provider + model + scope hash**
4. **Egress allowlist enforcement** (HTTPS only, host allowlist per provider)
5. **Failure‑mode mapping** for auth, unavailable, network, egress blocked
6. **Checkpoint lifecycle** (create, list, get, restore) + retention policy
7. **Clutter score + bar** computed from files + conversation + active model context
8. **UI support** for model selection, provider settings, checkpoints, clutter

### Out of Scope (explicitly not in M2)
- Provider‑specific fine‑tuning or model marketplace UX
- Cross‑workbench checkpoint sharing
- Draft‑level checkpoints (only published state snapshots)
- Automated retry/backoff policies beyond ADR‑0006 error mapping

---

## What “Done” Means (M2 Acceptance Criteria)
1. A user can configure multiple providers, validate keys, and select a default model.
2. Each workbench has a default model and an active model; switching active model updates the clutter signal.
3. Egress consent includes provider + model + scope hash; consent is re‑prompted when any of those change.
4. Provider failures are reported with ADR‑0006 error codes and actionable guidance.
5. Checkpoints can be created, listed, and restored; restore leaves the workbench in a consistent published state.
6. The clutter bar reflects current context pressure and updates on file changes or conversation changes.
7. No writes happen outside Draft or Published boundaries; checkpoints only snapshot published state.

---

## Major Design Decisions (M2)
1. **Model registry is engine‑owned** with a stable model ID format: `provider:model`.
2. **Egress allowlist is enforced at HTTP transport** with HTTPS‑only and host allowlist checks.
3. **Failure‑mode mapping** uses ADR‑0006 error codes and includes provider/model identifiers.
4. **Checkpoints snapshot published + meta** using hardlinks when possible for efficiency.
5. **Clutter score is relative to active model context size** to avoid misleading signals when switching models.

---

## Public API / Interface Changes
New RPCs:
`ModelsListSupported`, `ModelsGetCapabilities`, `UserGetDefaultModel`, `UserSetDefaultModel`, `WorkbenchGetDefaultModel`, `WorkbenchSetDefaultModel`, `WorkshopSetActiveModel`, `WorkbenchGetClutter`, `EgressListEvents`, `CheckpointsList`, `CheckpointsGet`, `CheckpointsCreate`, `CheckpointsRestore`, `ProvidersClearApiKey`.

Modified RPCs:
`EgressGetConsentStatus` returns provider + model + scope hash.
`EgressGrantWorkshopConsent` requires provider + model + scope hash + persist.

New notifications:
`WorkbenchClutterChanged`, `WorkshopModelChanged`.

New data fields:
`workbench.default_model_id`, `settings.user_default_model_id`, provider API keys in `secrets`.

---

## Implementation Plan (By Area)

### Engine
1. Add a model registry with provider IDs, display names, capabilities, and context size.
2. Introduce provider client interfaces for OpenAI, Anthropic, and Google and route calls based on active model.
3. Enforce egress allowlist in HTTP transport (HTTPS + host allowlist).
4. Bind consent to provider/model/scope hash; store session vs persisted consent.
5. Map provider errors to ADR‑0006 error codes; attach provider/model metadata.
6. Add clutter computation based on files + conversation + model context tokens.
7. Emit `WorkbenchClutterChanged` on file add, conversation append, and active model change.
8. Emit `WorkshopModelChanged` when active model changes.

### Workbench Storage
1. Store per‑workbench `default_model_id` and include in workbench metadata.
2. Implement checkpoint snapshots of `published/` and `meta/` with hardlink fallback.
3. Persist checkpoint metadata (`checkpoint_id`, `reason`, `description`, stats).
4. Implement restore with `pre_restore` checkpoint and cleanup of temp artifacts.
5. Apply retention policy: keep last 200 auto checkpoints, 50 manual, always preserve `publish` and `pre_restore`.

### UI (Flutter)
1. Settings screen: provider cards, enable/disable, API key save/validate, default model selection.
2. Workbench: model selector in toolbar, consent dialog shows provider + model and “Don’t ask again”.
3. Workbench: render clutter bar and update on engine notifications.
4. Checkpoints screen: list/create/restore with clear draft‑exists guard.

---

## Test Plan
1. `cd engine && go test ./...`
2. `cd app && flutter test`
3. Configure each provider, validate keys, and switch active model.
4. Confirm consent re‑prompts when model/provider changes or scope hash changes.
5. Create a checkpoint, restore it, and verify published files and metadata reflect the snapshot.
6. Add files and confirm clutter bar updates and notification flow.

---

## Rollout / Migration Notes
- Existing workbenches default to the first supported model in the registry if no defaults are set.
- New settings fields are optional and backward‑compatible; absence falls back to registry defaults.
- Checkpoint storage lives under `meta/checkpoints/` and can be pruned without affecting published data.
- Egress consent is now model‑specific; existing consent without provider/model will re‑prompt once.
