# Design: Workshop

## Status
Draft (v1)

## Version
v0.6

## Last Updated
2026-02-16

## PRD References
- `docs/prd/capabilities/workshop.md`
- `docs/prd/keenbench-prd.md` (Workshop definition; FR5)
- Related: `docs/prd/capabilities/multi-model.md`, `docs/prd/capabilities/draft-publish.md`, `docs/prd/capabilities/review-diff.md`, `docs/prd/capabilities/clutter-bar.md`, `docs/prd/capabilities/failure-modes.md`
- Design details:
  - `docs/design/capabilities/multi-model.md`
  - `docs/design/capabilities/draft-publish.md`
  - `docs/design/capabilities/review-diff.md`
  - `docs/design/capabilities/file-operations.md`
  - `docs/design/capabilities/file-operations-tabular-text.md`
  - `docs/design/capabilities/security-egress.md`
  - `docs/design/capabilities/clutter-bar.md`
  - `docs/design/capabilities/failure-modes.md`
  - `docs/design/capabilities/document-styling.md`

## Summary
Workshop is the interactive mode: a Workbench-scoped chat with streaming responses, auto-applied Draft changes (no separate propose/approve step), saved conversation history, seamless model switching, and **message-level undo (“rewind”) + regenerate** that restores both conversation and Draft file state (“go back in time”).

Key v1 choices (v1 target):
- The assistant may complete read-only turns or perform Draft-producing write operations based on task intent.
- When a Draft appears (or already exists on Workbench reopen), Workshop auto-opens Review.
- Review-critical summaries (Docx + non-text) are generated during draft generation/execution and stored; review does not make model calls.
- Users can **regenerate** an assistant response from any rewound point (and after interrupted streaming) without branching history.
- Rewind restores both conversation and Draft state to the selected point (stronger than PRD; intentional).
- Draft changes are generated against Published; Workshop input is blocked while a Draft exists.
- Publish checkpoints are rendered as inline chat system cards with a Restore action; restore stays blocked while a Draft exists.

## Goals / Non-Goals
### Goals
- Fast, low-friction iteration with tight feedback loops.
- Keep the user in control: AI edits only happen inside Draft; users publish or discard explicitly.
- Preserve safety invariants:
  - Scope is Workbench-only.
  - Published is never edited directly.
  - Users can always discard or publish atomically.
- Support message-level undo (“rewind”) and regenerate from a prior point, keeping chat and Draft file state aligned.
- Make model switching easy and transparent; switching persists as the Workbench default.

### Non-Goals
- Branching/parallel conversations in v1 (linear history only).
- “Try with another model” fork (v1.5).
- Multi-user collaboration or real-time co-editing.

## User Experience

### Layout (v1)
- Left: Workbench file list + Clutter Bar (shared Workbench chrome).
- Main: Chat transcript.
- Top bar: Workbench name + current model selector + icon-only Checkpoints action (history icon).
- Bottom-left of file panel: icon-only Settings action (gear icon).
- Composer: input + send (Enter to send; Shift+Enter for newline). Hidden when a Draft exists.
- Draft actions (when a Draft exists): **Publish**, **Discard** (composer hidden; Review is auto-opened).
- Messages render with selectable text and a small icon-only copy button; assistant messages render markdown and disable link navigation.

### Analysis-Only Turns
Workshop supports both:
- analysis/Q&A turns that may complete without Draft writes, and
- edit turns that produce Draft changes for review.

### Draft Status & Reminders
- When a Draft exists, show a persistent “Draft in progress” indicator that includes:
  - created timestamp (e.g., “Draft from Jan 25”)
  - source (Workshop)
- Draft age reminder (from `docs/design/capabilities/draft-publish.md`): if a Draft is older than 24 hours, show a gentle reminder on Workbench open and when entering Workshop:
  - “You have an open Draft from [date]. Review and publish or discard?”
  - Actions: **Review** / **Discard** / **Dismiss**

### Conversation Semantics
- Normal assistant replies are explanatory/planning guidance only; file mutations are applied only through the draft pipeline.
- Conversation is restored per Workbench on reopen.
- Conversation is linear. Undo rewinds to an earlier point and discards later messages.

### Publish Checkpoint Cards in Chat
- Successful publish appends a system event card in the Workshop timeline with checkpoint details and a **Restore** action.
- Restore from a chat checkpoint card calls `CheckpointRestore`, then refreshes Workbench state.
- Restore is disabled while a Draft exists, with guidance to publish or discard first.
- Chat cards are a convenience surface for publish checkpoints only; full history/manual checkpoint actions remain in the Checkpoints screen.

