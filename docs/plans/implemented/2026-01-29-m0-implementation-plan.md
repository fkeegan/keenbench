# Implementation Plan: M0 — Workshop Draft → Review → Publish (Text/CSV, OpenAI)

## Status
Implemented (2026-01-29)
Note (2026-02-02): Workshop UI now uses a single Send action with auto-generated draft changes; the explicit “Propose changes” button is removed. Proposal artifacts remain internal.

## Goal (M0)
Ship the **first file-producing loop** as a real desktop app:
**Workbench → Workshop → Draft changes (auto-applied) → Review (diff) → Publish/Discard**,
implemented as **Flutter UI + Go engine** communicating via **JSON-RPC over stdio**.

This milestone is intentionally **text-first** and targets **`.txt` + `.csv`** (and `.md` for outputs/diffs).
It uses **OpenAI** as the only provider and **disallows deletes** in Workshop draft changes (proposal artifacts) to keep the first publish gate simple.

---

## Key References (Impacted Design)

This milestone should align with these design docs and ADRs:

### Architecture & boundaries
- `docs/design/design.md` — overall architecture (Flutter UI + Go engine, Workbench/Draft concepts).
- `docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md` — IPC decision and framing.

### Workbench, Workshop, Draft/Publish, Review
- `docs/design/capabilities/workbench.md` — Workbench scope + add semantics + limits + “copies” UX.
- `docs/design/capabilities/workshop.md` — Workshop semantics: chat, auto-applied draft changes, saved conversation.
- `docs/design/capabilities/draft-publish.md` — Draft lifecycle, atomic publish/discard invariants.
- `docs/design/capabilities/review-diff.md` — Review is offline; change-set computation; text diff UX.

### Provider / secrets / egress / errors
- `docs/design/capabilities/multi-model.md` — provider configuration UX patterns; model registry shape (we implement OpenAI only in M0, but keep interfaces compatible).
- `docs/design/adr/ADR-0004-encrypted-local-secrets-store-for-provider-keys.md` — encrypted at-rest key storage.
- `docs/design/capabilities/security-egress.md` — allowlist + consent gating before first model call.
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md` — stable error codes/actions across UI and engine.

### Design docs explicitly deferred in M0 (but inform future work)
- `docs/design/capabilities/checkpoints.md` and `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md` — checkpoints/restore and snapshot store (M0 will not expose checkpoints UI).
- `docs/design/capabilities/clutter-bar.md` — clutter indicator (not needed to ship the first loop).
- `docs/design/capabilities/onboarding.md` — first-run walkthrough (we may reuse the suggested “small job” prompt, but not ship the walkthrough UI in M0).
- `docs/design/capabilities/file-operations.md` and `docs/design/adr/ADR-0007-sdk-based-file-operations.md` — SDK-based office editing (M0 is text-only, so we can implement minimal text file IO without office SDKs).

---

## Scope

### In Scope (M0)
1. **Engine process + IPC**
   - Go engine runs as a child process; Flutter speaks JSON-RPC 2.0 over stdin/stdout with line-delimited framing.
   - Version handshake (`EngineGetInfo`) and stable structured errors.

2. **Settings: OpenAI provider key**
   - Flutter Settings screen for OpenAI API key.
   - Engine persists the key encrypted at rest and validates on save.

3. **Workbench**
   - Create/open a Workbench.
   - Add files (copy into `published/`), supporting at minimum `.txt` and `.csv`.
   - Enforce v1 limits at add time (10 files max; 25MB per file), per Workbench design.
   - Block add/remove while a Draft exists (matches Draft/Workbench designs).

4. **Workshop chat (OpenAI)**
   - Send user messages; stream assistant replies.
   - Persist conversation per Workbench and restore it on reopen.

5. **Draft changes flow (no deletes)**
   - A separate model call generates a proposal after each assistant reply; `no_changes: true` yields no Draft.
   - Auto-apply creates a Draft if needed, then applies file writes to `draft/`.
   - Proposal validation **rejects deletes** and any path traversal attempts.

6. **Review (offline)**
   - Show Draft→Published change list (Added/Modified only for M0).
   - Provide a line diff viewer for `.txt`, `.csv`, and `.md`.
   - Review performs **no model calls**.

7. **Publish / Discard**
   - Publish swaps Draft into Published atomically (directory swap); Discard deletes Draft.
   - After publish, reopen shows updated Published state.

### Out of Scope (explicitly not in M0)
- Autonomous job execution (multi-session, audit).
- Multi-provider support / model switching / multi-model cohort.
- Checkpoints UI + restore.
- Message-level undo / rewind / regenerate (Workshop).
- Binary/office previews and office-text inline diffs.
- Clutter Bar.
- Deletions in draft changesets and deletion confirmations in review.

---

## What “Done” Means (M0 Acceptance Criteria)

### Functional loop
- From a Workbench containing at least one `.txt` and one `.csv`, a user can:
  1) ask in Workshop for a deliverable,
  2) see Draft auto-applied changes that include writing `summary.md`,
  3) open **Review** and see Added/Modified + line diffs,
  4) click **Publish**, and
  5) confirm `summary.md` exists in `published/` after reopen.

### Safety / policy invariants (M0)
- All file reads/writes are confined to the Workbench root; any path escape attempt fails fast with `SANDBOX_VIOLATION`. (ADR-0006)
- The engine makes no network calls except to OpenAI’s official endpoint (allowlist enforced). (`docs/design/capabilities/security-egress.md`)
- The first model call in a Workbench is blocked until the user grants consent for provider + file scope.
- Proposals cannot delete files; any `delete` op is rejected as `VALIDATION_FAILED`.
- Review makes no model calls.

### UX baseline
- Users can accomplish the loop without touching the terminal.
- Core errors are actionable (missing key → open settings; missing consent → show consent dialog; auto-apply failure → assistant explains limitation).

---

## Major Design Decisions for M0 (and why)

### 1) Disallow deletes in draft changes
Why: It avoids implementing deletion confirmation gating in Review for the very first usable slice, while still delivering “create a real file and publish it”.

How: Proposal schema does not include a `delete` op in M0; engine validation rejects deletes if present.

Future: Add deletes + confirmation gating per `docs/design/capabilities/review-diff.md`.

### 2) Text-first file support: `.txt`, `.csv`, `.md`
Why: It keeps Review/Diff tractable (line diff only) and proves the workflow.

How: Workbench can import `.txt` and `.csv` in M0; draft changes auto-apply supports writing `.md` (and optionally updating `.txt`/`.csv`).

Future: Add previews and office-text diffs per `docs/design/capabilities/review-diff.md` and `docs/design/adr/ADR-0002-in-app-previews-for-review.md`.

### 3) Keep interfaces aligned with v1 designs even if M0 implements a subset
Why: We want M0 code to become the foundation, not a throwaway prototype.

How: Use the same conceptual artifacts and naming from the design docs (Workbenches with `published/`, `draft/`, `meta/`; JSON-RPC methods shaped like the capability docs; ADR-0006 errors).

---

## Repository Structure (Target)

Per `docs/design/design.md`, set up a monorepo layout:
```
/
  Makefile
  app/                # Flutter desktop app
  engine/             # Go engine
  docs/
