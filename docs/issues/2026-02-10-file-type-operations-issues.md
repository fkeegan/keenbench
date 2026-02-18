# File-Type Operations Issues (2026-02-10)

Source runs:
- Initial run: `<tmp-run-dir>`
- Initial engine log: `<tmp-run-dir>`
- Follow-up verification run: `<tmp-run-dir>`
- Follow-up engine log: `<tmp-run-dir>`
- Previous latest verification run: `<tmp-run-dir>`
- Previous latest engine log: `<tmp-run-dir>`
- Most recent verification run: `<tmp-run-dir>`
- Most recent engine log: `<tmp-run-dir>`
- Post-fix verification rerun (TC-150): `<tmp-run-dir>` (2026-02-11 01:32-01:34 local)
- Post-fix verification log segment: `<tmp-run-dir>` (lines 1716+)
- Latest screenshots: `<repo>/artifacts/manual-fix-screenshots/`
- Test section: `docs/test/sections/12-file-type-operations.md`
- Mode: manual system run via `$keenbench-ai-manual`

## Issue 1: Review baseline missing for modified office files

- Status: Fixed in most recent verification run (2026-02-11)
- Severity: High (historical)
- Expected: For modified files, Review should load a published baseline for side-by-side comparison.
- Actual (most recent run): `ReviewGetTextDiff` returns `baseline_missing:false` for modified DOCX/PPTX/XLSX files.
- Evidence:
- XLSX (TC-130): `<tmp-run-dir>` (`cuentas_octubre_2024_anonymized_draft.xlsx`, `baseline_missing:false`)
- XLSX (TC-131): `<tmp-run-dir>` (`quarterly_data.xlsx`, `baseline_missing:false`)
- DOCX (TC-140): `<tmp-run-dir>` (`invoice_template.docx`, `baseline_missing:false`)
- PPTX (TC-150): `<tmp-run-dir>` (`slides.pptx`, `baseline_missing:false`)
- UI screenshots:
  - `<repo>/artifacts/manual-fix-screenshots/tc131_draft_review_2026-02-11.png`
  - `<repo>/artifacts/manual-fix-screenshots/tc140_draft_review_2026-02-11.png`
  - `<repo>/artifacts/manual-fix-screenshots/tc150_draft_review_2026-02-11.png`

## Issue 2: Review preview defaults to first slide/sheet instead of changed target

- Status: Fixed in post-fix verification rerun (2026-02-11)
- Severity: Medium
- Expected: For TC-specific office edits, Review should default to the changed entity (e.g., new slide, changed sheet).
- Actual:
  - TC-131 now opens the changed `Annual` sheet by default (fixed).
  - TC-150 now opens at `Slides 4/4` by default (newly added final slide).
- Evidence:
- TC-131 fixed:
  - `<tmp-run-dir>` (`ReviewGetChangeSet`, `focus_hint.sheet:"Annual"`)
  - `<tmp-run-dir>` (`ReviewGetXlsxPreviewGrid`, `sheet:"Annual"`, draft)
- TC-150 fixed (post-fix rerun):
  - `<tmp-run-dir>` (`PptxApplyOps`, `ops_kinds:["add_slide"]`)
  - `<tmp-run-dir>` and `:1732` (`PptxGetMap`, `root:"draft"` after apply)
  - `<tmp-run-dir>` (`ReviewGetChangeSet`, `focus_hint.slide_index:3`)
  - `<tmp-run-dir>` (`ReviewGetPptxPreviewSlide`, `version:"draft"`, `slide_index:3`)
  - Screenshot: `<repo>/artifacts/manual-fix-screenshots/tc150_fix_focus_default_2026-02-11.png`
  - Note: published preview for `slide_index:3` is absent (expected for newly added draft-only slide); UI now soft-falls back with `No published preview`.

## Notes

- Previously listed issues (TC-120 merge output, DOCX placeholder fill, PPTX write target/content, TC-131 totals, preview worker unavailability, office baseline missing, and review focus default targeting) are now validated as fixed.
