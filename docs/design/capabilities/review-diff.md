# Design: Review / Diff

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/review-diff.md`
- `docs/prd/keenbench-prd.md` (FR3; safety + review requirements)
- Related: `docs/prd/capabilities/draft-publish.md`, `docs/prd/capabilities/failure-modes.md`, `docs/design/capabilities/checkpoints.md`, `docs/design/capabilities/failure-modes.md`

## Summary
Review/Diff is the trust layer between **Draft** and **Publish**. It answers:
- What changed? (A/M/D list)
- How did it change? (diff/preview)
- Is it safe to publish? (delete confirmations, summaries expected for office-text + non-text)

Key v1 choices:
- **No model calls during review.** Any AI-generated summaries shown in review are produced during Workshop execution (best-effort) and stored with the Draft.
- **In-app previews** for non-text files (PDF/images/XLSX/other binaries). No "open in external app" as the primary review path.
- **DOCX/PPTX review is renderer-independent by default**: structured content diff is available even when raster preview renderers are unavailable.

**Styling note**: Styled office documents (created via inline style parameters; see `docs/design/capabilities/document-styling.md`) are reviewed using the same mechanisms described here. XLSX grid previews show styled cells (fonts, colors, borders). DOCX/PPTX structured diffs and raster previews should surface styling where feasible; normalized text-style fallbacks may omit some styling detail and appear unstyled. No style-specific review controls are required in v1.

## Goals / Non-Goals
### Goals
- Make it easy to confidently review and publish or discard a Draft.
- Provide a consistent review experience across file types.
- Require explicit confirmation for deletions before publish.
- Keep review deterministic and offline: no model calls; no network.
- Make failures non-blocking where possible (fallback preview/summary paths).

### Non-Goals
- Semantic Office diffs (v1.5+).
- Merge/conflict resolution (v1 is single Draft; publish is atomic).
- Reviewer cohort workflows (out of scope for v1).

## User Experience

### Entry Points
- Draft banner actions: **Review**, **Publish**, **Discard**.
- Workshop: "Review Draft" button.

### Review Screen Layout (v1)
- Left pane: Changed files grouped by **Added / Modified / Deleted** with type icons.
  - Show **size deltas** for non-text/binary files only (per PRD).
  - Optional: quick filter “Text / Docs / Sheets / Slides / Images / Other”.
- Main pane: Per-file view (diff or preview).
- Header: Draft context (created by Workshop, timestamp) + actions:
  - **Publish** (disabled until required confirmations are satisfied)
  - **Discard Draft**

### Per-File View
- **Text/code/config**: inline line diff with context (expand/collapse hunks).
- **Office text docs (e.g., .docx, .odt)**: inline diff (best-effort; like code review) plus structured section diff for `.docx` (paragraph/runs/tables where available) and **expected** summary.
- **PDF**: side-by-side before/after page previews with zoom and pagination.
- **PPTX**: side-by-side structured slide diff is the default review path (M4 layout-lite, M5 positioned canvas), with raster slide preview as an optional visual aid when available. The diff pane compares `Reference (Draft start)` vs `Draft`; the preview pane compares `Published (current preview)` vs `Draft`.
- **XLSX**: side-by-side preview (two grids) with sheet tabs and zoom; when possible, the initial view zooms to changed areas.
- **Images**: side-by-side before/after preview with zoom.
- **Other binaries**: preview surface if supported (e.g., hex/metadata fallback) + expected summary.

### Summary Source Contract
When Review needs summary text, the UI resolves it in this order:
1. Per-file summary (`change.summary`)
2. Draft-level assistant summary (`draft_summary`)
3. `Summary unavailable.`

### Delete Confirmation UX
- Deleted files are listed with a checkbox: “I understand this file will be deleted on publish.”
- Publish remains disabled until all deleted files are confirmed.
- Confirmation state persists across app restarts (stored in Draft review metadata).

### Accessibility
- File list supports keyboard navigation; selection changes update the main pane.
- Diff hunks are reachable by keyboard; screen readers announce file name + change type.
- Preview surfaces provide keyboard-accessible zoom controls and page navigation.
- Side-by-side comparisons must not rely on color alone; labels `Reference` / `Published` / `Draft` are always visible according to the active mode.

## Architecture

### UI Responsibilities (Flutter)
- Render the change list (A/M/D) and per-file views.
- Drive preview navigation (page/slide, zoom, sheet selection).
- Render structured DOCX/PPTX reference vs draft comparisons (section/slide scoped), using focus hints for initial selection.
- Capture deletion confirmations and show publish gating state.
- Present fallback states (structured diff unavailable, preview unavailable, diff failed) with clear user actions.

### Engine Responsibilities (Go)
- Compute the Draft→Published change set (A/M/D) and classify file types.
- Provide text diffs for text/code/config, and best-effort inline diffs for office text documents.
- Provide structured DOCX/PPTX diff payloads aligned to reference semantics (draft vs Draft-start review reference snapshot).
- Provide or generate in-app preview artifacts for binary formats (and optional DOCX/PPTX visual aids when available).
- Persist and serve focus hints for xlsx/docx/pptx so Review can open changed regions first.
- Store and serve **execution-generated** file summaries for office-text and non-text changes.
- Return optional Draft-level assistant summary (`draft_summary`) in change-set payloads for fallback rendering.
- Validate publish preconditions (including deletion confirmations) before allowing `DraftPublish`.

### IPC / API Surface
API names are illustrative (protocol decided separately).

**Commands**
- `ReviewGetChangeSet(workbench_id) -> {draft_id, draft_summary?, changes[]}`
  - `changes[]`: `{path, kind=ADDED|MODIFIED|DELETED, type=text|office_text|pdf|pptx|xlsx|image|binary, bytes_before?, bytes_after?, summary?, summary_expected, focus_hint?}`
- `ReviewGetTextDiff(workbench_id, path) -> {hunks}` (for text/code)
- `ReviewGetOfficeTextDiff(workbench_id, path) -> {hunks}` (best-effort normalized extraction suitable for code-style review)
- `ReviewGetDocxContentDiff(workbench_id, path, section_index?) -> {baseline, draft, section_count, baseline_missing, reference_source, reference_warning?}` (structured section payload)
- `ReviewGetPptxContentDiff(workbench_id, path, slide_index?) -> {baseline, draft, slide_count, baseline_missing, reference_source, reference_warning?}` (structured slide payload; M4 layout-lite, M5 positioned)
- `ReviewGetSummary(workbench_id, path) -> {summary_markdown}` (stored at execution time)
- `ReviewGetPdfPreviewPage(workbench_id, path, version=PUBLISHED|DRAFT, page_index, scale) -> {image_ref, page_count}`
- `ReviewGetDocxPreviewPage(...) -> {image_ref, page_count}` (optional visual aid; renderer-dependent)
- `ReviewGetPptxPreviewSlide(...) -> {image_ref, slide_count}` (optional visual aid; may be backed by PDF-like renderer)
- `ReviewGetXlsxPreviewGrid(workbench_id, path, version, sheet_name, row_start, row_count, col_start, col_count) -> {cells, sheets[]}`
  - v1 defaults:
    - Sheet tabs: UI shows the first 3 sheets by default (based on the returned `sheets[]` ordering); “load more sheets” reveals additional tabs.
    - Grid: UI typically requests `row_start=0, row_count=200` for the initial view; “load more rows” increases the window.
- `ReviewSetDeletionConfirmations(workbench_id, paths[], confirmed=true|false) -> {}`
- `ReviewGetDeletionConfirmations(workbench_id) -> {confirmed_paths[]}`

**Events**
- `ReviewPreviewProgress(workbench_id, {path, version, phase, percent?})` (optional; for long renders)

Notes:
- `baseline` is a legacy field name kept for wire compatibility; semantically it represents the reference side of the comparison.
- `reference_source` values:
  - `draft_start_snapshot`: authoritative Draft-start reference.
  - `published_current_fallback`: fallback reference when Draft-start reference is unavailable.
  - `none`: no reference available.
- `baseline_missing=true` indicates no reference payload is available (`reference_source=none`).

## Data & Storage

### Review Metadata (per Draft)
Store review state under `meta/review/<draft_id>/`:
- `change_set.json` (cached computed A/M/D set; invalidated on Draft writes)
- `summaries/<path>.md` (execution-generated; copied from the producing job or Workshop draft generation)
- `draft_summary.md` (assistant text fallback used when per-file summary is missing)
- `focus_hints/<path>.json` (best-effort focus window/index hints for xlsx/docx/pptx)
- `deletions.json` (confirmed deletions + timestamp)
- `baseline/<path_hash>.txt` (legacy directory name; stores Draft-start review reference extraction for office/PDF text diffs)
- `previews/...` (cached preview artifacts; generated locally as needed)

Illustrative layout:
```
workbenches/<id>/
  published/...
  draft/...
  meta/
    draft.json
    review/<draft_id>/
      change_set.json
      deletions.json
      baseline/
        <path_hash>.txt
      focus_hints/
        docs/report.docx.json
        slides/deck.pptx.json
      summaries/
        docs/report.docx.md
        slides/deck.pptx.md
      previews/
        pdf/<file_fingerprint>/<version>/page_0001@1.0x.png
        pptx/<file_fingerprint>/<version>/slide_0001@1.0x.png
        images/<file_fingerprint>/<version>/full.png
