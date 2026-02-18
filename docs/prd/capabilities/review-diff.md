# PRD: Review & Diff

## Status
Draft

## Purpose
Provide a clear, trustworthy review experience so users can understand Draft changes before publishing or discarding.

## Scope
- In scope (v1): file-type aware review, change list (added/modified/deleted), text diffs, **inline diffs for office text documents** (best-effort), in-app side-by-side before/after previews for non-text files, delete confirmations.
- In scope (v1.5): semantic diffs for Office formats (Docx formatting, Pptx layout, Xlsx cells/formulas).
- Out of scope: reviewer cohort workflows (see multi-model.md), publish/merge mechanics (see draft-publish.md).

## User Experience
- Review screen lists changed files with type (A/M/D) and size deltas.
- Text-based files show an inline diff view.
- Non-text files (PDFs, images, slides, spreadsheets) show in-app before/after previews with a side-by-side view.
- A short change summary is shown for office-text/non-text files using fallback order: per-file summary, then Draft-level assistant summary, then `Summary unavailable.`.
- Deletions require explicit confirmation before publish.

## Review Tiers (By File Type)

### v1 Review Capabilities
| File Type | Primary Review | Fallback Review | Summary |
| --- | --- | --- | --- |
| Text (md, txt, csv, json, code) | Inline line diff | In-app before/after view | Optional |
| Office text docs (e.g., .docx, .odt) | Inline diff (best-effort; like code review) | In-app before/after preview | Required |
| Pdf | Side-by-side in-app preview | In-app before/after preview | Required |
| Pptx | Side-by-side in-app preview (slides) | In-app before/after preview | Required |
| Xlsx | Side-by-side in-app preview, zoom to changed areas when possible | In-app before/after preview | Required |
| Images | Side-by-side in-app preview | In-app before/after preview | Required |
| Other binaries | In-app preview (or metadata fallback) | In-app before/after preview | Required |

**Note on office text docs in v1**: The app attempts an inline diff suitable for code-style review (best-effort). Formatting/layout changes may still require a summary callout.

### v1.5 Enhancements
- **Docx**: Semantic diffs that surface formatting, heading, and table changes.
- **Pptx**: Slide-level change summaries with layout awareness.
- **Xlsx**: Cell-level diffs and formula change detection.

## Preview Behavior (Non-Text)
- Pdf/Pptx: show page or slide thumbnails with pagination; allow zoom and page navigation.
- Images: show fit-to-view with zoom and toggle/side-by-side.
- Xlsx: show sheet tabs and a preview grid; allow switching sheets.

## Preview Limits (v1)
- Pdf/Pptx: preview up to 20 pages/slides by default, with "load more" if needed.
- Xlsx: preview up to 3 sheets and 200 rows per sheet by default, with "load more" if needed.

## Summary Fallback (v1)
When review summary text is needed, the app resolves it in this order:
1. Per-file summary (`change.summary`)
2. Draft-level assistant summary (`draft_summary`)
3. `Summary unavailable.`

## Functional Requirements

### v1
1. Show a review list of Draft changes grouped by Added/Modified/Deleted with size deltas.
2. Render diffs for text-based files with inline context.
3. For office text documents (e.g., .docx), provide an inline diff (best-effort; like code review).
4. For non-text files (e.g., pdf/images/pptx/xlsx), provide an in-app side-by-side before/after preview.
5. For office-text and non-text files, show summary text using fallback order: per-file summary, then Draft-level assistant summary, then `Summary unavailable.`.
6. Require explicit confirmation for deletions before publish.
7. Provide a consistent in-app before/after affordance from the review screen.

### v1.5
8. Docx diffs include formatting, heading, and table structure changes.
9. Pptx diffs include slide-level layout awareness.
10. Xlsx diffs include cell-level and formula change detection.

## Failure Modes & Recovery
- Diff rendering fails: show a fallback message and allow in-app before/after preview.
- Summary unavailable across all fallback sources: show `Summary unavailable.` and still allow in-app before/after preview.
- Before/after preview fails (missing file/permission): surface error and keep review list intact.
- Deletion confirmation not captured: block publish and prompt again.

## Security & Privacy
- Review and diff operate only on Workbench files inside the Draft sandbox.
- No data is sent outside the Workbench scope during review.

## Acceptance Criteria
- Users can answer "what changed" from the review list and per-file view.
- Text files show a diff view; office-text/non-text files show before/after comparisons and summary text using the documented fallback order.
- Deleted files always require explicit confirmation before publish.
- If a diff or summary fails, the user still has a path to review via in-app before/after preview.

## Open Questions
~~What is the minimum acceptable quality bar for AI non-text summaries in v1?~~ → **Removed**: Not a PRD-level question. Users review and publish/discard; summary quality is an implementation detail.

~~Do we need size deltas for all file types or just binaries?~~ → **Resolved**: Just binaries (non-text files).
