# Design: File Operations — Tabular Text (CSV First)

## Status
Draft (v1 extension of local file operations)

## Version
v0.4

## Last Updated
2026-02-16

## PRD References
- `docs/prd/capabilities/file-operations.md`
- `docs/prd/capabilities/workshop.md`
- `docs/prd/capabilities/workbench.md`
- Related:
  - `docs/design/capabilities/file-operations.md`
  - `docs/design/capabilities/workshop.md`
  - `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md`

## Summary
CSV files are common real-world inputs and are often large enough that line-based reads are insufficient for reliable analysis/transformation. This design introduces a first-class local tabular workflow:
- map table structure,
- inspect schema and stats,
- run deterministic read-only queries,
- export results to CSV/XLSX in Draft.

The workflow remains local-only and sandboxed. No cloud database dependency is required.
v1 ships this as a single-phase capability, including embedded SQL query support from launch.

## Goals / Non-Goals

### Goals
- Treat CSV as a first-class tabular format in Workshop.
- Support robust large-file processing without forcing script-generation fallbacks.
- Provide deterministic table operations (describe/profile/query/export) through tools.
- Keep the execution path local, sandboxed, and provider-agnostic.

### Non-Goals
- Cloud-hosted database execution (for example Supabase) as a default path.
- Arbitrary DDL/DML workflows (create/drop/alter/update/delete) in v1.
- End-user SQL IDE features in the UI.
- Replacing existing `.xlsx` office tools.
- Styling for tabular text exports in v1. For styled xlsx output, use `xlsx_operations` with inline style parameters (see `docs/design/capabilities/document-styling.md`). Tabular text exports (csv) are content-only.

## Format Scope

### v1
- `.csv` (required, first-class)

### Planned Extension
- Other delimited text table formats (`.tsv`, `.psv`, delimiter-sniffed text tables) once import/type policy is expanded.
  - Auto-routing for delimiter-sniffed tabular text is enabled.
  - Worker must attach caution metadata when format identity is inferred rather than explicit.

## Architecture

### Local-Only Runtime Model
- Engine continues to mediate all tool calls.
- Worker continues to execute inside Workbench sandbox boundaries.
- Tabular operations run in-process in the local worker.
- No network/database egress required for table operations.

### Execution Strategy (Single-Phase v1)
v1 launches with the full stack in one phase:
- CSV parsing with encoding detection.
- DuckDB as the local embedded SQL engine for read-only analytics queries.
- Deterministic export through worker-controlled write paths.

Parser-only release is not part of the design.

#### Engine Choice: DuckDB
The embedded SQL engine is DuckDB (`duckdb` Python package):
- Native CSV ingestion with automatic delimiter and type inference.
- Columnar in-process engine; no external server or daemon.
- Cross-platform Python wheels available for Linux, macOS, and Windows (single shared library).
- Analytical SQL dialect with window functions, aggregations, and joins.
- Supports persistent on-disk database files (no in-memory requirement).
- Single-file dependency; no system-level installation required.

#### Encoding Detection
Real-world CSV files arrive in varied encodings (UTF-8, UTF-8 BOM, Latin-1, Windows-1252, etc.). The worker must detect encoding before parsing.

Strategy:
- Attempt UTF-8 decode first (with and without BOM).
- On failure, fall back to `charset-normalizer` library detection.
- If detection confidence is below a threshold, include a warning in the map response.
- DuckDB ingestion uses the detected encoding for correct column parsing.
- Encoding metadata (`encoding_detected`, `encoding_confidence`) is included in the `TabularGetMap` response.

#### Table Lifecycle and Storage
Each CSV file gets an on-disk DuckDB database file that persists for the lifetime of the source file in the workbench:
- **Storage**: The `.duckdb` file is stored alongside workbench metadata (e.g., `meta/tabular/<file_hash>.duckdb`). Not inside `published/` or `draft/`.
- **Creation**: The database is created on the first tabular tool call for a given CSV file (lazy initialization). The CSV is ingested once into a persistent DuckDB table on disk.
- **Persistence**: The database survives worker restarts. Subsequent tool calls open the existing `.duckdb` file without re-ingesting the CSV.
- **Invalidation**: The database is deleted and re-created when the source CSV is modified in Draft (model writes a new version via `write_text_file` or `table_export`).
- **Destruction**: The database file is deleted when the source CSV is removed from the workbench or the workbench is deleted.
- **Scope**: Each CSV file gets its own isolated `.duckdb` file. Databases from different files or workbenches never share state.
- **Disk overhead**: DuckDB's columnar format is compact; a 25MB CSV typically produces a smaller `.duckdb` file. No memory pressure from table storage.

## Tool Surface

### Workshop-Level Tools (Proposed)
- `table_get_map(path)`
- `table_describe(path)`
- `table_stats(path, columns?)`
- `table_read_rows(path, row_start, row_count, columns?)`
- `table_query(path, query, window_rows?, window_offset?)`
- `table_export(path, query?, target_path, format, sheet?)`

