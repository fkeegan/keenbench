# AGENTS.md — KeenBench Repo Guide

Use this file when working in this repository. Keep changes aligned with the implementation plans in `docs/plans/` and the UI style guide in `docs/design/style-guide.md`.

---

## MANDATORY TESTING POLICY — REAL MODELS ONLY

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**

- Do NOT use `KEENBENCH_FAKE_OPENAI` or any fake/mock AI client in tests.
- Do NOT write conditional branches based on fake vs real AI mode.
- Do NOT create mock LLM clients for tests that exercise AI-driven features.
- `testOpenAI` in Go unit tests is ONLY acceptable for testing pure engine plumbing (error propagation, JSON-RPC routing, sandbox enforcement) where AI response content is irrelevant.
- Assertions on AI output must be structural/numerical, never exact-text-match on prose.
- Required: valid API keys in `.env` (`KEENBENCH_OPENAI_API_KEY`, and optionally `KEENBENCH_ANTHROPIC_API_KEY`, `KEENBENCH_GEMINI_API_KEY`).

See also: `CLAUDE.md` for the full policy and `docs/test/test-plan.md` for the test case catalog.

---

## Architecture
- **Flutter app** in `app/` (desktop only).
- **Go engine** in `engine/`, JSON‑RPC 2.0 over stdio (line‑delimited).
- **Python tool worker** in `engine/tools/pyworker/` for office file operations.
- Workbench/Draft layout is engine‑owned; UI never touches files directly.

## Key paths
- UI entry: `app/lib/main.dart`
- Theme: `app/lib/theme.dart`
- Workbench UI: `app/lib/screens/workbench_screen.dart`
- Review UI: `app/lib/screens/review_screen.dart`
- Engine RPC: `engine/internal/engine/engine.go`
- Workbench storage: `engine/internal/workbench/manager.go`
- Tool worker: `engine/tools/pyworker/worker.py`
- Error taxonomy: `engine/internal/errinfo/errinfo.go`
- E2E tests: `app/integration_test/`
- Test plan: `docs/test/test-plan.md`
- E2E harness scripts: `scripts/e2e/`

## Style guide (Flutter)
- Follow `docs/design/style-guide.md` (warm minimal palette, 4px spacing, Inter + JetBrains Mono).
- Keep UI desktop‑first (no mobile layouts).
- Prefer subtle borders/shadows; avoid saturated colors and heavy gradients.

## Commands
- Run app: `make run`
- Build engine: `make engine`
- Format: `make fmt`
- Tests: `make test`
- E2E tests (Linux, serial): `scripts/e2e/run_e2e_serial.sh`
- E2E tests (suite wrapper; serial by default, single invocation with `KEENBENCH_E2E_SINGLE=1`): `scripts/e2e/run_e2e.sh`

### Go tests + coverage
- Run: `cd engine && go test ./... -coverprofile=coverage.out`
- Coverage target: **≥65%** total.

### Flutter tests
- Run: `cd app && flutter test`

### E2E tests (Linux/X11)
- Requires ImageMagick `import` and `xdotool` or `wmctrl`.
- Screenshots save to `artifacts/screenshots/` (gitignored).
- Pause on failure: `KEENBENCH_E2E_PAUSE_ON_FAILURE=1` (resume with `touch artifacts/screenshots/.e2e_resume`).
- E2E scripts load `.env`. A valid `KEENBENCH_OPENAI_API_KEY` is required.

### System testing hierarchy
- E2E scripts in `app/integration_test/` and `scripts/e2e/` are scripted regression checks; they are not the primary source of system-level validation.
- Primary system testing (AI-assisted black-box/gray-box cases) should be run from `docs/test/test-plan.md` using the `$keenbench-ai-manual` skill (`.codex/skills/keenbench-ai-manual/SKILL.md`).
- For any case that involves model calls, follow the real-model policy above (no fake AI mode).

## Runtime notes
- The app spawns the engine automatically; override with `KEENBENCH_ENGINE_PATH`.
- Engine data directory is resolved via `engine/internal/appdirs` (uses `KEENBENCH_DATA_DIR` when set).

## Safety/consent expectations
- Model calls require explicit egress consent per Workbench.
- AI writes go to Draft only, never directly to Published.
- Review/diff must be offline and never trigger model calls.

## When editing
- Keep proposal/draft operations sandbox‑safe.
- Preserve structured error codes from ADR‑0006.
- Update or add tests for new invariants.
- Do not read or reference `docs/prompt.md`; treat it as off-limits for any guidance or source material.
