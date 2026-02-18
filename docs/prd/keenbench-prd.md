# PRD: KeenBench

## Status
Draft

## Version
v0.4

## Last Updated
2026-02-13

## Technical Scope
This PRD is **stack-agnostic**. Technology stack and architecture details will be defined in a separate technical design document. The product is **cloud-only** for v1 (no local/offline model support).

## Summary
KeenBench is a desktop app that lets users drop files into a **Workbench** and then use AI to produce real deliverables (documents, decks, spreadsheets, code, plans) **safely** via a built-in "draft → review → publish" workflow.

It combines:
- a **bounded workspace** (Workbench = sandboxed file container)
- **hidden versioning** (Git-like checkpoints/branches without "Git" UX)
- **Workshop**: interactive, step-by-step collaboration (chat-based)

The product's core value: **agentic execution with built-in safety, reviewability, and reversibility**.

---

## Problem Statement
AI chat is good at generating text, but it struggles with:
- multi-file, multi-step work
- consistency and follow-through
- safety (unreviewed edits, destructive actions)
- reproducibility (“what changed and why?”)

Users want AI to “do the work,” but they don’t trust it with their real files or don’t want to manually stitch outputs together.

---

## Goals
### Product Goals
1. Make it easy to turn a messy bundle of inputs (files) into polished outputs.
2. Provide **safe delegation**: AI can make changes, but users can review, publish, or discard.
3. Support both interactive iteration and one-shot “do it for me” execution.
4. Be model-agnostic: allow users to run tasks using their preferred model(s).
5. Minimize fear: users should always understand the scope and impact of what the AI did.

### User Goals
- “I can drop stuff in and get a great result out.”
- “I can review changes confidently.”
- “I can undo anything.”
- “I can choose speed vs quality (and cost) deliberately.”

---

## Non-Goals (for v1)
- Full "control my entire computer" agent.
- Reading arbitrary OS files outside the Workbench (no background scanning).
- Automatic ingestion of email/calendar/drive connectors.
- Real-time co-editing inside Office apps (Word/PowerPoint live control).
- A marketplace for third-party skills/workflows (phase 3).
- Local/offline model support (cloud-only for v1).
- Multiple concurrent Workbenches (single-workbench focus for v1).
- Workbench archive export (.zip) functionality.

---

## v1 Constraints & Limits

### Supported File Types
| Category | Formats |
| --- | --- |
| Text | .md, .txt, .csv, .json, .xml, .yaml, .html |
| Code | .js, .ts, .py, .java, .go, .rb, .rs, .c, .cpp, .h, .css, .sql |
| Documents | .docx, .odt, .pdf |
| Presentations | .pptx |
| Spreadsheets | .xlsx |
| Images | .png, .jpg, .jpeg, .gif, .svg, .webp |

Unsupported file types can be added to a Workbench but will be treated as opaque binaries (no content extraction, summary-only review).

CSV is a first-class tabular input in v1. The File Operations capability PRD defines a local queryable tabular toolset (schema/stats/query/export) for CSV workflows, and establishes the path for additional delimited table formats in future revisions.

### File Limits
- **Max files per Workbench**: 10
- **Max file size**: 25 MB per file
- **Archives**: .zip and other archive formats are not auto-extracted; treat as unsupported binaries for v1.

---

## Target Users (ICP)
### Primary ICP: Power Knowledge Workers
- Founders, consultants, operators, PMs, tech leads, writers
- Often switching between AI models
- Need structured deliverables frequently
- Value time savings and consistent quality

### Secondary ICP: Small Teams
- 2–20 person teams generating docs/analysis/plans repeatedly
- Want consistent outputs and process safety

### User Constraints / Reality
- Many users hate configuration.
- They want “safe automation,” not “another settings dashboard.”

---

## Key Concepts & Definitions

### Workbench
A persistent, bounded container for user files and context.
- User explicitly drags/selects files to add.
- The app **copies files into the Workbench**; originals remain untouched.
- Users can add **context items** (company info, department info, situation, document style) that shape how the AI works. See Workbench Context below.
- Users can extract Published Workbench files to a selected local folder (copy-only; no archive format in v1).
- The AI can read/write only inside the Workbench sandbox.
- Jobs and tasks operate on the Workbench files; new files created by jobs stay in the Workbench.
- Workbenches are stored locally in an **app-managed** directory (not user-configurable in v1).

### Published vs Draft
- **Published**: the current "approved" state of the Workbench.
- **Draft**: AI works in an isolated draft state (one Draft at a time).
- Users review Draft changes and decide to publish (merge) or discard.