Notes:
- `table_query` is read-only.
- `table_export` writes to Draft only (`.csv` or `.xlsx` targets).
- `format` is explicit and validated.
- Query window controls are model-selected. Tooling does not enforce fixed hard row caps.

### Worker RPCs (Proposed)
- `TabularGetMap`
- `TabularDescribe`
- `TabularGetStats`
- `TabularReadRows`
- `TabularQuery`
- `TabularExport`

### Model-Facing Tool Schemas

These are the tool definitions sent to the model in Workshop prompts. The table name used in SQL queries is `data`.

#### `table_get_map`
```json
{
  "name": "table_get_map",
  "description": "Get a structural map of a tabular text file (CSV). Returns column names, inferred types, row count, chunk boundaries, and encoding metadata. Use this to understand the table structure before reading or querying.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "Workbench file path"}
    },
    "required": ["path"]
  }
}
```

#### `table_describe`
```json
{
  "name": "table_describe",
  "description": "Get detailed column descriptions for a tabular text file. Returns per-column inferred type, nullability, and distinct-value estimates. More detailed than the map's column list.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "Workbench file path"}
    },
    "required": ["path"]
  }
}
```

#### `table_stats`
```json
{
  "name": "table_stats",
  "description": "Get summary statistics for columns in a tabular file. Numeric columns: min, max, mean, sum, stddev. String columns: min/max length, most common values. Date columns: min, max. Specify columns to limit scope, or omit for all columns.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "Workbench file path"},
      "columns": {"type": "array", "items": {"type": "string"}, "description": "Column names to get stats for. Omit for all columns."}
    },
    "required": ["path"]
  }
}
```

#### `table_read_rows`
```json
{
  "name": "table_read_rows",
  "description": "Read a range of rows from a tabular file by position. Use for browsing data without writing SQL. Returns rows with column headers and types.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "Workbench file path"},
      "row_start": {"type": "integer", "description": "First row to read (1-based, excluding header)"},
      "row_count": {"type": "integer", "description": "Number of rows to read"},
      "columns": {"type": "array", "items": {"type": "string"}, "description": "Column names to include. Omit for all columns."}
    },
    "required": ["path", "row_start", "row_count"]
  }
}
```

#### `table_query`
```json
{
  "name": "table_query",
  "description": "Run a read-only SQL query against a tabular file. The table is available as 'data' in the query (e.g., SELECT * FROM data WHERE amount > 100). Only SELECT statements are allowed. For large results, use window_rows and window_offset to paginate. Run a COUNT(*) first to estimate result size before fetching rows.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "Workbench file path"},
      "query": {"type": "string", "description": "SQL SELECT query. Reference the table as 'data'."},
      "window_rows": {"type": "integer", "description": "Maximum rows to return in this response. Omit for engine default."},
      "window_offset": {"type": "integer", "description": "Row offset for pagination. Omit or 0 for the first window."}
    },
    "required": ["path", "query"]
  }
}
```

#### `table_export`
```json
{
  "name": "table_export",
  "description": "Export tabular data to a file in Draft. Can export the full table or a query result subset. Target must be a .csv or .xlsx file path in Draft.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "description": "Source workbench file path"},
      "query": {"type": "string", "description": "Optional SQL query to filter/transform data before export. If omitted, exports the full table."},
      "target_path": {"type": "string", "description": "Destination file path in Draft"},
      "format": {"type": "string", "enum": ["csv", "xlsx"], "description": "Output format"},
      "sheet": {"type": "string", "description": "Sheet name for xlsx output. Defaults to 'Sheet1'."}
    },
    "required": ["path", "target_path", "format"]
  }
}
```

## Map and Chunk Contracts

### `TabularGetMap` Response Shape (Illustrative)
```json
{
  "format": "csv",
  "delimiter": ",",
  "quote_char": "\"",
  "encoding_detected": "utf-8",
  "encoding_confidence": 1.0,
  "has_header": true,
  "row_count": 8421,
  "column_count": 19,
  "columns": [
    {"name": "id", "index": 0, "inferred_type": "integer"},
    {"name": "status", "index": 1, "inferred_type": "string"}
  ],
  "chunks": [
    {"index": 0, "rows": "1-500"},
    {"index": 1, "rows": "501-1000"}
  ]
}
```

### Chunking Rules
- Chunking is row-based, not line-based.
- Default chunk size target: 500 rows (tunable).
- Chunk boundaries must be deterministic for the same file content.
- Header row is included in metadata and repeated in row-read/query outputs where useful.

### `TabularDescribe` Response Shape (Illustrative)
```json
{
  "row_count": 103,
  "column_count": 5,
  "columns": [
    {
      "name": "FECHA OPERACIÓN",
      "index": 0,
      "inferred_type": "date",
      "nullable": true,
      "non_null_count": 103,
      "distinct_estimate": 22
    },
    {
      "name": "IMPORTE EUR",
      "index": 3,
      "inferred_type": "float",
      "nullable": false,
      "non_null_count": 103,
      "distinct_estimate": 98
    }
  ]
}
```

