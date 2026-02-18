# OpenClaw Learnings That May Benefit KeenBench

This note captures OpenClaw patterns that look transferable to KeenBench,
given KeenBench's current direction:

- Community tier is local-only.
- Workbench model selection is manual (no silent fallback to other providers/models).
- Safety/consent rules stay strict (explicit egress consent; no UI touching files directly).

OpenClaw repo (code references below):

- https://github.com/openclaw/openclaw

## High-Value, Low-Philosophy-Change Ideas

### 1. Auth Profiles as a First-Class Abstraction

Even if KeenBench stays BYOK, a single "API key per provider" model tends to break
down in real usage (multiple accounts, multiple keys, rotating keys, separating
personal vs work, subscription auth vs API keys).

OpenClaw patterns worth copying:

- Typed credentials (`api_key`, `oauth`, `token`) and a single local store.
- Per-provider profile ordering and display labels.
- One store as the "token sink" to avoid refresh token invalidation conflicts.

References:

- Types: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/types.ts
- Store read/merge: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/store.ts
- Ordering: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/order.ts
- Cooldown tracking: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/usage.ts

### 2. Manual Rotation + Cooldowns (Explicit User Choice)

OpenClaw does automatic rotation and model fallback. KeenBench can keep the useful
parts (tracking and guidance) while staying strictly user-driven:

- Track per-profile cooldowns/disabled windows by failure category.
- Sort cooled-down profiles to the bottom in the UI.
- Show actionable messaging: "This credential is cooling down for 25m."
- Offer explicit actions: "Switch credential..." and optionally "Try next profile".

References:

- Cooldowns/backoff: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/usage.ts
- Profile ordering (oauth > token > api_key; round robin by lastUsed): https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/order.ts
- Human-facing status aggregation: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-health.ts

### 3. Strict Provider Separation for Subscription vs API

OpenClaw treats subscription-backed access as a distinct provider namespace
(example: `openai-codex/...` vs `openai/...`) rather than "just another auth mode".

This helps KeenBench by:

- Avoiding accidental use of a subscription token against an API endpoint.
- Keeping egress allowlists and consent bindings clean (`provider_id + model_id`).
- Allowing different UI labels and limitations to be explicit.

Reference:

- OpenAI docs in OpenClaw: https://raw.githubusercontent.com/openclaw/openclaw/main/docs/providers/openai.md

### 4. Concurrency-Safe Local Stores (Locking + Atomic Writes)

Any local "state file" that can be updated by multiple processes (or multiple
threads in the same process) benefits from:

- File locking around updates.
- Atomic write patterns (write temp + rename).

OpenClaw uses file locking for the auth store to avoid clobbering cooldown/usage
data during concurrent updates.

Reference:

- Auth store locking: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-profiles/store.ts

For KeenBench, the same pattern is relevant for:

- Provider secrets/profile store.
- Consent state and audit logs (if multiple writers exist).
- Workbench meta files updated from multiple code paths.

### 5. Better Provider Diagnostics (Without Auto-Switching)

Even if model selection is manual, users need quick answers to:

- "Do I have a credential configured for this provider?"
- "Is it expired / missing / rejected?"
- "Am I rate-limited and for how long?"

OpenClaw’s "status/doctor" idea is mostly about surfacing state and next actions,
not about auto-switching.

References:

- Auth health summary: https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/auth-health.ts
- Auth UX doc (copy warnings and failure-mode phrasing): https://raw.githubusercontent.com/openclaw/openclaw/main/docs/gateway/authentication.md

Potential KeenBench adaptation:

- Engine RPC: `providers.status` returns per-provider/per-profile status and last error category.
- Flutter Settings: show a "Status" section under each provider card.

### 6. Failure Classification as Structured Data (Not String Matching in UI)

OpenClaw has extensive heuristics to bucket errors into reasons (auth, rate limit,
billing, timeout, etc.) and uses those for cooldowns and user messaging.

KeenBench already has an error taxonomy and safety/consent boundary; the transferable
idea is:

- Keep UI logic simple: engine returns `errinfo.code`, `category`, `retryAfterMs?`,
  `cooldownUntil?`, etc.
- Use those structured fields for UX: disabled buttons, "Try again at...", etc.

Reference (conceptual; OpenClaw implements this across helpers/tests):

- Failover reason classification docs mention helper functions and test coverage:
  https://raw.githubusercontent.com/openclaw/openclaw/main/docs/pi.md

### 7. Permissions and Secret Hygiene Documentation

OpenClaw is explicit about local permissions (chmod) and what files hold secrets.
KeenBench already intends to encrypt secrets at rest; this is still useful to reduce
accidental leaks and support burden.

Reference:

- Security docs (paths/permissions): https://raw.githubusercontent.com/openclaw/openclaw/main/docs/gateway/security/index.md

Potential KeenBench adaptation:

- Document where KeenBench stores encrypted secrets, how to wipe them, and log redaction rules.

## Ideas That Are Potentially Useful, But Need Careful Fit

### A. Reusing Other Tools’ Credentials (Keychain / External CLIs)

OpenClaw reads credentials from other CLIs (Codex, Claude Code, etc.) to reduce
auth friction. This can be a major UX win but may not align with KeenBench’s goals
or desired threat model.

Reference:

- CLI credential import (Keychain + file fallbacks): https://raw.githubusercontent.com/openclaw/openclaw/main/src/agents/cli-credentials.ts

If KeenBench ever does this, keep it explicit:

- "Import from Codex/Claude Code" button; do not silently read arbitrary files.

### B. Session/Transcript Repair and Compaction

OpenClaw has a lot of machinery to keep long-running sessions healthy (repairing
transcripts, handling tool-call ordering, compaction retries).

This could become relevant if KeenBench workbenches become long-lived with rich
tool usage, but it’s not obviously needed today.

## Likely Not A Fit For KeenBench (Given Current Direction)

- Multi-channel chat gateway patterns (OpenClaw is a messaging gateway-first product).
- Automatic model/provider fallback and silent best-effort selection.
- Extensive external automation scripts for gateways/phones (Termux/systemd).

## Concrete Next Steps (If/When Implementing)

1. Define `ProviderID` namespace rules (e.g., `openai` vs `openai-codex`) and keep egress allowlists keyed to provider.
2. Introduce an auth-profile store schema (encrypted at rest) with types `api_key|oauth|token` and per-profile cooldown stats.
3. Add engine-side error classification fields (`category`, `retryAfterMs`, `cooldownUntil`) and surface them in the Settings UI.
4. Add manual "Switch credential" UI and a single explicit "Try next profile" action (no automatic switching).