### Checkpoints (Hidden Versioning)
- The app automatically creates checkpoints before major actions.
- Users can restore to any prior checkpoint.
- Under the hood, this can be implemented using Git or a snapshot store, but UX avoids "Git terminology."
- Restore is **time travel**: restoring a checkpoint rewinds the Workbench’s Published files **and** Workbench history (conversation + job records) back to that point.
- Publish checkpoints are also surfaced inline in Workshop chat with a Restore affordance.
- Restore is blocked while a Draft exists; users must publish or discard first.
- The standalone Checkpoints screen remains the full history/manual checkpoint management surface.

### Workshop (interactive path)
A chat-based, iterative collaboration:
- user asks
- AI responds (may draft changes)
- Review auto-opens when a Draft appears (including Workbench reopen with an existing Draft)
- user reviews draft changes and publishes or discards
- fast feedback, lower cost, high control
- **conversation is saved per Workbench** and restored on reopen
- **message-level undo**: users can revert to a previous point in the conversation (similar to Cursor)
- model can be switched mid-conversation; new model continues from conversation history
- Workshop uses a single active model at a time; switching replaces the active model

### Supported Models (v1)
v1 supports exactly these four models:
- OpenAI: GPT-5.2 (`openai:gpt-5.2`)
- Anthropic: Claude 4.5 Opus (`anthropic:claude-opus-4.5`)
- Google: Gemini 3 Pro (`google:gemini-3-pro`)
- Mistral: Mistral Large (`mistral:mistral-large`)

Users bring their own provider keys (BYOK). A model is available only if its provider is configured and enabled.

**Model Capabilities**: All supported models can analyze Workbench files and drive Draft-producing edits through the same local file-operation path. The AI may choose analysis-only responses or write operations based on user intent. See `docs/prd/capabilities/file-operations.md` for details. For inline style parameters on write operations and built-in format style skills, see `docs/prd/capabilities/document-styling.md`.

---

## Product Principles
1. **Bounded by default**: AI only sees what is inside the Workbench.
2. **Draft-first**: AI never edits Published directly.
3. **Reversible**: every job is undoable.
4. **Trust through visibility**: show what changed and why.
5. **Opinionated workflow > configuration**: success comes from guided flows, not settings.

---

## Primary Use Cases
1. **Turn messy inputs into a deliverable**
   - Notes + docs → structured report + summary + action plan
2. **Multi-file transformation**
   - Many documents → consistent formatting + consolidated outputs
3. **Complex change across many files**
   - “Update all references of X, regenerate index, update doc”
4. **Draft a deck from source material**
   - Inputs → outline → draft slides (file generated) + speaker notes
5. **Analysis tasks**
   - PDFs/spreadsheets → extracted insights + new spreadsheet/report
6. **Functional analysis and test plan creation**
   - Requirements (any doc type) → test plan with test cases

---

## User Experience Overview

### Create a Workbench
- New Workbench → choose name (location is app-managed in v1)
- Add files (drag/drop or select)
- Display "Workbench contains copies of your files; originals are untouched."

### First-Run Walkthrough (v1)
- Offer a sample Workbench with example files.
- Explain scope boundaries, Draft vs Published, and how review works.
- End with a quick "run a small job" prompt to build confidence.

### Workshop Loop
- user prompt → AI response
- per turn, the AI may do analysis only (no Draft changes) or perform edits that are auto-applied to Draft
- app auto-opens Review when Draft appears
- user reviews small diffs/summaries
- user publishes or discards
- publish checkpoints appear inline in Workshop chat with Restore controls (disabled while Draft exists)
- user can switch models at any time; new model picks up conversation history

---

## Functional Requirements

### FR1 — Workbench & File Management
- Users can create and rename Workbenches.
- Users can add files only through explicit UI actions (drag-drop or file picker).
- Added files are copied into the Workbench; originals are never modified.
- Users can extract Published Workbench files to a selected local folder.
- The system never reads arbitrary files outside the Workbench.
- Workbench has a flat file structure (no subfolders).
- Users cannot rename files within the Workbench.
- File add/remove/extract operations are blocked while a Draft exists.

**Acceptance Criteria**
- App never accesses outside paths except user-selected files.
- UI clearly indicates what's included in Workbench scope.

---

