# Design Doc: KeenBench (Overview)

## Status
Draft (high-level system design)

## Version
v0.3

## Last Updated
2026-02-13

## Source Documents
- General PRD: `docs/prd/keenbench-prd.md`
- Capability PRDs: `docs/prd/capabilities/*`
- Milestones: `docs/prd/milestones/*`

## Design-Docs Strategy
This document stays **high-level** (architecture, data boundaries, major flows). Feature-level design docs should live alongside the capability PRDs and go deeper only where needed.

Recommended structure:
- `docs/design/design.md` (this doc): product + architecture overview
- `docs/design/capabilities/<capability>.md`: per-capability design (UI + engine API + storage + edge cases)
- `docs/design/adr/ADR-XXXX-*.md`: short ADRs for “one-way door” decisions (storage, IPC, diffing approach, etc.)

This keeps any single design doc small enough to fit comfortably in an AI context window while still being navigable via links.

Current deep-dives (v1):
- Workbench: `docs/design/capabilities/workbench.md`
- Workbench Context: `docs/design/capabilities/workbench-context.md`
- Workshop: `docs/design/capabilities/workshop.md`
- Multi-Model: `docs/design/capabilities/multi-model.md`
- File Operations: `docs/design/capabilities/file-operations.md`
- File Operations (Tabular Text): `docs/design/capabilities/file-operations-tabular-text.md`
- Document Styling: `docs/design/capabilities/document-styling.md`
- Draft / Publish: `docs/design/capabilities/draft-publish.md`
- Review / Diff: `docs/design/capabilities/review-diff.md`
- Network Egress & Upload Guardrails: `docs/design/capabilities/security-egress.md`
- Checkpoints: `docs/design/capabilities/checkpoints.md`
- Clutter Bar: `docs/design/capabilities/clutter-bar.md`
- Failure Modes & Recovery: `docs/design/capabilities/failure-modes.md`
- Onboarding: `docs/design/capabilities/onboarding.md`
- Test Framework (E2E + screenshots): `docs/design/test-framework-design.md`
- Accessibility: `docs/design/capabilities/accessibility.md`
- Snapshot storage ADR: `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md`
- In-app previews ADR: `docs/design/adr/ADR-0002-in-app-previews-for-review.md`
- IPC ADR: `docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md`
- Secrets storage ADR: `docs/design/adr/ADR-0004-encrypted-local-secrets-store-for-provider-keys.md`
- Atomic unit ADR: `docs/design/adr/ADR-0005-file-operation-batches-as-atomic-unit.md`
- Error taxonomy ADR: `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`
- Local tool worker ADR: `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md` (supersedes ADR-0007)
- SDK-based file operations ADR (superseded): `docs/design/adr/ADR-0007-sdk-based-file-operations.md`

## Technical Foundation
Selected stack (v1):
- Flutter (desktop UI) + Go engine (separate process)
- Monorepo structure: `app/` (Flutter) + `engine/` (Go)
- Developer workflow: repo-root `Makefile` for `run`, `build`, `clean`, etc.
- Configuration: `.env` for local development environment variables

## Testing & QA (M0)
- E2E coverage is driven by Flutter `integration_test` on Linux desktop.
- UI screenshots are captured via OS-level X11 tooling and stored under `artifacts/screenshots/` (gitignored).
- Details live in `docs/design/test-framework-design.md`.

## Product Overview (v1)
KeenBench is a desktop app where users add files into a bounded **Workbench**, then run AI-assisted workflows to produce deliverables (docs/decks/sheets/code) with **Draft → Review → Publish** safety.

Working mode:
- **Workshop**: interactive chat where the AI can either analyze files or perform Draft-producing edits based on user intent, saved per Workbench, with message-level undo and seamless model switching.

Core safety promises:
- AI can only read/write inside the Workbench.
- AI never edits Published directly (Draft-first).
- Users can review diffs/summaries and publish/discard atomically.
- Checkpoints allow restore to prior states.

## Terminology Clarification

**"Draft changes" vs "Proposal"**: These terms are related:
- **Draft changes** are file edits that land in Draft when the AI chooses to perform write operations. Analysis turns may produce no writes. (Internal storage/RPCs may still use the term "proposal".)

**Checkpoints vs Draft Revisions**: These are separate snapshot mechanisms:
- **Checkpoints** snapshot **Published** state. They are system-wide, visible to users, and enable restore to prior approved states.
- **Draft Revisions** (Workshop-internal) snapshot **Draft** state at each conversation point. They enable message-level undo to restore both chat and Draft together. Draft Revisions are never exposed in the Checkpoints UI.

