# Implementation Plan: M1 — File Operations (Local Tool Worker, Office Formats + Images Read)

## Status
Draft (2026-01-31)

## Goal (M1)
Ship **local-only file operations** in the existing Workshop loop using a **Python tool worker**, adding **office formats** and **read-only PDFs/images**, while preserving M0 safety and Draft/Publish invariants.

M1 also introduces:
- **Proposal schema v2** (structured ops + per-file summaries, backward-compatible with v1).
- **Opaque file support** (import allowed, metadata-only review).
- **Side‑by‑side image review** (Draft vs Published) without UI direct file access.

This milestone focuses on:
- **docx/xlsx/pptx read + write** via local tooling
- **pdf + images read-only**
- **JSON-RPC tool worker** (long-lived local process)
- **Review** based on text extraction **plus** paginated full‑resolution previews (see Review UX)

---

## Key References (Impacted Design)

### Architecture & boundaries
- `docs/design/design.md` — overall architecture (Flutter UI + Go engine, Workbench/Draft concepts)
- `docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md` — IPC framing

### File operations + review
- `docs/design/capabilities/file-operations.md` — local tool worker design (core of M1)
- `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md` — local-only tool worker decision
- `docs/design/capabilities/review-diff.md` — review UX and diff behavior
- `docs/design/adr/ADR-0002-in-app-previews-for-review.md` — preview approach (we keep it minimal in M1)
- `docs/design/capabilities/workshop.md` — draft changes flow + summaries (internal proposal artifacts; source of v2 expectations)

### Workbench / Draft / Review / Egress
- `docs/design/capabilities/workbench.md`
- `docs/design/capabilities/draft-publish.md`
- `docs/design/capabilities/security-egress.md`
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`

---

## Scope

### In Scope (M1)
1. **Local Tool Worker (Engine)**
   - Engine spawns a long-lived Python worker at startup.
   - JSON-RPC 2.0 over stdio for tool calls.
   - Worker preloads `python-docx`, `openpyxl`, `python-pptx`, `odfpy`, `pypdf`, `PyMuPDF`, `Pillow`,
     plus optional `CairoSVG`/`librsvg` for SVG rasterization.
   - Strict Draft sandbox mapping for **writes** (no Published writes; no path traversal).

2. **Format support**
   - **Read + write**: `.docx`, `.xlsx`, `.pptx`.
   - **Read-only**: `.pdf`, `.odt`, images (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`).
   - **Text/code (read + write as text)**: `.md`, `.txt`, `.csv`, `.json`, `.xml`, `.yaml`, `.yml`, `.html`,
     and code extensions in PRD v1 (`.js`, `.ts`, `.py`, `.java`, `.go`, `.rb`, `.rs`, `.c`, `.cpp`, `.h`,
     `.css`, `.sql`).
   - **Opaque**: allow other file types to be imported; metadata-only in Review.

3. **Create new office files**
   - Allow draft-changeset-driven creation of new `docx/xlsx/pptx` in Draft.

4. **Proposal schema v2**
   - Add a structured ops schema for office file edits.
   - Include per-file summaries for office + non-text outputs (stored for Review).
   - Maintain v1 proposal compatibility (`writes[]` for text/code files).

5. **Review UX**
   - Office + PDF: text-extraction diff (line-based) for Draft vs **Published baseline**.
   - Office/PDF previews: **full-resolution** paginated preview (Draft vs Published), with pagination
     semantics per file type (see Review UX).
   - Images: **side-by-side** preview with metadata (dimensions, size, format); no diff.
   - Opaque/binary: metadata-only preview.
   - Newly created files: show **New file** indicator with Draft-only preview (no baseline diff).

6. **Egress + safety**
   - Consent scope hash is based on the **Workbench file list** (manifest paths only),
     **not** file contents. Consent re-prompts only when the file list changes.
   - The file list includes **all** imported paths (supported + opaque).
   - Reject write attempts to read-only formats with `VALIDATION_FAILED`.
   - No file uploads to model providers for file operations.
   - Workshop context uses the **active view** (Draft if present) **for files that exist in the manifest**.
     Draft-only outputs are excluded from prompts until they become part of the Workbench file list.
   - Consent scope **only** changes when the Workbench file list changes (e.g., `WorkbenchFilesAdd`).
     Draft-only outputs never affect consent until they are added to the Workbench file list.