### Draft Changes Flow (v1)
1. User and assistant converge on what to do via normal chat.
2. The assistant decides whether the turn requires writes:
   - analysis turns: no Draft writes,
   - edit turns: execute file operations against Draft.
3. App shows an “Applying draft changes…” state when writes are in progress.
4. Engine applies writes immediately in the background (no approval step).
5. On success, the app transitions to Draft state and auto-opens Review.
6. On failure, the assistant responds with a clear limitation + suggested alternative; Draft remains unchanged.

### Prompt Assembly + File Context Contract (v1)
Every Workshop model call must include explicit Workbench file context so the model does not ask users to re-upload files that already exist.

**Required prompt payload**
- **File manifest**: for every Workbench file, include path, type, size, and whether content is available (readable vs opaque).
- **Structural maps**: for structured files (xlsx, docx, pptx, pdf) and tabular text files (csv), include the file map (metadata describing regions, sizes, chunk boundaries, and for csv: schema/row metadata). See `docs/design/capabilities/file-operations.md` and `docs/design/capabilities/file-operations-tabular-text.md`.
- **Small text files**: for text files below the chunk threshold, full content may be inlined directly.
- **Unavailable content**: if extraction fails or the format is opaque, include a placeholder that the file is present but content is unavailable (plus the reason if known).
- **Context items** (always-inject representation): all active Workbench context items are represented in every model call. For Document Style, representation may be either standalone `document-style` skill injection or format-gated style-skill injection (merged when possible, generic fallback on merge failure):
  - **Situation context** (if present): injected as a clearly delimited section in the system message, after the file manifest and before the conversation history.
  - **Agent Skills** (company-wide, department, and document-style only when no format-gated style skill is injected for the call): full `SKILL.md` content plus any referenced files (e.g., `references/summary.md`) injected as clearly delimited skill sections in the system message.
  - See `docs/prd/capabilities/workbench-context.md` for category details and storage layout.
- **Format style skills** (format-gated): when the Workbench contains or is expected to produce `.xlsx`, `.docx`, or `.pptx` files, the corresponding bundled generic format style skill is injected. If a Document Style context item is present, it is merged with the generic skill and injected as one merged skill (not as two separate skills). If merge fails, inject the generic style skill and surface a non-blocking notice. See `docs/design/capabilities/document-styling.md` for format-gating algorithm, merge logic, and injection order.

**Required system instruction**
- The listed files are already present in the Workbench.
- Do **not** ask the user to upload files that appear in the manifest.
- Use the structural map to understand file layout, then use `read_file`/table tools with specific selectors (sheet+range, page range, section, row-range/query) to read content on demand.
- For large files, read in chunks rather than requesting all content at once.
- For CSV workflows, prefer table describe/stats/query/export tools over line-by-line text parsing.
- For CSV broad retrieval, run count-first probes and then use model-selected query chunking/windows instead of one-shot full result pulls.
- For delimiter-sniffed tabular files, treat format inference as best-effort and validate assumptions before high-impact writes/exports.
- If Workbench context items are present, follow their instructions (skills provide behavioral guidance; situation context provides project-specific state).
- If format style skills are present, use the tool capability catalog as the authoritative reference for style parameters. Do not hallucinate parameters not in the catalog.

**Fail-safe**
- If the engine cannot assemble a complete file manifest, it must **fail the model call** with a structured error (per ADR-0006) rather than sending a prompt without file context.

### OpenAI Request Profile (v1 Runtime Defaults)
For OpenAI Workshop calls, the engine applies a deterministic profile to reduce drift and improve tool-grounded behavior in large-file workflows:

- `truncation=disabled`
- reasoning effort policy:
  - default: `reasoning.effort=medium`
  - RPI phase override (OpenAI providers only): Research/Plan/Implement use provider-configured values from Settings → Model Providers:
    - OpenAI API (`openai:gpt-5.2`): `none|low|medium|high`
    - OpenAI Codex (`openai-codex:gpt-5.3-codex`): `low|medium|high|xhigh`
  - Summary and non-RPI Workshop calls continue using default `medium`
- sampling compatibility policy:
  - for GPT-5 family models, omit `temperature` / `top_p` by default (official GPT-5.2 compatibility guidance rejects these fields unless specific model/effort conditions are met)
  - for non-GPT-5 models, keep deterministic sampling defaults: `temperature=0`, `top_p=1`
