# ADR-0006: Structured Error Codes and Failure Taxonomy

## Status
Accepted

## Context
KeenBench needs predictable failure handling across:
- Workshop streaming + proposal/apply flows
- Workbench filesystem operations (add/remove)
- Draft/Publish, Checkpoints/Restore
- Network egress guardrails and consent gating

Constraints:
- UI↔engine IPC is JSON-RPC over stdio (ADR-0003), so errors must be represented in a JSON-RPC-compatible way.
- The UI must show **actionable** recovery affordances (retry, resume, switch model, open settings, restore checkpoint) without relying on brittle string matching.
- Errors must be logged to job/workbench artifacts for auditability, while avoiding raw prompt/file content by default.

Without a stable taxonomy, each capability risks inventing its own ad-hoc errors and UI logic, making recovery inconsistent and hard to evolve.

## Decision
Define a stable, shared **ErrorInfo** payload and a canonical **error_code taxonomy** used by:
- JSON-RPC error responses
- long-running job notifications (pause/fail events)
- persisted job/workbench event logs and reports

### JSON-RPC Error Envelope
For application-level failures, return a JSON-RPC `error` object with:
- `code`: `-32000` (generic server error; JSON-RPC reserved range)
- `message`: short human-readable summary suitable for UI display
- `data`: an `ErrorInfo` object (below)

Rationale:
- keep JSON-RPC `code` simple and spec-compliant,
- route all UI branching through stable `data.error_code` and `data.actions[]`.

### ErrorInfo Shape (v1)
`ErrorInfo` (conceptual fields):
- `error_code` (string, stable): e.g., `EGRESS_CONSENT_REQUIRED`
- `phase` (string enum): `workbench|workshop|review|publish|restore|settings`
- `subphase?` (string enum, optional): `stream|proposal|apply|add_files`
- `retryable` (bool): whether the engine considers automatic retry safe (and whether UI should offer retry/resume)
- `actions[]` (string enum): recommended user actions
- `provider_id?`, `model_id?` (optional): when provider/model involvement is relevant
- `job_id?`, `workbench_id?` (optional): correlation
- `detail?` (string, optional): short extra context (no file contents)
- `detail_ref?` (string path, optional): reference to a local artifact file with extended structured details

### Canonical error_code Taxonomy (v1)
The set is intentionally small and cross-cutting; capabilities may add fields in `detail_ref` rather than creating new codes casually.

Egress / consent:
- `EGRESS_CONSENT_REQUIRED`
- `EGRESS_BLOCKED_BY_POLICY`

Provider / network:
- `PROVIDER_NOT_CONFIGURED`
- `PROVIDER_AUTH_FAILED`
- `PROVIDER_RATE_LIMITED`
- `PROVIDER_UNAVAILABLE`
- `NETWORK_UNAVAILABLE`

Sandbox / validation:
- `SANDBOX_VIOLATION`
- `VALIDATION_FAILED`
- `CONTEXT_EXHAUSTED`

Filesystem / storage:
- `DISK_FULL`
- `FILE_READ_FAILED`
- `FILE_WRITE_FAILED`
- `CHECKPOINT_CREATE_FAILED`
- `CHECKPOINT_RESTORE_FAILED`

Workflow / gating:
- `BUDGET_CAP_REACHED`
- `CONFLICT_PUBLISHED_CHANGED`
- `USER_CANCELED`

### Canonical actions[] (v1)
The UI maps `actions[]` to affordances:
- `retry` (re-run the failed operation from the same safe boundary)
- `resume` (continue a paused job)
- `switch_model` (choose a different provider/model, then retry/resume)
- `open_settings` (configure provider keys, enable providers)
- `restore_checkpoint`
- `free_disk_space`
- `view_report`
- `review_draft`
- `discard_draft`

### Logging Contract
When a failure occurs, the engine must:
- emit an event/notification containing `ErrorInfo`,
- persist a corresponding event to:
  - Workshop: `meta/conversation.jsonl` as a `system_event`
  - Workbench-level: `meta/workbench_events.jsonl` (recommended)

## Consequences

### Positive
- Consistent recovery UX across the product without fragile string parsing.
- Stable analytics/debugging fields (locally) and more legible job reports.
- Enables future UI iteration (new buttons, better copy) without changing engine behavior.
- Encourages keeping error information safe by default (no prompt/file content in `detail`).

### Negative
- Requires ongoing discipline: new error scenarios must map to existing codes or justify adding a new one.
- Up-front effort to keep per-provider error mapping consistent.
- A too-small taxonomy can become overloaded; mitigate via `detail_ref` and careful `phase/subphase`.

## Alternatives Considered
- **String matching on error messages**
  - Pros: fastest to start.
  - Cons: brittle, hard to localize, inconsistent behavior, and difficult to evolve.

- **Use JSON-RPC numeric codes for every error**
  - Pros: single field to branch on.
  - Cons: awkward to manage and extend; less readable in logs; more coupling.

- **gRPC status codes**
  - Pros: typed status and rich details.
  - Cons: IPC choice is already JSON-RPC in v1 (ADR-0003).

## References
- `docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md`
- `docs/prd/capabilities/failure-modes.md`
- `docs/design/capabilities/failure-modes.md`
- `docs/design/capabilities/security-egress.md`
