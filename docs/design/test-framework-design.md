# Design Doc: Test Framework (E2E + Desktop Screenshots)

## Status
Draft (M0-ready)

## Last Updated
2026-01-30

## Goals
- Run automated end-to-end (E2E) tests on Linux desktop.
- Capture real OS-level window screenshots for visual review (pre-golden baselines).
- Keep the harness scriptable and compatible with the M0 engine/UI boundary.

## Non-Goals (M0)
- Golden regression baselines (planned after visual review approves the UI).
- Wayland-native capture support.
- Mobile, web, or CI cross-platform coverage.

## Architecture Summary
### E2E harness
- Tests live under `app/integration_test/` and use Flutter `integration_test`.
- Default runner (serial): `scripts/e2e/run_e2e_serial.sh` (runs each test file in its own `flutter test` invocation).
- Full-suite runner: `scripts/e2e/run_e2e.sh` (serial by default; set `KEENBENCH_E2E_SINGLE=1` for a single `flutter test integration_test -d linux` invocation).
- Tests call a Dart helper to capture screenshots after each new screen.
- E2E scripts load `.env` from the repo root.

### Screenshot capture
- Script: `scripts/e2e/capture_window.sh`
- Uses X11 window lookup and OS-level capture (no Flutter in-app screenshot API).
- Window lookup priority:
  1) `_NET_WM_PID` via `xdotool search --pid` (preferred, deterministic).
  2) PID match via `wmctrl -lp` (fallback).
  3) Window class/title via `wmctrl` (opt-in env vars).
- Output path: `artifacts/screenshots/<timestamp>_<label>.png` (gitignored).

### Pause-on-failure
- Env: `KEENBENCH_E2E_PAUSE_ON_FAILURE=1`.
- On error, the test captures a `failure` screenshot, prints a resume file path, and waits.
- The test timeout is disabled when pause-on-failure is enabled.
- Resume by creating the file (default: `artifacts/screenshots/.e2e_resume`).

## E2E Screenshot Coverage (M0)
- Home screen.
- Settings screen.
- New Workbench dialog.
- Workbench screen (empty state).
- Review screen is captured only if a Draft exists (requires successful auto-generated draft changes).

## Tooling Dependencies (Linux/X11)
- `xdotool` or `wmctrl` (window lookup).
- ImageMagick `import` (window capture).
- X11 session (`DISPLAY` set). Wayland is out of scope for M0.

## Testing Policy: Real Models Only
All E2E tests that involve AI interaction MUST use real model API calls. No fake/mock AI modes are permitted. See `CLAUDE.md` for the full policy.

## Environment Variables
- `KEENBENCH_OPENAI_API_KEY` — required for all AI-driven E2E tests.
- `KEENBENCH_E2E_SCREENSHOTS=0` to disable captures.
- `KEENBENCH_E2E_SCREENSHOTS_DIR` to override the output directory.
- `KEENBENCH_E2E_CAPTURE_SCRIPT` to override the capture script path.
- `KEENBENCH_E2E_PAUSE_ON_FAILURE=1` to pause after a test failure.
- `KEENBENCH_E2E_PAUSE_FILE` to override the resume file path.
- `KEENBENCH_E2E_WINDOW_CLASS` or `KEENBENCH_E2E_WINDOW_TITLE` to force window lookup by class/title.
- `KEENBENCH_E2E_DEVICE` to override the Flutter target device (default: `linux`).
- `KEENBENCH_E2E_TESTS="path1 path2 ..."` limits serial runs to specific test files.
- `KEENBENCH_E2E_INCLUDE_SMOKE=1` includes `e2e_smoke_test.dart` in serial runs.

## Running Locally
- `scripts/e2e/run_e2e_serial.sh`
- `scripts/e2e/run_e2e.sh` (serial by default; `KEENBENCH_E2E_SINGLE=1` for single-invocation)
- Or manually: `cd app && flutter test integration_test -d linux`

## Notes
- This harness does not move or resize windows; tests should keep the app unobstructed for consistent screenshots.
- For MCP-driven inspection, reproduce failing steps after the pause or re-run with a debug launch so the tooling daemon is available.