- function tools are sent with `strict=true`; parameters are normalized to strict-mode requirements (object schemas set `additionalProperties=false`, and properties are marked required with optional fields represented as nullable)
- `parallel_tool_calls=false` for tool-capable calls
- tool choice policy:
  - first tool turn (no prior `tool` output in the turn history): `tool_choice=required`
  - subsequent turns (after at least one tool result has been provided): `tool_choice=auto`

This policy is intended to keep early turns grounded in local tool evidence, then allow the model to conclude naturally once enough evidence has been gathered.

### Apply Draft Changes (v1)
- Creates a Draft if none exists, then applies changes to the Draft view automatically.
- If apply succeeds, Workshop records a new “Draft revision” so rewind can restore this exact state.
- If apply fails, the assistant responds with a clear limitation + suggested alternative; Draft is left unchanged.

#### When a Draft Already Exists
Workshop input is blocked while a Draft exists (v1: one Draft total).

Rules:
- The composer is replaced with Publish/Discard actions.
- Entering Workshop with an existing Draft auto-opens Review.
- Starting a new session while a Draft exists is handled by the Draft subsystem prompt (**Publish / Discard / Cancel**) per `docs/design/capabilities/draft-publish.md`.

### Message-Level Undo (“Go Back in Time”)
User can select a prior message and choose **Rewind to here…**.

Confirmation copy (recommended):
> “Rewinding will discard all messages after this point and revert Draft changes made after this point.”

Effects:
- Conversation head moves to the selected message.
- Draft is restored to the exact state that was current at that message (or discarded if no Draft existed yet).
- Any draft changesets/messages after that point are considered inactive and no longer influence context (v1 linear history).

#### Regenerate After Rewind
After a rewind, the transcript shows a “Rewound to here” banner with a **Regenerate** button when the head is a user message. Regenerate creates a fresh assistant response from the current head (no branching; the new message becomes the head).

### Model Switching
- Model selector is always visible.
- Switching is immediate, no confirmation dialog.
- A “switched model” system message is inserted into the transcript (for auditability).
- The selected model becomes the Workbench default for future Workshop turns.

### Accessibility
- Keyboard shortcuts for: focus composer, send, return to review, publish, discard, undo.
- Screen readers announce: model changes, draft changes readiness, draft created, publish/discard outcomes.
- Undo and destructive actions use explicit text confirmations (no color-only cues).

### Regenerate (General UX)
Regenerate is available in two places:
- **Assistant message overflow menu**: “Regenerate response”
  - Semantics: discards that assistant message and all messages after it (linear history), then regenerates a new assistant message from the preceding user message.
- **After a rewind** (when the head is a user message): the “Rewound to here” banner includes **Regenerate**.

Keyboard shortcut (suggested): `Ctrl/Cmd+R` to regenerate the most recent assistant response (or regenerate from the current head after a rewind).

## Architecture

### UI Responsibilities (Flutter)
- Render transcript with streaming assistant messages.
- Show auto-apply status and Draft state banner/actions.
- Auto-navigate to Review when a Draft is created or already present on Workbench reopen.
- Render inline publish checkpoint system cards in chat and enforce restore-disabled state while Draft exists.
- Provide a consistent Undo affordance per message and confirm impact.
- Provide a consistent Regenerate affordance per assistant message and after rewind.
- Provide model selector and show current model at all times.
- Drive Review/Diff UI for the current Draft.

### Engine Responsibilities (Go)
- Persist Workshop conversation and active head pointer per Workbench.
- Build model context from Workbench files + context items + conversation (with summarization if needed).
- Assemble the **file manifest + structural maps + active context items** for every model call and include the explicit system instructions about file availability and context (see `docs/design/capabilities/file-operations.md` and `docs/design/capabilities/file-operations-tabular-text.md` for map-first/tabular strategy; see `docs/prd/capabilities/workbench-context.md` for context items).
- Never send a prompt without the manifest; fail fast with a structured error if the manifest cannot be built.
- Stream assistant tokens to UI.
- Execute AI-requested file operations and apply resulting writes to Draft; keep Draft unchanged for read-only turns.
- Apply draft changes to Draft safely; create Draft revisions for time travel.
- Generate and store required per-file summaries during draft generation/execution.
- Append conversation `system_event` records for publish checkpoint creation and restore completion (with checkpoint metadata) so chat can render inline cards.
- Enforce sandbox boundaries for all file reads/writes and prevent path traversal.
- Update Clutter Bar metrics based on file + conversation growth.

