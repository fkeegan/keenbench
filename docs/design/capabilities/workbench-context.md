# Design: Workbench Context

## Status
Draft (v1)

## Version
v0.6

## Last Updated
2026-02-16

## PRD References
- `docs/prd/capabilities/workbench-context.md`
- `docs/prd/keenbench-prd.md` (FR1.5)
- Related:
  - `docs/prd/capabilities/workbench.md`
  - `docs/prd/capabilities/workshop.md`
  - `docs/prd/capabilities/clutter-bar.md`
  - `docs/design/capabilities/workshop.md` (prompt assembly, context building)
  - `docs/design/capabilities/workbench.md` (storage layout, Draft blocking)
  - `docs/design/capabilities/document-styling.md` (format style skills, merge logic)

## Summary
Workbench Context lets users attach persistent, structured context to a Workbench via four fixed categories. The engine processes raw user input (text or file + optional note) into reusable artifacts — three [Agent Skills](https://agentskills.io/specification) and one direct system-prompt injection — that are always represented in every Workshop model call.

Key v1 choices:
- **Always-inject representation**: all active context items are represented in every Workshop model call. For Document Style, this representation may be standalone `document-style` skill injection or format-gated style-skill injection (merged when possible, generic fallback on merge failure) per `docs/design/capabilities/document-styling.md`. No skill discovery/activation machinery.
  - In the RPI Workshop flow, this applies to all three internal model phases: Research, Plan, and Implement.
- **Single slot per category**: at most four context items per Workbench (company-wide, department, situation, document style).
- **Process once**: raw input is processed into a structured artifact by a synchronous model call at creation/edit time. The processed artifact is what the engine uses at runtime; the source is kept only for reference.
- **Agent Skills format**: company-wide, department, and document-style items produce standard Agent Skills (`SKILL.md` + `references/`). Situation produces a plain markdown injection.
- **Blocked during Draft**: consistent with file operations.

## Goals / Non-Goals

### Goals
- Eliminate repetitive prompting by persisting organizational, situational, and stylistic context at the Workbench level.
- Produce well-structured artifacts (Agent Skills) that the engine can inject predictably and models can follow reliably.
- Keep the processing pipeline simple: one synchronous model call per context item, no multi-step orchestration.
- Let technical users inspect and hand-edit the generated artifacts.

### Non-Goals
- User-level or cross-Workbench context sharing (v1 is per-Workbench only; a separate "Clone Workbench" feature addresses repetition).
- Custom/user-defined context categories (four fixed categories in v1).
- Asynchronous or background processing.
- Skill discovery, activation, or deactivation based on task type (always-inject in v1).
- Context item versioning or change history.

## User Experience

### Context Overview Screen
Accessed via an **"Add Context"** button in the Workbench UI (alongside the file list).

Layout: four category cards arranged vertically or in a 2x2 grid.

| Category | State: Empty | State: Active |
|----------|-------------|---------------|
| Company-Wide | "Add" button + brief description of what this is for | Summary line (from skill `description`) + Edit / Delete actions |
| Department | "Add" button + brief description | Summary line + Edit / Delete |
| Situation | "Add" button + brief description | Summary line + Edit / Delete |
| Document Style | "Add" button + brief description | Summary line + Edit / Delete |

### Add / Edit Flow
1. User selects a category (clicks "Add" or "Edit").
2. **Input screen** opens:
   - **Mode selector**: "Write text" or "Upload file"
   - **Text mode**: multi-line text box. No file picker.
   - **File mode**: single-file picker + short text field labeled "Note (optional)" for guiding processing (e.g., "focus on the executive communications section").
   - **Confirm button**: "Process" (or "Reprocess" when editing).
3. **Processing state**: progress indicator with category label (e.g., "Processing company context..."). UI blocks further input until processing completes.
4. **Success**: return to the context overview. Category shows as active.
5. **Failure**: show error message + **Retry** / **Cancel** buttons. No artifact is saved.

### Inspect / Direct Edit
- Clicking the summary line on an active category expands to show the full processed artifact:
  - Skill categories: `SKILL.md` plus any generated `references/*` files.
  - Situation category: `context.md`.
- A **"Direct Edit"** toggle switches the view to editable file-mode:
  - Skill categories: UI shows a file list and editable content panes for `SKILL.md` and generated `references/*` files.
  - Situation category: UI shows editable `context.md`.
- Direct edits do NOT trigger reprocessing — the user's changes are saved as-is. The artifact is flagged as manually edited (`has_direct_edits = true` in `source.json`).
- Direct edit is intentionally permissive in v1: users may save invalid skill content. No Agent Skills hard-validation gate is applied on direct save.
- Items with direct edits show a subtle **"manually edited"** indicator in the context overview (e.g., a small badge or label on the category card).
- A **"Reprocess from source"** action is always available. If the artifact has been directly edited, reprocessing shows a confirmation warning: **"This context item was manually edited. Reprocessing will overwrite your manual changes."** with **Proceed** / **Cancel**.
- Reprocessing resets `has_direct_edits` to `false` and clears `last_direct_edit_at`.

### Delete
- Each active category has a **Delete** action. Deletion is immediate (no confirmation — the item can be re-added at any time).

### Draft Blocking
- All context operations (add, edit, delete) are disabled while a Draft exists.
- Disabled controls show tooltip: "Publish or discard your Draft to modify context."

### Accessibility
- Category cards are keyboard-navigable and focusable.
- Mode selector (text/file) uses radio buttons or equivalent accessible pattern.
- Processing progress is announced to screen readers.
- Active/empty states use text labels, not color alone.

## Architecture

### UI Responsibilities (Flutter)
- Render the context overview screen with four category cards.
- Provide the add/edit input flow (text box, file picker, note field, mode selector).
- Show processing progress and handle success/failure states.
- Render the inspect/direct-edit view for processed artifacts, including multi-file editing for skill categories (`SKILL.md` + generated `references/*`).
- Enforce Draft-blocking state on all context controls.
- Navigate to context overview from the Workbench chrome (button placement TBD — near the file list or as a tab).

### Engine Responsibilities (Go)
- Persist context items in the Workbench metadata directory.
- Orchestrate the processing model call: build a processing prompt from the raw input, call the Workbench default model (or user default), parse the structured output into skill/injection artifacts.
- Validate processed artifacts against Agent Skills hard requirements (for skill categories): `SKILL.md` present, parseable YAML frontmatter, required `name` + `description`, valid `name` constraints, and path-safe references.
- If validation fails, run one repair re-prompt that includes the validator errors; persist only if repaired output passes validation.
- Serve context items to the prompt assembly pipeline for Workshop model calls.
- Support CRUD operations (add, get, update, delete) per category.
- Block context operations when a Draft exists (return structured error per ADR-0006).
- Report context item weights to the Clutter Bar calculation.

### IPC / API Surface
API names are illustrative; protocol is JSON-RPC over stdio (ADR-0003).

**Commands**
- `ContextList(workbench_id) -> {items[]}` — returns all context items with category, status (active/empty), summary, `has_direct_edits` flag, and timestamps.
- `ContextGet(workbench_id, category) -> {item}` — returns the full context item including processed artifact files, source metadata, source text (if text mode), and `has_direct_edits` flag.
- `ContextProcess(workbench_id, category, input) -> {item}` — processes raw input into a context item. Blocked if Draft exists. Resets `has_direct_edits` to `false`. `input` contains:
  - `mode`: `"text"` or `"file"`
  - `text`: raw text content (text mode)
  - `source_path`: path to the file to upload (file mode)
  - `note`: optional guidance note (file mode)
- `ContextGetArtifact(workbench_id, category) -> {files[], has_direct_edits}` — returns the processed artifact as a file map for UI inspection/editing.
  - `files[]`: `{path, content}` where paths are artifact-relative (`SKILL.md`, `references/summary.md`, `references/style-rules.md`, or `context.md`).
- `ContextUpdateDirect(workbench_id, category, files[]) -> {item}` — saves direct edits to the processed artifact file map without reprocessing. Sets `has_direct_edits` to `true` and updates `last_direct_edit_at`. Blocked if Draft exists.
  - No Agent Skills validation gate is applied on this path (intentional raw power mode).
- `ContextDelete(workbench_id, category) -> {}` — deletes the context item for a category. Blocked if Draft exists.

**Events**
- `ContextChanged(workbench_id, {category, action: added|updated|deleted})` — notifies the UI that a context item changed (triggers Clutter Bar recalc and prompt cache invalidation).

## Data & Storage

### On-Disk Layout
Context items live under `meta/context/` in the Workbench directory:

```
workbenches/<workbench_id>/
  meta/
    context/
      company-context/
        SKILL.md                # Generated Agent Skill (frontmatter + instructions)
        references/
          summary.md            # Compressed company facts
        source.json             # Input metadata
        source_file/            # Original uploaded file (if file mode)
      department-context/
        SKILL.md
        references/
          summary.md
        source.json
        source_file/
      situation/
        context.md              # Direct injection markdown (not a skill)
        source.json
        source_file/
      document-style/
        SKILL.md
        references/
          style-rules.md        # Detailed formatting rules
        source.json
        source_file/
```

Category directory names are fixed: `company-context`, `department-context`, `situation`, `document-style`.

### source.json Schema
```json
{
  "mode": "text" | "file",
  "text": "...",                     // present if mode=text
  "original_filename": "...",        // present if mode=file
  "note": "...",                     // present if mode=file and note was provided
  "created_at": "2026-02-12T...",
  "last_processed_at": "2026-02-12T...",
  "last_direct_edit_at": null,       // set when user directly edits the artifact
  "model_id": "openai:gpt-5.2",     // model used for processing
  "has_direct_edits": false          // true if artifact was hand-edited after processing
}
```

### Agent Skill Format (Company-Wide Example)
```yaml
---
name: company-context
description: >
  Consider this company background when analyzing content, writing documents,
  or making recommendations about business strategy, messaging, or positioning.
metadata:
  category: company-context
  generated_by: keenbench
  version: "1"
---

# Company Context

When working with files in this Workbench, keep the following company information
in mind. This context should inform your analysis, recommendations, and any content
you create or modify.

## How to use this context

- **When analyzing content**: consider whether findings align with the company's
  position, audience, and competitive landscape.
- **When writing or editing**: ensure tone and framing are appropriate for the company.
- **When making recommendations**: ground suggestions in the company's reality.

See [company summary](references/summary.md) for the full company context.
```

### Agent Skill Format (Document Style Example)

Note: At runtime, this user-provided Document Style skill is merged with the bundled generic format style skills (xlsx, docx, pptx) at prompt-assembly time. The merge produces one cohesive skill per format that combines tool API knowledge with user-specific rules. See `docs/design/capabilities/document-styling.md` for merge algorithm details.

```yaml
---
name: document-style
description: >
  Apply these formatting and style rules when creating or editing documents,
  spreadsheets, or presentations in this Workbench.
metadata:
  category: document-style
  generated_by: keenbench
  version: "1"
---

# Document Style & Formatting Rules

Apply these rules whenever you create or modify files in this Workbench.
These are strict requirements, not suggestions.

## Formatting Rules

- Use Title Case for all headings (H1 through H4).
- Do not use Oxford commas.
- ...

## Tone & Voice

- ...

See [detailed rules](references/style-rules.md) for the complete style guide.
```

### Situation Injection Format (Example)
```markdown
## Current Situation

You are working in the context of the following situation. Keep this in mind
for all analysis, recommendations, and content you produce.

- **Project**: Q4 Board Deck preparation
- **Deadline**: Friday
- **Audience**: Board of directors + lead investor
- **Tone**: Confident but honest about challenges
- **Key constraint**: Must highlight progress on ARR targets
```

## Algorithms / Logic

### Processing Pipeline

Processing transforms raw user input into a structured artifact. The pipeline is the same for all categories; only the **processing prompt** differs.

```
User Input (text or file+note)
    │
    ▼
┌──────────────────┐
│  Engine reads     │  • text mode: use text directly
│  raw input        │  • file mode: extract text content via pyworker
│                   │    (same extraction path as Workbench file reads)
└──────────────────┘
    │
    ▼
┌──────────────────┐
│  Build processing │  • Category-specific system prompt (see below)
│  prompt           │  • Raw content as user message
│                   │  • Optional note included as guidance
└──────────────────┘
    │
    ▼
┌──────────────────┐
│  Call model       │  • Workbench default model (or user default)
│  (synchronous)    │  • Standard provider path (same as Workshop)
│                   │  • Timeout: 120s (same as single-model AI calls)
└──────────────────┘
    │
    ▼
┌──────────────────┐
│  Parse & validate │  • Extract SKILL.md + references (or context.md)
│  output           │  • Validate skill frontmatter if applicable
│                   │  • Validate structure (non-empty, well-formed)
│                   │  • If invalid, run one repair re-prompt with validator errors
└──────────────────┘
    │
    ▼
┌──────────────────┐
│  Write to disk    │  • Write artifacts to meta/context/<category>/
│                   │  • Write source.json with metadata
│                   │  • Copy source file if file mode
│                   │  • Emit ContextChanged event
└──────────────────┘
```

### Processing Prompts (per category)

Each category uses a different system prompt that instructs the model on what to produce. The prompts share a common structure:

1. **Role**: "You are a context processor for a document workspace tool."
2. **Task**: Category-specific instructions for what to extract and how to structure it.
3. **Output format**: Explicit schema for the artifacts to produce.
4. **Constraints**: Agent Skills hard requirements, tone requirements, what to include/exclude.

Shared system-prompt requirements for skill categories (company-wide, department, document style):
- Produce one skill rooted at the category directory (`company-context`, `department-context`, or `document-style`) with required `SKILL.md`.
- `SKILL.md` must contain YAML frontmatter with required fields:
  - `name`: 1-64 chars, unicode lowercase alphanumeric + hyphen, no leading/trailing hyphen, no consecutive hyphens, and must match the skill directory name.
  - `description`: 1-1024 chars, non-empty, clearly describes what the skill does and when to use it.
- Body should remain concise; move detailed material into `references/` files and link via relative paths.
- Follow Agent Skills progressive-disclosure guidance: target `SKILL.md` under 500 lines and under 5000 tokens (guidance, not hard rejection).

**Company-Wide** processing prompt directives:
- Extract: company name, industry, products/services, target audience, competitive position, culture/values, key terminology.
- Produce: a `summary.md` with compressed facts; a `SKILL.md` with suggestive behavioral instructions ("consider", "keep in mind").
- Tone: suggestive. The model should be informed, not constrained.

**Department** processing prompt directives:
- Extract: department name, function, goals, KPIs, reporting structure, key workflows, tools, terminology.
- Produce: a `summary.md` with compressed facts; a `SKILL.md` with imperative behavioral instructions ("you must consider", "always account for").
- Tone: imperative. The model should actively apply this context.

**Situation** processing prompt directives:
- Extract: current project/task, deadline, audience, tone requirements, constraints, phase, key priorities.
- Produce: a single `context.md` with a concise, structured situational summary.
- Format: bullet points or short sections, not narrative prose. Must be scannable.

**Document Style** processing prompt directives:
- Extract: formatting rules (headings, lists, spacing), tone/voice guidelines, terminology preferences, structural conventions, language preferences.
- Produce: a `style-rules.md` with the detailed rule set; a `SKILL.md` with prescriptive instructions scoped to file creation/modification.
- Format: checklists, dos/don'ts, explicit examples. Rules must be unambiguous and actionable.
- Tone: prescriptive. "Always do X. Never do Y."
- Merge interaction: the processed artifact will be merged with bundled generic format style skills at prompt-assembly time. The processing prompt should produce both format-agnostic rules (tone, terminology, general structure) and format-specific rules (xlsx cell formatting, docx paragraph styles, pptx slide layout) where the user's input warrants them. See `docs/design/capabilities/document-styling.md` for merge algorithm details.

### File Content Extraction (for file-mode input)

When the user uploads a file, the engine must extract its text content before sending it to the processing model. This reuses the existing file-reading infrastructure:

- **Text files** (.txt, .md, .csv, etc.): read directly.
- **Office documents** (.docx, .xlsx, .pptx): use the pyworker's existing read tools to extract text content.
- **PDF** (.pdf): use the pyworker's PDF read path.
- **Images**: not supported as context input (no OCR in v1). Block with error: "Image files cannot be used as context input. Please provide text or a document file."
- **Unsupported formats**: block with error: "This file type is not supported for context input."

Supported input file types for context processing (v1):
| Category | Formats |
|----------|---------|
| Text | .md, .txt, .csv, .json, .xml, .yaml, .html |
| Code | .js, .ts, .py, .java, .go, .rb, .rs, .c, .cpp, .h, .css, .sql |
| Documents | .docx, .odt, .pdf |
| Presentations | .pptx |
| Spreadsheets | .xlsx |

### Prompt Injection at Runtime

During Workshop prompt assembly (see `docs/design/capabilities/workshop.md`, Context Building section), the engine includes active context items by standalone or merged representation:

**Injection order in the system message:**
1. System instructions (existing)
2. File manifest + structural maps (existing)
3. **Situation context** (if present): injected as a delimited section:
   ```
   <workbench-situation>
   [contents of situation/context.md]
   </workbench-situation>
   ```
4. **Agent Skills** (whichever are present): each skill injected as a delimited section. This includes `document-style` only when no format-gated style skill is injected for the call.
   ```
   <workbench-skill name="company-context">
   [full contents of SKILL.md]
   [contents of referenced files, inlined]
   </workbench-skill>
   ```
5. **Format style skills** (when relevant formats present): generic or merged with Document Style. When a Document Style context item is present for a relevant format, the format style skill path (merged when possible, generic fallback on merge failure) is the injected representation for that call, not an extra duplicate skill alongside standalone `document-style`. See `docs/design/capabilities/document-styling.md` for format-gating, merge logic, and injection format.
6. Conversation history (existing)

Referenced files (e.g., `references/summary.md`) are inlined within the skill section so the model has the full content without needing to load resources separately. This is consistent with the always-inject representation strategy.

**Scope**: This injection path covers Workshop model calls only.

### Clutter Weight Calculation

Context items contribute dynamic weights to the Clutter Bar based on actual injected content (including format style skills when injected):

```
context_items_weight = estimate_tokens(
  injected_situation_context +
  all_injected_agent_skill_bodies +
  all_injected_format_style_skill_bodies +
  all_inlined_skill_references
)

context_share = context_items_weight / selected_model_context_window_tokens
```

- No fixed per-category constants are used; weights are computed at prompt-assembly time from the exact injected bytes/tokens.
- Context item count is still bounded (max 4), but per-item size is variable because there is no hard token cap.
- If `context_share >= 0.35`, show a non-blocking Clutter warning: "Context is using a large share of the prompt window. Consider shortening context items."

### Validation Rules

After processing, the engine validates:
1. **Required files** (skill categories): `SKILL.md` exists and is readable.
2. **Frontmatter parse** (skill categories): YAML frontmatter is present and parseable.
3. **`name` hard constraints** (skill categories): 1-64 chars; unicode lowercase alphanumeric + hyphen only; no leading/trailing hyphen; no consecutive hyphens; must match the category skill directory name.
4. **`description` hard constraints** (skill categories): present, non-empty, <=1024 chars, and describes what the skill does/when to use it.
5. **References integrity**: any referenced files exist in output.
6. **No path traversal**: all referenced paths are relative and remain inside the context item directory.
7. **Non-empty body**: artifact body contains meaningful content.

If a hard validation check fails, the engine runs one repair re-prompt with the validator errors. Validation behavior should match Agent Skills `skills-ref validate` semantics for required files/frontmatter/name rules. If repaired output still fails, `ContextProcess` fails and nothing is saved.

Validation scope note:
- These hard rules apply to model-processed output (`ContextProcess`).
- Direct-edit saves (`ContextUpdateDirect`) are intentionally permissive in v1 and are not blocked by Agent Skills validation.

Size guidance (not hard rejection):
- Target `SKILL.md` under 500 lines and under 5000 tokens per Agent Skills guidance.
- If output exceeds those targets, the engine surfaces a warning and the processing prompt should move details into `references/`.

## Error Handling & Recovery

- **Processing model call fails** (timeout, rate limit, provider error): return structured error (per ADR-0006). UI shows error with Retry / Cancel. No artifact is saved. Standard retry policy does NOT apply here (user controls retry explicitly).
- **File extraction fails** (corrupt file, unsupported format): return structured error identifying the issue. UI shows error suggesting a different file or text mode.
- **Processing produces invalid output** (spec violation, bad frontmatter, empty content): run one repair re-prompt with validation errors. If still invalid, return error with Retry / Cancel and save nothing.
- **Context item missing at runtime** (corrupt artifact, deleted outside app): skip the item for the current model call, log a warning, surface a non-blocking notice to the user: "[Category] context could not be loaded. You may need to re-add it."
- **No provider configured**: `ContextProcess` returns a structured error. UI guides user to Settings to configure a provider first.
- **Draft exists**: `ContextProcess`, `ContextUpdateDirect`, and `ContextDelete` return a structured error. UI shows Draft-blocking tooltip.

## Security & Privacy

- Context processing uses the same model provider path as Workshop — subject to the same egress consent model (SR3).
- Uploaded source files are copied into the Workbench `meta/context/<category>/source_file/` directory. They are never read from external paths at runtime.
- Source files follow the same sandbox rules as Workbench file adds (no symlinks, no path traversal, regular files only).
- Processed artifacts are stored locally alongside other Workbench metadata.
- The processed skill/injection content is included in model prompts under the same consent model as other Workbench content — users have already consented to sending Workbench content to the configured provider.
- Source material is never sent to the model at runtime — only the processed artifact is injected.

## Telemetry (If Any)
Local-only by default (v1):
- Context item add/edit/delete counts by category.
- Processing success/failure rates by category and model.
- Direct edit frequency (how often users hand-edit vs. reprocess).
- Average processed artifact size (tokens) by category.

## Open Questions
- Should checkpoint-restore also restore context items to their state at the checkpoint? Currently checkpoints snapshot Published files and conversation/job history — context items are not explicitly included. If context items are Workbench metadata that checkpoints should cover, the checkpoint snapshot scope needs to expand.

## Resolved Questions
- ~~What is the right maximum input size for file-mode context processing?~~ **Resolved**: Context file-mode input uses the same pyworker extraction path and regular Workbench file limits: 25 MB max per file in v1 (Workbench add-file baseline is 10 files max, 25 MB per file). No special chunking or size limits beyond what the regular file pipeline already handles. See `docs/prd/capabilities/workbench.md` (FR v1 #4-#5) and `docs/design/capabilities/workbench.md` (Goals).
- ~~Should there be a hard token cap on processed artifacts?~~ **Resolved**: No. The model produces what the input warrants. The Clutter Bar reflects runtime token estimates of actual injected content and shows a non-blocking warning when context share is high relative to the selected model context window.

## Self-Review (Design Sanity)
- Follows the existing pattern: UI handles presentation, engine handles persistence and model calls, IPC is JSON-RPC over stdio.
- Processing reuses existing infrastructure: provider clients for model calls, pyworker for file extraction.
- Always-inject keeps prompt assembly simple — no conditional skill loading logic.
- Storage layout is self-contained per category, making add/edit/delete operations atomic (replace the directory).
- Runtime Clutter estimation and a high-context-share warning prevent context pressure from being hidden when items become large.
- Forward-only application avoids retroactive conversation invalidation.
