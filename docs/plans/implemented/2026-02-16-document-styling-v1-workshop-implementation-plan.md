# Implementation Plan: Document Styling v1 (Workshop-Only)

## Status
Implemented (2026-02-16)

## Summary
Document Styling v1 adds inline office formatting support in Workshop mode, format-gated bundled style skills, and user document-style merge behavior.

## Scope
- Add inline style parameters for Workshop office operations (`xlsx_operations`, `docx_operations`, `pptx_operations`).
- Add new XLSX operations: `set_column_widths`, `set_row_heights`, `freeze_panes`.
- Add bundled format style skills for `xlsx`, `docx`, and `pptx` under `engine/skills/bundled/`.
- Add format-gated style-skill injection and document-style merge in Workshop context assembly.
- Preserve backward compatibility for existing non-style operation calls.

## Out of Scope
- New style UI controls.
- Tier-2 styling features (conditional formatting, charts, transitions, etc.).

## Public Interface and Contract Changes
1. Workshop tool schemas (`engine/internal/engine/workshop_tools.go`)
- `xlsx_operations` supports style objects and new ops:
  - `set_column_widths` (`columns[]: {column, width}`)
  - `set_row_heights` (`rows[]: {row, height}`)
  - `freeze_panes` (`row`, `column`)
- `docx_operations` supports run-level and paragraph-level style fields (`runs`, alignment/spacing/indent fields).
- `pptx_operations` supports `title_runs`, `body_runs`, and paragraph-level style fields.

2. Proposal validation and prompt rules (`engine/internal/engine/engine.go`)
- Proposal prompt allows new XLSX ops.
- Op validator accepts and validates new XLSX operation payloads.
- XLSX focus hints treat new sheet-level ops as touched context.

3. Workbench context injection (`engine/internal/engine/context.go`)
- Detect relevant formats from manifest and conversation intent.
- Inject bundled format style skill per relevant format.
- Merge user `document-style` with generic format skill into `{format}-style-custom`.
- Suppress standalone `document-style` skill when format-gated style skill injection occurs.
- On failure, fall back to generic format skill and emit style notice events.

4. Pyworker office write behavior (`engine/tools/pyworker/worker.py`)
- XLSX: apply inline style fields on `set_cells`/`set_range`; implement new layout/freeze ops.
- DOCX: support run-level and paragraph-level style application in `set_paragraphs` and `append_paragraph`.
- PPTX: support `title_runs`/`body_runs` and paragraph formatting.
- Invalid style values and unknown style keys: warn + skip (non-fatal).

5. Error/notice codes (`engine/internal/errinfo/errinfo.go`)
- Add `STYLE_SKILL_LOAD_FAILED`.
- Add `STYLE_MERGE_FAILED`.

## Test Cases and Scenarios
1. Engine context and injection tests (`engine/internal/engine/context_test.go`)
- Standalone document-style retained when no format signal exists.
- Format-gated injection from manifest and from conversation intent.
- Document-style merge success creates `*-style-custom` and preserves catalog content.
- Merge failure falls back to generic style skill and records style notice.
- Bundled skill load failure falls back to standalone document-style and records style notice.

2. Workshop schema and validator tests (`engine/internal/engine/workshop_tools_test.go`, `engine/internal/engine/engine_prompt_test.go`)
- Office tool schemas include new style-capable fields and new XLSX ops.
- Proposal prompt includes new XLSX ops.
- Proposal op validation accepts valid new op payloads and rejects invalid payloads.
- XLSX focus hint behavior remains valid with sheet-level style/layout ops.

3. Real pyworker integration tests (`engine/internal/toolworker/manager_test.go`)
- XLSX inline style application + new ops + mixed valid/invalid style values.
- DOCX runs + paragraph formatting + mixed valid/invalid style values.
- PPTX title/body runs + paragraph formatting + mixed valid/invalid style values.

## Quality Gates
- `make fmt`
- `cd engine && go test ./... -coverprofile=coverage.out`
- `cd app && flutter test`
- Maintain Go coverage above project floor during rollout.

## Assumptions and Defaults
- Workshop is the only runtime path in scope for style-skill injection and merge.
- Worker remains authoritative for style semantic handling; engine validates envelopes/contracts.
- Style-skill merge is markdown section based (best-effort), with explicit generic fallback.
