# Design: Document Styling

## Status
Draft (v1)

## Version
v0.1

## Last Updated
2026-02-16

## PRD References
- `docs/prd/capabilities/document-styling.md`
- `docs/prd/keenbench-prd.md` (FR1, FR7)
- Related:
  - `docs/design/capabilities/file-operations.md`
  - `docs/design/capabilities/workbench-context.md`
  - `docs/design/capabilities/workshop.md`
  - `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md`

## Summary
Document styling adds the ability to produce visually polished office documents from scratch. It introduces a three-layer style model:

1. **Model world knowledge** (implicit) — the model already knows what common documents should look like.
2. **Bundled generic format style skills** — one Agent Skill per format (xlsx, docx, pptx) teaching the model our tool API for style parameters and providing best-practice formatting guidance.
3. **User custom style guide** — the user's Document Style context item (see `docs/design/capabilities/workbench-context.md`), merged with the generic skill at prompt-assembly time.

This capability adds:
- (a) Inline style parameters on existing write operations (`set_cells`, `set_range`, `set_paragraphs`, `append_paragraph`, `add_slide`, etc.)
- (b) Three new xlsx operations: `set_column_widths`, `set_row_heights`, `freeze_panes`
- (c) Three bundled generic format style skills (xlsx, docx, pptx)
- (d) Format-gated injection of style skills into model prompts
- (e) Merge logic combining user Document Style with generic skills

Key principle: styling is AI-to-engine-to-worker only. No new UI-engine IPC. No new UI affordances. The user's only touchpoint for style customization is the existing Document Style context item (per `docs/design/capabilities/workbench-context.md`).

## Goals / Non-Goals

### Goals
- Styled office output from scratch using inline style parameters on write operations.
- Format-specific guidance via bundled generic style skills.
- User customization via merge of Document Style context items with generic skills.
- Full backward compatibility — existing tool calls without style params produce identical output.

### Non-Goals
- Style-specific UI controls (no color pickers, formatting toolbars, theme designers).
- Styling for read-only formats (pdf, odt, images) or plain-text formats (csv, txt, md, json, xml, yaml).
- Tier 2 features in v1 (conditional formatting, auto-filter, data validation, image insertion, chart creation, slide transitions/animations).
- Style previews in Review (v1 Review uses existing mechanisms).

## User Experience
No direct user interaction with styling. The AI decides styling via the three-layer model. The user's touchpoint is the existing Document Style context item (see `docs/design/capabilities/workbench-context.md`). Review uses the same Review/Diff mechanisms already designed (see `docs/design/capabilities/review-diff.md`).

Refer to `docs/prd/capabilities/document-styling.md` for the full user experience description.

## Architecture

### UI Responsibilities (Flutter)
None specific to document styling. Styled files are reviewed via the existing Review/Diff mechanisms. XLSX grids show styled cells. DOCX/PPTX structured diffs and previews should surface style changes when feasible; best-effort normalized text diffs may omit some styling detail and fall back to unstyled representation. No style-specific UI changes in v1.

### Engine Responsibilities (Go)

#### Bundled Generic Format Style Skills

The engine ships three read-only skill directories bundled with the engine binary:

**Storage**: `engine/skills/bundled/{xlsx,docx,pptx}-style-skill/`

Each skill directory contains:
- `SKILL.md` — Agent Skill with YAML frontmatter + tool capability catalog + formatting principles + worked examples
- `references/` — supplementary reference material (if needed)

**Skill content structure** (per `SKILL.md`):
- **YAML frontmatter**: `name` (e.g., `xlsx-style-skill`), `description`, `metadata` with `category: format-style` and `generated_by: keenbench`.
- **Tool capability catalog**: Complete reference of available style parameters, types, valid values, and defaults. This section is authoritative — the model must not hallucinate parameters not in the catalog.
- **General formatting principles**: Best-practice guidance for the format (typography, color, spacing).
- **Worked examples**: 2-3 complete tool call examples with style parameters for common document types.

**Loading**: Skills are loaded at engine startup and validated against Agent Skills hard requirements (`SKILL.md` present, parseable YAML frontmatter, required `name` + `description`). If a skill is missing or invalid, the engine logs an error but operations continue — the model just lacks guidance for that format.

