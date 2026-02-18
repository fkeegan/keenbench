# PRD: Document Styling

## Status
Draft

## Version
v0.1

## Last Updated
2026-02-16

## Purpose
Enable the AI to produce visually polished office documents from scratch by providing inline style parameters on write operations, built-in format style skills, and a layered override model that integrates user-provided style guides. Today the write tools are style-blind — they set content (text, cell values) with no formatting. The existing `*_get_styles` / `*_copy_assets` tools handle derivative fidelity (copying styles from source to target), but creating a well-formatted new document requires style control at write time.

## Scope
- In scope (v1): Tier 1 inline style parameters on write operations (fonts, colors, fills, borders, number formats, alignment, column widths, row heights, freeze panes, paragraph formatting); one generic format style skill per format (xlsx, docx, pptx); merge logic for user Document Style context items with generic skills.
- In scope (future — Tier 2): Conditional formatting, auto-filter, data validation, image insertion from scratch, chart creation, slide transitions and animations; style previews in Review.
- Out of scope: Style-specific UI controls, live formatting preview, styling for read-only formats (pdf, odt, images), styling for plain-text formats (csv, txt, md, json, xml, yaml), custom theme designer.
- Cross-references: [`file-operations.md`](file-operations.md) (section 11 — derivative style preservation), [`workbench-context.md`](workbench-context.md) (category 4 — Document Style & Formatting), [`workshop.md`](workshop.md) (prompt assembly).

## Key Concepts

### Three-Layer Style Model
Styling decisions flow through three layers, each overriding the previous:

1. **Layer 1 — Model World Knowledge**: The model already knows what a CV, bank statement, pitch deck, or budget spreadsheet should look like. This layer is implicit and always available.
2. **Layer 2 — Generic Format Style Skill**: A bundled Agent Skill per format (xlsx, docx, pptx) that teaches the model our tool API for style parameters and provides general best-practice formatting guidance. Injected when the format is relevant to the current task.
3. **Layer 3 — User Custom Style Guide**: The user's Document Style context item (see [`workbench-context.md`](workbench-context.md), category 4). When present, it is merged with the generic skill at processing time, producing one cohesive skill that combines tool API knowledge with user-specific rules.

### Style vs Function Boundary
"How it looks" is style (in scope). "What it does" is function (deferred to Tier 2 or beyond). Freeze panes is an exception — it is functional but so commonly expected alongside styled spreadsheets that it is included in Tier 1.

### Inline Style Parameters
Optional parameters added to existing write operations (e.g., `set_cells`, `set_range`, `set_paragraphs`, `append_paragraph`, `add_slide`). When omitted, write operations behave exactly as they do today — no regression.

### Generic Format Style Skills
Bundled Agent Skills that the engine injects into the model prompt when the task involves creating or editing files of a given format. Each skill contains:
- A tool capability catalog (what style parameters exist, their types, valid values)
- General formatting principles (typography, color, spacing best practices)
- Worked examples (complete tool calls with style params for common document types)

## User Experience

Users do not interact with document styling directly. There is no style picker, color palette, or formatting toolbar. Instead:

1. The user asks the AI to create or edit a document (e.g., "Create a budget spreadsheet from this data" or "Make a professional CV from my notes").
2. The AI decides how to style the document using the three-layer model:
   - It draws on world knowledge for purpose-appropriate design choices.
   - The generic format skill teaches it which tool parameters to use and how.
   - If the user has set a Document Style context item, those rules override the generic defaults.
3. The AI calls write tools with inline style parameters. The tool worker applies the styles.
4. The user reviews the output in Draft and publishes or discards as usual.

### Style Guide Integration (Document Style Context Item)
- Users who want consistent branding or formatting across documents add a Document Style context item to their Workbench (see [`workbench-context.md`](workbench-context.md), category 4).
- When a Document Style context item is present, its rules are merged with the relevant generic format skill at processing time.
- When no Document Style context item is present, the generic format skill is used as-is.

## Functional Requirements

### v1

#### Inline Style Parameters — xlsx

1. **Cell-level styles on `set_cells` / `set_range`**: Each cell or range specification may include an optional `style` object with the following Tier 1 parameters:

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `font_name` | string | Font family name | `"Calibri"` |
| `font_size` | number | Font size in points | `11` |
| `font_bold` | boolean | Bold weight | `true` |
| `font_italic` | boolean | Italic style | `true` |
| `font_color` | string | Font color as hex RGB | `"#333333"` |
| `fill_color` | string | Cell background fill as hex RGB | `"#F2F2F2"` |
| `fill_pattern` | string | Fill pattern type | `"solid"` |
| `number_format` | string | Excel number format code | `"#,##0.00"`, `"0.0%"`, `"yyyy-mm-dd"` |
| `h_align` | string | Horizontal alignment | `"left"`, `"center"`, `"right"` |
| `v_align` | string | Vertical alignment | `"top"`, `"center"`, `"bottom"` |
| `wrap_text` | boolean | Enable text wrapping | `true` |
| `border_top` | object | Top border `{style, color}` | `{"style": "thin", "color": "#000000"}` |
| `border_bottom` | object | Bottom border `{style, color}` | `{"style": "thin", "color": "#000000"}` |
| `border_left` | object | Left border `{style, color}` | `{"style": "thin", "color": "#000000"}` |
| `border_right` | object | Right border `{style, color}` | `{"style": "thin", "color": "#000000"}` |

   Border `style` values: `"thin"`, `"medium"`, `"thick"`, `"dashed"`, `"dotted"`, `"double"`, `"none"`.