### IPC / API Surface
API names are illustrative.

Two internal Workshop execution paths can coexist:
- **Agentic flow** (`WorkshopRunAgent`): primary path where the model drives a tool-use loop to explore files and apply operations iteratively. See ADR-0010.
- **Compatibility flow** (`WorkshopStreamAssistantReply` + proposal/apply): internal fallback path; still supports Draft-producing edits when writes are requested.

**Commands**
- `WorkshopGetState(workbench_id) -> {model_id, has_draft, conversation_head_id, pending_proposal_id?}` (pending proposal is internal)
- `WorkshopSendUserMessage(workbench_id, text) -> {message_id}`
- `WorkshopRunAgent(workbench_id, message_id) -> {message_id, has_draft}` (agentic flow)
- `WorkshopStreamAssistantReply(workbench_id, message_id, model_id) -> stream(tokens/events)` (compatibility flow)
- `WorkshopProposeChanges(workbench_id, basis=ACTIVE_VIEW) -> {proposal_id, no_changes}` (internal, compatibility flow)
- `WorkshopGetProposal(workbench_id, proposal_id) -> {proposal}` (internal)
- `WorkshopDismissProposal(workbench_id, proposal_id?) -> {}` (internal)
- `WorkshopApplyProposal(workbench_id, proposal_id) -> {draft_id?, draft_revision_id?, no_changes?}` (internal)
- `WorkshopUndoToMessage(workbench_id, message_id) -> {draft_revision_id?}`
- `WorkshopRegenerate(workbench_id) -> {message_id}`
- `WorkshopSwitchModel(workbench_id, model_id) -> {}`

**Events**
- `WorkshopMessageAdded(workbench_id, {message})`
- `WorkshopAssistantStream(workbench_id, {message_id, token_delta})`
- `WorkshopProposalProgress(workbench_id, {proposal_id, phase, percent?})` (internal)
- `WorkshopProposalReady(workbench_id, {proposal_id})` (internal)
- `WorkshopDraftRevisionCreated(workbench_id, {draft_revision_id, message_id})`
- `WorkshopUndoCompleted(workbench_id, {conversation_head_id, draft_revision_id?})`
- `WorkshopRegenerateStarted(workbench_id, {from_message_id})`
- `WorkshopRegenerateCompleted(workbench_id, {message_id})`
- Non-blocking style guidance notices (for example bundled-style-skill load fallback or merge fallback) are emitted as `system_event` messages via `WorkshopMessageAdded`.

## Data & Storage

### Conversation Log (v1)
Persist as an append-only JSONL log:
- `meta/conversation.jsonl` (already referenced in `docs/design/design.md`)

Record types:
- `user_message`
- `assistant_message` (including streaming final text)
- `system_event` (model switch, draft changes created/applied, undo, regenerate, publish checkpoint created, checkpoint restored)
- `summary_message` (context compression output)

Conversation state:
- `meta/workshop_state.json`:
  - `conversation_head_id` (the active “tip” after undo operations)
  - `active_model_id`
  - `active_draft_revision_id?` (null if no Draft)

### Draft Changes (Internal Proposal Artifacts)
Store draft changesets (proposal artifacts) so apply is deterministic and review summaries exist without extra model calls:
```
meta/workshop/proposals/<proposal_id>.json
meta/workshop/proposals/<proposal_id>/
  edits/...
  summaries/...
```

Draft changeset metadata (illustrative):
- `proposal_id`
- `created_at`
- `changes[]` (A/M/D intent, paths, type)
- `edits[]` (validated engine instructions to apply)
- `summaries[]` (per-file summary markdown; expected for Docx/non-text; best-effort)

### Draft Metadata (Draft Source)
Draft metadata is owned by the Draft subsystem (`docs/design/capabilities/draft-publish.md`). Workshop depends on:
- `source`: `{kind: workshop|system}`

### Draft Revisions (for Time Travel)
Workshop maintains an internal Draft revision store to restore Draft file state during rewinds:
```
meta/workshop/draft_revisions/<rev_id>/
  draft_snapshot/...        # snapshot of draft/ at that point
  review_snapshot/...       # optional: summaries/deletions state as-of that point
  rev.json
```

`rev.json` (illustrative):
- `rev_id`, `created_at`
- `base_rev_id?` (for tracing; still linear in v1)
- `conversation_head_id` (message id at which this revision became active)
- `file_fingerprint_hint` (optional; cache invalidation)

