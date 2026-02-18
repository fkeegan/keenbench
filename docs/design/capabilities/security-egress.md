# Design: Network Egress & Upload Guardrails

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/security-egress.md`
- `docs/prd/keenbench-prd.md` (SR2, SR3, SR5)
- Related:
  - `docs/prd/capabilities/multi-model.md`
  - `docs/prd/capabilities/workshop.md`

## Summary
This capability ensures KeenBench’s network behavior is:
- **bounded** (only official model-provider endpoints in v1),
- **explicit** (user consent before sending Workbench content to providers),
- **auditable** (egress activity is recorded in job/workbench logs),
- **non-expanding by default** (no background browsing, no URL fetching in v1).

Key v1 choices (confirmed):
- Network egress is limited to official OpenAI/Anthropic/Google/Mistral model endpoints (no custom endpoints).
- "Fetch URL" / external retrieval is **not implemented** in v1.
- Upload confirmations are **not per-call**; they are one-time per **Workbench** and are re-triggered only when scope/provider changes.

## Goals / Non-Goals

### Goals
- Prevent hidden network calls and accidental exfiltration beyond configured model providers.
- Make the "upload boundary" legible: users should know which provider(s) are used and which Workbench files are in scope.
- Record sufficient audit data to answer "what was sent where?" at a high level without storing raw prompts.

### Non-Goals
- Web browsing, background retrieval, or connectors (v1).
- Fine-grained DLP/exfil detection (PII scanning, secret scanners) in v1.
- Per-request upload prompts (too much friction).
- Perfect accounting of bytes/tokens sent (best-effort is acceptable).

## User Experience

### Always-Visible Indicators
When the app may send Workbench content externally, the chrome should continuously show:
- **Provider + model** currently in use.
- **Scope**: "Workbench files" (all files in scope by default).

### Consent Model (v1)
Consent is collected at the moment model calls would begin.

Workshop (per Workbench):
- Trigger: first assistant call in a Workbench **or** when switching to a provider that hasn’t been consented **or** when Workbench scope changes (files added/removed).
- Dialog content:
  - Provider + model name.
  - Statement that Workbench content may be sent to the provider to answer prompts.
  - File list (current Workbench files) and sizes.
  - Checkbox: “Don’t ask me again for this Workbench” (recommended default: checked). If unchecked, consent applies only to the current session and will be requested again on next app start (even if provider/scope are unchanged).
  - Buttons: **Continue** / **Cancel**.
- After consent: no further prompts until provider/scope changes again.
  - If “Don’t ask me again” is checked, consent persists across app restarts.
  - If unchecked, consent resets on app restart.

Recommended copy:
> “KeenBench will send selected Workbench content to the model provider(s) to generate responses. No web browsing or URL fetching occurs in v1.”

### "Fetch URL" Behavior (Not Implemented in v1)
If a user requests web retrieval (e.g., "look up this URL"):
- The Workshop assistant should respond with a consistent limitation message and ask the user to paste relevant text instead.
- The engine should not expose any tool/RPC to fetch arbitrary URLs.

## Architecture

### UI Responsibilities (Flutter)
- Display current provider/model and file scope indicators.
- Present consent dialogs and block the first model call until the user confirms.
- Ensure consent UX is accessible (keyboard, screen reader announcements, clear wording).

### Engine Responsibilities (Go)
- Enforce the network allowlist: **block all HTTP(S) requests** except to approved provider endpoints.
- Provide a single internal “egress client” abstraction used by:
  - provider key validation calls
  - model inference calls (streaming + non-streaming)
- Track and persist consent state per Workbench (provider + scope revision).
- Record egress audit events without storing raw prompts by default.

### IPC / API Surface
API names are illustrative (JSON-RPC per ADR-0003).

**Consent**
- `EgressGetConsentStatus(workbench_id) -> {workshop: {consented, provider_id?, scope_hash?}}`
- `EgressGrantWorkshopConsent(workbench_id, {provider_id, scope_hash}) -> {}`

**Audit (read-only for UI)**
- `EgressListEvents(workbench_id, {job_id?}) -> {events[]}`
  - `events[]`: `{timestamp, kind, provider_id, model_id?, scope_hash, tokens_in?, tokens_out?, usd_estimate?}`

Note: Workshop RPCs should fail fast with a structured error if consent is missing:
- `error_code = EGRESS_CONSENT_REQUIRED` and include the required provider/scope.
- See also: `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`

## Data & Storage

### Consent State
Persist consent state locally per Workbench:
- `meta/egress_consent.json`
  - `schema_version`
  - `workshop`: `{provider_id, model_id, scope_hash, consented_at}`

`scope_hash` should be computed from the ordered list of in-scope file IDs + fingerprints (size+mtime or sha256 if available) so the engine can detect scope changes deterministically.

### Egress Event Log
Record an append-only log: optionally append `egress_*` events to `meta/conversation.jsonl` as `system_event`s for a user-visible audit trail without exposing raw content.

## Algorithms / Logic

### Network Allowlist (v1)
All engine network calls must go through a policy gate that enforces:
- `https://` only (no plaintext `http://`).
- Host allowlist for official endpoints:
  - OpenAI: `api.openai.com`
  - Anthropic: `api.anthropic.com`
  - Google Gemini: `generativelanguage.googleapis.com`
  - Mistral: `api.mistral.ai`
