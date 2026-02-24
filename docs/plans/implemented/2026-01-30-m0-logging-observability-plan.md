# Plan: M0 Addendum — Local Logging & Observability

## Status
Implemented (2026-01-30)

## Goal
Add local debug-mode logging and `.env` loading for the engine and UI so devs can
debug behavior without changing M0 safety guarantees.

## Scope
- Engine reads `.env` on startup (repo root or `KEENBENCH_ENV_PATH`).
- Debug flag via `KEENBENCH_DEBUG` (default false).
- Go engine logs to a file only in debug mode; log level is debug.
- Redact secrets (keys/tokens) while allowing payload logging.
- Flutter uses a Logger-based system, with info default and debug when enabled.

## Non-Goals
- Telemetry exporters or remote shipping.
- Log rotation/retention policies.
- In-app log viewer UI.

## Implementation Notes
- Engine loads `.env` without overriding existing process environment values.
- Log file path: `<data_dir>/logs/engine.log`.
- Redaction masks secrets while preserving last characters for debugging.
- JSON-RPC requests/responses and OpenAI payloads are logged in debug mode.
- Flutter logs key UI/engine actions and errors with redaction helpers.

## Acceptance Criteria
- Setting `KEENBENCH_DEBUG=1` enables engine file logging and debug-level UI logs.
- Without debug enabled, the engine does not create a log file.
- Secret values are redacted in logs (API keys, auth headers, tokens).
- `.env` is honored for local runs and test/dev workflows.
