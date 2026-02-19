# CLAUDE.md — KeenBench

## MANDATORY TESTING POLICY — REAL MODELS ONLY

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**

- Do NOT use `KEENBENCH_FAKE_OPENAI=1` or the `fakeOpenAI` client for any test.
- Do NOT use `testOpenAI` or any mock/stub for tests that exercise the AI path (workshop chat, proposals, drafts, review of AI-generated content).
- Do NOT write `if (useFakeOpenAI)` branches in tests. There is one code path: real.
- Do NOT create new mock LLM clients for testing AI-driven features.
- `testOpenAI` in Go unit tests is ONLY acceptable for testing engine plumbing that does not depend on AI behavior (e.g., error propagation, JSON-RPC routing, sandbox enforcement where the AI response content is irrelevant to what's being tested).

**Why:** This tool's purpose is AI-assisted file operations. If a test doesn't prove the feature works with a real model, it proves nothing. A passing fake-AI test gives false confidence — the test validates the fake, not the product.

**Test assertions for AI-driven tests must be:**
- Structural (file was created, is valid, has expected columns/sections)
- Numerical (sums match known values from source data)
- Invariant-based (draft exists, sandbox respected, consent flowed)
- Never exact-text-match on AI-generated prose

**Required environment for AI tests:**
- `KEENBENCH_OPENAI_API_KEY` must be set to a valid key in `.env`
- For multi-provider tests: `KEENBENCH_ANTHROPIC_API_KEY`, `KEENBENCH_GEMINI_API_KEY`, `KEENBENCH_MISTRAL_API_KEY`
- Tests must have appropriate timeouts (60-120s for single model calls)

## Architecture

- **Flutter app** in `app/` (desktop only).
- **Go engine** in `engine/`, JSON-RPC 2.0 over stdio (line-delimited).
- **Python tool worker** in `engine/tools/pyworker/` for office file operations.
- Workbench/Draft layout is engine-owned; UI never touches files directly.

## Key paths

- UI entry: `app/lib/main.dart`
- Engine RPC: `engine/internal/engine/engine.go`
- Workbench storage: `engine/internal/workbench/manager.go`
- Tool worker: `engine/tools/pyworker/worker.py`
- E2E tests: `app/integration_test/`
- Test plan: `docs/test/test-plan.md`
- Implementation plans: `docs/plans/`

## Commands

- Run app: `make run`
- Build engine: `make engine`
- Format: `make fmt`
- Tests: `make test`
- E2E tests (serial): `scripts/e2e/run_e2e_serial.sh`
- E2E tests (single invocation): `scripts/e2e/run_e2e.sh`

## When writing tests

1. Read `docs/test/test-plan.md` for the test case catalog and format.
2. Every test case that sends a prompt to the model MUST use a real API key.
3. Assertions must be deterministic even though model output is not. Assert structure, not prose.
4. Use real-world test data from `engine/testdata/` and `app/integration_test/support/`.
5. Do not add `KEENBENCH_FAKE_OPENAI` checks or branches anywhere.

## When editing code

- Keep proposal/draft operations sandbox-safe.
- Preserve structured error codes from ADR-0006.
- Update or add tests for new invariants.
- Do not read or reference `docs/prompt.md`.