- No direct IP-literal destinations (avoid bypassing DNS allowlist).
- No arbitrary redirects to non-allowlisted hosts.

If the request violates policy:
- fail fast with a structured error (`EGRESS_BLOCKED_BY_POLICY`)
- surface a user-friendly message explaining that only configured providers are reachable in v1.

### Consent Checking
Before every model inference call that can include Workbench content, the engine verifies:
- provider key exists and is configured
- consent exists: consented provider + current scope hash

The engine should not "auto-consent" silently.

Provider key validation calls are initiated explicitly from Settings and do not require Workbench/job consent (they do not upload Workbench file content).

### Upload Guardrails (Model Calls)
Within the allowlisted provider calls, “upload” is still meaningful: file contents are sent as part of prompts or attachments.

Guardrails (v1):
- Only include Workbench files that are in scope (default: all Workbench files).
- Ensure that any extracted text or previews used for prompting originate only from Workbench content.
- Record which files were in scope at time of consent (scope hash + file list).

## Error Handling & Recovery
- **Blocked by policy**: surface error; allow user to continue without external retrieval (v1 fetch not supported) or switch model/provider.
- **Provider unreachable**: retry with exponential backoff (per PRDs); keep Draft safe; offer switching providers.
- **Upload failure** (request sent but connection dropped mid-stream): treat as transient failure; retry with exponential backoff up to 3 times. If retries exhausted, pause execution and surface error with options to retry manually or cancel. Draft state is preserved.
- **Consent missing**: block model call; show consent UI; do not partially execute.
- **Scope changed mid-job**: treat as a job precondition violation; pause and require user confirmation before continuing (v1 default: block and ask).

## Security & Privacy
- Default deny: only official provider endpoints.
- Consent is explicit and stored locally; UI keeps provider + scope visible.
- In production, egress audit logs exclude raw file contents and raw prompts by default. In debug mode, raw prompt/file excerpts may be logged for development triage and must be clearly labeled and off by default.
- No URL fetch feature in v1 reduces prompt injection exfil vectors.

## Telemetry (If Any)
v1: none.

Local-only debug metrics (optional):
- Egress blocked events by reason.
- Provider error rates (timeouts, 401/403).

## Open Questions
None currently.

## Self-Review (Design Sanity)
- Enforces the v1 promise: only official provider endpoints; no browsing/fetching.
- Uses consent at meaningful boundaries (per Workbench) without prompting on every model call.
- Keeps auditability without storing raw prompt data by default, aligning with user trust goals.
- Resolves the main UX/security tension by requiring re-consent on any scope change (and making consent persistence explicit).
