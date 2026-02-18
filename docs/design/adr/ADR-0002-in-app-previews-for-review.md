# ADR-0002: In-App Previews for Review (Engine-Generated Artifacts)

## Status
Accepted

## Context
KeenBench v1 requires users to review Draft changes **in-app** before publishing:
- Text files use diffs.
- PDFs/images/PPTX/XLSX/other binaries require before/after previews (toggle or side-by-side) plus best-effort summaries (review must handle missing summaries gracefully).

Constraints:
- Cross-platform desktop (macOS/Windows/Linux) with a Flutter UI + Go engine.
- Workbenches are local-first; review must not rely on network services.
- Review must be safe against untrusted document content (no macro/script execution).
- v1 optimizes for correctness and predictable UX over perfect semantic diffs.

This ADR decides how the app produces previews that are rendered inside the UI.

## Decision
Use an **engine-generated preview artifact pipeline** with on-disk caching:

1. The Go engine is the single component responsible for preview generation and cache invalidation.
2. The engine produces preview artifacts (PNGs and/or structured grids) under the Workbench `meta/` tree, keyed by a file fingerprint and version:
   - `version = published | draft`
3. The Flutter UI renders preview artifacts directly (no external app opens required for the primary path).

Preview strategies (v1):
- **Images**: UI renders original image bytes; engine may optionally generate downsampled thumbnails for performance.
- **PDF**: engine renders pages to PNG at requested scale (thumbnails + zoom), with caching and pagination.
- **PPTX**: engine converts PPTX → PDF (headless conversion) and then uses the PDF renderer to generate slide/page images.
- **XLSX**: engine parses workbook to a **grid preview** (cells) for a bounded window (default: UI shows up to 3 sheet tabs and 200 rows per sheet, with “load more”) plus sheet tabs; optional PDF conversion can be added later for higher-fidelity “print” preview.
- **Other binaries**: engine provides a best-effort preview (when possible) or a metadata-only fallback; summaries remain the trust backstop.

Implementation note (not locked by this ADR):
- The PDF render backend should be a sandboxed library (e.g., PDFium/MuPDF) embedded with the engine.
- The PPTX→PDF converter may be implemented via a bundled headless office converter (e.g., LibreOffice) or a future pure-library replacement.

## Consequences
### Positive
- Predictable, offline, cross-platform review UX with a single implementation surface.
- UI stays relatively simple (render images/grids); heavy parsing stays in the engine.
- Caching makes repeated review fast and supports “load more” pagination.
- Safer posture: no embedded scripts/macro execution; conversion/rendering is controlled.

### Negative
- Bundling or implementing Office conversion can increase app size and maintenance cost.
- Rendering large documents can be CPU/memory intensive; requires throttling and cache limits.
- Preview fidelity may vary by conversion backend; semantic diffs still deferred to v1.5+.

## Alternatives Considered
- **OS-native preview frameworks embedded in app**
  - Pros: high fidelity on some platforms (e.g., macOS Quick Look).
  - Cons: inconsistent availability/behavior across platforms; may depend on installed Office; hard to make uniformly reliable.

- **UI-side WebView rendering (PDF.js + Office-to-HTML JS libs)**
  - Pros: avoids bundling heavy native converters; rich zoom/pagination in one surface.
  - Cons: desktop WebView availability/runtime bundling varies; security surface expands; more complex IPC/file serving story.

- **External app “open before/after” as primary**
  - Pros: simplest to implement.
  - Cons: conflicts with the v1 requirement for in-app review and undermines a cohesive trust UX.

## References
- PRD links:
  - `docs/prd/capabilities/review-diff.md`
  - `docs/prd/keenbench-prd.md` (FR3; review)
- Design links:
  - `docs/design/capabilities/review-diff.md`
  - `docs/design/design.md` (open decision: preview strategy)