#### Format-Gating Algorithm

At prompt-assembly time, the engine decides which format style skills to inject:

1. Scan the Workbench file manifest for `.xlsx`, `.docx`, `.pptx` files.
2. Scan the user message and conversation context for intent to create those formats (e.g., "create a spreadsheet", "make a presentation").
3. If a format is present in the manifest OR expected based on intent, inject the corresponding skill.

The algorithm is conservative: inject when in doubt. Each skill adds ~500-800 tokens, a modest cost relative to the benefit of having format guidance available.

```
for each format in [xlsx, docx, pptx]:
    if manifest_contains_format(format) OR intent_targets_format(format):
        inject skill for format
```

#### Merge Logic

**Trigger**: A Document Style context item exists (per `docs/design/capabilities/workbench-context.md`) AND a generic format style skill is being injected for the current prompt.

**When**: Prompt-assembly time. Merged skills are ephemeral — never persisted to disk.

**Agent Skills compatibility note**: Agent Skills are markdown-first with required frontmatter, and body structure is intentionally flexible. Merge logic must therefore operate on markdown sections (`SKILL.md` + referenced files) with best-effort extraction, not on a rigid typed object.

**Algorithm per relevant format**:
1. Start with the generic skill content as the base.
2. Preserve the **tool capability catalog** section verbatim (immutable — user rules cannot remove or redefine tool parameters).
3. Extract user style content from the Document Style artifact (`SKILL.md` + `references/*`) using heading-aware parsing:
   - Prefer explicit sections such as "Formatting Rules", "Tone & Voice", "Worked Examples".
   - If sections are missing/renamed due to direct edits, treat the full artifact as raw style guidance text.
4. Merge **formatting principles**:
   - User rules override conflicting generic principles (v1 conflict detection: coarse keyword matching; false negatives preferred over false positives).
   - Non-conflicting generic principles are retained.
   - Additional user rules are appended.
5. **Worked examples**: Supplement generic examples with user-provided examples when present; for direct conflicts, prioritize user examples.
6. Set the merged skill name to `{format}-style-custom` (e.g., `xlsx-style-custom`).

**When format style guidance is format-gated for the call, standalone Document Style is not injected as an additional skill** — the merged skill (or generic fallback on merge failure) is the injected representation for that format context. If no format style skill is relevant for the call, standalone Document Style remains injected via the normal Workbench context path.

**Fallback**: If the merge fails (e.g., malformed user style guide), inject the generic skill unmodified (no standalone `document-style` duplicate for that call) and surface a non-blocking notice: "Your Document Style could not be merged with the format style guide. Default formatting guidance is being used."

#### Prompt Injection

Format style skills are injected using the same `<workbench-skill>` delimited format as other context items:

```
<workbench-skill name="xlsx-style-skill">
[full contents of SKILL.md]
[contents of referenced files, inlined]
</workbench-skill>
```

**Injection order** in the system message (see `docs/design/capabilities/workbench-context.md` and `docs/design/capabilities/workshop.md`):
1. System instructions (existing)
2. File manifest + structural maps (existing)
3. Situation context (existing, if present)
4. Agent Skills — company-wide, department, and document-style only when no format-gated style skill is injected for this call
5. **Format style skills** (generic or merged, when relevant formats present)
6. Conversation history (existing)

#### Style Parameter Flow

The model emits tool calls with style params. The engine validates operation envelopes and dispatches to the pyworker:

```
Model → tool call with style params
  → Engine validates op envelope (tool/op name, required top-level fields, path/sandbox safety)
    → Pyworker applies via library APIs (openpyxl, python-docx, python-pptx)
```

No new engine-to-pyworker RPCs. Existing `XlsxApplyOps`, `DocxApplyOps`, and `PptxApplyOps` accept extended op schemas with optional style parameters. Three new ops are added within the existing `XlsxApplyOps` method:
- `set_column_widths`
- `set_row_heights`
- `freeze_panes`

### Pyworker Responsibilities (Python)

