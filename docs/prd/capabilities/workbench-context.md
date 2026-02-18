# PRD: Workbench Context

## Status
Draft

## Purpose
Give users a way to attach persistent, structured context to a Workbench so the AI produces better, more consistent results without repetitive prompting. Context items are processed into [Agent Skills](https://agentskills.io/) (or direct system-prompt injections) that the engine includes in every model call.

## Scope
- In scope (v1): four fixed context categories, add/edit/delete context items, synchronous processing via model call, Agent Skills output format, always-inject strategy, transparent skill inspection and editing, blocked during Draft.
- Out of scope (v1): custom/user-defined categories, user-level (cross-Workbench) context, context sharing between Workbenches, automatic context suggestions, context item versioning/history.
- Related (separate feature): "Clone Workbench" with optional context and file inclusion (to be added to Workbench PRD independently).

## Key Concepts

### Context Item
A piece of persistent information attached to a Workbench that shapes how the AI works. Each context item belongs to one of four fixed categories. A Workbench has **at most one context item per category**.

### Processing
When a user adds or reprocesses a context item, the engine makes a synchronous model call that reads the user's raw input (text or file + optional note) and produces a structured output artifact. Direct edits do not call the model. For skill categories, output must satisfy Agent Skills hard requirements (`SKILL.md` with valid YAML frontmatter and required `name` + `description` fields). The raw source is kept for reference; the processed artifact is what the engine uses at runtime.

### Agent Skills (Open Standard)
Three of the four context categories produce [Agent Skills](https://agentskills.io/specification): a folder with a `SKILL.md` file (YAML frontmatter + Markdown instructions) and optional `references/` content. Skills follow the open standard published at agentskills.io. The fourth category (Situation) produces a direct system-prompt injection rather than a skill.

### Always-Inject Strategy
Because a Workbench has at most three skills and one direct injection, context item count is bounded (max 4). There is no hard token cap on processed artifacts — the model produces what the input warrants. The engine **always includes the full content** of all active context items in every Workshop model call — no discovery/activation dance. Clutter uses runtime token estimates of the actual injected content and warns when context share is high relative to the selected model's window.

## Context Categories

### 1. Company-Wide Information
**What it captures**: Company name, mission, industry, key products/services, culture, values, target audience, competitive landscape — the broad organizational backdrop.

**Output**: Agent Skill (suggestive tone).
- `SKILL.md` body instructs the model to "keep this company context in mind when analyzing content, writing documents, or making recommendations."
- `references/summary.md` contains the compressed company facts.
- Tone is suggestive: "consider", "keep in mind", "be aware of".

**Example input**: User uploads a 5-page company overview PDF and adds the note: "We are a B2B SaaS startup, focus on the product positioning section."

### 2. Department-Specific Information
**What it captures**: Department name, function, goals, KPIs, terminology, reporting structure, key workflows, tools in use — the team-level operational context.

**Output**: Agent Skill (imperative tone).
- `SKILL.md` body instructs the model to "always account for this department context when making decisions about content structure, priorities, and terminology."
- `references/summary.md` contains the compressed department facts.
- Tone is imperative: "you must consider", "always account for", "ensure alignment with".

**Example input**: User types: "Finance department. We report to the CFO. Main KPIs are ARR, burn rate, and runway. We use NetSuite for accounting and produce monthly board reports."

### 3. Situation
**What it captures**: Current project, phase, deadline, constraints, audience for the current work, any temporary conditions — the "where you are right now" context.

**Output**: Direct system-prompt injection (NOT a skill).
- Produces a concise markdown document that the engine injects into the system message on every model call, alongside the file manifest and structural maps.
- Always in context because the situation is always relevant — no activation needed.
- This is analogous to an AGENTS.md file: persistent instructions attached to every prompt.

**Example input**: User types: "Preparing Q4 board deck. Deadline is Friday. Audience is the board of directors + lead investor. Tone must be confident but honest about challenges."

### 4. Document Style & Formatting
**What it captures**: Brand style guide, formatting rules, tone of voice, terminology preferences, structural conventions (heading levels, bullet styles, naming conventions), output language preferences.

**Output**: Agent Skill (prescriptive, file-operation-scoped).
- `SKILL.md` description narrows activation to file creation and modification: "Apply these formatting and style rules when creating or editing documents, spreadsheets, or presentations."
- `SKILL.md` body contains precise, actionable rules — not vague guidance. Checklists, dos/don'ts, examples.
- `references/style-rules.md` contains the detailed rule set if the rules are extensive.

**Example input**: User uploads a brand guidelines PDF and adds: "Focus on the section about executive communications. We always use Title Case for headings and never use Oxford commas."

## User Experience

### Adding a Context Item
1. User clicks **"Add Context"** in the Workbench UI.
2. A modal (or dedicated screen) opens showing the four context categories, each as a card or section.
3. Categories that already have a context item show a filled/active state with a summary. Empty categories show an "Add" affordance.
4. User selects a category.
5. User provides input:
   - **Text input**: a text box for typing/pasting context.
   - **File upload + optional note**: a file picker (single file) plus a short text field for guidance (e.g., "focus on the executive communications section").
   - User chooses one mode: text or file. Not both simultaneously (file mode includes the optional note).
6. User confirms. The engine processes the input synchronously.
7. During processing: show a progress indicator with the category name (e.g., "Processing company context...").
8. On success: return to the context overview. The category now shows as active with a brief summary.
9. On failure: show the error with **Retry** and **Cancel** options. No partial/bad context item is saved.

### Viewing a Context Item
- From the context overview, each active category shows a brief summary (the skill's `description` field or the first ~100 characters of the situation context).
- Users can click to expand and see the full processed output:
  - Skill categories: the full artifact file set (`SKILL.md` plus any `references/*` files such as `references/summary.md`).
  - Situation category: `context.md`.
- This is opt-in — non-technical users see only the summary; curious users can inspect the full artifact.

### Editing a Context Item
Users can edit a context item in two ways:
1. **Reprocess** ("Edit" action on a category): re-provide raw input (text or file+note) and trigger a new processing model call. This replaces the previous artifact entirely. If the artifact was previously hand-edited (directly edited), show a confirmation warning: **"This context item was manually edited. Reprocessing will overwrite your manual changes."** with Proceed / Cancel.
2. **Direct edit** (from the inspect view): edit the processed artifact files directly and save.
   - Skill categories: UI must present editable files for `SKILL.md` and any generated `references/*` files (for example, `references/summary.md` or `references/style-rules.md`).
   - Situation category: UI edits `context.md`.
   - Direct edit is raw power mode: users can save any content they want (including invalid skill content). The app does not block these edits with Agent Skills validation.
   - Direct edits are flagged on the artifact (`has_direct_edits`) so the system can warn on future reprocessing.

Both editing paths produce a new context item that applies **forward only** — past conversation turns are not retroactively affected. The context overview shows a subtle "manually edited" indicator on items that have been directly edited, so users can tell which artifacts diverge from their processed form.

### Deleting a Context Item
- Users can delete a context item from any category. Deletion is immediate (no confirmation needed — the item can be re-added).
- Deletion applies forward only.

### Blocked During Draft
- Adding, editing, and deleting context items is **blocked while a Draft exists**, consistent with file add/remove/extract behavior.
- Tooltip on disabled controls: "Publish or discard your Draft to modify context."

### Accessibility
- Context category cards/sections are keyboard navigable.
- Progress indicators during processing are announced to screen readers.
- The text input and file picker are standard accessible form controls.
- Active/empty category states do not rely on color alone.

## Functional Requirements

### v1
1. Users can add one context item per category (four fixed categories: company-wide, department, situation, document style).
2. Input modes: text box, or single file upload + optional short note. User selects one mode per context item.
3. Processing is synchronous: the engine calls the Workbench default model (or user default if Workbench default is not set) to process the raw input into a structured output artifact.
4. At least one model provider must be configured before adding context items.
5. Company-wide, department, and document-style context items produce [Agent Skills](https://agentskills.io/specification) (folder with `SKILL.md` + optional `references/`).
6. Situation context items produce a direct system-prompt injection (markdown document, not a skill).
7. Processed context items are always injected into every Workshop model call (always-inject strategy; no discovery/activation).
8. The raw source (text or uploaded file + note) is preserved alongside the processed output for reference and re-editing.
9. Users can view the processed output (skill content or situation markdown) by clicking into a context item.
10. Users can edit context items via two modes:
    - **Reprocess**: re-provide input and trigger a new processing model call. If the artifact has been directly edited, show a confirmation warning that manual changes will be overwritten.
    - **Direct edit**: hand-edit the processed artifact files and save without reprocessing.
      - Skill categories expose editable files for `SKILL.md` and generated `references/*`.
      - Situation exposes editable `context.md`.
      - Direct edits are not blocked by Agent Skills validation.
      - The artifact is flagged as manually edited (`has_direct_edits`).
11. Both edit modes produce changes that apply forward only. The context overview shows a "manually edited" indicator on directly-edited items.
12. Users can delete context items. Deletion applies forward only.
13. Adding, editing, and deleting context items is blocked while a Draft exists.
14. Processing failure shows an error with Retry and Cancel options. No partial/bad context item is saved.
15. Context items contribute to the Clutter Bar calculation.

### Skill Compliance Rules (v1)
- The `ContextProcess` system prompt for skill categories must explicitly instruct the model to produce Agent Skills-compliant output:
  - Required `SKILL.md` with YAML frontmatter.
  - `name` constraints: 1-64 chars, unicode lowercase alphanumeric + hyphen, no leading/trailing hyphen, no consecutive hyphens, and equal to the skill directory name.
  - `description` constraints: non-empty, <=1024 chars, and clearly describes what the skill does and when to use it.
- The engine validates those hard constraints after model output (equivalent to `skills-ref validate` semantics for required frontmatter/name rules). If validation fails, the engine runs one repair re-prompt using the validator errors.
- If repaired output still fails hard validation, processing fails and no artifact is saved.
- Agent Skills size guidance from the open spec is treated as soft guidance (target under 500 lines and under 5000 tokens); oversized skills should be compressed by moving detail into `references/` and surfaced with warnings rather than hard rejection.

### Processing Flows (per category)

16. **Company-wide**: model reads raw input, produces a `SKILL.md` with suggestive behavioral instructions and a `references/summary.md` with compressed company facts.
17. **Department**: model reads raw input, produces a `SKILL.md` with imperative behavioral instructions and a `references/summary.md` with compressed department facts.
18. **Situation**: model reads raw input, produces a concise markdown document for direct system-prompt injection.
19. **Document style**: model reads raw input, produces a `SKILL.md` with prescriptive formatting rules (checklists, dos/don'ts, examples) scoped to file creation/modification, and a `references/style-rules.md` if rules are extensive.

## Storage

Context items are stored under the Workbench metadata directory:

```
workbenches/<workbench_id>/
  meta/
    context/
      company-context/
        SKILL.md                # Generated Agent Skill
        references/
          summary.md            # Compressed facts
        source.json             # Original input metadata (mode, note text, timestamps)
        source_file/            # Original uploaded file (if any)
      department-context/
        SKILL.md
        references/
          summary.md
        source.json
        source_file/
      situation/
        context.md              # Direct injection content (not a skill)
        source.json
        source_file/
      document-style/
        SKILL.md
        references/
          style-rules.md        # Detailed rules
        source.json
        source_file/
```

- `source.json` stores: input mode (text/file), raw text (if text mode), note text (if file mode), original filename (if file mode), timestamps (created, last processed).
- `source_file/` stores the original uploaded file (if file mode was used).
- Source artifacts are kept for reference and re-editing but are never sent to the model at runtime — only the processed output is injected.

## Prompt Integration

The engine's prompt assembly (see `docs/design/capabilities/workshop.md`, Prompt Assembly section) includes context items as follows:

1. **Situation context** (if present): injected as a clearly delimited section in the system message, after the file manifest and before the conversation history. Labeled so the model can distinguish it from file context.
2. **Agent Skills** (company-wide, department, document-style — whichever are present): full `SKILL.md` content + referenced files injected as skill sections in the system message. Each skill is clearly delimited with its name and purpose.

All active context items are included in every Workshop model call (always-inject). No skill discovery or activation mechanism is needed.

## Clutter Bar Impact

Context items contribute to the Clutter Bar calculation:
- Each active context item adds a dynamic weight based on runtime token estimates of the actual injected content (`context.md`, `SKILL.md`, and inlined `references/*`).
- The Clutter Bar reflects the total context pressure including files + conversation + context items.
- Context item count is bounded (max 4), but per-item size is variable because there is no hard token cap.
- Show a non-blocking Clutter warning when `context_items_weight / selected_model_context_window_tokens >= 0.35`: "Context is using a large share of the prompt window. Consider shortening context items."

## Failure Modes & Recovery
- **Processing fails (model error, timeout, rate limit)**: show error with Retry and Cancel. No artifact saved. User can retry immediately or cancel and try later.
- **Uploaded file unreadable (corrupt, unsupported format)**: show error identifying the issue. User can try a different file or switch to text input.
- **Provider not configured**: block "Add Context" with guidance to configure a provider in Settings first.
- **Processing produces invalid skill output** (spec violation, bad frontmatter, invalid `name`/`description`): run one repair re-prompt with validation errors. If still invalid, fail with Retry/Cancel and save nothing.
- **Context item references missing at runtime**: if a processed artifact is missing or corrupt, the engine skips that context item for the current model call, logs a warning, and surfaces a non-blocking notice to the user: "Company context could not be loaded. You may need to re-add it."

## Security & Privacy
- Context processing uses the same model provider path as Workshop — subject to the same egress consent model.
- Uploaded files for context processing follow the same sandbox rules as Workbench file adds: copied into the Workbench metadata directory, never read from external paths at runtime.
- Source files and processed artifacts are stored locally alongside other Workbench metadata.
- The processed skill content is included in model prompts under the same consent model as other Workbench content.

## Acceptance Criteria
- Users can add one context item per category from the Workbench UI.
- Users can provide input as text or as a single file + optional note.
- Processing is synchronous; a progress indicator is shown during processing.
- Processing failure shows a clear error with Retry and Cancel.
- Processed context items are visible (on click) and directly editable as files.
- Skill categories show editable `SKILL.md` and generated `references/*` files in the direct-edit UI.
- Directly-edited items are flagged with a "manually edited" indicator in the context overview.
- Reprocessing a directly-edited item shows a confirmation warning before overwriting manual changes.
- Users can re-provide input to reprocess a context item, or delete it entirely.
- Adding/editing/deleting context items is blocked while a Draft exists.
- Context items are injected into every Workshop model call (always-inject).
- Situation context appears in the system message.
- Skill-based context items (company, department, document-style) appear as skill sections in the prompt.
- Generated skills satisfy Agent Skills hard requirements (`SKILL.md`, required `name` + `description`, valid `name` constraints).
- Invalid generated skill output triggers one repair attempt; if still invalid, the operation fails with Retry/Cancel and persists nothing.
- Context items contribute to the Clutter Bar score.
- When context items consume >=35% of the selected model's context window, the Clutter Bar shows a non-blocking warning.
- A Workbench with no context items functions identically to today (no regression).

## Open Questions
- Should the onboarding walkthrough introduce context items, or is it better to let users discover them organically?

## Resolved Questions
- ~~Should context be user-level or Workbench-level?~~ **Resolved**: Workbench-level only. The app is unauthenticated; users may work for multiple organizations. A separate "Clone Workbench" feature (outside this PRD) addresses the repetition concern.
- ~~Should context items use the Agent Skills discovery/activation pattern?~~ **Resolved**: No. Always-inject. Context is always present at runtime, and Clutter uses runtime token estimates of actual injected content (with a high-context-share warning) so prompt pressure remains visible.
- ~~Text-only or file-only input?~~ **Resolved**: File mode includes an optional short note to guide processing. Text mode is text-only.
- ~~Should source material be discarded after processing?~~ **Resolved**: Keep source material for reference and re-editing. It is never sent to the model at runtime.
- ~~What is the right token budget cap for processed context items?~~ **Resolved**: No hard cap. The model produces what the input warrants. Clutter uses runtime token estimates of actual injected content and shows a non-blocking warning when context share is high relative to the selected model's context window.
- ~~What file size limits apply to context file-mode input?~~ **Resolved**: Context file-mode input uses the same limits as regular Workbench file operations: 25 MB max per file in v1 (the Workbench add-file baseline is 10 files max, 25 MB per file). See `docs/prd/capabilities/workbench.md` (FR v1 #4-#5) and `docs/design/capabilities/workbench.md` (Goals).
