# ADR-0004: Encrypted Local Secrets Store for Provider Keys

## Status
Accepted

## Context
KeenBench v1 uses a BYOK model: users provide API keys for OpenAI/Anthropic/Google/Mistral (see `docs/prd/capabilities/multi-model.md`). We need to store these keys locally so:
- the user doesn’t re-enter them on every launch,
- the engine can make model calls on demand,
- keys are not stored as plaintext on disk.

Constraints:
- Cross-platform desktop (macOS/Windows/Linux).
- v1 is local-first; no cloud account system for secrets.
- The app should minimize configuration burden (no mandatory passphrase UX in v1).
- The UI and engine are separate processes (keys should not be widely replicated/stored).

This ADR decides the v1 mechanism for at-rest key storage.

## Decision
Store provider API keys in a single **encrypted local file** owned by the engine:
- Secrets file: `secrets.enc` stored in the app’s global data directory (outside Workbenches).
- Encryption: AEAD (e.g., AES-256-GCM or ChaCha20-Poly1305) with per-install master key.
- Master key: randomly generated on first run and stored locally in a separate file with restrictive permissions (e.g., `0600`), alongside the secrets file.
- Format includes:
  - `schema_version`
  - `nonce`
  - `ciphertext`
  - optional integrity metadata (e.g., associated data including app id and schema version)

Operational rules:
- Only the engine reads/writes `secrets.enc`; UI passes keys over IPC for “set key” operations.
- Keys are never logged; logs must redact values by default.
- Clearing a key removes it from the encrypted store.

## Consequences

### Positive
- Keeps secrets out of plaintext JSON and out of Workbench directories.
- Cross-platform and does not require OS-specific keychain APIs in v1.
- Simple migration path via `schema_version`.
- Engine ownership reduces accidental leakage in UI logs/crashes.

### Negative
- Without an OS-protected secret or user passphrase, this is **best-effort at-rest protection**; a determined attacker with the same-user filesystem access can likely recover both key and ciphertext.
- Requires careful file permission handling across platforms.
- Future addition of OS keychain support may require migration/envelope encryption changes.

## Alternatives Considered
- **OS keychain / credential manager**
  - Pros: stronger at-rest protection bound to user/session.
  - Cons: platform-specific behavior and packaging; more complex to implement/test across macOS/Windows/Linux.

- **Plaintext config file**
  - Pros: simplest.
  - Cons: unacceptable for accidental disclosure risk.

- **User-supplied passphrase**
  - Pros: strong cryptographic posture without OS keychain.
  - Cons: adds onboarding friction; conflicts with “many users hate configuration”.

## References
- PRD links:
  - `docs/prd/capabilities/multi-model.md`
  - `docs/prd/keenbench-prd.md` (SR* safety requirements)
- Design links:
  - `docs/design/capabilities/multi-model.md`
  - `docs/design/design.md` (open decision: secure key storage)

