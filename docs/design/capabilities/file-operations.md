# Design: File Operations (Local Tool Worker)

## Status
Draft (v1, local-only tooling)

## Version
v0.8

## Last Updated
2026-02-16

## PRD References
- `docs/prd/capabilities/file-operations.md`
- `docs/prd/capabilities/document-styling.md`
- `docs/prd/keenbench-prd.md` (FR1, FR7)
- Related:
  - `docs/prd/capabilities/multi-model.md`
  - `docs/prd/capabilities/workshop.md`
  - `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md`
  - `docs/design/capabilities/file-operations-tabular-text.md`
  - `docs/design/capabilities/document-styling.md`

## Summary
File operations in KeenBench are executed **locally** using a long-lived **Python tool worker**. The engine communicates with the worker over **JSON-RPC 2.0 via stdio** and applies all changes **inside the Workbench Draft**. No remote tool containers are used, and no files are uploaded to providers for file editing.

Supported formats are handled through mature local libraries:
- **DOCX**: `python-docx`
- **XLSX**: `openpyxl`
- **PPTX**: `python-pptx`
- **PDF (read-only)**: `pypdf`
- **Images (read-only)**: metadata/preview via local libraries
- **Tabular text (CSV-first)**: dedicated local tabular map/query/export path with embedded SQL query backend in v1 (see `docs/design/capabilities/file-operations-tabular-text.md`)

This design keeps the model-provider boundary separate from file operations: models generate **structured ops**, while the worker performs deterministic edits locally.

---

## Goals / Non-Goals

