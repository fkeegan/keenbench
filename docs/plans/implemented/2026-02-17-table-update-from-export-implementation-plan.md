# Fix Plan: Add `table_update_from_export` for In-Place XLSX Injection

## Status
Implemented (2026-02-17)

## Problem Summary
`table_export` is designed for stand-alone file creation and recreates XLSX outputs. In multi-step workbook assembly flows this caused sheet loss when writing repeatedly to the same target file.

## Goal
Add a dedicated XLSX in-place update tool that preserves unrelated sheets while keeping `table_export` behavior unchanged for backward compatibility.

## Locked Decisions
1. `table_export` remains unchanged.
2. New write tool: `table_update_from_export`.
3. New tool is Draft-write-only and `.xlsx`-only.
4. Unrelated sheets must be preserved when target workbook already exists.
5. Explicit modes control write behavior: `replace_sheet`, `append_rows`, `write_range`.

## Tool Contract

### Name
`table_update_from_export`

### Parameters
- `path` (required): source CSV path in workbench.
- `query` (optional): read-only SQL query; defaults to full table ordered by `rowid`.
- `target_path` (required): destination XLSX in Draft.
- `sheet` (required): target sheet name.
- `mode` (required): `replace_sheet` | `append_rows` | `write_range`.
- `start_cell` (optional): A1 anchor for `write_range` (default `A1`).
- `include_header` (optional, default `true`).
- `create_workbook_if_missing` (optional, default `true`).
- `create_sheet_if_missing` (optional, default `true`).
- `clear_target_range` (optional, default `false`; valid only for `write_range`).

### Result
- `target_path`
- `sheet`
- `mode`
- `row_count`
- `column_count`
- `written_range`
- `warnings[]`

## Mode Semantics

### `replace_sheet`
- Clears target sheet content and writes the new block at `A1`.
- Preserves other sheets.

### `append_rows`
- Appends below existing used range.
- If `include_header=true` and sheet already has data, header is skipped with warning `header_skipped_on_append; sheet already has data`.
- Preserves other sheets.

### `write_range`
- Writes block at `start_cell`.
- Optional `clear_target_range=true` clears exactly the destination rectangle before writing.
- Preserves other sheets.

## Implemented Changes

### Engine
- `engine/internal/engine/workshop_tools.go`
  - Added `table_update_from_export` schema to `WorkshopTools`.
  - Added dispatch case in `ToolHandler.Execute`.
  - Added `tableUpdateFromExport(argsJSON string)` handler:
    - validates args and mode constraints,
    - enforces `.xlsx` target,
    - ensures Draft exists,
    - calls worker method `TabularUpdateFromExport`,
    - sets focus hint from `sheet` + top-left of `written_range`.
  - Added helper `parseWrittenRangeTopLeftCell(...)`.
  - Updated system prompt/guidance text to steer:
    - `table_export` for stand-alone outputs,
    - `table_update_from_export` for existing workbook/sheet updates.

- `engine/internal/engine/engine.go`
  - Added `table_update_from_export` to loop-detection fingerprinting.
  - Added boolean normalizer helper `toBoolDefault(...)`.
  - Updated CSV context manifest guidance line to include `table_update_from_export`.

- `engine/internal/engine/rpi_state_test.go`
  - Added `table_update_from_export` to the forbidden write-capable tools in research phase checks.

### Pyworker
- `engine/tools/pyworker/worker.py`
  - Added helpers:
    - `_tabular_export_query_and_warnings`
    - `_tabular_query_rows_for_export`
    - `_xlsx_parse_anchor_cell`
    - `_xlsx_written_range`
    - `_xlsx_sheet_has_data`
    - `_xlsx_clear_sheet_contents`
    - `_xlsx_clear_rectangle`
    - `_xlsx_write_tabular_block`
    - `_xlsx_save_workbook_atomic`
  - Added `tabular_update_from_export(params)` with full mode behavior and validation.
  - Registered method:
    - `"TabularUpdateFromExport": (tabular_update_from_export, "write")`.

### Toolworker Fake + Integration Tests
- `engine/internal/toolworker/fake.go`
  - Added fake method branch for `TabularUpdateFromExport`.

- `engine/internal/toolworker/manager_test.go`
  - Added `TestWorkerTabularUpdateFromExportModes` covering:
    1. `replace_sheet` preserving unrelated sheet(s),
    2. `append_rows` placement + header-skip warning,
    3. `write_range` anchor writes + `written_range`.

### Engine Unit Tests
- `engine/internal/engine/workshop_tools_test.go`
  - Tool schema include list now requires `table_update_from_export`.
  - Added validation tests for required args/mode constraints.
  - Added routing test to `TabularUpdateFromExport` and Draft enforcement checks.
  - Added focus-hint test using returned `written_range`.
  - Added loop-fingerprint stable-fields test for the new tool.

## Error Model
- `VALIDATION_FAILED`
  - invalid mode/start cell/non-xlsx target,
  - unsupported `clear_target_range` usage,
  - missing workbook or sheet when create flags are disabled.
- `FILE_READ_FAILED`
  - cache/query/open existing workbook failures.
- `FILE_WRITE_FAILED`
  - workbook save/replace filesystem failures.

## Verification
Executed after implementation:

1. Targeted engine tests
```bash
cd engine && go test ./internal/engine -run 'TestWorkshopToolsIncludesTabularTools|TestToolHandlerTableUpdateFromExportValidation|TestToolHandlerTableUpdateFromExportRoutesToTabularWorker|TestToolHandlerTableUpdateFromExportXlsxCollectsFocusHint|TestToolCallFingerprintTableUpdateFromExportStableFields|TestRPIToolFiltering' -count=1
```

2. Targeted toolworker tests
```bash
cd engine && go test ./internal/toolworker -run 'TestWorkerTabularUpdateFromExportModes|TestWorkerTabularCSVFlow' -count=1
```

3. Full Go suite + coverage gate
```bash
cd engine && go test ./... -coverprofile=coverage.out
cd engine && go tool cover -func=coverage.out | tail -n 1
```

Observed total coverage: `56.7%` (gate satisfied, `>= 50%`).