Snapshot mechanism should reuse ADR-0001's hardlink-or-copy + atomic replace discipline:
- `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md`

#### Draft Revisions vs Checkpoints
Draft Revisions are distinct from Checkpoints (see `docs/design/design.md` Terminology Clarification):
- **Draft Revisions** are Workshop-internal, snapshot Draft state, and enable message-level undo. They are never exposed in the Checkpoints UI.
- **Checkpoints** are system-wide, snapshot Published state, and enable restore to prior approved states.

**On Publish**: When the user publishes a Draft, Draft Revisions for that Draft are pruned (they no longer serve a purpose since the Draft is gone and the conversation continues forward). The system creates a Checkpoint of the pre-publish Published state, not of the Draft Revisions.

**On Discard**: When the user discards a Draft, Draft Revisions for that Draft are pruned. No Checkpoint is created (the user chose to abandon the Draft).

## Algorithms / Logic

### Context Building (Workshop Turn)
Inputs:
- Active model id
- Conversation up to `conversation_head_id` (plus summaries)
- Workbench files (Published only; Workshop input is blocked while a Draft exists)
- Active context items (always-inject representation: situation injection + skill contents, with Document Style possibly represented via format-gated style skills)

Process (high-level):
1. Load active context items and include in system message (situation context + skill sections).
1b. Run format-gating: scan the Workbench manifest and user intent for `.xlsx`/`.docx`/`.pptx` formats. For each relevant format, load the bundled generic style skill. If a Document Style context item is present, attempt merge and inject one merged skill for that format; on merge failure, inject the generic style skill. Standalone `document-style` is injected only when no format-gated style skill is relevant for the call. See `docs/design/capabilities/document-styling.md`.
2. Select relevant files/sections based on user mentions and context.
3. Apply context compression when clutter is high:
   - summarize older transcript segments into `summary_message` entries
   - append a non-blocking `system_event` indicating compression occurred (e.g., `context_compressed`)
   - Note: context items are never compressed — they are bounded and always included in full.
4. Send prompt to provider, stream output back.

### Regenerate (Assistant Response)
Regenerate creates a fresh assistant response from the current conversation head without mutating files.

Behavior:
- If the head is a **user message**: generate a new assistant response to that message.
- If the head is an **assistant message**: treat regenerate as “replace this response”:
  1. Rewind the head to the preceding user message (discarding the current assistant message).
  2. Generate a new assistant response and advance the head to the new assistant message.

The engine records a `system_event` with enough linkage for auditability (e.g., `replaced_message_id`, `new_message_id`).

### Draft Changes Generation (Internal Proposal)
In the compatibility flow, draft changes generation is a separate model call that produces structured output and is invoked after assistant replies when Draft writes are needed:
1. Build draft-generation context from:
   - Current active view files (Draft or Published)
   - The recent conversation goal/constraints
2. Prompt the model to output a strict schema:
   - `no_changes: true` when no file edits are needed
   - otherwise file operations (create/update; **no delete**)
   - for each changed file: a human-readable summary (expected for Docx/non-text; best-effort)
   - optionally: safety warnings (deletes, large changes)
3. Validate:
   - All paths are relative and within Workbench.
   - No forbidden operations (symlink creation, absolute paths, path traversal).
   - Edits are well-formed and size-bounded.
4. Persist the changeset to disk so apply is deterministic.

### Applying Draft Changes
1. Ensure Draft exists (create from Published if needed).
2. Apply edit operations atomically (temp write + fsync + rename).
3. Copy/store changeset summaries into Review metadata (see Review/Diff design):
   - `docs/design/capabilities/review-diff.md`
4. Create a new Draft revision snapshot and associate it with the new `conversation_head_id`.
5. Emit `WorkshopDraftRevisionCreated`.

### Undo (Rewind)
Given a target `message_id`:
1. Compute the intended `conversation_head_id = message_id`.
2. Determine `target_draft_revision_id` (the latest revision at-or-before that head).
3. Restore:
   - Update `meta/workshop_state.json` head pointer.
   - Restore Draft directory to `target_draft_revision_id` via directory swap (or discard Draft if none).
   - Invalidate any cached review change set/previews (since Draft changed).
4. Append a `system_event` “undo” record to `conversation.jsonl` (audit trail).

Retention:
- Keep the last N draft revisions (default N=200). If pruning removes a revision that a user tries to undo to, show a clear error and offer:
  - “Rewind chat only” (no file rewind), or
  - “Discard Draft and rewind” (safe but loses intermediate Draft history)