### Goals
- Local-only file editing within the Workbench sandbox.
- Read/write for office formats (`.docx`, `.xlsx`, `.pptx`).
- Read-only support for `.pdf` and images (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`).
- Deterministic map/query/export support for large tabular text datasets (CSV first).
- Deterministic, tool-based edits (no raw OOXML edits from the model).
- Draft-first safety: all modifications land in `draft/`.

### Non-Goals
- Remote file tooling or cloud containers for editing.
- Cloud database dependencies for CSV analysis.
- Raw OOXML edits emitted directly by the model.
- Real-time collaborative editing in office documents.
- Full editor-grade WYSIWYG parity in Review. Review relies on structured content diff plus optional raster previews.

---

## Architecture

### Local Tool Worker
The Go engine spawns a persistent Python worker at startup:
- **Transport**: JSON-RPC 2.0 over stdio
- **Lifecycle**: start with engine, restart on failure
- **Capabilities**: per-format read/write tool calls

The worker loads its libraries once (no per-call import overhead) and processes file operations by path within the Draft sandbox.

### Tool Schema (v1)
Tools are **operation-focused**, not raw file bytes. Example tool surfaces:
- `DocxApplyOps(path, ops[])`
- `XlsxApplyOps(path, ops[])`
- `PptxApplyOps(path, ops[])`
- `PdfExtractText(path)`
- `ImageGetMetadata(path)`
- `TabularGetMap(path)` / `TabularDescribe(path)` / `TabularGetStats(path)`
- `TabularQuery(path, query_spec)` / `TabularExport(path, query_spec, target_path)`

The engine validates all tool calls and restricts paths to Draft.

#### Docx ops (v1)
- `set_paragraphs`: replace the document body with ordered paragraphs
- `append_paragraph`: append a paragraph to the end
- `replace_text`: search/replace plain text

Example ops:
```
{"op":"set_paragraphs","paragraphs":[{"text":"Summary","style":"Heading1"},{"text":"Notes...","style":"Normal"}]}
{"op":"append_paragraph","text":"Next steps...","style":"Normal"}
{"op":"replace_text","search":"Old term","replace":"New term","match_case":false}
```
Canonical field is `search`. The engine also accepts legacy `find` in workshop tool payloads for backward compatibility.

Styled example (run-level + paragraph-level formatting; see `docs/design/capabilities/document-styling.md`):
```
{"op":"set_paragraphs","paragraphs":[
  {"runs":[{"text":"Q3 Financial Report","bold":true,"font_name":"Arial","font_size":18,"font_color":"#1A1A1A"}],"style":"Heading1","alignment":"center","space_after":12},
  {"runs":[{"text":"Revenue grew ","font_name":"Arial","font_size":11},{"text":"23%","bold":true,"font_color":"#2E7D32","font_size":11}],"style":"Normal","line_spacing":1.15,"space_after":6}
]}
```

#### Xlsx ops (v1)
- `ensure_sheet`: create the sheet if missing
- `set_cells`: set specific cell values
- `set_range`: set a 2D range of values

Example ops:
```
{"op":"ensure_sheet","sheet":"Summary"}
{"op":"set_cells","sheet":"Summary","cells":[{"cell":"A1","value":"Metric","type":"string"},{"cell":"B1","value":12,"type":"number"}]}
{"op":"set_range","sheet":"Summary","start":"A2","values":[["Q1",120],["Q2",140]]}
```

Styled example (cell-level styles; see `docs/design/capabilities/document-styling.md`):
```
{"op":"set_cells","sheet":"Summary","cells":[
  {"cell":"A1","value":"Category","type":"string","style":{"font_name":"Calibri","font_size":11,"font_bold":true,"font_color":"#FFFFFF","fill_color":"#2F5496","fill_pattern":"solid","h_align":"center","border_bottom":{"style":"medium","color":"#1F3864"}}},
  {"cell":"B1","value":"Amount","type":"string","style":{"font_name":"Calibri","font_size":11,"font_bold":true,"font_color":"#FFFFFF","fill_color":"#2F5496","fill_pattern":"solid","h_align":"center","number_format":"#,##0.00"}}
]}
```

New xlsx ops (v1, added for document styling):
- `set_column_widths`: set column widths for one or more columns
- `set_row_heights`: set row heights for one or more rows
- `freeze_panes`: freeze rows and/or columns

```
{"op":"set_column_widths","sheet":"Summary","columns":[{"column":"A","width":25.0},{"column":"B","width":15.0},{"column":"C","width":15.0}]}
{"op":"set_row_heights","sheet":"Summary","rows":[{"row":1,"height":20.0}]}
{"op":"freeze_panes","sheet":"Summary","row":1,"column":0}
```

#### Pptx ops (v1)
- `add_slide`: add a slide with title/body
- `set_slide_text`: replace title/body on an existing slide
- `append_bullets`: append bullets to a slide body

Example ops:
```
{"op":"add_slide","layout":"title_and_content","title":"Overview","body":"Highlights"}
{"op":"set_slide_text","index":0,"title":"Updated Title","body":"New body text"}
{"op":"append_bullets","index":1,"bullets":["Point A","Point B"]}
```
Canonical field is `index`. The engine also accepts legacy `slide_index` in workshop tool payloads for backward compatibility.

Styled example (run-level formatting on slide content; see `docs/design/capabilities/document-styling.md`):
```
{"op":"add_slide","layout":"title_and_content","title_runs":[{"text":"Q3 Results","bold":true,"font_name":"Arial","font_size":28,"font_color":"#1A1A1A"}],"body_runs":[{"text":"Revenue: ","font_name":"Arial","font_size":18},{"text":"$4.2M","bold":true,"font_color":"#2E7D32","font_size":18}],"alignment":"center","space_after":6}
```

#### Pdf and Image tools (read-only)
- `PdfExtractText(path, pages?)` returns extracted text (entire doc or selected pages).
- `ImageGetMetadata(path)` returns format, dimensions, and size.

#### JSON-RPC example (DocxApplyOps)
Request:
```
{"jsonrpc":"2.0","id":1,"method":"DocxApplyOps","params":{"path":"summary.docx","ops":[{"op":"set_paragraphs","paragraphs":[{"text":"Summary","style":"Heading1"},{"text":"Notes...","style":"Normal"}]}]}}
```
Response:
```
{"jsonrpc":"2.0","id":1,"result":{"ok":true}}
```

#### JSON-RPC example (XlsxApplyOps with styled operations)
Request:
```
{"jsonrpc":"2.0","id":2,"method":"XlsxApplyOps","params":{"path":"report.xlsx","ops":[
  {"op":"ensure_sheet","sheet":"Summary"},
  {"op":"set_cells","sheet":"Summary","cells":[{"cell":"A1","value":"Category","type":"string","style":{"font_bold":true,"fill_color":"#2F5496","font_color":"#FFFFFF"}}]},
  {"op":"set_column_widths","sheet":"Summary","columns":[{"column":"A","width":25.0}]},
  {"op":"freeze_panes","sheet":"Summary","row":1,"column":0}
]}}
```
Response:
```
{"jsonrpc":"2.0","id":2,"result":{"ok":true}}
```

### Sandbox Mapping
- **Writes** are only allowed within `draft/` (including staged draft paths).
- **Reads** may target `draft/` or `published/` (engine selects the active root).
- All paths are validated by the engine (allowlist + traversal rejection).
- The worker performs a secondary path sanity check (defense in depth).

### Format Support

| Format | Read | Write | Local Library |
|--------|------|-------|---------------|
| `.docx` | Yes | Yes | `python-docx` |
| `.xlsx` | Yes | Yes | `openpyxl` |
| `.pptx` | Yes | Yes | `python-pptx` |
| `.pdf` | Yes | No | `pypdf` |
| Images | Yes | No | local metadata/preview |
| `.csv` | Yes | Yes | local tabular engine + deterministic export |
| Other delimited tabular text | Planned | Planned | follows CSV tabular-engine path |

### Draft Changes Integration
Models generate **structured ops** (for example: add heading, update cell range, insert slide text). The engine validates ops and dispatches them to the worker. Analysis turns may produce no writes; edit turns produce Draft changes that are reviewed offline.

### Review Comparison Contract (Cross-Capability)
File operations and review use two different comparison sources that must be labeled clearly in UX:
- **Diff reference side**: frozen Draft-start reference (legacy internal/wire term: "baseline"), captured when the Draft is created.
- **Preview side**: live `published/` and `draft/` renders used as visual aids.

Implications:
- User-facing copy should prefer `Reference (Draft start)` and `Published (current preview)`; avoid exposing internal "baseline" terminology.
- If Draft-start reference extraction is unavailable, review should fall back to current `published/` comparison when possible and surface explicit source/warning metadata rather than presenting a dead-end state.

---

## File Read Strategy

### Map → Browse → Act

The AI reads files in three phases, never receiving a raw content dump:

1. **Map**: The engine generates a structural map of the file (metadata only, no content extraction) and includes it in the initial Workshop context. The map tells the AI what regions exist and how large they are.
2. **Browse**: The AI reads specific regions using tool calls with explicit coordinates (sheet+range, page range, section, tabular row range/query). Responses include chunk metadata (index, total, has_more) so the AI knows whether it has seen everything.
3. **Act**: The AI performs its task (analysis, file creation, etc.) with full awareness of the data's scope.

This replaces the prior approach where `buildWorkshopFileContext` dumped extracted text for all files, truncated by `maxContextLinesPerFile` / `maxContextCharsTotal`, which caused silent data loss for large files.

### Initial Context Strategy

For every file in the Workbench, the initial context includes:
- **File manifest**: path, type, size, readable/opaque (unchanged from v1).
- **Structural map**: the format-specific map (see below). This replaces raw extracted text for structured/tabular files.
- **Small text files** (below chunk threshold): full content may still be inlined.
- **Placeholders**: for opaque files or extraction failures, a placeholder indicating the file is present but content is unavailable.

The system instruction must state:
- The listed files are already present in the Workbench.
- Do **not** ask the user to upload files that appear in the manifest.
- Use the map to understand file structure, then use `read_file`/tabular tools with specific coordinates or query specs to read content.
- For large files, read in chunks rather than requesting all content at once.

**Fail-safe**: If the engine cannot assemble a complete file manifest, it must **fail the model call** with a structured error (per ADR-0006) rather than sending a prompt without file context.

### File Mapping — Pyworker Methods

Each format has a dedicated map method. Maps are cheap to compute (metadata only) and are generated on every Workshop context build.

#### `XlsxGetMap(workbench_id, path, root)`

Returns:
```json
{
  "sheets": [
    {
      "name": "Movimientos",
      "used_range": {"min_row": 1, "max_row": 111, "min_col": 1, "max_col": 5},
      "row_count": 111,
      "col_count": 5,
      "islands": [
        {
          "range": "A1:E8",
          "label": "header",
          "row_count": 8,
          "col_count": 5,
          "headers": ["FECHA OPERACIÓN", "FECHA VALOR", "CONCEPTO", "IMPORTE EUR", "SALDO"]
        },
        {
          "range": "A9:E111",
          "label": "data",
          "row_count": 103,
          "col_count": 5,
          "headers": null
        }
      ],
      "chunks": [
        {"index": 0, "range": "A1:E50", "rows": 50},
        {"index": 1, "range": "A51:E100", "rows": 50},
        {"index": 2, "range": "A101:E111", "rows": 11}
      ],
      "has_charts": false,
      "has_merged_cells": true,
      "has_conditional_formatting": false,
      "has_formulas": false
    }
  ]
}
```

**Island detection algorithm** (v1, simple):
- Scan rows top-to-bottom. A "blank row" is one where all cells in the used column range are empty.
- Consecutive non-blank rows form an island.
- For each island, detect headers by checking if the first row contains only string values (heuristic).
- Future: smarter detection considering blank columns, merged header rows, etc.

**Chunking algorithm**:
- Divide the used range into chunks of `CHUNK_SIZE` rows (default 50).
- Chunk boundaries align to row numbers, not island boundaries (simpler, deterministic).
- Each chunk includes the column headers row (repeated from the first island's headers) for context.

#### `DocxGetMap(workbench_id, path, root)`

Returns:
```json
{
  "sections": [
    {"heading": "Executive Summary", "level": 1, "char_count": 1200, "has_tables": false, "has_images": false},
    {"heading": "Financial Analysis", "level": 1, "char_count": 8500, "has_tables": true, "has_images": true,
     "chunks": [
       {"index": 0, "paragraphs": "1-25", "char_count": 4200},
       {"index": 1, "paragraphs": "26-48", "char_count": 4300}
     ]
    }
  ],
  "tables": [{"index": 0, "section": "Financial Analysis", "rows": 15, "cols": 4}],
  "images": [{"index": 0, "section": "Financial Analysis", "alt_text": "Revenue chart"}],
  "has_headers_footers": true,
  "total_char_count": 12400
}
```

**Section detection**: Walk paragraphs looking for Heading styles (Heading 1, Heading 2, etc.). Group content under headings to form sections. If no headings exist, treat the entire document as one section.

#### `PptxGetMap(workbench_id, path, root)`

Returns:
```json
{
  "slides": [
    {"index": 0, "title": "Q3 Results", "layout": "Title Slide", "has_images": true, "has_charts": false, "has_notes": false},
    {"index": 1, "title": "Revenue Breakdown", "layout": "Title and Content", "has_images": false, "has_charts": true, "has_notes": true}
  ],
  "slide_count": 12
}
```

Presentations are inherently paginated by slide, so chunking is rarely needed. Each slide is a natural unit.

#### `PdfGetMap(workbench_id, path, root)`

Returns:
```json
{
  "page_count": 42,
  "has_toc": true,
  "toc": [
    {"title": "Introduction", "page": 1},
    {"title": "Methodology", "page": 5}
  ],
  "has_forms": false,
  "has_annotations": false,
  "chunks": [
    {"index": 0, "pages": "1-5"},
    {"index": 1, "pages": "6-10"}
  ]
}
```

#### Text File Maps

For plain text files, the map is lightweight:
```json
{
  "line_count": 350,
  "char_count": 12000,
  "chunks": [
    {"index": 0, "lines": "1-200"},
    {"index": 1, "lines": "201-350"}
  ]
}
```

For structured text (CSV): additionally report column count and column headers.
For JSON: report top-level keys and whether it's an array or object.

Tabular-text behavior is now specified in detail in:
- `docs/design/capabilities/file-operations-tabular-text.md`

That doc defines row-based chunking and deterministic local query/export tools for CSV-first workflows.

### Chunked Read — Tool Updates

The existing `read_file` tool already accepts `sheet` and `range` parameters for xlsx. The following changes extend chunked reading:

- **xlsx**: `read_file(path, sheet, range)` — unchanged. The map provides the ranges to request.
- **docx**: `read_file(path, section)` — new optional `section` parameter (heading text or section index). Without it, reads the full document (subject to truncation as before, but now the AI knows via the map that it should specify a section).
- **pptx**: `read_file(path, slide_index)` — new optional `slide_index` parameter.
- **pdf**: `read_file(path, pages)` — new optional `pages` parameter (e.g., "1-5"). The pyworker `PdfExtractText` already supports this.
- **text**: `read_file(path, line_start, line_count)` — new optional line-range parameters for large text files.
- **tabular text (csv)**: use tabular map/query tools for schema/stats/query and row-range reads; avoid line-based parsing for analytics workflows.

Every chunked read response includes:
```json
{
  "text": "...",
  "chunk_info": {
    "chunk_index": 1,
    "total_chunks": 3,
    "has_more": true,
    "range": "A51:E100"
  }
}
```

If the file is small enough to return in one read, `chunk_info` is omitted or `total_chunks: 1`.

### Workshop Agent Tool Definitions (Updated)

The `read_file` tool description is updated to reflect the map-first workflow:

```
Read content from a file. For large files, use get_file_info first to see the structural
map, then read specific regions:
- xlsx: specify sheet name and optional range (e.g. A1:E50)
- docx: specify section heading or index
- pptx: specify slide_index
- pdf: specify page range (e.g. "1-5")
- text: specify line_start and line_count for large files
- csv/tabular: use tabular schema/stats/query tools for table workflows and export results to csv/xlsx
- csv/tabular: run count-first probes before broad fetches, then use model-selected chunk/window progression for retrieval

The map in the file context shows available regions, sizes, and chunk boundaries.
Always check the map before reading to avoid requesting more data than needed.
```

The `get_file_info` tool is enhanced to return the full structural map (it currently returns only basic sheet dimensions for xlsx). Alternatively, a new `get_file_map` tool could be added — the choice depends on whether the map is always cheap enough to compute alongside basic info.

---

## File Write Strategy
- Writes are executed locally by the Python worker.
- The worker performs edits using library APIs, preserving OOXML structure.
- Read-only formats (`.pdf`, images) reject writes with structured errors.

### Chunked Writing

For large write operations (e.g., writing a 500-row spreadsheet), the AI can issue multiple `xlsx_operations` calls with `set_range` targeting successive row ranges. The existing `set_range` op already supports this — no new mechanism is needed, just model guidance in the system prompt.

### Style and Asset Preservation

This section covers **derivative style preservation** — querying and copying styles/assets from existing source files to new targets. For **inline styling** of new documents from scratch (style parameters on write ops, bundled format style skills, user style guide merge), see `docs/design/capabilities/document-styling.md`.

New pyworker methods for querying and copying styles/assets (future):

- `XlsxGetStyles(path, sheet)` — returns cell format descriptions, conditional formatting rules, named ranges
- `XlsxCopyAssets(source_path, target_path, assets[])` — copies specified charts, images, or style definitions from source to target
- `DocxGetStyles(path)` — returns paragraph and character style definitions
- `DocxCopyAssets(source_path, target_path, assets[])` — copies headers/footers, images, styles
- `PptxGetStyles(path)` — returns slide master and layout information
- `PptxCopyAssets(source_path, target_path, assets[])` — copies layouts, images, media

These are surfaced as Workshop tools so the AI can explicitly request asset copying when creating derivative files.

---

## Error Handling
Worker failures are mapped to ADR-0006 error codes:
- `VALIDATION_FAILED` for unsupported formats or read-only writes
- `SANDBOX_VIOLATION` for invalid paths
- `FILE_READ_FAILED` / `FILE_WRITE_FAILED` for IO errors
- Map generation failures fall through to `FILE_READ_FAILED` with a descriptive message; the engine falls back to basic metadata.

---

## Packaging
The worker is shipped alongside the app:
- **Option A**: bundled virtual environment
- **Option B**: PyInstaller-built executable per OS
- **Target platforms**: Linux first, macOS next; Windows deferred.

The engine locates the worker via a configured path and falls back to a bundled default.
Preview rendering can optionally be overridden via `KEENBENCH_PREVIEW_RENDERER_PATH`
(expects a LibreOffice-compatible CLI for PDF conversion).
For dev/CI on Linux, `scripts/package_worker.sh` builds a local venv wrapper and prints the
`KEENBENCH_TOOL_WORKER_PATH` to use.

---

## IPC / API Surface
The UI does not call the worker directly. The engine exposes existing Review and Workshop RPCs; internally it uses worker RPCs such as:
- `WorkerGetInfo`
- `DocxApplyOps`, `DocxExtractText`, **`DocxGetMap`**, `DocxGetSectionContent`
- `XlsxApplyOps`, `XlsxExtractText`, `XlsxReadRange`, `XlsxGetInfo`, **`XlsxGetMap`**
- `PptxApplyOps`, `PptxExtractText`, **`PptxGetMap`**, `PptxGetSlideContent`
- `PdfExtractText`, `PdfGetInfo`, **`PdfGetMap`**
- `ImageGetMetadata`
- `TabularGetMap`, `TabularDescribe`, `TabularGetStats`, `TabularQuery`, `TabularExport`

Future (derivative style/asset preservation — for inline styling see `docs/design/capabilities/document-styling.md`):
- `XlsxGetStyles`, `XlsxCopyAssets`
- `DocxGetStyles`, `DocxCopyAssets`
- `PptxGetStyles`, `PptxCopyAssets`

---

## Security Notes
- All edits are local-only.
- No file uploads to model providers are required for file operations.
- Egress consent applies to LLM prompts only (not to local tool execution).
- File maps are metadata-only and do not contain file content; they are included in prompts under the same egress consent as other file context.
- Tabular query execution is local-only and must not depend on external database services.

## Open Questions
- Should `get_file_info` be extended to include the map, or should `get_file_map` be a separate tool? Separate is cleaner but adds tool surface area.
- Island detection heuristics: blank-row scanning works for simple spreadsheets but may misfire on spreadsheets with intentional blank rows (e.g., section separators). Need to test with real-world files.
- Chunk size tuning: 50 rows for xlsx, 4000 chars for docx, 5 pages for pdf are starting points. May need per-model tuning based on context window size.