### FR1.5 — Workbench Context
- Users can add **context items** to a Workbench across four fixed categories: company-wide information, department-specific information, situation, and document style & formatting.
- One context item per category (single slot, not a list).
- Input: text box, or single file upload with an optional short note.
- Adding a context item triggers a synchronous model call that processes the raw input into a structured output artifact.
- Company-wide, department, and document-style items produce [Agent Skills](https://agentskills.io/specification) (`SKILL.md` + references). Situation produces a direct system-prompt injection.
- All active context items are injected into every Workshop model call (always-inject strategy).
- Users can view, edit, and delete context items. Two edit modes: **reprocess** (re-provide input, triggers a new model call) and **direct edit** (hand-edit the processed artifact without reprocessing). Direct edits are flagged; reprocessing a directly-edited item shows a warning that manual changes will be overwritten. All changes apply forward only.
- Context item operations are blocked while a Draft exists.
- Context items contribute to the Clutter Bar calculation.

**Acceptance Criteria**
- Users can add, view, edit, and delete context items per category.
- Processing failure shows a clear error with Retry/Cancel; no bad artifact is saved.
- Context is injected into Workshop model calls and improves output relevance.
- A Workbench with no context items functions identically to today.

See `docs/prd/capabilities/workbench-context.md` for full details.

---

### FR2 — Draft System (Safe Edits)
- AI changes occur only in Draft.
- Only one Draft at a time (no concurrent Drafts in v1).
- Draft is isolated from Published.
- Users can discard Draft without affecting Published.
- Users can publish Draft changes to Published.

**Acceptance Criteria**
- "Publish" applies Draft changes atomically.
- "Discard" removes Draft and restores state.
- Starting a new job while a Draft exists prompts: Publish, Discard, or Cancel.

---

### FR3 — Change Review UX
- Review screen lists changed files:
  - Added / Modified / Deleted
  - Size deltas (non-text/binary files only)
- Text-based files: show diff view.
- Office text documents (e.g., .docx, .odt): show an inline diff (best-effort; like code review).
- Spreadsheets (e.g., .xlsx): show an in-app side-by-side preview that zooms to changed areas when possible.
- PDFs/images/pptx/other non-text: show an in-app side-by-side before/after preview.
- Office-text/non-text files: show summary text using fallback order: per-file summary, then Draft-level assistant summary, then `Summary unavailable.`.
- Explicit confirmation required for deletions.

**Acceptance Criteria**
- Users can confidently answer: “What changed?”
- Users can preview both versions in-app for non-text files.
- Size deltas are shown only for non-text/binary files.

---

### FR4 — Checkpoints & Restore
- Auto-checkpoint before each job execution.
- Manual checkpoint option (optional in v1, recommended).
- Restore UI to revert to a previous checkpoint.
- Publish checkpoints are surfaced inline in Workshop chat with Restore affordances.
- Restore is blocked while a Draft exists.
- The standalone Checkpoints screen remains responsible for full history and manual checkpoint actions.

**Acceptance Criteria**
- Restore works reliably and is understandable to non-technical users.
- Restoring a checkpoint “goes back in time”: Published files and Workbench history (conversation + job records) are rewound to the checkpoint.
- Publish checkpoints are visible inline in Workshop chat and can trigger restore when no Draft exists.

---

### FR5 — Workshop Mode (Interactive)
- A chat-like interface scoped to Workbench.
- Buttons/actions:
  - "Send" (Enter to send; Shift+Enter for newline)
  - "Publish" / "Discard" (when a Draft exists; composer hidden)
- Workbench chrome actions:
  - Checkpoints as a history icon in the top bar
  - Settings as a gear icon in the bottom-left of the file panel
- Per turn, the AI decides whether to produce Draft writes:
  - analysis turns: no Draft changes,
  - edit turns: apply Draft changes directly.
- Review opens automatically when a Draft is created and when reopening with an existing Draft.
- Users review Draft changes; there is no separate "Propose changes" or approval step.
- Publish checkpoints are surfaced inline in Workshop chat with Restore controls; restore is blocked while a Draft exists.
- Supports streaming responses.
- **Conversation is saved per Workbench** and restored on reopen.
- **Message-level undo**: users can revert the conversation to a previous message and regenerate from that point (linear history in v1; no branching).
- **Model switching**: user can switch models at any time; new model receives conversation history and continues from there (no confirmation dialog).

**Acceptance Criteria**
- Users can work incrementally with tight feedback.
- Draft changes appear when the AI performs write operations; users publish or discard.
- Review opens automatically when Draft appears and on Workbench reopen with an existing Draft.
- Publish checkpoints are visible in the Workshop chat timeline and provide a Restore affordance.
- Restore from Workshop chat is blocked while a Draft exists.
- Conversation is restored when reopening the Workbench.
- Users can undo/revert to earlier points in the conversation.
- Model switch is seamless; new model picks up conversation context.

---

### FR6 — Multi-Model Support
- Users can configure multiple providers simultaneously (per-provider API key configuration).
- **User default model**: set in user settings, applies to new Workbenches.
- **Workbench default model**: inherited from user default, can be changed per Workbench and persists.
- In **Workshop**: user can switch models at any time; new model picks up conversation history and continues (no confirmation).
- Switching models in Workshop updates the Workbench default model.

**Acceptance Criteria**
- Switching models in Workshop is seamless; conversation context is preserved.
- The UI shows the current model at all times.
- Model switch persists as the Workbench default model.
- Multiple providers can be configured and used within the same Workbench.
- File editing remains available regardless of selected provider; all writes go through Draft for review.

See also: `docs/prd/capabilities/file-operations.md` for file operation capabilities by provider.

---

## Safety, Privacy, and Security Requirements

### SR1 — Scope & Permissions
- AI read/write limited to Workbench directory.
- No implicit OS scanning.
- Import is explicit and copy-based.

### SR2 — Network Egress Controls (v1 minimum)
- Default: model calls only to configured providers.
- v1: no URL fetching/external retrieval feature.
- If external retrieval is introduced (future):
  - require explicit user action
  - show what will be fetched and included

### SR3 — Upload / Exfiltration Guardrails
- If any feature involves uploading files to third-party services/models:
  - show which files are being sent
  - require explicit confirmation (v1: one-time per Workbench/job; re-confirm on scope/provider change)
  - provide “send excerpt/summary only” option (future)

### SR4 — Destructive Operations
- No silent deletes.
- Deletes require explicit confirmation.
- Workbench/file deletes are blocked while a Draft exists.
- Auto-checkpoint before destructive steps.

### SR5 — Audit Trail
- For each job:
  - model used
  - steps performed
  - files changed
  - checkpoints created

### SR6 — Data Handling
- Local-first storage for Workbenches by default.
- If cloud sync is introduced (future):
  - explicit opt-in
  - encryption at rest and in transit
  - clear retention policy

---

## Non-Functional Requirements
- Cross-platform: macOS, Windows, Linux (target; may phase rollout).
- Performance:
  - handle Workbenches with up to 10 files (v1 limit)
  - responsive UI during long jobs
- Reliability:
  - job execution survives transient failures
  - safe cancellation
- Transparency:
  - users can always see the current mode, scope, and model

### Clutter Bar (Context Health Indicator)
The Clutter Bar is a visual indicator showing how "full" the Workbench is relative to model context limits. It abstracts away technical context management from users.

**What contributes to clutter:**
- **File count**: more files = more clutter
- **File weight**: files are indexed by processing complexity (file type + size); heavier files add more clutter. Code files are treated as text files for weighting purposes.
- **Conversation history**: accumulated Workshop messages add to clutter

**UX behavior:**
- Clutter Bar is visible in the Workbench UI (Workshop, Review/Diff, Checkpoints).
- Shows a simple visual (e.g., progress bar or meter) from "light" to "heavy."
- When clutter is high, display a warning: "Workbench is cluttered — performance may be degraded."
- Does not block actions; informational only.

**Purpose:** Let users understand context pressure intuitively without thinking about tokens or context windows.

### Token & Context Management (Implementation)
- Large files may exceed model context windows; the system must handle this gracefully.
- Strategies (implementation detail, but inform UX):
  - **Chunking**: split large files into segments for processing.
  - **Summarization**: summarize portions that don't fit in context.
  - **Selective inclusion**: let users or the system choose which files/sections are active context.
  - **Context compression**: when context limits are approached, automatically compress older conversation history and less-relevant file content via summarization (similar to Claude Code / Codex approach).
- Context compression is automatic and transparent to users; the Clutter Bar reflects compressed state.
- Token usage should be visible in cost estimates.
- The Clutter Bar provides user-facing visibility into context pressure.

### Accessibility
- Target **WCAG 2.1 Level AA** compliance.
- Keyboard navigation for all primary workflows.
- Screen reader compatibility for review and publish/discard flows.
- Sufficient color contrast and non-color-dependent status indicators.
- Accessibility requirements apply to v1 core flows; full audit in v1.5.

## Failure Modes & Recovery (v1)
- Model unavailable / provider error: **auto-retry 3 times with exponential backoff**. If problem persists, show error and offer option to switch provider.
- Rate limits / timeouts: pause job, auto-retry with backoff, resume from last step or checkpoint.
- **Network interruption**: conversation and history are saved up to the point of failure; user can resume from before the failed call.
- Corrupted or unreadable files: surface which files failed and continue if possible.
- Disk full / write failure: halt Draft writes and prompt user to free space.
- Publish conflict detected (Published changed outside Draft): block publish and prompt user to discard or fork.

---

## Competitive Differentiators (Intended)
1. **Workbench sandbox** (explicitly bounded context)
2. **Draft + Review + Restore** (trust through reversibility)
3. **Hidden versioning** (Git-like safety without Git UX)
4. **Multi-model support** (seamless model switching in Workshop)
5. **Clutter Bar** (intuitive context management without technical jargon)
6. **Workbench Context** (persistent, structured context that eliminates repetitive prompting)

---

## MVP Scope (v1)
### Must Have
- Workbench (add files via drag/select copy + context items)
- Workbench Context (company-wide, department, situation, document style — processed into Agent Skills)
- Workshop mode (interactive chat)
- Draft vs Published (one Draft at a time)
- Change review:
  - file list + diffs for text files
  - summaries + in-app before/after preview for non-text files
- Checkpoints + restore
- Multi-model support (at least 2 providers or BYOK abstraction)
- Model switching in Workshop (seamless, no confirmation)
- Clutter Bar
- First-run walkthrough with a sample Workbench
- Accessibility baseline for v1 core flows (WCAG 2.1 AA targets)

### Should Have
- Document styling: inline style parameters on write operations, built-in format style skills, user style guide integration (see `docs/prd/capabilities/document-styling.md`)
- Enhanced audit reporting with detailed execution metrics

### Deferred to v1.5
- "Try with another model" fork for Workshop responses (requires concurrent Drafts)

### Not in v1 (explicit)
- Workflow templates for common job types
- Cloud sync
- Connectors (Drive/Notion/etc.)
- Marketplace / third-party workflow store
- Full semantic diffs for Office files (can be v1.5)
- Concurrent Drafts

---

## Metrics & Success Criteria
### Activation
- % of users who create a Workbench and run at least 1 job within 10 minutes

### Core Value
- % of jobs that reach "Published" (goal: minimize Draft rejection rate)
- Avg time from job start to publish
- Restore rate (should be low, but not zero—restoring is a trust feature)

### Quality
- User rating after publish (thumbs up/down)

### Safety/Trust
- Incidents: unintended file deletion, user-reported scary behavior
- Opt-out rates for automation after first job

---

## Risks & Mitigations
### Risk: Users don’t trust AI edits
- Mitigation: Draft-by-default + review + restore + explicit scope

### Risk: Binary changes are hard to review
- Mitigation: in-app before/after previews, semantic summaries, checkpoints

### Risk: Costs spiral
- Mitigation: estimate range + max spend cap + stop conditions

### Risk: Prompt injection / data exfiltration
- Mitigation: strict Workbench sandbox, network controls, explicit upload confirmation, audit logs

### Risk: "Too generic"
- Mitigation: requirements gathering session enforces specificity and produces testable acceptance criteria

---

## Open Questions
1. ~~How much of Office-file semantic diff is needed for v1 trust?~~ → **Resolved**: For office text documents (e.g., .docx, .odt) in v1, the app provides a best-effort inline diff suitable for code-style review. Formatting/layout changes may still require summary callouts. Deeper semantic diffs for Office files are deferred to v1.5.
2. ~~Will users prefer one Project per outcome or a long-lived "Main Project"?~~ → **Resolved**: Neither. Workbench is a persistent place for files; jobs/tasks operate on them.
3. ~~How should we handle large files and token limits?~~ → **Resolved**: Clutter Bar + Token & Context Management section.
4. ~~How do we present model capability constraints (context limits, file support) without adding config burden?~~ → **Resolved**: Only models with image support are included. Context window limits are handled by the Clutter Bar (model-aware calculation). When context limits are hit, use context compression (summarization of older context). No additional user-facing configuration needed.

---

## Proposed Roadmap
### v1 (Sellable)
- Workbench + Workshop
- Draft/Publish + checkpoints + review
- Clutter Bar
- BYOK or integrated providers (TBD)

### v1.5 (Quality + Trust Upgrade)
- Enhanced audit reporting with detailed execution metrics
- "Try with another model" fork (Workshop only)
- Concurrent Drafts support
- Improved semantic diffs for DOCX/PPTX/XLSX (partial)
- Better estimate accuracy from telemetry
- Full accessibility audit

### v2 (Power Features)
- Optional team features (shared Workbenches, roles, approvals) — TBD