### RPI Workflow (Research → Plan → Implement)

`WorkshopRunAgent` runs as an RPI orchestrator with fresh context per phase.

#### Phase Sequence

1. **Research**:
   - Uses a read-only tool subset.
   - Produces `meta/workshop/_rpi/research.md`.
   - Turn budget: `30`.
2. **Plan**:
   - Uses minimal tools (`read_file`, `recall_tool_result`).
   - Produces `meta/workshop/_rpi/plan.md` with checklist items (`- [ ] N. ...`).
   - Includes `<!-- original_count: N -->` metadata for inflation guard.
   - Turn budget: `10`.
3. **Implement**:
   - Engine-driven outer loop over plan checklist items.
   - Full workshop tool set per item.
   - Marks item status in `plan.md` as:
     - `- [x]` for done
     - `- [!]` for failed (`[Failed: <reason>]` appended)
   - Retries each failed item once, then continues.
   - Supports plan-item append-only amendments from model final text, capped at `2x` original item count.
   - Turn budget per item: `30`.
4. **Summary**:
   - Non-tool model call that reads completed plan + current file manifest.
   - This is the only phase whose assistant text streams into chat and is persisted as the user-visible assistant response.

R/P/I phase outputs are internal artifacts and are not persisted as assistant chat turns.

#### Tool and Context Rules

- **Research tools**: `list_files`, `get_file_info`, `get_file_map`, `read_file`, `table_get_map`, `table_describe`, `table_stats`, `table_read_rows`, `table_query`, `recall_tool_result`, `xlsx_get_styles`, `docx_get_styles`, `pptx_get_styles`.
- **Plan tools**: `read_file`, `recall_tool_result`.
- **Implement tools**: full `WorkshopTools`.
- Workbench context items are injected in Research, Plan, and Implement phases.

#### Resumability and Invalidation

- `_rpi` artifacts define current progress.
- Invocation resumes from the first missing/incomplete phase.
- `_rpi` is cleared when prior output is invalidated:
  - new user message (`WorkshopSendUserMessage`)
  - regenerate (`WorkshopRegenerate`)
  - undo/rewind (`WorkshopUndoToMessage`)

#### Notifications

- `WorkshopPhaseStarted` / `WorkshopPhaseCompleted` with `phase`:
  - `research`, `plan`, `implement`, `summary`
- `WorkshopImplementProgress` with:
  - `current_item`, `total_items`, `item_label`
- Tool-level notifications (`WorkshopToolExecuting` / `WorkshopToolComplete`) remain active across all phases.

## Error Handling & Recovery
- Streaming interrupted: preserve partial assistant message; allow "regenerate" (new assistant message) from the same head.
- Draft changes generation fails: assistant responds with a clear limitation + suggested alternative; no file state changes.
- Apply fails mid-way: rollback partial writes where possible; Draft remains consistent; assistant responds with a clear limitation + suggested alternative.
- Undo restore fails: fall back to restoring the latest available draft revision; surface honest error.
- Provider/model unavailable: retry with backoff (per PRD); offer model switch.
- Agent loop stuck: loop detection injects a warning after `loopWarningThreshold` repetitions; terminates after `loopHardStopThreshold` with a user-facing error. Conversation is preserved up to the last successful tool execution.

## Security & Privacy
- Workshop reads/writes only inside the Workbench sandbox.
- Draft changes validation rejects any path traversal or symlink-based escapes.
- No network egress beyond configured model providers; no URL fetching/external retrieval in v1.
- Undo/rewind never triggers model calls; it restores Draft from local snapshots only.

## Telemetry (If Any)
Local-only by default:
- Count of Workshop turns, draft changes generated, draft changes applied.
- Undo usage rate and common rewind distances.
- Model switch frequency.
- Draft generation/apply failure reasons (validation, disk, provider).

## Open Questions
- What is the right default retention for Draft revision snapshots given large binary files can balloon disk usage?

## Self-Review (Design Sanity)
- Satisfies the PRD “revert + regenerate” requirement by tying conversation points to restorable Draft revision snapshots.
- Aligns with the auto-draft decision, keeping normal chat messages non-mutating and making file edits auditable.
- Keeps review deterministic by ensuring summaries are produced during draft generation/execution and stored for the Review screen.
- Keeps iteration simple by generating draft changes against the active view (no merge/rebase in v1).