```

Notes:
- `file_fingerprint` can be a cheap content hint (mtime+size) or SHA256 (lazy) to invalidate caches when files change.
- Summaries are expected to exist for office-text and non-text changes produced by AI; review does not create them.
- Focus hints are additive metadata; missing hints fall back to default section/slide selection.

## Algorithms / Logic

### Change Set Computation (Draft vs Published)
1. Walk `published/` and `draft/` (excluding `meta/`).
2. Compute A/M/D by relative path.
3. Classify file type:
   - `text`: known text/code extensions + UTF-8 sniff
   - `office_text` (e.g., `.docx`, `.odt`)
   - `pdf`, `pptx`, `xlsx`, `image`
   - otherwise `binary`
4. For binaries: compute `bytes_before/after` and show delta.
5. Determine `summary_expected`:
   - Expected for `office_text` and all non-text types.
   - Optional for plain text.

### Summary Resolution (UI)
For each selected change:
1. Use `changes[].summary` when present.
2. Else use top-level `draft_summary` when present.
3. Else render `Summary unavailable.`.

### Text Diffing (v1)
- Use a line-based diff (Myers or patience) producing hunks with context.
- Limit maximum diff size rendered in UI (e.g., cap by line count); if exceeded, show a “diff too large” fallback with search + open sections.

### Office Text Inline Diff (v1, Best-Effort)
Goal: show office text edits in a code-style diff view.

Approach (conceptual):
- Extract a normalized text representation for diffing (paragraphs/headings/table rows as stable “lines” where possible).
- Run the same hunking/line-diff algorithm used for text files.
- Where the normalized representation cannot fully represent formatting/layout, rely on the summary to call out those changes.

Notes:
- For `.docx`, extraction can be based on `document.xml` plus a normalization pass.
- For `.odt` (LibreOffice Writer), extraction uses stdlib ZIP + XML parsing on the ODF package:
  1. Open the `.odt` as a ZIP archive (`archive/zip`).
  2. Parse `content.xml` (`encoding/xml`).
  3. Walk the XML tree, extracting text from `<text:p>` (paragraphs), `<text:h>` (headings), `<text:list-item>` (list items), and `<table:table-row>` (table rows).
  4. Normalize each element to a stable "line" for diffing (e.g., heading level prefix, list indent, table row as delimited cells).
  - ODF is an ISO standard (ISO/IEC 26300) with well-documented XML namespaces; no external dependencies required.
- For other office-text formats, extraction may require a converter/extractor; exact implementation is TBD.

### DOCX/PPTX Structured Content Diff (M4/M5)
Goal: keep review usable and informative without requiring a raster preview renderer.

Approach:
- Persist focus hints at proposal-apply time for changed DOCX sections and PPTX slides.
- On review load, request structured reference+draft content for the focused section/slide.
- M4 payloads are text/layout-lite (paragraph/runs and slide shape/text blocks).
- M5 adds full positioned PPTX shape payloads for scaled canvas rendering.

Reference semantics:
- Structured diff compares Draft content to the reference snapshot captured when the Draft was created (same semantics as existing `ReviewGetTextDiff` for non-text files).
- Preview rendering is a separate visual path that may read current `published/`; it must be labeled `Published (current preview)` to avoid confusion with Draft-start reference.
- If the Draft-start reference is unavailable, Review should fall back to current `published/` comparison when available and return `reference_source=published_current_fallback` with warning text.
- If no reference is available, return `reference_source=none`, `baseline_missing=true`, and show a clear non-blocking fallback state.

### Preview Generation (In-App)
Binary preview is a core v1 requirement; implementation strategy is captured in an ADR:
- `docs/design/adr/ADR-0002-in-app-previews-for-review.md`

High-level behavior:
- Generate thumbnails lazily when the user opens a file (and cache them).
- Respect PRD limits:
  - Pdf/Pptx visual previews: preview up to 20 pages/slides by default (“load more”).
  - Xlsx: UI shows up to 3 sheet tabs and 200 rows per sheet by default (“load more”).
- Support zoom by rendering at different scales (cache per scale bucket).

XLSX “zoom to changed areas” (v1 best-effort):
- Engine computes a coarse change hint (sheet + row/col bounds) and returns it in `changes[].focus_hint` so the UI can start the side-by-side grids at a relevant window.

DOCX/PPTX focus-to-change behavior (M4+):
- Engine computes coarse focus hints (section index for docx, slide index for pptx) and returns them in `changes[].focus_hint` so the UI starts at the most relevant region.

### Publish Gating
Before `DraftPublish`, the engine validates:
- Draft exists and is consistent.
- All deleted file paths are confirmed in `deletions.json`.
- (Best-effort) expected summaries exist for changed office-text and non-text files.
  - v1 decision: if missing, publish is still allowed but review UI must clearly warn “summary unavailable” (avoid hard-bricking a Draft due to missing metadata).

## Error Handling & Recovery
- Diff fails: show “diff unavailable” and fall back to preview or metadata + resolved summary text.
- Structured DOCX/PPTX diff fails: fall back to existing text diff, then metadata + summary.
- Preview render fails: keep structured diff path active for docx/pptx and show clear fallback state; keep the change list usable.
- Cache corruption: invalidate and regenerate locally (previews/diffs).
- Deletion confirmations missing: block publish and bring user to Deleted group.

## Security & Privacy
- Review operates strictly within the Workbench sandbox (Published + Draft only).
- No network calls or external rendering services during review.
- Preview generation must not execute embedded macros/scripts (treat Office content as untrusted data).

## Telemetry (If Any)
Local-only by default:
- Diff render failures by file type.
- Preview generation timings and failure rates.
- Time-to-publish from opening review.

## Open Questions
- What minimum positioned-shape fidelity is acceptable for M5 PPTX canvas rendering (group transforms, rotation, SmartArt/chart placeholders)?

## Self-Review (Design Sanity)
- Honors the “no model calls during review” constraint by requiring summaries to be produced at execution time and stored with the Draft.
- Treats in-app preview as a first-class requirement and isolates the one-way decision behind an ADR.
- Keeps publish safety straightforward: delete confirmations are hard gates; missing summaries are soft failures to avoid bricking Drafts.