7. **Draft baseline extraction (to support side-by-side review)**
   - When a Draft is created, **before any Draft mutations**, capture baseline extracted text for office/PDF files
     from the fresh Draft copy (which matches Published at creation time).
   - Baseline is stored under `meta/review/<draft_id>/baseline/` and used for “Published”
     comparisons in Review. This keeps the worker Draft‑only while still enabling side‑by‑side.
   - Clean up baseline artifacts on **Publish** and **Discard**.
   - On engine startup, clean up orphaned baseline directories for drafts that no longer exist.

8. **Tests**
   - Engine unit tests for worker lifecycle, sandbox, read/write enforcement.
   - Integration tests for draft changes apply + draft publish on office files.
   - E2E covering office + image in the Workshop loop.
9. **Workshop prompt context (file manifest + content)**
   - Every Workshop model call includes a Workbench file manifest plus extracted text/summaries for readable files.
   - Opaque or failed extractions are represented with explicit placeholders (file exists; content unavailable).
   - System instruction states files are already available and must not be re-requested.
   - If the manifest cannot be assembled, fail fast with a structured error instead of sending a partial prompt.
   - Add tests that assert prompt payloads include the manifest and per-file payloads.

### Out of Scope (explicitly not in M1)
- Remote tool containers or provider SDK file tooling.
- Audit trail, checkpoints UI.
- Semantic office previews (editable tables/slide structure) beyond paginated render.
- Image or PDF editing.
- ODT **write/edit** operations (read-only in M1).
- Deletions in draft changesets (still rejected).

---

## What “Done” Means (M1 Acceptance Criteria)

### Functional loop
- From a Workbench with `docx/xlsx/pptx/pdf/odt` + an image (including SVG):
  1) User asks for changes in Workshop.
  2) Auto-generated draft changes include edits to an office file and a new office file.
  3) Auto-apply succeeds; Draft contains new/updated office files (via local worker).
  4) **Review** shows text diff for office/PDF and **side‑by‑side** paginated previews
     (docx/odt/pdf/pptx/xlsx/images).
  5) **Publish** swaps Draft into Published; reopen shows updated files.

### Safety / policy invariants
- Worker edits are Draft-only; Published is never written directly.
- Path traversal attempts fail with `SANDBOX_VIOLATION`.
- Read-only formats reject write attempts with `VALIDATION_FAILED`.
- Review performs no model calls.
- No remote file uploads are required for file operations.
- Opaque files are importable but never parsed for content.
- Workshop prompts always include the Workbench file manifest and per-file content payloads (or placeholders).

---

## Major Design Decisions for M1

### 1) Local tool worker over remote SDKs
Why: remote tool containers are explicitly disallowed; local-only keeps Draft boundaries intact.

How: Python worker process runs locally, tool calls over JSON-RPC.

### 2) Structured tool ops instead of raw OOXML
Why: raw OOXML edits are brittle and risk file corruption.

How: model outputs structured ops; worker performs deterministic edits with libraries.

### 3) Read-only PDF + images
Why: keeps review/diff tractable and avoids complex binary editing workflows.

How: expose read operations to models; block write attempts at validation.

### 4) Preview rendering strategy (M1)
We need a concrete path to render **visual** previews for office/PDF/image types. The worker will use
local, offline tooling and may delegate rendering to a small helper binary to isolate native deps.

**Recommended toolchain**
- **DOCX/PPTX/ODT**: convert to PDF via **LibreOffice headless** (`soffice --headless --convert-to pdf`)
  into a temp dir, then rasterize PDF pages.
- **PDF**: rasterize pages directly via **PyMuPDF** (fitz) or `pdf2image` (Poppler).
- **SVG**: rasterize via **librsvg/CairoSVG** (fallback to Pillow if embedded raster only).
- **PNG/JPEG/GIF/WEBP**: decode via Pillow; keep original for metadata.

