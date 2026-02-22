# Improvement Plan: Mixed SQL + File Read for XLSX Retrieval

## Status
Draft (2026-02-22)

## Goal
Enable SQL-first retrieval for `.xlsx` content while keeping existing file-map/style/write tools for workbook editing workflows.

The intended outcome is lower context bloat during Research/Plan and fewer repetitive `read_file`/`recall_tool_result` cycles for large spreadsheets.

---

## Key References (Impacted Design)
- `engine/internal/engine/workshop_tools.go`
- `engine/internal/engine/engine.go`
- `engine/tools/pyworker/worker.py`
- `docs/issues/2026-02-22-rpi-research-recall-tool-result-payload-blowup.md`
- `docs/plans/m2-improvements.md` (format/style reference)

---

## Scope

### In Scope
1. Introduce an `.xlsx` tabular extraction/indexing path so workbook data can be queried with SQL-like tools (`table_query` family semantics).
2. Preserve existing file-structure and office-edit workflows (`get_file_map`, `xlsx_get_styles`, `xlsx_operations`, etc.).
3. Define routing rules so the model prefers SQL tools for data retrieval tasks and `read_file` for targeted textual/section reads.
4. Add explicit observability to compare retrieval quality/cost between SQL and file-read paths.

### Out of Scope
- Replacing existing XLSX write/style/mutation tools.
- Full semantic type inference beyond pragmatic numeric/text/date handling.
- Cross-file SQL joins as a first implementation.

---

## What “Done” Means (Acceptance Criteria)
1. The system can materialize worksheet tabular data from `.xlsx` into a queryable backing store.
2. The model can run bounded, paginated SQL retrieval against `.xlsx` data without line/range chunking prompts.
3. Research/Plan prompts and tool guidance clearly steer analysis tasks toward SQL retrieval for spreadsheet data.
4. Receipt behavior for tabular SQL tools remains compact and informative (rows/columns/has_more preview).
5. Existing editing workflows on `.xlsx` continue to work unchanged.
6. Telemetry shows reduced `read_file`+`recall_tool_result` churn on spreadsheet analysis workloads.

---

## Major Design Decisions
1. **Mixed mode, not replacement**: SQL for retrieval; file tools for structure/style/write.
2. **Workbook-to-tabular projection**: each sheet maps to a logical table with stable column naming.
3. **Windowed query contract**: retrieval remains bounded (`limit`/offset-like semantics) to avoid oversized payloads.
4. **Deterministic refresh**: tabular projection invalidates and rebuilds when source workbook changes.
5. **Graceful fallback**: unsupported/irregular sheets can fall back to `read_file` with clear tool guidance.

---

## Public API / Interface Changes
1. Extend tabular indexing/query tools to support `.xlsx` sources (currently CSV-only).
2. Add metadata endpoints (or response fields) to expose workbook-sheet-to-table mapping.
3. Update tool descriptions/instructions to prioritize SQL retrieval for spreadsheet analysis tasks.
4. Add instrumentation fields for retrieval path selection and fallback reason.

---

## Implementation Plan (By Area)

### Engine (Go)
1. Update tool routing/validation so tabular tools can accept `.xlsx` paths when projection exists.
2. Add clear errors and guidance when projection is unavailable/stale.
3. Tighten receipt generation contracts for large tabular responses and keep recall guidance explicit.
4. Record retrieval-path telemetry (`xlsx_sql`, `xlsx_read_file_fallback`).

### Pyworker / Data Layer (Python)
1. Implement workbook ingestion into tabular backing store (DuckDB) with sheet-level table creation.
2. Normalize headers/types with deterministic rules and null handling.
3. Store projection metadata/signatures for invalidation on workbook changes.
4. Reuse current tabular query execution and paging semantics where possible.

### Prompting / Tool Guidance
1. Update workshop tool instructions: for XLSX analysis use tabular SQL tools first.
2. Keep `read_file` guidance for narrow, coordinate-specific inspection and non-tabular reads.
3. Add fallback policy text so model behavior is predictable when SQL projection cannot be used.

### Docs
1. Add capability docs describing mixed SQL+file behavior for `.xlsx`.
2. Document migration path from current `read_file`-heavy spreadsheet workflows.

---

## Test Plan
1. Unit tests (Go):
   - XLSX path accepted/rejected correctly by tabular tools.
   - Fallback and error messaging behavior.
   - Receipt size/shape assertions for large SQL results.
2. Unit tests (Python):
   - Workbook ingestion across multi-sheet files.
   - Type normalization and null handling.
   - Signature invalidation after workbook modification.
3. Integration tests:
   - End-to-end spreadsheet analysis tasks favor SQL tools over `read_file`.
   - Query pagination and result consistency across repeated runs.
4. Manual validation:
   - Compare token/tool churn and latency on representative XLSX workloads.

---

## Rollout / Migration Notes
1. Ship behind a feature flag for `.xlsx` tabular projection.
2. Keep legacy `read_file` path available during rollout.
3. Monitor telemetry and gradually raise SQL-first preference as confidence increases.

---

## Open Questions
1. How to represent merged cells and multi-row headers in table projection.
2. Whether formulas should be projected as values only, formulas only, or dual columns.
3. Best default column naming when header rows are sparse/blank.
4. Caching strategy limits for very large workbooks.