2. **New operation `set_column_widths`**: Set column widths for one or more columns.

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `sheet` | string | Target sheet name | `"Sheet1"` |
| `columns` | array | List of `{column, width}` objects | `[{"column": "A", "width": 15.0}]` |

   `width` is in Excel character-width units (approximate character count that fits).

3. **New operation `set_row_heights`**: Set row heights for one or more rows.

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `sheet` | string | Target sheet name | `"Sheet1"` |
| `rows` | array | List of `{row, height}` objects | `[{"row": 1, "height": 20.0}]` |

   `height` is in points.

4. **New operation `freeze_panes`**: Freeze rows and/or columns.

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `sheet` | string | Target sheet name | `"Sheet1"` |
| `row` | integer | First unfrozen row (0-indexed) | `1` (freezes row 0) |
| `column` | integer | First unfrozen column (0-indexed) | `0` (no column freeze) |

#### Inline Style Parameters — docx

5. **Run-level formatting on paragraph write operations**: The `set_paragraphs` / `append_paragraph` operations (and equivalents) accept an optional `runs` array instead of plain `text`. Each run is an object:

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `text` | string | Run text content | `"Hello "` |
| `font_name` | string | Font family | `"Arial"` |
| `font_size` | number | Font size in points | `12` |
| `bold` | boolean | Bold | `true` |
| `italic` | boolean | Italic | `false` |
| `underline` | boolean | Underline | `false` |
| `font_color` | string | Font color as hex RGB | `"#1A1A1A"` |
| `highlight_color` | string | Text highlight color name | `"yellow"` |

   When `runs` is provided, the plain `text` field for that paragraph is ignored. When `runs` is absent, `text` is used as a single unstyled run (backward compatible).

6. **Paragraph-level formatting**: `set_paragraphs` paragraph entries and `append_paragraph` accept optional paragraph-level style parameters:

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `alignment` | string | Paragraph alignment | `"left"`, `"center"`, `"right"`, `"justify"` |
| `space_before` | number | Space before paragraph in points | `6` |
| `space_after` | number | Space after paragraph in points | `12` |
| `line_spacing` | number | Line spacing multiplier | `1.15` |
| `indent_left` | number | Left indent in inches | `0.5` |
| `indent_right` | number | Right indent in inches | `0.0` |
| `indent_first_line` | number | First line indent in inches | `0.25` |

#### Inline Style Parameters — pptx