**Packaging**
- If preview rendering uses a separate binary, name it `keenbench-preview-renderer` and locate it via
  `KEENBENCH_PREVIEW_RENDERER_PATH`. The worker calls it over stdio or CLI.
- If deps are missing, preview RPCs return `TOOL_WORKER_UNAVAILABLE` (not a hard crash).
- If LibreOffice headless is unavailable, DOCX/PPTX/ODT preview RPCs return
  `TOOL_WORKER_UNAVAILABLE`; text extraction still works via python-docx/python-pptx/odfpy.

---

## Implementation Plan (Changes by Area)

### Workshop prompt context (new)
Ensure the Workshop prompt builder always injects Workbench file context so the model does not ask users to re-upload existing files.

Tasks:
- Build a **file manifest block** from the Workbench file list (path, type, size, readable/opaque).
- Attach extracted text/summaries for readable files with clear per-file delimiters.
- Add explicit **system instruction**: listed files are already present; do not ask to upload them; request clarification or pasted excerpts if needed.
- Fail fast with a structured error if the manifest cannot be assembled.
- Add unit tests for prompt assembly and an E2E regression case where a file is present and the assistant does not ask to upload it.

### Proposal Schema v2 (new)
Introduce a versioned proposal format that supports structured ops while preserving v1 (and allowing explicit no-change proposals):

**Proposal v2 shape (illustrative)**
```
{
  "schema_version": 2,
  "summary": "…",
  "no_changes": false,
  "writes": [
    {"path":"summary.md","content":"…"}
  ],
  "ops": [
    {"path":"report.docx","kind":"docx","summary":"…","ops":[{"op":"set_paragraphs",…}]},
    {"path":"metrics.xlsx","kind":"xlsx","summary":"…","ops":[{"op":"set_range",…}]},
    {"path":"deck.pptx","kind":"pptx","summary":"…","ops":[{"op":"add_slide",…}]}
  ],
  "warnings": []
}
```

Rules:
- `schema_version=2` is required for structured ops; if absent, treat as v1.
- `writes[]` continues to allow **text/code** writes.
- `ops[]` is required for office edits; each entry includes a **per‑file summary** used in Review.
- **Either** `writes[]` **or** `ops[]` must be present (ops‑only proposals are valid), **or** `no_changes:true` with empty `writes/ops`.
- No deletes; nested paths remain disallowed (flat Workbench).
- Validation limits:
  - Max ops per file: **100**
  - Max ops per proposal: **500**
  - Ops are **atomic per file** (all ops for a file apply or none do).
  - Proposal apply is **best‑effort per file** (some files may apply while others fail).

Prompting guidance:
- Create `docs/design/prompts/proposal-v2.md` describing the expected structured ops schema,
  examples, and constraints (used by `WorkshopProposeChanges`).

### Tool Schema (v1, initial)
Tools are local JSON-RPC methods invoked by the engine. The model generates structured ops and the engine translates them into tool calls.

**DocxApplyOps**
- `set_paragraphs`: replace body with ordered paragraphs
- `append_paragraph`: append paragraph
- `replace_text`: search/replace plain text

Example:
```
{"jsonrpc":"2.0","id":1,"method":"DocxApplyOps","params":{"path":"summary.docx","ops":[{"op":"set_paragraphs","paragraphs":[{"text":"Summary","style":"Heading1"},{"text":"Notes...","style":"Normal"}]}]}}
```

**XlsxApplyOps**
- `ensure_sheet`
- `set_cells`
- `set_range`

Example:
```
{"jsonrpc":"2.0","id":2,"method":"XlsxApplyOps","params":{"path":"metrics.xlsx","ops":[{"op":"ensure_sheet","sheet":"Summary"},{"op":"set_range","sheet":"Summary","start":"A1","values":[["Metric","Value"],["Q1",120]]}]}}
```

**PptxApplyOps**
- `add_slide`
- `set_slide_text`
- `append_bullets`

Example:
```
{"jsonrpc":"2.0","id":3,"method":"PptxApplyOps","params":{"path":"deck.pptx","ops":[{"op":"add_slide","layout":"title_and_content","title":"Overview","body":"Highlights"}]}}
```

