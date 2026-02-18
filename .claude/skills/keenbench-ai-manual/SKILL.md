---
name: keenbench-ai-manual
description: Run AI-assisted manual (black-box/gray-box) KeenBench system tests from markdown plans by driving the app with Flutter MCP Driver, validating with widget tree plus screenshots, and tailing engine logs.
argument-hint: <test-plan-section-file>
---

# KeenBench System Driver

Use this skill for AI-assisted manual/system/exploratory testing from test plans like `docs/test/test-plan.md`.
This is not regression automation. For scripted integration regression runs, use `$keenbench-e2e`.

The argument is the path to a test plan section file (e.g. `docs/test/sections/10-bank-statement-analysis.md`).
Read it first to extract the TC-### cases, preconditions, and expected outcomes.

## Test mode
- Execute selected `TC-###` cases interactively from markdown plans.
- Drive UI with Flutter MCP Driver commands.
- Validate each case with both:
  - widget evidence (`get_widget_tree` and/or `get_diagnostics_tree`)
  - visual evidence (`flutter_driver` `screenshot`)
- Tail `make run` logs to capture engine behavior during steps.

## Launch flow (required for Driver + DTD)
1. Use `make run` so engine build/tool-worker/deps stay consistent with repo workflow.
2. Override Flutter launch so run output includes DTD URI and driver extension is enabled.
3. Use a temporary `KEENBENCH_DATA_DIR` when the test case requires fresh state.
4. Record the data dir and log path up front: `$KEENBENCH_DATA_DIR/logs/engine.log`.

Example:

```bash
cat >/tmp/flutter-with-dtd-driver.sh <<'SH'
#!/usr/bin/env bash
FLUTTER_REAL="${FLUTTER_REAL:-flutter}"
if [ "$1" = "run" ]; then
  shift
  exec "$FLUTTER_REAL" --print-dtd run -t lib/driver_main.dart "$@"
fi
exec "$FLUTTER_REAL" --print-dtd "$@"
SH
chmod +x /tmp/flutter-with-dtd-driver.sh

FLUTTER_REAL=/home/fkeegan/tools/flutter/bin/flutter \
KEENBENCH_DATA_DIR=/tmp/keenbench-system-$(date +%s) \
make run FLUTTER_BIN=/tmp/flutter-with-dtd-driver.sh
```

Then connect MCP to the printed DTD URI (`ws://...`) with `connect_dart_tooling_daemon`.

## Execution workflow
1. Parse requested test cases and extract exact preconditions and expected outcomes.
2. Run steps with `flutter_driver` (`tap`, `get_text`, `scroll`, `get_diagnostics_tree`, `screenshot`).
3. Before tapping Send in Workshop, assert composer content with `get_text` on `AppKeys.workbenchComposerField` so later failures are not misattributed.
4. Use named screenshots only: pass `screenshotName` on every `flutter_driver screenshot` call.
5. For toggle assertions, use `get_diagnostics_tree` with:
   - `diagnosticsType: widget`
   - `includeProperties: "true"`
   - then verify the `value` property (`on` / `off`).
6. After key checkpoints, capture:
   - Driver response evidence
   - Widget-tree/diagnostics evidence
   - Screenshot evidence
7. Prefer `rg` queries against `$KEENBENCH_DATA_DIR/logs/engine.log` for RPC/error evidence; use `make run` PTY polling only as a live fallback.
8. Report step-by-step pass/fail with caveats where test-plan text and implementation differ.

## Useful MCP calls
- Driver actions: `tap`, `get_text`, `enter_text`, `scroll`, `screenshot`
- Widget inspection: `get_widget_tree`, `get_diagnostics_tree`
- Runtime debugging: `get_runtime_errors`

## Log collection pattern
- Primary: `rg -n "<pattern>" "$KEENBENCH_DATA_DIR/logs/engine.log"`.
- Typical patterns: `ProvidersSetEnabled|EgressGetConsentStatus|EgressGrantWorkshopConsent|WorkshopSendUserMessage|WorkshopRunAgent|PROVIDER_|EGRESS_`.
- Only use large PTY dumps when line-level log queries are insufficient.

## Secret handling
- Load API keys from `.env` without echoing them to output.
- Never paste full keys in commentary/final reports.
- If key validation evidence is needed, rely on masked values from logs (`****xxxx`) or status transitions (`Not configured` -> `Configured`).

## Navigation fallback
- Back navigation can be flaky by semantics label. Use this fallback sequence:
  1. `tap` by `BySemanticsLabel` (`Back`) when available.
  2. If that times out, `tap` the first app-bar `IconButton`.
  3. Immediately assert destination screen with `waitFor` on the expected key.

## Known pitfalls and fixes
- If driver says extension is not enabled, run target must be `lib/driver_main.dart`.
- If DTD URI is missing, Flutter must be launched with `--print-dtd`.
- If `waitFor`/`timeout` behaves inconsistently in MCP, use direct `get_text`/`get_diagnostics_tree` checks with explicit retries.
- `screenshot` without `screenshotName` may fail in some MCP server builds; always provide `screenshotName`.
- `get_diagnostics_tree` does not accept `diagnosticsType: properties`; use `diagnosticsType: widget` or `renderObject`.
- Keep `make run` session alive while collecting logs and evidence.

## Policy notes
- For cases involving AI calls, enforce real-model policy:
  - never use `KEENBENCH_FAKE_OPENAI`
  - use structural/numerical assertions for AI output
  - ensure valid API keys in `.env`

## Teardown
- Quit the `make run` process cleanly (`q`) after collecting results.
