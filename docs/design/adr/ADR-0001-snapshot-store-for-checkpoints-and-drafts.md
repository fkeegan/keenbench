# ADR-0001: Snapshot Store for Checkpoints and Drafts

## Status
Proposed

## Context
KeenBench needs “hidden versioning” that enables:
- Auto-checkpoints before major actions (jobs, publish, restore).
- Reliable restore to previous Published states.
- A Draft workspace that is isolated from Published (v1: single Draft).

Constraints:
- Cross-platform desktop (macOS/Windows/Linux).
- Workbenches can include large binaries (up to 10 files × 25MB).
- UX must avoid Git concepts/terminology.
- v1 optimizes for correctness and simplicity; deep power-user branching is v1.5+.

We must choose how to store and restore snapshots of the Published tree (and how to materialize Draft efficiently).

## Decision
Use a **filesystem snapshot store** implemented as directory trees under the Workbench, with a “best available” materialization strategy:

- Workbench layout includes:
  - `published/` (approved)
  - `draft/` (mutable, present only when Draft exists)
  - `meta/checkpoints/<checkpoint_id>/published_snapshot/` (read-only snapshots of Published)
  - `meta/checkpoints/<checkpoint_id>/meta_snapshot/` (snapshots of context-relevant metadata like conversation + jobs; excludes `meta/checkpoints/`)

- Snapshot materialization strategy:
  1. Attempt file-level **hardlinks** from source trees into the snapshot trees (fast + space efficient).
  2. If hardlinking fails (filesystem limitation), fall back to **full copy**.

- Draft creation strategy:
  - Create `draft/` as a snapshot of `published/` using the same hardlink-or-copy approach.

- Write safety rule:
  - Engine must perform all file writes as **atomic replace** (`write temp → fsync → rename`) and avoid in-place mutation. This naturally “breaks” hardlinks on change, preserving snapshot integrity.

- Retention:
  - Implement a retention policy (count-based + optional size cap) that prunes old auto checkpoints first.

## Consequences
### Positive
- Simple mental model: “Checkpoints are snapshots of your Workbench state.”
- Cross-platform without bundling git binaries or exposing git workflows.
- Space efficient on common filesystems (hardlinks) while preserving a safe fallback path.
- Enables atomic publish/restore by directory swaps plus checkpointing.

### Negative
- Requires discipline in engine write paths (no in-place edits).
- Snapshot trees are still “real files,” so disk usage must be managed via retention.
- Future advanced features (true branching, merge) will require additional design (v1.5+).

## Alternatives Considered
- **Git-backed storage (e.g., `go-git`)**
  - Pros: native commits/branches, potential future power features, deduplication.
  - Cons: complexity, repo corruption/recovery surface area, large binary performance, and increased risk of “Git UX leakage” even if hidden.

- **Full directory copies only**
  - Pros: simplest to reason about; no hardlink write discipline required.
  - Cons: disk usage grows quickly with large binaries and frequent checkpoints.

- **Content-addressed object store**
  - Pros: strong deduplication; flexible restore.
  - Cons: overkill for v1; more bespoke complexity than snapshots.

## References
- PRD links:
  - `docs/prd/keenbench-prd.md` (FR2, FR4; Hidden Versioning; constraints)
  - `docs/prd/capabilities/draft-publish.md`
  - `docs/prd/capabilities/failure-modes.md`
  - `docs/prd/milestones/v1.md`
- Design links:
  - `docs/design/capabilities/draft-publish.md`
  - `docs/design/capabilities/checkpoints.md`