**PdfExtractText** (read-only):
```
{"jsonrpc":"2.0","id":4,"method":"PdfExtractText","params":{"path":"report.pdf","pages":[1,2],"root":"draft"}}
```

**DocxExtractText / XlsxExtractText / PptxExtractText** (read-only):
```
{"jsonrpc":"2.0","id":6,"method":"DocxExtractText","params":{"path":"report.docx","root":"draft"}}
{"jsonrpc":"2.0","id":7,"method":"XlsxExtractText","params":{"path":"metrics.xlsx","root":"draft"}}
{"jsonrpc":"2.0","id":8,"method":"PptxExtractText","params":{"path":"deck.pptx","root":"draft"}}
{"jsonrpc":"2.0","id":9,"method":"OdtExtractText","params":{"path":"notes.odt","root":"draft"}}
```

**ImageGetMetadata** (read-only):
```
{"jsonrpc":"2.0","id":5,"method":"ImageGetMetadata","params":{"path":"chart.png"}}
```
Returns `{format, width, height, size_bytes, color_depth?}`. For SVG, width/height may be null if
not defined in the source.

**Preview render methods** (read-only, full‑res pagination):
```
{"jsonrpc":"2.0","id":10,"method":"PdfRenderPage","params":{"path":"report.pdf","page_index":0,"scale":1.0,"root":"draft"}}
{"jsonrpc":"2.0","id":11,"method":"DocxRenderPage","params":{"path":"report.docx","page_index":0,"scale":1.0,"root":"draft"}}
{"jsonrpc":"2.0","id":12,"method":"PptxRenderSlide","params":{"path":"deck.pptx","slide_index":0,"scale":1.0,"root":"draft"}}
{"jsonrpc":"2.0","id":13,"method":"OdtRenderPage","params":{"path":"notes.odt","page_index":0,"scale":1.0,"root":"draft"}}
{"jsonrpc":"2.0","id":14,"method":"XlsxRenderGrid","params":{"path":"metrics.xlsx","sheet":"Summary","row_start":0,"row_count":200,"col_start":0,"col_count":20,"root":"draft"}}
```
Responses return `bytes_base64` (for page/slide renders) plus `page_count`/`slide_count`, and for
`XlsxRenderGrid` return `cells` plus `sheets[]`.

**XlsxRenderGrid response shape**
```
{
  "sheets": ["Summary","Details"],
  "row_count": 200,
  "col_count": 20,
  "cells": [
    [{"value":"Header","type":"string","formula":null}, {"value":123.45,"type":"number","formula":null}],
    [{"value":"Total","type":"string","formula":null}, {"value":null,"type":"blank","formula":"=SUM(B2:B10)"}]
  ]
}
```
- Merged cells: value appears **only** in the top-left cell; other merged cells return `null`.

Notes:
- All worker paths are **relative** (no absolute paths).
- **Write** ops must target Draft (enforced by engine).
- Read ops accept an optional `root` (`draft|published`). Engine maps the root to the allowed
  sandbox directory and never passes absolute paths to the worker.
- Baseline for Review diffs uses the **stored** baseline extraction; the worker is not asked
  to re-read baseline files.
- Preview scale bounds: **0.25 to 2.0** (default 1.0). Values outside range are clamped.
- Preview size bounds: max **2048x2048** pixels or **10MB** base64 payload; if exceeded,
  responses include `scaled_down: true`.
- Xlsx grid bounds: `row_count` max **200**, `col_count` max **50** (clamped).

