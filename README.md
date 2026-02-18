# KeenBench

[![CI](https://github.com/KeenBench/keenbench/actions/workflows/ci.yml/badge.svg)](https://github.com/KeenBench/keenbench/actions/workflows/ci.yml)

KeenBench is a desktop app for safe, reviewable AI-assisted file analysis and editing.

It combines:
- A Flutter desktop UI.
- A Go engine over JSON-RPC (stdio).
- A local Python tool worker for office and document operations.

The core loop is:
**Workbench -> Workshop -> Draft -> Review -> Publish/Discard**, with explicit egress consent and local review gates.

## Current status (2026-02-17 snapshot)

Implemented plan milestones include:
- `docs/plans/m0-implementation-plan.md`
- `docs/plans/m2-implementation-plan.md`
- `docs/plans/m2.1-review-auto-summary-checkpoints.md`
- `docs/plans/m2.2-workbench-extract-and-ui-polish.md`
- `docs/plans/m3-implementation-plan.md`
- `docs/plans/workbench-context-implementation-plan.md`
- `docs/plans/document-styling-v1-workshop-implementation-plan.md`
- `docs/plans/fixes/table-update-from-export-implementation-plan.md`
- `docs/plans/open-source-readiness-checklist.md`

In progress:
- `docs/plans/m4-implementation-plan.md`

Planned:
- `docs/plans/m5-implementation-plan.md`

## What the app supports today

- Workbench lifecycle: create/open/delete, add/remove files, extract published files.
- Provider/model stack:
  - `openai` (API key)
  - `openai-codex` (OAuth connect flow)
  - `anthropic` (API key)
  - `google` (API key)
- RPI Workshop agent workflow (Research -> Plan -> Implement -> Summary) with tool progress events.
- Draft safety model: model output writes to Draft only, then explicit Publish/Discard.
- Checkpoints: create/list/restore, plus publish/restore timeline events.
- Workbench Context (4 categories): `company-context`, `department-context`, `situation`, `document-style`.
- Clutter bar/context pressure signal.
- Office and tabular tools:
  - Text/code writes for supported extensions.
  - CSV mapping/query/export.
  - In-place XLSX updates via `table_update_from_export`.
  - DOCX/XLSX/PPTX operations (including styling-related fields and style/asset tools).
- Review:
  - Text diffs.
  - Side-by-side previews for PDF/DOCX/ODT/PPTX/XLSX/images.
  - Structured DOCX/PPTX review with fallback handling.
  - Opaque file change tracking.

## Repository layout

```
/
  app/      # Flutter desktop UI
  engine/   # Go JSON-RPC engine + workbench storage + tool contracts
  docs/     # Product/design docs, plans, test plan
  scripts/  # Packaging and E2E helpers
```

## Prerequisites

- Go 1.22+ (engine)
- Flutter desktop + Dart SDK (app)
- Python 3 with `venv` support (tool worker packaging)

## Quick start

1. Create `.env` from `.env.example` and set at minimum:
   - `KEENBENCH_OPENAI_API_KEY=...`
2. Run the app:

Linux:
```
make run
```

macOS:
```
make run-macos
```

`make run`/`make run-macos` will fetch dependencies, build the engine, set up the Python worker wrapper, and launch Flutter.

## Common commands

```
make run                       # Linux dev run
make run-macos                 # macOS dev run
make engine                    # Build Go engine
make package-worker            # Build Python worker wrapper + venv
make check-worker              # Worker health check
make fmt                       # gofmt + dart format
make test                      # Go tests + Flutter tests
scripts/e2e/run_e2e_serial.sh # Linux E2E (serial)
scripts/e2e/run_e2e.sh         # Linux E2E wrapper
```

## Testing policy (real models only)

- Any test that exercises AI behavior must call real models.
- Do not use fake/mock AI paths for AI feature tests.
- Required key for AI tests: `KEENBENCH_OPENAI_API_KEY`.
- Optional for multi-provider tests: `KEENBENCH_ANTHROPIC_API_KEY`, `KEENBENCH_GEMINI_API_KEY`.
- Go coverage target is `>=65%` total (`engine`).

See:
- `CLAUDE.md`
- `docs/test/test-plan.md`

## E2E notes (Linux/X11)

- Requires ImageMagick `import` and `xdotool` or `wmctrl`.
- Scripts load `.env` from repo root.
- Screenshots are written to `artifacts/screenshots/` (gitignored).
- `KEENBENCH_FAKE_OPENAI=1` is rejected by E2E scripts.

## Runtime configuration

- `KEENBENCH_OPENAI_API_KEY`, `KEENBENCH_ANTHROPIC_API_KEY`, `KEENBENCH_GEMINI_API_KEY`
- `KEENBENCH_ENGINE_PATH` (override engine binary path)
- `KEENBENCH_TOOL_WORKER_PATH` (override worker wrapper path)
- `KEENBENCH_DATA_DIR` (override app data root)
- `KEENBENCH_ENV_PATH` (override `.env` location)
- `KEENBENCH_DEBUG=1` (enable debug logging)

## Safety model

- Egress consent is explicit per Workbench and provider/model scope.
- Network egress is allowlisted at provider endpoints.
- AI operations write to Draft only; publish/discard is explicit.
- Review/diff flows are offline and do not trigger model calls.
- Context mutations and file extraction are blocked while a Draft exists.

## Packaging (macOS)

```
make package-macos
make package-macos-universal
make notarize-macos
make notarize-macos-universal
```

See `Makefile` and `scripts/notarize_macos.sh` for signing/notarization env vars.

## Documentation links

- Contribution guide: `CONTRIBUTING.md`
- Governance: `GOVERNANCE.md`
- Support: `SUPPORT.md`
- Security: `SECURITY.md`
- Code of Conduct: `CODE_OF_CONDUCT.md`
- Release process: `RELEASING.md`
- Design/style: `docs/design/style-guide.md`
- Milestone plans: `docs/plans/`
- Test plan: `docs/test/test-plan.md`

## Open-source policy

- Code in this repository is licensed under MIT (`LICENSE`).
- KeenBench name and logos are trademarks of the project maintainers and are not granted by the MIT license.
- Enterprise-only features may be developed in a separate proprietary repository/distribution.
