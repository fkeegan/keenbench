# ADR-0005: File-Operation Batches as the Atomic Unit (Stop/Resume)

## Status
Accepted

## Context
AI-assisted execution needs a crisp definition of "atomic step" to support:
- Safe cancellation ("stop after the current step").
- Crash recovery and resume.
- Consistent audit logging (what changed in each step).

Constraints:
- Workbench contains multiple files, including large binaries.
- Draft/Publish relies on atomic replace writes and snapshot integrity (ADR-0001).
- Concurrent writes to Draft must be avoided.

The general design doc lists "atomic step definition for execution" as a one-way-door decision (`docs/design/design.md`).

## Decision
Define the atomic unit of AI execution as a **file-operation batch**:
- A batch is a validated set of filesystem operations (create/update/delete/rename) applied by the engine to the Draft view.
- The engine applies a batch transactionally:
  - each write uses atomic replace (`write temp → fsync → rename`),
  - batch completion is recorded only after all operations succeed,
  - if a batch fails, the engine reports failure and leaves Draft in a consistent state (best-effort rollback; otherwise checkpoint restore path).

Cancel/stop semantics:
- User cancellation is checked **between batches**.
- When stopping is requested (user cancel), the engine completes the current batch (if safe), persists state, and stops before starting the next batch.

Resume semantics:
- `tracker.json` records `last_completed_batch_id`.
- Resume starts at `last_completed_batch_id + 1`, using the Draft filesystem as the source of truth.

## Consequences

### Positive
- Aligns with snapshot integrity and avoids half-written files.
- Provides a predictable “stop after current step” user promise.
- Gives a stable boundary for progress reporting.
- Makes audit logs and recovery logic straightforward (batch IDs are durable).

### Negative
- Requires a well-designed batch format and careful validation.
- Some user-visible “steps” may take longer if a batch is large; coarse batching can reduce responsiveness.
- Rollback inside a batch is non-trivial for deletes/renames without staging; may rely on checkpoints for recovery in some cases.

## Alternatives Considered
- **Per-file operation as the atomic unit**
  - Pros: more granular progress and stopping.
  - Cons: higher overhead; harder to keep multi-file invariants; can create inconsistent intermediate states across related files.

- **Per-plan-step as the atomic unit**
  - Pros: maps cleanly to UI “step” progress.
  - Cons: too coarse; a plan step can touch many files and can leave Draft inconsistent if interrupted.

- **Full “transactional FS” staging directory per step**
  - Pros: stronger rollback guarantees.
  - Cons: heavier implementation complexity and disk usage; unnecessary for v1 given checkpoint safety net.

## References
- PRD links:
  - `docs/prd/capabilities/failure-modes.md`
- Design links:
  - `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md`