### Engine (Go) — package map (M1)
- `engine/internal/engine/engine.go`
  - Update `WorkshopProposeChanges` prompt to allow office formats (v2 schema).
  - Include extracted text for office/PDF files and metadata-only summaries for images/opaque files.
  - Build Workshop context from the **active view** (Draft if present) **but only for manifest files**;
    Draft-only files are excluded from prompts until they enter the Workbench file list.
  - Draft-only files are detected by comparing Draft directory contents against manifest `files[]`.
  - When no Draft exists, extract office/PDF text from **Published** via the worker with `root=published`
    (read-only). Text/code files are read directly from Published.
  - Implement proposal v2 parse/validate + backward compatibility with v1 (allow ops-only).
  - Expand proposal validation to accept text/code extensions for direct writes.
  - Reject writes targeting **opaque** file paths with `VALIDATION_FAILED`.
  - Update `WorkshopApplyProposal` to:
    - validate ops (kind, path, read-only constraints),
    - route **office ops** via the local worker,
    - keep text/code writes on the existing text-write path.
  - Apply proposals **atomically** by writing into a staged Draft copy (worker targets the staging path)
    and swapping on success:
    - Staging path: `draft.<proposal_id>.staging/`
    - Swap: rename existing `draft/` → `draft.prev/`, rename staging → `draft/`, then remove `draft.prev/`
    - On failure: leave `draft/` untouched and delete the staging dir.
  - Persist per‑file summaries from proposal v2 under `meta/review/<draft_id>/summaries/` as
    UTF-8 plain text named by `sha256(path)`.
  - On Draft creation (before mutations), generate baseline extraction for office/PDF files and store under
    `meta/review/<draft_id>/baseline/`.
  - Update `ReviewGetChangeSet` to include `file_kind`, `preview_kind`, `mime_type`, `is_opaque`.
  - Update `ReviewGetTextDiff` to use extracted text for office/PDF (Draft vs baseline) and
    to support larger text/code extension set.
  - Add paginated preview RPCs for full‑resolution review:
    - `ReviewGetPdfPreviewPage`
    - `ReviewGetDocxPreviewPage`
    - `ReviewGetPptxPreviewSlide`
    - `ReviewGetXlsxPreviewGrid`
    - `ReviewGetImagePreview` (Draft + Published)
    - Engine returns base64 bytes to keep UI from touching files directly.

- `engine/internal/workbench/manager.go`
  - Split extension policies:
    - **Importable types** (supported + opaque).
    - **Text-write types** (all text/code extensions above) for direct content writes.
  - Allow unsupported types to be added as **opaque** (metadata-only).
  - Store file metadata (size, modified, mime, kind, is_opaque) in manifest entries.
  - Add binary-safe read helpers for Review previews.
  - Update `ComputeScopeHash` to hash the **sorted file list only** (no size/mtime),
    so consent re-prompts only when the file list changes.
  - Add manifest schema migration (v1 → v2) on load:
    - `file_kind` inferred from extension (`.txt` → text, `.docx` → docx, etc.).
    - `mime_type` from `mime.TypeByExtension()` with a fallback map.
    - `is_opaque=true` when extension is not in supported lists.
  - On startup, clean up any orphaned `draft.*.staging/` and `draft.prev/` directories.

- `engine/internal/toolworker/` (new)
  - Worker process management (start/stop/restart).
  - JSON-RPC client and timeouts.
  - Health/info RPC (`WorkerGetInfo`).
  - Extract‑text helpers for office/PDF (Draft or Published, read-only).
  - Preview helpers for docx/pdf/pptx/xlsx (paginated full‑resolution).
  - Restart policy: 3 attempts with 1s/2s/4s backoff. After 3 failures, return
    `TOOL_WORKER_UNAVAILABLE` for all worker calls until engine restart.
  - In-flight requests on crash return `TOOL_WORKER_UNAVAILABLE`.

- `engine/tools/pyworker/` (new)
  - `worker.py` implementing:
    - `DocxApplyOps`
    - `XlsxApplyOps`
    - `PptxApplyOps`
    - `DocxExtractText`
    - `OdtExtractText`
    - `XlsxExtractText`
    - `PptxExtractText`
    - `PdfExtractText`
    - `ImageGetMetadata`
    - `PdfRenderPage`
    - `DocxRenderPage` (page-by-page)
    - `OdtRenderPage` (page-by-page)
    - `PptxRenderSlide`
    - `XlsxRenderGrid` (rows/cols window)
  - `requirements.txt` (minimum versions):
    - `python-docx>=0.8.11`
    - `openpyxl>=3.1.0`
    - `python-pptx>=0.6.21`
    - `odfpy>=1.4.1`
    - `pypdf>=3.0.0`
    - `PyMuPDF>=1.23.0`
    - `Pillow>=10.0.0`
    - `CairoSVG>=2.7.0` (optional, for SVG)