**xlsx styles** (via openpyxl):
- `Font` objects: `name`, `size`, `bold`, `italic`, `color`
- `PatternFill` objects: `fgColor`, `patternType`
- `Alignment` objects: `horizontal`, `vertical`, `wrap_text`
- `Border` / `Side` objects: `style`, `color` per edge
- `number_format` property on cells
- Column width via `worksheet.column_dimensions[col].width`
- Row height via `worksheet.row_dimensions[row].height`
- Freeze panes via `worksheet.freeze_panes`

**docx styles** (via python-docx):
- Run-level: `Run` objects with `font` properties (`name`, `size`, `bold`, `italic`, `underline`, `color.rgb`, `highlight_color`)
- Paragraph-level: `ParagraphFormat` properties (`alignment`, `space_before`, `space_after`, `line_spacing`, `left_indent`, `right_indent`, `first_line_indent`)

**pptx styles** (via python-pptx):
- Run-level: `Run` objects in text frames with `font` properties (same as docx runs)
- Paragraph-level: `ParagraphFormat` properties (`alignment`, `space_before`, `space_after`, `line_spacing`)

**Validation at pyworker level**:
- Type and value checks on style parameters (authoritative for style semantics).
- Invalid values: warn + skip the invalid parameter, continue applying remaining valid params.
- Unrecognized keys: warn + ignore (forward compatibility for Tier 2 parameters).

### IPC / API Surface
- **No new UI-engine IPC** — styling flows through existing Workshop tool execution.
- **Existing worker RPCs** accept extended op schemas with optional style parameters. No new RPC methods needed (the three new xlsx ops are operations within the existing `XlsxApplyOps` method).
- **Non-blocking notices**:
  - Missing/invalid bundled skill and merge failures are surfaced through existing mode channels (Workshop `system_event` notices) and recorded in engine logs.
  - Diagnostic codes: `STYLE_SKILL_LOAD_FAILED`, `STYLE_MERGE_FAILED`.

## Data & Storage

### Bundled Skills
- **Location**: `engine/skills/bundled/` (read-only, shipped with engine binary)
- **Structure**:
  ```
  engine/skills/bundled/
    xlsx-style-skill/
      SKILL.md
      references/
    docx-style-skill/
      SKILL.md
      references/
    pptx-style-skill/
      SKILL.md
      references/
  ```
- Skills are read-only assets. Not modifiable by users except through the Document Style merge mechanism.

### Per-Workbench Storage
No new per-workbench storage. Document Style context items are already stored at `meta/context/document-style/` per `docs/design/capabilities/workbench-context.md`.

### Merged Skills
Ephemeral — constructed at prompt-assembly time and not persisted to disk.

### Bundled Skill Content Format
Each `SKILL.md` follows the Agent Skills specification:
```yaml
---
name: xlsx-style-skill
description: >
  Authoritative reference for xlsx inline style parameters and best-practice
  spreadsheet formatting. Use the tool capability catalog as the definitive
  source of available style parameters.
metadata:
  category: format-style
  generated_by: keenbench
  version: "1"
---

# XLSX Style Guide

## Tool Capability Catalog
[Complete parameter reference for xlsx style params]

## Formatting Principles
[General best-practice guidance]

## Worked Examples
[2-3 complete tool call examples]
```

## Algorithms / Logic

### Format-Gating Decision Tree
```
For each Workshop model call:
  manifest_formats = set of extensions in Workbench file manifest
  intent_formats = set of extensions detected from user message / conversation
  suppress_standalone_document_style = false

  for format in [xlsx, docx, pptx]:
    skill = load_bundled_skill(format)
    if skill is None:
      log_warning("bundled skill missing for {format}")
      continue

    if format in manifest_formats OR format in intent_formats:
      user_style = load_document_style_context_item()

      if user_style is not None:
        suppress_standalone_document_style = true
        merged = try_merge(skill, user_style, format)
        if merged is not None:
          inject(merged)
        else:
          inject(skill)  # fallback: generic unmodified
          log_warning("merge failed for {format}")
      else:
        inject(skill)  # no user style, use generic

  # Standalone Document Style injection is handled by Workbench Context.
  # If suppress_standalone_document_style is true, do not inject standalone
  # document-style as an additional skill for this call.
```

