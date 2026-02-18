# PRD: File Operations

## Status
Draft

## Version
v0.7

## Last Updated
2026-02-13

## Purpose
Enable AI to read and write office documents (docx, odt, xlsx, pptx, pdf) and tabular text files (csv first, with other delimited table formats to follow) within the Workbench, with full structural awareness and reliable handling of files of any size. The AI must be able to understand a file's structure before reading it, browse large files in manageable chunks without data loss, query tabular datasets deterministically, and preserve style, formatting, and embedded assets when creating derivative files.

## Scope
- In scope (v1): File reading and file writing for all configured providers via the local tool worker (see ADR-0008), with intent-aware behavior (analysis requests may produce no writes; edit requests produce Draft changes).
- In scope (v1 — File Intelligence): Structural mapping for all supported formats (including basic island detection for spreadsheets and tabular text schema detection), chunked reading for large files, map-first initial context (replacing raw content dump), and a queryable toolset for tabular text backed by a local embedded SQL engine.
- In scope (future): Style and asset preservation (logos, charts, conditional formatting), advanced island detection heuristics (multi-region merging, blank-column scanning), graph/chart copying between files.
- Out of scope: Custom file parsing libraries, arbitrary format-conversion workflows outside deterministic table-export tools, real-time collaborative editing, direct filesystem access outside Workbench.

## Provider Capabilities

### Capability Matrix (v1)
| Provider | File Read | File Write | Mechanism |
|----------|-----------|------------|-----------|
| OpenAI | Yes | Yes | Model reasoning + local tool worker operations |
| Anthropic | Yes | Yes | Model reasoning + local tool worker operations |
| Google | Yes | Yes | Model reasoning + local tool worker operations |
| Mistral | Yes | Yes | Model reasoning + local tool worker operations |

### Supported File Formats
| Format | Read | Write | Notes |
|--------|------|-------|-------|
| .docx | Yes | Yes | Full structure support |
| .odt | Yes | Yes (best-effort) | LibreOffice Writer; best-effort structure support |
| .xlsx | Yes | Yes | Cells, formulas, formatting |
| .pptx | Yes | Yes | Slides, notes, basic layout |
| .pdf | Yes | Limited | Read: full; Write: annotations only |
| .csv | Yes | Yes | First-class tabular text in v1: schema/profile/query/export workflows |
| Other delimited tabular text (for example .tsv, .psv) | Planned | Planned | Same queryable workflow as CSV once format-policy expansion lands |
| Other text/code files | Yes | Yes | Direct content manipulation |

## User Experience

### Transparent Operation
Users interact with file operations through Workshop without provider-specific write restrictions:
- Workshop: "Edit the summary in report.docx" works regardless of selected provider.

### Model Capability Visibility
Users see provider/model differences (quality, speed, cost) in two places:

1. **Settings > Model Providers**: Informational note in the providers panel:
   > "All configured models can analyze and edit Workbench files. File edits execute locally in Draft and are reviewed before publish."

2. **Model Selector** (Workshop): Users can switch models freely; model choice does not disable file editing workflows.

### Intent-Aware Behavior
The AI can either analyze or write, based on task intent:
- Analysis/Q&A requests may complete without creating a Draft.
- Edit/creation requests produce Draft changes.
- The user always controls final acceptance via Publish/Discard.

## Functional Requirements

### v1

1. **File Reading**: All configured providers can read supported file formats.
2. **File Writing (All Providers)**: File writes are available regardless of selected provider and are executed by the local tool worker.
3. **Intent-Aware Execution**: The AI may perform read-only analysis or write operations, depending on the user request and task needs.
4. **Draft Integration**: All file writes occur in the Draft sandbox; the Published state is never modified directly.
5. **Audit Trail**: File operations log which model performed reasoning and which files were read/written.
6. **Tabular Query Toolset**: CSV and other tabular text workflows support deterministic table inspection and querying (schema description, stats/profile, filtered/aggregated/joined reads) without requiring script generation.
7. **Deterministic Table Export**: Query results can be exported locally to CSV and XLSX in Draft.
8. **Embedded SQL in v1**: Tabular query capabilities ship in v1 with a local embedded SQL backend (single-phase launch; parser-only mode is not acceptable for release).
9. **Model-Led Query Sizing**: Tooling does not enforce fixed hard row caps on tabular queries. The model must use count-first and chunked retrieval patterns to manage result size intentionally.

### File Intelligence