- `engine/internal/errinfo/errinfo.go`
  - Add consistent error detail for read-only write attempts.
  - Add a structured error for worker/renderer unavailable (e.g., `TOOL_WORKER_UNAVAILABLE`)
    so UI can surface a clear action.

- `engine/internal/diff/diff.go`
  - Keep line diff; add helper for extracted-text normalization if needed.
  - Cap diff size and return a “diff too large” flag for UI fallback.

### UI (Flutter)
- `app/lib/screens/workbench_screen.dart`
  - Allow import of office/PDF/images + opaque types.
  - Add file list badges for `read-only` formats and `opaque`.
  - Replace the draft changes card with a minimal auto-apply status indicator.

- `app/lib/screens/review_screen.dart`
  - Add **side‑by‑side** preview panes for:
    - PDF/docx/pptx (paginated full‑res pages/slides)
    - XLSX (grid window with pagination by rows/cols + sheet tabs)
    - Images (full‑res)
  - Render office/PDF diffs based on extracted text (Draft vs baseline).
  - Handle `diff too large` with a clear fallback state.
  - For **New file** changes, show Draft-only preview with a "New file" label and no diff.
  - Show read-only indicators for PDF/images and opaque types.
  - Show per‑file summaries when available; otherwise display “summary unavailable”.

- `app/lib/models/models.dart`
  - Extend `WorkbenchFile` + `ChangeItem` with `mime_type`, `file_kind`, `preview_kind`, `is_opaque`.
  - Add `ProposalOp` model and extend `Proposal` to parse `ops[]` and `no_changes` (for minimal list display).

- `app/lib/theme.dart`
  - Extend preview styles (subtle borders, warm minimal palette).

### IPC / API updates (exact names)
- `WorkshopApplyProposal` (existing)
  - Validate read-only write attempts; return `VALIDATION_FAILED` with clear detail.
  - Accept proposal v2 schema with ops.

- `ReviewGetChangeSet` (existing)
  - Add `file_kind` (`text|docx|odt|xlsx|pptx|pdf|image|binary`),
    `preview_kind` (`diff|image|grid|none`), `mime_type`, `is_opaque`,
    and optional `focus_hint` (xlsx: row/col bounds).

- `ReviewGetTextDiff` (existing)
  - Use extracted text for `docx/odt/xlsx/pptx/pdf`; use raw content for text/code; keep line diff format.
  - Return `{hunks, too_large}` when diff exceeds max size.

- **Paginated preview RPCs** (new)
  - `ReviewGetPdfPreviewPage(workbench_id, path, version, page_index, scale) -> {bytes_base64, page_count, mime_type, scaled_down}`
  - `ReviewGetDocxPreviewPage(workbench_id, path, version, page_index, scale) -> {bytes_base64, page_count, mime_type, scaled_down}`
  - `ReviewGetOdtPreviewPage(workbench_id, path, version, page_index, scale) -> {bytes_base64, page_count, mime_type, scaled_down}`
  - `ReviewGetPptxPreviewSlide(workbench_id, path, version, slide_index, scale) -> {bytes_base64, slide_count, mime_type, scaled_down}`
  - `ReviewGetXlsxPreviewGrid(workbench_id, path, version, sheet, row_start, row_count, col_start, col_count) -> {cells, sheets[], row_count, col_count}`
  - `ReviewGetImagePreview(workbench_id, path) -> {draft:{mime_type,bytes_base64,metadata}, published:{...}, has_published}`
    - For SVG, engine returns a rasterized preview (PNG) and sets `mime_type=image/png`.

---

## Review UX (M1 details)

### Change list (left rail)
- Each file row shows: filename, change badge (Added/Modified), and a small type tag (DOCX/ODT/XLSX/PPTX/PDF/IMG).
- Read-only formats show a subtle `Read-only` chip.
- Opaque files show a subtle `Opaque` chip.

### Diff/preview pane (right)
- **Office/PDF**: extracted text diff with helper copy “Formatting may differ from the original file.”
- **Docx/ODT/PDF/Pptx preview**: full‑resolution, paginated **side‑by‑side** pages/slides.
- **XLSX preview**: side‑by‑side grid window (rows/cols) with sheet tabs and pagination;
  defaults to a **focus hint** window when available and supports zoom by adjusting row/col windows.