### Merge Algorithm Pseudocode
```
function try_merge(generic_skill, user_style, format):
  try:
    merged = copy(generic_skill)

    # 1. Tool capability catalog: preserve verbatim
    # (no changes — this section is immutable)

    # 2. Extract user markdown content (Agent Skills are flexible markdown)
    user_sections = extract_sections_from_markdown(
      user_style.SKILL_md,
      user_style.references
    )  # heading-aware, best-effort
    user_rules = extract_formatting_rules(user_sections)
    user_examples = extract_worked_examples(user_sections)

    if user_rules is empty:
      user_rules = [as_raw_guidance_block(user_style.SKILL_md, user_style.references)]

    # 3. Formatting principles: merge with user overrides
    for each user_rule in user_rules:
      matching_generic = find_conflicting_rule(generic_skill.principles, user_rule)
      if matching_generic:
        replace(merged.principles, matching_generic, user_rule)
      else:
        append(merged.principles, user_rule)

    # 4. Worked examples: supplement generic with user examples
    if user_examples is not empty:
      merged.examples = merge_examples(generic_skill.examples, user_examples)

    # 5. Update frontmatter
    merged.name = "{format}-style-custom"

    return merged
  catch error:
    log_warning("merge failed: {error}")
    return None

function find_conflicting_rule(principles, user_rule):
  # v1: coarse keyword matching
  # Extract key terms from user_rule, check for overlap with generic principles
  # False negatives > false positives (prefer keeping both rules over dropping one)
  user_keywords = extract_keywords(user_rule)
  for principle in principles:
    principle_keywords = extract_keywords(principle)
    if overlap_ratio(user_keywords, principle_keywords) > CONFLICT_THRESHOLD:
      return principle
  return None
```

### Style Application Order
Content first, then formatting. The pyworker applies operations in this order:
1. Create/set content (cell values, paragraph text, slide content).
2. Apply styles to the created content (fonts, colors, fills, borders, alignment).
3. Apply structural styling (column widths, row heights, freeze panes).

## Error Handling & Recovery

| Scenario | Behavior | User Impact |
|----------|----------|-------------|
| Invalid style value (bad hex color, unknown border style) | Pyworker warns, skips invalid param, applies remaining valid params | Operation succeeds with partial styling; warning in logs |
| Unrecognized style key | Pyworker warns, ignores key | Operation succeeds; forward compatibility for Tier 2 params |
| Missing/invalid bundled skill at startup | Engine logs error, ops continue without skill guidance | Model lacks formatting guidance but can still use style params if it knows them |
| Merge failure (malformed user style guide) | Engine injects generic skill unmodified, surfaces non-blocking notice | User sees notice; generic formatting guidance used instead of merged |
| Pyworker library error applying style | Pyworker logs error, skips the failing style operation | Operation succeeds with partial styling; error in logs |
| Style param type mismatch (string where number expected) | Pyworker validates, warns, skips | Operation succeeds without that param |
| Fatal operation error (invalid op envelope, sandbox violation, IO failure) | Engine/worker returns structured error (`VALIDATION_FAILED`, `SANDBOX_VIOLATION`, `FILE_WRITE_FAILED`, etc.) | Operation fails; batch semantics follow ADR-0005 |

Validation-only styling issues degrade gracefully (warn + skip). Fatal non-style failures still fail the operation as normal.

## Security & Privacy
- Style parameters are data values in the existing tool worker pipeline. No new execution paths, no new egress vectors.
- Bundled skills are read-only assets shipped with the engine. Not user-modifiable except through the Document Style merge mechanism.
- Merged skills are ephemeral (prompt-assembly only). They follow the same egress consent model as other prompt content — users have already consented to sending Workbench content to the configured provider.
- No new network communication. Style application is entirely local.

## Open Questions
- What is the right set of worked examples per format skill? Needs iteration based on real usage patterns after initial implementation.
- Should the merge logic support per-format user style rules (e.g., "use these rules only for xlsx"), or is one Document Style context item for all formats sufficient for v1?
- How should the format-gating heuristic detect intent to create new-format files from the user message/conversation? Simple keyword matching may suffice for v1.
- Is coarse keyword-based conflict detection acceptable for v1 merge logic, or do we need more sophisticated matching?