The AI must understand a file's structure before reading its content. Dumping raw text into the context and hoping the model notices truncation is unreliable. Instead, every file read follows a **map → browse → act** pattern:

1. **Mapping**: Before the AI reads any content, the system provides a structural map of the file. The map describes what the file contains (sheets, sections, pages, slides, table columns/row ranges) and how large each region is, so the AI can plan its reads.
2. **Chunked Reading**: When a file region exceeds a size threshold, the system divides it into chunks with explicit boundaries. The AI reads chunks on demand rather than receiving a single truncated blob.
3. **Complete Coverage**: No data is silently dropped. If content is too large for a single read, the system tells the AI exactly how many chunks exist and lets the AI iterate.

#### 8. File Mapping (all formats)

The system shall provide a structural map for every supported file format. The map is the AI's table of contents — it answers "what is in this file and how big is each part?" without transferring the actual content.

**Generic map fields** (all formats):
- File path, format, total size
- List of top-level regions (sheets, sections, pages, slides, tabular row ranges)
- Per-region: name/label, row/column count or page count, whether content exceeds the chunk threshold

**The map is always available** — it is cheap to compute (metadata only, no content extraction) and is included in the initial Workshop context for every file.

#### 9. Chunked Reading (all formats)

When a file region exceeds the chunk threshold, the system shall divide it into chunks and report chunk boundaries in the map.

- The AI requests chunks by index or range (e.g., "sheet Movimientos, rows 1–50").
- Each chunk response includes: the chunk data, the chunk index, the total chunk count, and whether more chunks remain.
- The AI can read all chunks sequentially, read a specific chunk, or skip chunks that are irrelevant to the task.
- Chunking is deterministic: the same file always produces the same chunk boundaries.

#### 10. Map-First Context Strategy

The initial Workshop context for structured files (xlsx, docx, pptx, pdf) and tabular text files (csv) shall include **only the map**, not raw extracted content. This replaces the current approach of dumping truncated text into the initial context.

- For small non-tabular text files (below the chunk threshold), full content may still be inlined.
- For structured and tabular files, the map provides enough orientation for the AI to decide what to read/query.
- The AI uses read/query tools (sheet/range/page/row-range/query parameters) to fetch the actual content it needs.

#### 11. Style and Asset Preservation (Future — not v1)

When the AI creates a derivative file (e.g., a new spreadsheet based on an existing one), the system shall support preserving visual and structural assets from the source:

- **Styles**: cell formatting, conditional formatting, paragraph styles, slide layouts
- **Embedded assets**: logos, images, headers/footers
- **Charts and graphs**: chart definitions, data references, chart styling
- **Structural elements**: merged cells, named ranges, formulas, slide transitions

The AI shall have tools to query what styles and assets exist in a source file and to copy them into a target file.

> **Cross-reference**: This section covers *derivative* style preservation (copying styles from an existing source file into a new target). For *inline* style parameters on write operations (creating styled content from scratch), built-in format style skills, and style guide integration, see the dedicated [`document-styling.md`](document-styling.md) PRD.

### Format-Specific Requirements

#### Spreadsheets (xlsx)

**Map contents:**
- List of sheets with names
- Per-sheet: used range (first/last row and column with data), total row and column count
- Per-sheet: list of data regions ("islands") — contiguous blocks of non-empty cells separated by blank rows/columns. Each island has a bounding range (e.g., A1:E111)
- Per-sheet: list of charts (name, type, data range reference)
- Per-sheet: whether conditional formatting, merged cells, or formulas are present
- Per-sheet: column headers (first non-empty row of each island, typically the header row)

**Chunking:**
- Chunk unit: row ranges within a sheet (e.g., rows 1–50, 51–100, 101–111)
- Chunk threshold: configurable, default ~50 rows per chunk
- Each chunk includes column headers repeated for context

**Style preservation (future):**
- Copy cell styles (font, color, borders, number format)
- Copy conditional formatting rules
- Copy charts (with updated data references if the data range changes)
- Copy images/logos
- Preserve merged cell ranges
- Preserve named ranges and formulas

#### Documents (docx)

**Map contents:**
- List of sections or heading structure (heading text + level, forming a TOC-like outline)
- Per-section: approximate character/word count
- List of tables (position, row/column count)
- List of embedded images (position, alt text if available)
- Whether headers/footers are present

**Chunking:**
- Chunk unit: sections (by heading) or page-equivalent blocks
- Chunk threshold: configurable, default ~4000 characters per chunk
- Each chunk includes the section heading for context