7. **Run-level formatting on slide content**: The `add_slide` (and equivalent) operation accepts optional `title_runs` and `body_runs` arrays. Each run object has the same fields as docx runs (FR #5 above).

   When `title_runs` / `body_runs` are provided, plain `title` / `body` parameters are ignored. When absent, plain text is used (backward compatible).

8. **Paragraph-level formatting for slides**: Slide text paragraphs accept optional:

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `alignment` | string | Text alignment | `"left"`, `"center"`, `"right"` |
| `space_before` | number | Space before in points | `6` |
| `space_after` | number | Space after in points | `12` |
| `line_spacing` | number | Line spacing multiplier | `1.0` |

#### Backward Compatibility & Error Handling

9. **All style parameters are optional**: Every style parameter on every operation defaults to the format's native default when omitted. Existing tool calls without style parameters produce identical output to today.
10. **Graceful degradation on invalid style values**: If a style parameter value is invalid (e.g., unrecognized border style, malformed color hex), the tool worker logs a warning, skips the invalid parameter, and continues applying remaining valid styles. The operation does not fail.

#### Generic Format Style Skills

11. **One skill per format**: The engine bundles three generic format style skills:
    - `xlsx-style-skill/` — spreadsheet formatting
    - `docx-style-skill/` — document formatting
    - `pptx-style-skill/` — presentation formatting

    Each skill is stored as an Agent Skill (`SKILL.md` + `references/`).

12. **Skill content structure**: Each generic skill contains:
    - **Tool capability catalog**: Complete reference of available style parameters, their types, valid values, and defaults. This section is authoritative — the model must use these parameters, not hallucinate others.
    - **General formatting principles**: Best-practice guidance for the format (e.g., "use consistent font sizes", "limit color palette to 3–4 colors", "leave breathing room with whitespace").
    - **Worked examples**: 2–3 complete tool call examples with style parameters for common document types (e.g., a financial summary spreadsheet, a professional report, a conference presentation).

13. **Format-gated injection**: The engine injects a generic style skill only when the corresponding format is relevant to the current task:
    - xlsx skill: injected when xlsx files are present in the Workbench, or the task is expected to produce xlsx output.
    - docx skill: injected when docx files are present or expected.
    - pptx skill: injected when pptx files are present or expected.
    - Multiple skills may be injected if multiple formats are involved.

#### Style Guide Merge Logic

14. **Merge, not two-pass**: When a user has a Document Style context item (see [`workbench-context.md`](workbench-context.md), category 4) and the task involves a format with a generic style skill, the engine merges them into one cohesive skill at processing time. The model receives one combined injected skill for that format, not two separate competing skills.

15. **Merge rules**:
    - The **tool capability catalog** from the generic skill is always preserved — the user's style guide cannot remove or redefine tool parameters.
    - The **general formatting principles** from the generic skill are replaced by the user's rules where they conflict. Non-conflicting generic principles are retained.
    - The **worked examples** from the generic skill may be replaced or supplemented by user-provided examples.
    - Because Agent Skills are markdown-first and user-editable, merge operates as a best-effort section merge over `SKILL.md` + references (not a rigid typed object contract).
    - The merged skill's `SKILL.md` frontmatter `name` follows the pattern `{format}-style-custom` (e.g., `xlsx-style-custom`).

16. **No merge when no user style**: When no Document Style context item is present, the generic skill is injected as-is.
17. **Standalone Document Style path remains valid**: When a Document Style context item exists but no format style skill is relevant for the call, Document Style continues through the normal Workbench context injection path.

## Acceptance Criteria

### Inline Style Parameters
- An xlsx file created with `set_cells` including `style` parameters contains the specified formatting when opened in Excel or LibreOffice Calc (font, color, fill, borders, number format, alignment verified).
- An xlsx file created with `set_column_widths`, `set_row_heights`, and `freeze_panes` reflects the specified dimensions and freeze state.
- A docx file created with `set_paragraphs` / `append_paragraph` using `runs` and paragraph-level formatting contains per-run font styles and paragraph alignment/spacing.
- A pptx file created with `title_runs` / `body_runs` contains per-run formatting on slide text.
- Write operations called **without** any style parameters produce byte-identical (or functionally identical) output to current behavior — no regression.
- Invalid style parameter values (bad hex color, unknown border style) produce a warning in tool worker logs but do not cause the operation to fail. Valid parameters on the same call are still applied.

### Generic Format Style Skills
- Each bundled skill (`xlsx-style-skill`, `docx-style-skill`, `pptx-style-skill`) passes Agent Skills validation (`SKILL.md` with valid frontmatter, required `name` + `description` fields).
- The tool capability catalog in each skill accurately documents all Tier 1 style parameters for that format.
- Skills are injected into model prompts only when the corresponding format is relevant (format-gated).
- When no files of a format are involved, the corresponding skill is not injected.

### Style Guide Merge
- When a Document Style context item is present, the merged skill preserves the full tool capability catalog from the generic skill.
- User formatting rules from the Document Style context item are reflected in the merged skill's principles section.
- When no Document Style context item is present, the generic skill is injected unmodified.

### Backward Compatibility
- A Workbench with no Document Style context item and no style parameters on tool calls functions identically to today.
- Existing tool calls in past conversations are unaffected.

## Failure Modes & Recovery
- **Invalid style parameter value**: Warn, skip the invalid parameter, apply remaining valid styles. Log the warning for diagnostics.
- **Missing generic style skill at startup**: Engine logs an error. File operations still work — style parameters are applied if present, but the model has no skill guidance. Surface a non-blocking notice: "Style guidance could not be loaded for {format}."
- **Merge failure (malformed user style guide)**: Fall back to the generic skill unmodified. Log the merge error. Surface a non-blocking notice: "Your Document Style could not be merged with the format style guide. Default formatting guidance is being used."
- **Unsupported style parameter in future model calls**: The tool worker ignores unrecognized parameter keys with a warning, ensuring forward compatibility as Tier 2 parameters are added.

## Security & Privacy
- Style parameters are data values passed through the existing tool worker pipeline — no new execution paths, no new egress vectors.
- Generic format style skills are bundled read-only assets. They are not user-modifiable except through the Document Style merge mechanism.
- Merged skills follow the same egress consent model as other Workbench context items.

## Open Questions
- What is the right set of worked examples per format skill? Needs iteration based on real usage patterns after initial implementation.
- Should the merge logic support per-format user style rules (e.g., "use these rules only for xlsx"), or is one Document Style context item for all formats sufficient for v1?
- Should freeze panes support more complex configurations (split panes, multiple freeze regions) in Tier 1, or is single-point freeze sufficient?
