---
name: keenbench-e2e
description: Create, maintain, and run automated KeenBench integration/E2E regression tests on Linux using flutter integration_test and scripts/e2e, with real-model policy and screenshot artifacts.
---

# KeenBench E2E Automation

Use this skill for regression automation in `app/integration_test/`.
Do not use this skill for AI-assisted manual/system/exploratory runs; use `$keenbench-ai-manual` for that workflow.

## Scope
- Author and refactor integration tests under `app/integration_test/`.
- Execute tests via `scripts/e2e/run_e2e_serial.sh` (recommended) or `scripts/e2e/run_e2e.sh`.
- Reuse existing E2E helpers and screenshot harness utilities.
- Debug failures using saved screenshot artifacts and pause-on-failure mode.

## Real-model policy
- All AI-interaction tests must use real model API calls.
- Never enable `KEENBENCH_FAKE_OPENAI` and do not add fake/mock AI branches.
- Keep AI assertions structural/numerical, never exact prose matching.
- Ensure `.env` has valid keys (`KEENBENCH_OPENAI_API_KEY`; optionally Anthropic/Gemini keys for multi-provider cases).

## Standard workflow
1. Read target cases in `docs/test/test-plan.md` and map them to automation coverage.
2. Implement or update tests in `app/integration_test/` using:
   - `app/integration_test/support/e2e_utils.dart`
   - `app/integration_test/support/e2e_screenshots.dart`
   - `app/lib/app_keys.dart`
3. Run serial harness:
   - `scripts/e2e/run_e2e_serial.sh`
4. Run subsets when needed:
   - `KEENBENCH_E2E_TESTS="app/integration_test/my_test.dart" scripts/e2e/run_e2e_serial.sh`
5. Investigate failures from `artifacts/screenshots/`; pause if live inspection is needed:
   - `KEENBENCH_E2E_PAUSE_ON_FAILURE=1 scripts/e2e/run_e2e_serial.sh`
6. Report pass/fail by test case ID and include artifact paths.

## Test authoring guardrails
- Prefer key-based and structural assertions over brittle timing-only checks.
- Keep tests independent and isolate state with per-run/per-test `KEENBENCH_DATA_DIR`.
- Preserve Linux desktop assumptions used by the current harness.
- Keep screenshot capture points meaningful for triage and regression diffs.

## Useful environment variables
- `KEENBENCH_OPENAI_API_KEY` — required, real OpenAI key for all AI-driven tests.
- `KEENBENCH_E2E_SCREENSHOTS=0` disables captures.
- `KEENBENCH_E2E_SCREENSHOTS_DIR` overrides the output directory.
- `KEENBENCH_E2E_CAPTURE_SCRIPT` overrides the capture script path.
- `KEENBENCH_E2E_PAUSE_ON_FAILURE=1` enables pause on failure.
- `KEENBENCH_E2E_PAUSE_FILE` overrides the resume file path.
- `KEENBENCH_E2E_WINDOW_CLASS` or `KEENBENCH_E2E_WINDOW_TITLE` forces window lookup by class/title.
- `KEENBENCH_E2E_DEVICE` overrides the Flutter target device.
- `KEENBENCH_E2E_TESTS="path1 path2 ..."` limits serial runs to specific test files.
- `KEENBENCH_E2E_INCLUDE_SMOKE=1` includes `e2e_smoke_test.dart` in serial runs.

## Linux dependencies
- `xdotool` or `wmctrl`
- ImageMagick `import`
- X11 session (`DISPLAY` set)
