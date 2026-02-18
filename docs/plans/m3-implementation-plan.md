# Implementation Plan: M3 — Workbench + Workshop Safety-First Completion

## Status
Implemented (2026-02-06)

## Goal
Fully implement the Workbench/Workshop safety and reliability surfaces by completing and hardening:
- Workbench lifecycle and file operations.
- Workshop interaction and draft-generation loop.
- Draft/Publish invariants and publish checkpoint behavior.
- Security/egress consent and network policy enforcement.
- Failure-mode handling on Workbench and Workshop surfaces with actionable recovery UX.

This milestone prioritizes fail-closed behavior over convenience: no unsafe writes, no silent egress, and no ambiguous recovery states.

---

## Key References (Impacted Design)
- `docs/design/design.md`
- `docs/design/capabilities/workbench.md`
- `docs/design/capabilities/workshop.md`
- `docs/design/capabilities/draft-publish.md`
- `docs/design/capabilities/security-egress.md`
- `docs/design/capabilities/failure-modes.md`
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`
- `docs/design/style-guide.md`

---

## Scope

### In Scope
1. Workbench safety boundary completion:
- Copy-based add semantics, manifest integrity, opaque classification, and sandbox enforcement.
- File operations (add/remove/extract/delete workbench) with explicit Draft-state gating.

2. Workshop completion on Workbench surfaces:
- Streaming chat + saved conversation.
- Auto-propose/auto-apply draft flow with `no_changes` handling.
- Auto-open Review when Draft exists or appears.
- Inline publish checkpoint cards and restore affordance gating while Draft exists.

3. Draft/Publish completion:
- Draft-only AI writes.
- Atomic publish with checkpoint integration and consistent progress/error states.
- Discard path that preserves Published and clears Draft metadata safely.

4. Security/Egress completion:
- Provider/model/scope-bound consent checks before model calls.
- Persisted vs session consent handling.
- HTTPS + allowlist-only egress policy enforcement.

5. Failure-mode completion for Workbench/Workshop:
- ADR-0006 error code mapping and `actions[]` across all affected RPC paths.
- Unified, actionable UI handling for retry/settings/switch model/review draft/discard draft/restore.
- Failure artifact persistence for local audit and offline diagnostics.

### Out of Scope
- Unrelated execution flow changes beyond shared failure/error taxonomy behavior.
- New provider classes beyond existing v1 provider model.
- Remote browsing/connectors or non-allowlisted network features.
- Multi-draft or branching conversation models.

---

## Acceptance Criteria
1. Workbench add/remove/extract/delete operations are blocked while a Draft exists and return structured gating errors.
2. All Workshop model calls fail fast with `EGRESS_CONSENT_REQUIRED` when provider/model/scope consent is missing.
3. Engine egress blocks non-HTTPS, IP-literal, and non-allowlisted hosts with `EGRESS_BLOCKED_BY_POLICY`.
4. AI-driven file mutations never write directly to `published/`; all writes are Draft-routed.
5. Publish is atomic from the user perspective; on publish failure, Published remains unchanged or is restored and Draft is preserved.
6. Workbench and Workshop user-visible errors map to ADR-0006 codes and include actionable `actions[]`.
7. Review and diff workflows remain offline and do not trigger model calls.
8. Workbench/Workshop failure events are persisted locally in canonical artifacts without raw prompt/file-content leakage by default.

---

## API Changes

### Added / Finalized RPCs
- `WorkbenchFilesRemove(workbench_id, workbench_paths[]) -> {remove_results[]}`
- `WorkbenchFilesExtract(workbench_id, destination_dir, workbench_paths[]?) -> {extract_results[]}`
- `WorkbenchDelete(workbench_id) -> {}`
- `EgressGetConsentStatus(workbench_id) -> {workshop:{consented, provider_id, model_id, scope_hash}}`
- `EgressGrantWorkshopConsent(workbench_id, {provider_id, model_id, scope_hash, persist}) -> {}`
- `ReviewGetChangeSet(workbench_id) -> {changes[], draft_summary?}`

### Stabilized Draft / Workshop Contracts
- `DraftGetState(workbench_id)` and `DraftPublish(workbench_id)` responses include data needed for checkpoint-aware Workshop UX.
- Workshop conversation/event payloads include system events required for inline publish-checkpoint cards.

### Error Contract
- All Workbench/Workshop failures return JSON-RPC error envelope with ADR-0006 `ErrorInfo` in `error.data`.
- Error branching in UI is based on `error_code`, `phase/subphase`, and `actions[]`, not message text.

---

## Implementation Plan (By Area)

### Engine: Workbench
1. Enforce Draft gating in file/workbench mutation RPC handlers.
2. Keep manifest updates transactional with file operations; prevent partial manifest corruption.
3. Preserve flat-path sandbox guarantees and reject traversal/link escapes.

### Engine: Workshop
1. Ensure each turn assembles manifest + file context and fails closed if context cannot be built.
2. Keep auto-propose/auto-apply flow deterministic with persisted proposal/apply artifacts.
3. Emit consistent conversation system events for publish checkpoints and restore outcomes.

### Engine: Draft/Publish
1. Keep Draft creation lazy and deterministic on first write.
2. Use checkpoint-backed atomic publish sequence with progress phases and rollback-safe failure handling.
3. Preserve Draft on error; never leave ambiguous Draft/Published state.

### Engine: Security/Egress
1. Route all provider calls through policy-gated HTTP transport.
2. Bind consent validation to provider + model + scope hash on every eligible call.
3. Persist consent according to session/persist choice and invalidate on scope/provider/model changes.

### Engine: Failure Taxonomy
1. Map provider/network/filesystem/workflow failures to ADR-0006 canonical codes.
2. Persist structured failure artifacts in Workbench and Workshop logs.
3. Include recommended recovery `actions[]` on every recoverable error path.

### UI: Workbench/Workshop Surfaces
1. Keep Draft-state gating explicit in Workbench controls with clear guidance.
2. Render Workshop failure states inline with action-first affordances (retry, settings, switch model).
3. Render publish checkpoint cards in Workshop timeline and enforce restore-disabled behavior while Draft exists.
4. Maintain style-guide consistency for all error banners/modals and status messaging.

---

## Test Plan
1. `cd engine && go test ./... -coverprofile=coverage.out` and maintain total coverage >= 65%.
2. `cd app && flutter test` for Workbench/Workshop/Draft/Egress failure-state coverage.
3. `scripts/e2e/run_e2e_serial.sh` in fake mode and real-key mode for end-to-end safety gates.
4. Engine tests:
- Draft gating for `WorkbenchFilesAdd/Remove/Extract` and `WorkbenchDelete`.
- Consent-required and policy-blocked egress branches.
- Publish failure rollback + Draft preservation.
- ADR-0006 mapping assertions for key error paths.

5. Flutter integration/E2E tests:
- Auto-open Review on Draft create/reopen.
- Workbench actions disabled while Draft exists.
- Workshop consent prompt + retry path.
- Inline checkpoint card restore gating while Draft exists.
- Offline Review behavior with no model egress.

---

## Rollout / Migration Notes
- No destructive migration required; behavior changes are guardrail-tightening and contract stabilization.
- Existing Workbenches remain valid; consent prompts may reappear once where provider/model/scope fields were previously incomplete.
- Existing conversation/history files remain backward compatible with optional system-event fields.
- Rollout remains desktop-local and offline-safe for Review surfaces.

---

## Audit (Post-Implementation)
- Workbench safety boundary: Completed.
- Workshop draft loop and review auto-open behavior: Completed.
- Draft/Publish atomicity + checkpoint-linked UX: Completed.
- Security/egress consent + allowlist enforcement: Completed.
- Failure-mode coverage on Workbench/Workshop surfaces with ADR-0006 mapping: Completed.
- Scope deviations: None.
- Follow-up gaps: None required for M3 acceptance.
