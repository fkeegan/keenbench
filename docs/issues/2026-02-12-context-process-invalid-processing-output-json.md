# Context Processing Output JSON Validation Failure (2026-02-12)

Source runs:
- Reproduction run (post-crash-fix verification): `make run FLUTTER_BIN=<flutter-dtd-driver-wrapper>` (local, 2026-02-12 22:41-22:45)
- Engine log: `~/.config/keenbench/logs/engine.log`
- Test section: `docs/test/sections/13-workbench-context.md`
- Mode: manual system run via `$keenbench-ai-manual`

## Issue 1: `ContextProcess` intermittently fails with `invalid processing output json`

- Status: Open
- Severity: High (blocks context reprocess flow from succeeding)
- Area: Workbench Context (`company-context`, text mode, real model)
- Expected: Reprocess returns valid artifact JSON and updates context item.
- Actual: Engine returns `VALIDATION_FAILED` with detail `invalid processing output json`; UI shows `Processing failed` dialog with Retry/Cancel.

Evidence:
- Engine request:
  - `~/.config/keenbench/logs/engine.log:13232`
  - Method: `ContextProcess`
  - Category: `company-context`
  - Input sentinel: `COMPANY_TAG=RCA-VERIFIED`
- Engine error response:
  - `~/.config/keenbench/logs/engine.log:13233`
  - `error_code: VALIDATION_FAILED`
  - `detail: invalid processing output json`
- UI evidence:
  - MCP screenshot capture name: `rca_fix_no_crash_processing_error_dialog`
  - Dialog text: `EngineError(invalid processing output json, VALIDATION_FAILED)`

Notes:
- This is separate from the previously observed Flutter dialog lifecycle crash (`TextEditingController used after dispose` / `_dependents.isEmpty`), which was fixed in `app/lib/screens/workbench_context_screen.dart`.
- In this failure mode, the UI stays stable (no runtime Flutter exception); only engine-side output validation fails.