```

### `Makefile` targets (M0)
- `make run` — runs Flutter app in dev mode; app spawns engine.
- `make engine` — builds the engine binary.
- `make fmt` — format Dart/Go.
- `make test` — runs engine tests (and Flutter tests if present).

---

## Data & Storage (M0)

### Global app data (engine-owned)
Implements ADR-0004 in the engine’s global data directory (platform-specific via Go APIs):
```
<app_data_dir>/
  settings.json
  secrets.enc
  master.key
```

`settings.json` (minimal, conceptual):
- `schema_version`
- `providers: { openai: { enabled: bool } }`
- optional: `user_default_model_id` (can be hardcoded to OpenAI for M0)

### Workbench on-disk layout (engine-owned, local)
Minimal v1-aligned structure:
```
workbenches/<workbench_id>/
  published/
  draft/                      # present only when Draft exists
  meta/
    workbench.json
    files.json
    conversation.jsonl
    workshop_state.json
    draft.json                # present only when Draft exists
    egress_consent.json
    workshop/
      proposals/<proposal_id>.json
```

Notes:
- `files.json` should store a stable manifest so we can compute scope hashes and show file lists reliably.
- `conversation.jsonl` is append-only as in Workshop design; `workshop_state.json` stores the active head and active model (OpenAI only in M0).

---

## IPC / API Surface (M0)

Keep the API versioned and JSON-RPC 2.0 compliant (ADR-0003).
Frame as **one JSON message per line** on stdout/stderr.

### Required commands (M0)

#### Engine info / health
- `EngineGetInfo() -> {engine_version, api_version}`

#### Settings / providers (OpenAI only in M0)
- `ProvidersGetStatus() -> {providers[]}`
- `ProvidersSetApiKey({provider_id="openai", api_key}) -> {}`
- `ProvidersValidate({provider_id="openai"}) -> {ok, error?}`
- `ProvidersSetEnabled({provider_id="openai", enabled}) -> {}`

#### Workbench
- `WorkbenchCreate({name}) -> {workbench_id}`
- `WorkbenchOpen({workbench_id}) -> {workbench}`
- `WorkbenchList() -> {workbenches[]}` (optional but recommended for UX)
- `WorkbenchFilesList({workbench_id}) -> {files[]}`
- `WorkbenchFilesAdd({workbench_id, source_paths[]}) -> {add_results}` (blocked if Draft exists)

#### Egress consent (per workbench)
- `EgressGetConsentStatus({workbench_id}) -> {consented, provider_id?, scope_hash?}`
- `EgressGrantWorkshopConsent({workbench_id, provider_id="openai", scope_hash}) -> {}`

#### Workshop
- `WorkshopGetState({workbench_id}) -> {active_model_id, has_draft, pending_proposal_id?}`
- `WorkshopSendUserMessage({workbench_id, text}) -> {message_id}`
- `WorkshopStreamAssistantReply({workbench_id, message_id}) -> {}` (reply streams via notifications)
- `WorkshopProposeChanges({workbench_id}) -> {proposal_id, no_changes}`
- `WorkshopGetProposal({workbench_id, proposal_id}) -> {proposal}`
- `WorkshopDismissProposal({workbench_id, proposal_id?}) -> {}`
- `WorkshopApplyProposal({workbench_id, proposal_id}) -> {draft_id?, no_changes?}`

#### Review / diff (text-only)
- `ReviewGetChangeSet({workbench_id}) -> {draft_id, changes[]}`
- `ReviewGetTextDiff({workbench_id, path}) -> {hunks}`

#### Draft
- `DraftGetState({workbench_id}) -> {has_draft, draft_id, created_at}`
- `DraftPublish({workbench_id}) -> {published_at}`
- `DraftDiscard({workbench_id}) -> {}`

### Required notifications (M0)
- `WorkshopAssistantStreamDelta({workbench_id, message_id, token_delta})`
- `WorkshopAssistantMessageComplete({workbench_id, message_id})`
- `WorkshopProposalReady({workbench_id, proposal_id})`
- `DraftStateChanged({workbench_id, has_draft, draft_id?})`
- `EngineError({error})` (optional; otherwise errors are returned on the failing call)

### Error contract
All recoverable UI flows should be driven by ADR-0006 `ErrorInfo` payloads:
- missing key: `PROVIDER_NOT_CONFIGURED` + action `open_settings`
- invalid key: `PROVIDER_AUTH_FAILED` + action `open_settings`
- consent missing: `EGRESS_CONSENT_REQUIRED` + action `retry`
- sandbox escape: `SANDBOX_VIOLATION`
- invalid proposal schema/ops: `VALIDATION_FAILED`

---

## Engine Implementation Plan (Go)

### Phase 0 — Bootstrap (buildable engine + IPC)
1. Create `engine/` module with:
   - stdio JSON-RPC server (line-delimited framing).
   - request routing + notification emitter.
   - `EngineGetInfo` and API version checks.
2. Implement ADR-0006 error payload as a shared type:
   - helper to return JSON-RPC errors with `data: ErrorInfo`.
3. Add a minimal structured logger (debug-only) that never logs secrets.

**Self-check:** Before writing any product logic, ensure Flutter can spawn the engine and call `EngineGetInfo`.

### Phase 1 — Global settings + OpenAI key persistence
1. Implement global settings store (`settings.json`) with `schema_version`.
2. Implement secrets encryption per ADR-0004:
   - generate `master.key` on first run (restrict permissions),
   - encrypt/decrypt `secrets.enc` using AEAD,
   - store OpenAI key in secrets store (not in plaintext settings).
3. Implement `Providers*` RPCs:
   - `SetApiKey`, `Validate`, `GetStatus`, `SetEnabled`.
4. Add network allowlist enforcement (security-egress design):
   - engine HTTP client wrapper that rejects non-OpenAI hosts.

**Self-check:** If the OpenAI key is wrong, validation fails with a stable error code and no key leakage.

### Phase 2 — Workbench core (create/open/add/list)
1. Workbench directory management:
   - create `workbenches/<id>/published` and `meta/`.
2. Implement file add semantics from Workbench design (M0 subset):
   - batch reject if it would exceed 10 files,
   - skip oversize >25MB, add the rest,
   - reject duplicates by filename,
   - reject symlinks,
   - copy into `published/` (flat structure).
3. Maintain `meta/files.json` manifest and expose via `WorkbenchFilesList`.
4. Implement sandbox-safe path resolution:
   - reject absolute paths, `..`, and any resolved path outside workbench root.

**Self-check:** Attempting to import a symlink or a file that would collide is rejected predictably.

### Phase 3 — Egress consent (Workshop per Workbench)
1. Implement `meta/egress_consent.json` storing:
   - provider id, consented_at, scope_hash.
2. Define and compute `scope_hash` from:
   - ordered file list (relative paths) + size + mtime (or sha256 later).
3. Gate all model calls on:
   - provider configured + enabled,
   - consent exists for current scope hash.

**Self-check:** Adding/removing files invalidates consent and requires re-consent before the next model call.

### Phase 4 — Workshop chat (streaming + persistence)
1. Conversation persistence:
   - append `user_message` and `assistant_message` to `meta/conversation.jsonl`.
   - maintain `meta/workshop_state.json` for head pointer and active model (OpenAI only).
2. OpenAI streaming client:
   - stream response tokens/chunks and emit `WorkshopAssistantStreamDelta`.
   - on completion, persist the final assistant message to conversation log.
3. `WorkshopSendUserMessage` + `WorkshopStreamAssistantReply` RPCs.

**Self-check:** App restart restores conversation; sending a new message continues normally.

### Phase 5 — Draft changes generation + Auto-apply (no deletes)
1. Proposal schema (M0)
   - Define a strict JSON schema that expresses only **writes**:
     - `proposal_id`, `summary`, optional `no_changes`, `writes[]: {path, content}`, optional `warnings[]`.
2. Implement `WorkshopProposeChanges` (invoked automatically after each assistant reply when no Draft exists):
   - build prompt context from conversation tail + current Workbench file contents (txt/csv only),
   - call OpenAI with structured output expectations,
   - validate output and persist to `meta/workshop/proposals/<id>.json`.
3. Implement `WorkshopApplyProposal`:
   - create Draft if missing (copy `published/` → `draft/`),
   - apply each write using atomic replace (temp file + fsync + rename),
   - update `meta/draft.json` and emit `DraftStateChanged`.
4. Validation rules (must-haves):
   - paths are relative, normalized, and within Workbench root,
   - no deletes; reject if present,
   - cap max file count and max content size per write (prevent giant payloads),
   - allowed extensions for writes in M0: `.md`, `.txt`, `.csv` (optionally `.json`).

**Self-check:** A malformed proposal never mutates Draft or Published; the user can retry draft generation.

### Phase 6 — Review/Diff (text-only)
1. Implement Draft→Published change-set computation:
   - walk `published/` and `draft/`,
   - compute Added/Modified (Deleted should not occur in M0; treat as invariant violation if found).
2. Implement a line diff engine for `.txt`, `.csv`, `.md`:
   - return hunks suitable for UI rendering.
3. Ensure Review performs no model calls and reads only local files.

**Self-check:** Review works offline (e.g., disable network and still review/publish).

### Phase 7 — Publish/Discard
1. Implement `DraftPublish`:
   - validate Draft exists,
   - perform directory swap atomically:
     - rename `published/` → `published.prev/`
     - rename `draft/` → `published/`
     - cleanup `published.prev/` after success
   - remove `meta/draft.json` and Draft directory marker
2. Implement `DraftDiscard`:
   - delete `draft/`,
   - remove `meta/draft.json`.

**Self-check:** If publish fails mid-way, Published remains recoverable (via `published.prev/`), and Draft is preserved whenever possible.

---

## Flutter Implementation Plan (UI)

### Phase A — App shell + engine lifecycle
1. Create `app/` Flutter project (desktop enabled).
2. Engine process manager:
   - spawn engine binary,
   - manage stdin/stdout streams,
   - send JSON-RPC requests and route responses/notifications.
3. Implement a typed client layer:
   - request/response methods,
   - notification stream (broadcast).

### Phase B — Settings (OpenAI key)
1. Settings screen for OpenAI key + enabled toggle.
2. Validate on save; show actionable errors.

### Phase C — Workbench UI
1. “Workbenches” screen: list + create.
2. Workbench screen:
   - file list + “Add files…”
   - show “copies; originals untouched” scope copy.
   - disable add/remove when Draft exists.

### Phase D — Workshop UI
1. Chat transcript + composer + streaming rendering.
2. Auto-apply status indicator (e.g., “Applying draft changes…”).
3. When Draft exists, replace composer with Review / Publish / Discard actions.

### Phase E — Consent dialog
1. Before first model call per Workbench, show consent modal:
   - provider/model (OpenAI)
   - list of Workbench files + sizes
   - Continue/Cancel
2. Persist consent state via engine RPC.

### Phase F — Review UI (text-only)
1. Change list (Added/Modified).
2. Diff viewer with line numbers and added/removed highlights.
3. Publish/Discard actions.

---

## Testing & Validation Plan

### Engine unit tests (must-have)
- Sandbox path resolution: rejects `..`, absolute paths, symlinks.
- Workbench add semantics: batch reject on file-count overflow; oversize skip; duplicate name reject.
- Draft publish/discard: directory swap correctness; recovery behavior when rename fails (inject failures).
- Diff algorithm: stable hunks for `.csv` and `.txt` changes.
- Secrets store: encrypt/decrypt roundtrip; permissions best-effort (platform conditional).

### Engine integration test (recommended)
Spawn engine in-process test harness (or subprocess) and run:
1) create workbench
2) add sample `.txt` and `.csv`
3) fake consent granted
4) mock OpenAI draft generation response (inject via test mode / stubbed HTTP client)
5) auto-apply draft changes
6) get changeset + diff
7) publish
8) verify file contents in `published/`

### Manual test script (must-have)
1. Launch app.
2. Add OpenAI key (verify it persists after restart).
3. Create workbench; add `notes.txt` + `data.csv`.
4. Workshop: ask to create `summary.md`.
5. Consent appears; accept.
6. Draft auto-applies; Draft banner appears.
7. Review diff shows Added/Modified; publish.
8. Reopen workbench; confirm `summary.md` exists in Published.

---

## Risks & Mitigations (Self-Reflection)

### Risk: JSON-RPC streaming reliability
- **Concern:** token streaming emits many notifications; framing/backpressure bugs can lock up the UI.
- **Mitigation:** keep notifications small (`token_delta` chunks), debounce UI updates, and add engine-side rate limiting if needed.

### Risk: Model output not valid JSON / schema drift
- **Concern:** draft generation depends on strict structured output.
- **Mitigation:** use a strict schema prompt; validate hard; on failure return `VALIDATION_FAILED` with a retry action and store the raw response in a debug-only artifact (never in release logs).

### Risk: Draft/Publish correctness on Windows/macOS/Linux
- **Concern:** directory rename semantics differ across platforms; Windows file locking can break swaps.
- **Mitigation:** keep file handles short-lived; add retries for transient rename failures; design publish phases per `docs/design/capabilities/draft-publish.md`.

### Risk: Secrets “encrypted file + master key file” is only best-effort
- **Concern:** ADR-0004 is not OS-keychain-grade protection.
- **Mitigation:** follow ADR; keep keys out of Workbench; strictly redact logs; evaluate OS keychain migration post-M0.

### Risk: Scope hash / consent invalidation confusion
- **Concern:** users may be reprompted more often than expected.
- **Mitigation:** make consent dialog copy clear; compute scope hash deterministically; do not invalidate on harmless metadata changes.

### Risk: Text/CSV diff correctness vs user expectations
- **Concern:** line diffs for CSV are “good enough” for M0 but not semantic.
- **Mitigation:** label as plain diff; plan semantic CSV review later if needed.

---

## Follow-ups After M0 (Next Milestones)

Suggested next increments (not part of this plan):
1. **M1:** Deletions + deletion confirmations + publish gating (Review/Diff design).
2. **M2:** Checkpoints + restore UI (Checkpoints design + ADR-0001 decision finalization).
3. **M3:** Workshop undo/rewind + regenerate (Workshop design).
4. **M4:** Binary previews pipeline (ADR-0002) and office-text diffs.
5. **M5:** Advanced failure pause/resume semantics.

---

## Final Self-Review Checklist (before calling M0 “done”)
- [x] Engine/UI IPC is stable under streaming load (no hangs, no broken framing).
- [x] OpenAI key is persisted encrypted and never logged.
- [x] Workbench enforces sandbox and file add semantics.
- [x] Consent blocks first model call and re-prompts when scope changes.
- [x] Draft changes cannot delete files; invalid proposals never touch Draft.
- [x] Review is offline and shows clear text/CSV diffs.
- [x] Publish/discard are reliable and leave Workbench in a consistent state after failures.
