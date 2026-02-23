# Improvement Plan: XLSX Merged-Cell Write and Preservation

## Status
Draft (2026-02-23)

## Goal
Add first-class support for creating/preserving merged cells in XLSX write flows, with clear execution policy for CSV -> XLSX workflows.

The intended outcome is predictable layout fidelity for real-world spreadsheets (which commonly use merged headers/sections) without forcing fragile manual workarounds.

Companion retrieval plan: `docs/improvements/2026-02-22-xlsx-mixed-sql-file-read-improvement.md`.

---

## Context and Problem

Current behavior is merged-cell-aware for reads/maps, but write support is incomplete:

- `get_file_map` reports `has_merged_cells` but does not provide merge-write operations.
- `xlsx_operations` has no explicit `merge_cells`/`unmerge_cells` op.
- `table_update_from_export(mode=replace_sheet)` clears sheet content and unmerges existing ranges before writing.

Result: data can be written correctly, but layout semantics from merged ranges are not reliably preserved or recreated.

---

## Key References (Impacted Design)
- `engine/internal/engine/workshop_tools.go`
- `engine/tools/pyworker/worker.py`
- `docs/improvements/2026-02-22-xlsx-mixed-sql-file-read-improvement.md`
- `docs/design/capabilities/file-operations.md`

---

## Scope

### In Scope
1. Add explicit merged-cell write operations for XLSX tooling.
2. Define deterministic policy for when model should do one-pass write vs two-pass write+merge.
3. Ensure CSV -> XLSX workflows can reconstruct merged layouts when requested.
4. Add tests and telemetry for merge operation success/failure.

### Out of Scope
- Inferring business-intent merges from CSV without any user/requested layout signal.
- Full visual layout reconstruction (charts, complex print settings, all template semantics).

---

## What "Done" Means (Acceptance Criteria)
1. Model can explicitly merge/unmerge ranges through `xlsx_operations`.
2. Merged ranges can be reapplied after `table_update_from_export` writes.
3. CSV -> XLSX tasks with requested merged layout complete deterministically (no manual user repair needed).
4. Tasks without merge requirements do not add unnecessary merge operations.
5. Failures are explicit (invalid range, overlap conflict, unsupported merge action), not silent.

---

## Major Design Decisions
1. **Explicit merge operations, no hidden auto-merge**: merging is a deliberate action.
2. **Two-pass default for CSV sources**:
   - Pass 1: write tabular values (`table_update_from_export` or `set_range`).
   - Pass 2: apply structure (`merge_cells`) when layout requires it.
3. **One-pass allowed when safe**: if merge ranges are known up front and independent of runtime row-count ambiguity, model may perform write + merge in a single `xlsx_operations` call.
4. **Conflict behavior is deterministic**: overlapping/invalid merge requests return validation errors instead of partial silent merges.
5. **Do not infer merges from plain CSV by default**: CSV has no merge semantics; merge intent must come from user instructions, template metadata, or explicit mapping rules.

---

## One-Pass vs Two-Pass Execution Policy

### One-Pass (allowed)
Use one `xlsx_operations` call when:
1. Target sheet and merge ranges are fixed before writing.
2. Merge ranges do not depend on unknown row counts.
3. No intermediate inspection is required.

### Two-Pass (default/recommended)
Use two passes when:
1. Data is sourced from CSV export/query and merge intent is separate from raw table shape.
2. Merge ranges depend on actual rows written.
3. Write path may clear prior sheet structure (for example replace-sheet behavior).

This ensures model behavior is predictable for the case you called out: data dump first, then merge formatting pass when needed.

---

## Public API / Interface Changes
1. Extend `xlsx_operations` op enum with:
   - `merge_cells` (`sheet`, `range`)
   - `unmerge_cells` (`sheet`, `range` optional; if omitted, unmerge all sheet ranges)
2. Update tool descriptions/prompt guidance with merge policy (when to choose one-pass vs two-pass).
3. Optionally include merged-range details in map/read metadata where needed for planning (not just boolean presence).

---

## Implementation Plan (By Area)

### Engine (Go)
1. Update `xlsx_operations` schema and validation to include merge ops.
2. Add prompt guidance:
   - CSV writes do not imply merged layout.
   - If merged layout is requested, run merge pass after data write when needed.
3. Add structured receipt hints for merge operations (ranges applied, count, warnings).

### Pyworker (Python)
1. Implement `merge_cells` and `unmerge_cells` branches in `XlsxApplyOps`.
2. Validate A1 ranges and overlap conditions.
3. Ensure behavior is idempotent where feasible (repeat merge on already-merged range should not corrupt workbook).
4. Preserve existing anchor-write safety behavior for merged targets.

### Tool Guidance / Prompting
1. Add explicit instruction that CSV -> XLSX flows may require two passes for merged layout.
2. Add instruction to avoid inventing merges unless user/template requires them.
3. Add examples for:
   - "Write data only"
   - "Write then merge section headers"

### Docs
1. Update file-operations/workshop capability docs with merge write semantics.
2. Cross-link with mixed SQL XLSX improvement so retrieval and write stories are consistent.

---

## Test Plan
1. Unit tests (Python worker):
   - Merge single range.
   - Unmerge specific range and unmerge-all.
   - Overlap/invalid range validation.
2. Unit tests (Go engine/tool schemas):
   - `xlsx_operations` accepts merge ops and rejects malformed payloads.
3. Integration tests:
   - CSV -> XLSX write followed by merge pass creates expected merged header layout.
   - Replace-sheet write then merge reapplication yields expected structure.
4. Manual validation:
   - Real workbook with merged headers/section bands remains usable in Review and downstream operations.

---

## Rollout / Migration Notes
1. Ship merge-write ops behind a feature flag initially.
2. Keep current behavior as fallback if merge ops are unavailable.
3. Monitor merge-op failure rates and improve guidance before default rollout.

---

## Open Questions
1. Should `unmerge_cells` default to full-sheet unmerge when no `range` is provided, or require explicit range always?
2. Should merge-range details be included directly in `get_file_map` response or fetched via a dedicated merge-structure endpoint?
3. Should `table_update_from_export` later support an optional `post_merge_ranges` helper, or remain intentionally data-only?
