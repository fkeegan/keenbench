# Contributing to KeenBench

Thanks for contributing. This project is a desktop app (`app/`) plus a Go engine (`engine/`) connected over JSON-RPC.

## Ground Rules

- Keep changes aligned with active implementation plans in `docs/plans/`.
- Follow UI conventions in `docs/design/style-guide.md` for Flutter UI work.
- Preserve structured engine error codes (ADR-0006 references in `docs/design/adr/`).
- Keep contributions within public repository materials and avoid referencing private/internal prompts or notes.

## Local Setup

Prerequisites:
- Go
- Flutter + Dart SDK
- Python 3 (for the tool worker packaging path)

From repo root:

```bash
make deps
make engine
make run        # Linux desktop
# or
make run-macos
```

## Testing Requirements

Run all standard checks before opening a PR:

```bash
make fmt
make test
```

### Real-Model Policy (Mandatory)

Any test that exercises AI behavior must use real model API calls.

- Do not use `KEENBENCH_FAKE_OPENAI` or any fake/mock LLM client.
- Do not add fake-vs-real branches for AI paths.
- `testOpenAI` is only acceptable for pure plumbing tests where model output content is irrelevant.
- AI-output assertions must be structural, numerical, or invariant-based; never exact prose matching.

See `CLAUDE.md` and `docs/test/test-plan.md` for the full policy.

Required env vars for AI tests:
- `KEENBENCH_OPENAI_API_KEY` (required)
- `KEENBENCH_ANTHROPIC_API_KEY` (optional, multi-provider tests)
- `KEENBENCH_GEMINI_API_KEY` (optional, multi-provider tests)

## Pull Request Expectations

- Keep PRs focused and small enough to review.
- Add or update tests for behavior changes and new invariants.
- Update docs when behavior, workflows, or commands change.
- Include a short risk/regression note in the PR description.

PR checklist:
- [ ] Code formatted (`make fmt`)
- [ ] Tests pass (`make test`)
- [ ] No secrets committed
- [ ] Docs updated (if needed)

## Commit Guidance

Conventional-style commit messages are preferred but not required.

Examples:
- `engine: enforce draft mutation guard for context ops`
- `app: add clutter warning for context share`
- `docs: clarify e2e prerequisites`

## Reporting Bugs and Requesting Features

- Use GitHub Issues.
- Include reproduction steps, expected behavior, and actual behavior.
- For engine/UI bugs, include environment details (OS, Flutter/Go versions).

## Security Issues

Do not open public issues for vulnerabilities. Follow `SECURITY.md`.