Interaction: When a user Publishes a Draft, the system creates a Checkpoint of the pre-publish Published state (not the Draft). Draft Revisions for the just-published Draft are pruned (they're no longer meaningful since that Draft is gone). The new Published state becomes the baseline for future Checkpoints.

## Key Concepts (Data Model)
- **Workbench**: bounded container for added file copies, context items, and metadata; the AI's full scope.
- **Context Item**: a processed artifact (Agent Skill or direct injection) attached to a Workbench that provides persistent context across model calls. Four fixed categories: company-wide, department, situation, document style. See `docs/prd/capabilities/workbench-context.md`.
- **Published**: the currently approved Workbench state.
- **Draft**: an isolated writable state for AI changes (v1: one Draft at a time).
- **Checkpoint**: a restorable snapshot of Published (auto before major actions; optional manual). See terminology note above.
- **Draft changeset**: file edits applied to Draft when a turn requires write operations. Internally stored as a "proposal" artifact.
- **Draft Revision**: (Workshop-internal) a snapshot of Draft state tied to a conversation point, enabling message-level undo.
- **Conversation**: Workshop chat history scoped to a Workbench, with message-level undo and optional summarization.

## System Architecture

### Major Components
1. **Flutter Desktop App**
   - Workbench UI (file list, add/remove, scope banner)
   - Workshop UI (chat, model selector, message undo)
   - Review UI (diff/preview, delete confirmations)
   - Settings UI (provider keys, model defaults)
   - OS integration (file picker, "open file", accessibility)

2. **Go Engine (Local Service)**
   - Workbench storage + sandboxed filesystem access
   - Draft/Publish/Discard/Restore state machine
   - Checkpoints store
   - Diffing + file-type review adapters (text diff, docx text extraction, previews)
   - Model provider clients (BYOK keys, supported model registry, streaming)
   - File Operations Handler (local tool worker; JSON-RPC tool calls; Draft-only sandbox)
   - Context management (Clutter Bar scoring, summarization/compression)
   - Audit trail + local logs (and optional opt-in telemetry exporter later)

3. **External Model Providers**
   - OpenAI / Anthropic / Google Gemini / Mistral (BYOK in v1)

### Process Boundary + IPC
UI and engine are separate processes for isolation and responsiveness.

Proposed IPC characteristics:
- Request/response RPC for commands (create Workbench, add files, start job, publish, restore…)
- Server → client event stream for progress (streaming tokens, job stage updates, diff ready, errors)
- Versioned API surface (so UI/engine can evolve safely)

Implementation options (choose one early and ADR it):
Chosen (v1):
- JSON-RPC over stdio (`docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md`)

Alternatives considered:
- gRPC over loopback (strong typing; codegen for Dart/Go)

## Storage & State

### Workbench Storage Location
- Local-first by default (on the user’s machine).
- App-managed base directory (not user-configurable in v1).

### Proposed On-Disk Layout (Conceptual)
Each Workbench is a directory with:
- `published/` — user-visible approved files
- `draft/` — writable working set when a Draft exists (absent when no Draft)
- `meta/`
  - `workbench.json` (id, name, created/updated, limits, default model, etc.)
  - `conversation.jsonl` (Workshop messages + undo markers + summaries)
  - `context/<category>/` (processed context items: SKILL.md + references/ for skills, context.md for situation, source.json + source_file/ for raw input)
  - `checkpoints/<checkpoint_id>.json` (metadata + description)
  - `checkpoints/<checkpoint_id>/published_snapshot/...` (snapshot data; implementation-defined)
  - `checkpoints/<checkpoint_id>/meta_snapshot/...` (conversation/job history snapshot; excludes checkpoint store)

### Draft / Publish / Discard (v1)
State machine (simplified):
- No Draft → Create Draft (copy Published → Draft)
- Draft exists → AI writes only to Draft
- Review compares Draft vs Published
- Publish replaces Published with Draft (atomic) and creates a checkpoint of the pre-publish Published state
- Discard deletes Draft and returns to Published

See: `docs/design/capabilities/draft-publish.md`

### Checkpoints (Hidden Versioning)
v1 checkpoint implementation should optimize for correctness and simplicity:
- Snapshot Published at key points (before publish/restore; optional manual).
- Restore rewinds Published (and associated Workbench history) back to a snapshot.

Storage approach is a one-way-door decision. Current proposal:
- `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md` (snapshot store; hardlink-or-copy; atomic replace writes)

See: `docs/design/capabilities/checkpoints.md`

## Core Flows (High Level)

### Workbench Create / Open / Import
1. UI creates Workbench via engine.
2. User imports files via file picker/drag-drop.
3. Engine copies files into `published/`, enforces limits (10 files, 25MB each), and marks unsupported types as opaque binaries.
4. UI shows scope banner: “Workbench contains copies; originals untouched.”

See: `docs/design/capabilities/workbench.md`

### Workshop
1. User sends a message.
2. Engine builds context from selected files + conversation (with summarization if needed), including a Workbench file manifest and structural maps so the model knows files are already available and can read content on demand (see `docs/design/capabilities/file-operations.md`).
3. Engine streams model output to UI.
4. The AI may run analysis-only turns or perform write operations in Draft, depending on task intent. When writes occur, Draft changes are applied and surfaced for review.
5. User reviews Draft and publishes/discards.
6. Message-level undo rewinds conversation to a prior message and restores Draft state to match (“go back in time”), invalidating later messages (v1 linear history). Users can regenerate from a rewound point.
7. Model switching is immediate; new model continues from the saved conversation context and becomes the Workbench default model.

See: `docs/design/capabilities/workshop.md`

## Diffing & Review (v1)
Engine classifies files into review strategies:
- Text/code/config → inline line diff
- Office text documents (e.g., `.docx`, `.odt`) → inline diff (best-effort; like code review), with structured DOCX section comparisons when available
- Spreadsheets (e.g., `.xlsx`) → in-app side-by-side preview that zooms to changed areas when possible
- PDFs/images/other non-text → in-app side-by-side before/after preview
- PPTX → structured slide comparison (layout-lite first, positioned in later phase) with optional raster preview when available

Review screen needs:
- Added/Modified/Deleted list with size deltas for non-text
- Explicit delete confirmations before publish
- Fallback paths when diff/preview fails (in-app metadata view + summary when available)

See:
- `docs/design/capabilities/review-diff.md`
- `docs/design/adr/ADR-0002-in-app-previews-for-review.md`

## Context Management & Clutter Bar
The Clutter Bar is model-aware and reflects estimated context pressure from:
- file count
- file weights (type + size)
- conversation weight

The Clutter Bar is visible across Workbench UI surfaces (Workshop, Review/Diff, Checkpoints).

When approaching limits:
- summarize older conversation ("context compression")
- summarize or selectively include less-relevant file content
- keep UX simple: bar updates; actions are not blocked; compression is transparent to users

See: `docs/design/capabilities/clutter-bar.md`

## Provider Integration (BYOK, v1)
- Users configure providers in Settings (API keys stored securely/encrypted at rest).
- Keys are validated on save with a lightweight request.
- Supported models are a fixed allowlist in v1 (no dynamic model discovery).
- At least one provider must be configured to run Workshop.

Model selection rules:
- User default model → new Workbenches
- Workbench default model persists and is updated by Workshop model switching

## Security & Privacy
- **Filesystem sandbox**: engine must reject any path escaping the Workbench root (including symlink traversal); UI never reads arbitrary OS paths beyond explicit user file-add selection.
- **Network egress**: only configured provider endpoints; no URL fetching in v1; any future retrieval/upload requires explicit confirmation and is written to the audit trail.
- **Audit trail**: record models used, steps, files touched, checkpoints, estimate vs actual (best effort), and any egress approvals.

## Reliability & Failure Handling (v1)
- Auto-retry transient provider errors 3 times with exponential backoff.
- Preserve Draft on failures; never mutate Published on failure.
- Safe cancellation: stop at step boundaries and produce partial outputs + report.
- Restore via checkpoints when publish fails or user requests rollback.

See:
- `docs/design/capabilities/failure-modes.md`
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`

## Non-Goals (Design)
- Offline/local model execution in v1 (cloud-only model calls).
- OS-wide agent access outside Workbench.
- Multi-Workbench concurrency (v1 focuses on single Workbench at a time).

## Open Design Decisions (Candidates for ADRs)
1. Checkpoint/Draft storage approach (proposed in `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md`).

Decisions already captured in ADRs:
- IPC choice: `docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md`
- Secrets storage: `docs/design/adr/ADR-0004-encrypted-local-secrets-store-for-provider-keys.md`
- Atomic execution unit: `docs/design/adr/ADR-0005-file-operation-batches-as-atomic-unit.md`
- Structured error taxonomy: `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`
- SDK-based file operations: `docs/design/adr/ADR-0007-sdk-based-file-operations.md`