- **Images (including SVG)**: side‑by‑side full‑resolution previews + metadata block.
- **Opaque/binary**: metadata-only panel with size + type.
- **No diff available**: message “No diff available for this file type yet.”
- **Summary panel**: show per‑file summary if available; otherwise “Summary unavailable.”
- **Image metadata** shows: format, dimensions (WxH), file size, and color depth if available.
- For SVG without defined dimensions, show “scalable” instead of WxH.

### Empty/edge states
- No changes: “No changes in Draft yet.”
- Preview error: “Preview unavailable. Try reopening the Draft.”
- Baseline missing: “Published baseline not available for this file.” (pre‑M1 drafts only)

---

## Test Plan

### Engine tests (Go)
- Worker starts, answers `WorkerGetInfo`, and restarts on crash.
- Sandbox mapping rejects traversal outside Draft.
- Write to `.pdf` or image fails with `VALIDATION_FAILED`.
- Create/edit `docx/xlsx/pptx` succeeds with Draft present.
- Read-only `odt/pdf/svg` extraction + preview succeeds; write attempts rejected.
- Proposal v2 validation (ops + writes) + v1 backward compatibility.
- Proposal v2 ops‑only validation.
- Draft baseline extraction is created on Draft creation and used for diffs.
- Manifest v1→v2 migration preserves files and opaque flags.
- Consent scope hash changes only when file list changes.
- Preview RPCs return bytes + counts for docx/pdf/pptx and grid windows for xlsx.
- Preview RPCs clamp scale and return `scaled_down` when exceeding limits.

### Flutter tests
- Review renders **side‑by‑side** paginated previews (docx/pdf/pptx/xlsx/images).
- Review shows extracted text diff for office/PDF files and handles `diff too large`.
- Opaque files show metadata-only preview.
- New file changes show Draft-only preview with “New file” label.

### E2E (Linux)
- Import office + image, run Workshop auto-apply, review, publish.
- Import unsupported file, verify it appears as opaque and doesn’t block the flow.
- Add fixtures under `engine/testdata/office/` and use them in `app/integration_test/support/`:
  - `simple.docx` (2 paragraphs, 1 heading)
  - `notes.odt` (2 paragraphs)
  - `multi-sheet.xlsx` (2 sheets, formulas, merged cells)
  - `slides.pptx` (3 slides, bullet lists)
  - `report.pdf` (2 pages, text content)
  - `chart.png` (200x200 image)
  - `logo.svg` (simple vector)
  - `unknown.bin` (opaque file test)

---

## Risks / Open Questions
- Python packaging per OS (Linux first, macOS next; Windows deferred).
- Native dependencies for office libraries (lxml) on macOS/Windows.
- Performance for large office files (chunking strategy).
- Preview rendering deps (LibreOffice headless + PyMuPDF/Poppler + CairoSVG) and CI packaging.
- Large preview payload sizes (base64) and memory usage in Flutter (mitigated with size caps).
- UX clarity for read-only formats.
- Baseline extraction for **existing** drafts created pre‑M1 (plan: show baseline-missing state).

---

## Packaging (M1)
- **Linux (M1)**: bundle a local Python environment or PyInstaller-built worker alongside the app.
- **macOS (follow-on)**: ship the same worker packaging with codesign/notarization; Windows out of scope for M1.
- **Preview rendering deps**: bundle or vendor LibreOffice headless plus a PDF rasterizer (PyMuPDF/Poppler)
  and SVG rasterizer (CairoSVG/librsvg) as part of the worker package.
- **Dev/CI**: allow overriding worker path via `KEENBENCH_TOOL_WORKER_PATH`; if preview rendering
  uses a separate binary, add `KEENBENCH_PREVIEW_RENDERER_PATH`. Provide a fake worker mode for tests.

---

## Deliverables (Single Doc Plan + Change List)
This document defines both **the M1 plan** and **the concrete change list** (by file path) to implement file operations in the Workshop loop.