**Style preservation (future):**
- Copy paragraph styles and character formatting
- Copy headers/footers
- Copy embedded images
- Preserve table structure and formatting

#### Presentations (pptx)

**Map contents:**
- Slide count
- Per-slide: title text, layout name, whether it contains images/charts/tables
- Speaker notes presence per slide

**Chunking:**
- Chunk unit: individual slides (natural boundary)
- Presentations rarely need chunking since slides are inherently paginated

**Style preservation (future):**
- Copy slide layouts and master slide styles
- Copy images and media
- Preserve transitions and animations

#### PDF (read-only)

**Map contents:**
- Page count
- Table of contents / bookmarks (if present)
- Whether the PDF contains forms, annotations, or embedded images
- Per-page: approximate character count

**Chunking:**
- Chunk unit: page ranges (e.g., pages 1–5, 6–10)
- Chunk threshold: configurable, default ~5 pages per chunk

#### Tabular Text Files (CSV and compatible delimited tables)

**Map contents:**
- Delimiter and quoting metadata (detected delimiter, quote char, header-present flag)
- Row count and column count
- Ordered column descriptors (name, inferred type, nullability estimates)
- Row chunk boundaries suitable for deterministic full-file traversal
- Optional per-column profile stats (non-null count, distinct estimate, min/max for numeric/date-like fields)

**Chunking:**
- Chunk unit: row ranges
- Chunk threshold: configurable, default ~500 rows per chunk
- Each chunk includes headers + stable row index bounds

**Queryable toolset requirements:**
- Schema/column description
- Table-level and column-level statistics
- Read-only querying/filtering/aggregation/join operations
- Deterministic export to CSV and XLSX in Draft
- Automatic delimiter-sniff routing for tabular-looking text with explicit warning metadata when format identity is inferred
- Query retrieval supports model-controlled windows/chunks without fixed tool-imposed row ceilings

#### General Text Files (non-tabular)

**Map contents:**
- Line count, character count, file size
- For structured non-tabular text (JSON, XML, YAML): basic structure summary (for example JSON top-level keys)

**Chunking:**
- Chunk unit: line ranges
- Chunk threshold: configurable, default ~200 lines per chunk
- Small text files (below threshold) continue to be inlined in full

## Acceptance Criteria

### File Operations
- Users can read docx, odt, xlsx, pptx, pdf, and text files in Workshop.
- Users can modify supported file formats with any configured provider.
- File modifications appear in Draft and are reviewed before publishing.
- For analysis-only requests, the AI can complete tasks without creating Draft changes.
- Audit trail records which model performed each file operation.
- CSV is explicitly supported as a first-class tabular format.
- CSV workflows can be completed with local tools (describe/stats/query/export) without requiring generated helper scripts.
- CSV tabular workflows in v1 are backed by a local embedded SQL engine (no phased parser-only release).
- Query sizing behavior is model-led: count-first + chunked retrieval is required guidance, not a fixed hard cap in tooling.

### File Intelligence
- For every structured or tabular file in the Workbench, the AI receives a structural map before reading content.
- The AI can read any region of a large file by specifying chunk coordinates (sheet+range, page range, section name).
- No data is silently truncated. If a file region is too large for one read, the response includes chunk metadata (index, total, has_more).
- A spreadsheet with 100+ rows is correctly processed in its entirety (all rows accounted for, no silent truncation).
- A tabular text dataset with thousands of rows can be fully traversed/query-processed with deterministic chunking.


## Failure Modes & Recovery

- **File format unsupported**: Clear message identifying the unsupported format.
- **File too large for one read**: Offer chunked processing or suggest reducing scope.
- **Tool worker error during write**: Preserve Draft state; show error with retry option.
- **Map generation fails**: Fall back to basic metadata (path, size, type); log warning. The AI can still attempt reads but without structural guidance.
- **Chunk read fails**: Return error for the specific chunk; other chunks remain accessible.

## Security & Privacy

- File operations are sandboxed to the Workbench.
- Model prompts follow the standard egress consent model.
- In production, the audit trail records which files were in scope and which model performed the operation **without** storing raw file contents. In debug mode, raw prompt/file excerpts may be logged for development triage.
- File maps contain structural metadata (sheet names, dimensions, column headers) and are included in model prompts under the same egress consent as other Workbench content. Maps do not contain bulk file content — only enough structural information for the AI to plan its reads.
- Tabular query execution is local-only; no external database/service is required for CSV analysis workflows.

## Open Questions
- What is the right default chunk size per format? Needs tuning based on model context windows and real-world file sizes.