### `TabularGetStats` Response Shape (Illustrative)
```json
{
  "row_count": 103,
  "columns": [
    {
      "name": "IMPORTE EUR",
      "type": "float",
      "non_null_count": 103,
      "distinct_estimate": 98,
      "min": -1500.00,
      "max": 5000.00,
      "mean": -119.00,
      "sum": -12257.08,
      "stddev": 312.45
    },
    {
      "name": "CONCEPTO",
      "type": "string",
      "non_null_count": 103,
      "distinct_estimate": 45,
      "min_length": 4,
      "max_length": 80,
      "most_common": [
        {"value": "BIZUM", "count": 12}
      ]
    }
  ]
}
```

Notes:
- Numeric columns include: `min`, `max`, `mean`, `sum`, `stddev`.
- String columns include: `min_length`, `max_length`, `most_common` (top N values by frequency).
- Date columns include: `min`, `max`.
- Boolean columns include: `true_count`, `false_count`.
- `columns` parameter filters which columns are included; omit for all columns.

### `TabularReadRows` Response Shape (Illustrative)
```json
{
  "columns": ["FECHA OPERACIÓN", "FECHA VALOR", "CONCEPTO", "IMPORTE EUR", "SALDO"],
  "column_types": ["date", "date", "string", "float", "float"],
  "rows": [
    ["01/10/2024", "01/10/2024", "BIZUM RECIBIDO", -25.00, 31768.85],
    ["02/10/2024", "02/10/2024", "TRANSFERENCIA", -150.00, 31618.85]
  ],
  "row_start": 1,
  "row_count": 2,
  "total_rows": 103,
  "has_more": true
}
```

### `TabularQuery` Response Shape (Illustrative)
```json
{
  "columns": ["CONCEPTO", "total_amount", "tx_count"],
  "column_types": ["string", "float", "integer"],
  "rows": [
    ["BIZUM", -1234.56, 42],
    ["TRANSFERENCIA", -3456.78, 15]
  ],
  "row_count": 2,
  "total_row_count": 2,
  "window_rows": 100,
  "window_offset": 0,
  "has_more": false,
  "query_elapsed_ms": 12
}
```

### `TabularExport` Response Shape (Illustrative)
```json
{
  "target_path": "expense_summary.xlsx",
  "format": "xlsx",
  "sheet": "Sheet1",
  "row_count": 15,
  "column_count": 3,
  "warnings": []
}
```

## Query Safety and Determinism

### Query Guardrails
- Read-only statements only (`SELECT` class).
- Single statement per request.
- No filesystem path arguments inside query text.
- Execution timeout and memory guardrails.
- No hard row-result cap is imposed by tooling. If a result is large, the worker returns chunked windows with continuation metadata until complete.

### Deterministic Output Policy
- Stable column ordering in responses.
- Explicit null representation.
- Stable sort requirement for exports when query has no `ORDER BY`:
  - Either enforce a default deterministic order (original row index),
  - Or return a warning telling the model to request ordering.

## Export Behavior

### `table_export`
- Destination path must be inside Draft.
- Supported targets:
  - `format=csv` -> text CSV output
  - `format=xlsx` -> workbook output (sheet name optional, default `Sheet1`)
- Result payload includes:
  - row_count exported,
  - column_count exported,
  - target path,
  - warning list (if any coercions occurred)

## Workshop Integration

### Prompt/Behavior Requirements
- For large CSV tasks, agent should prefer table tools over line-based `read_file`.
- Agent should not return “run this script locally” when table tools can fulfill the request.
- For CSV -> XLSX workflows, preferred path is `table_export(..., format=xlsx)` (or equivalent deterministic tool flow), not generated helper scripts.
- Before broad result retrieval, agent should run a count-first probe (`COUNT(*)` or equivalent) to estimate result size.
- Agent should choose query windows/chunks explicitly (for example `window_rows` + offset progression) based on the count result.
- For delimiter-sniffed files, agent must treat type inference as best-effort and validate key assumptions before high-impact transformations.

### Loop Detection
Loop detection keying should include tabular tools:
- `table_read_rows`: hash `(path, row_start, row_count, columns)`
- `table_query`: hash `(path, normalized_query, window_rows, window_offset)`
- `table_export`: hash `(path, normalized_query, target_path, format, sheet)`

## Error Handling
- `VALIDATION_FAILED`: malformed query args, unsupported output format, invalid column reference.
- `SANDBOX_VIOLATION`: path escapes Workbench scope.
- `FILE_READ_FAILED`: source CSV not found/unreadable.
- `FILE_WRITE_FAILED`: export target not writable.
- `TOOL_WORKER_UNAVAILABLE`: tabular backend unavailable.

## Security Notes
- All table processing is local in worker process.
- No external DB/network calls are required for this capability.
- Data stays inside Workbench storage boundaries.
- Egress consent still applies only to model prompt content.

## Open Questions
None currently.
