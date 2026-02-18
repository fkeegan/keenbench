# Security Scan Critical Findings (2026-02-16)

Source runs:
- Static code scan: `rg -n "workbenchRoot|WorkbenchDelete|ContextProcess|ensureConsent|provider\.chat|provider\.stream_chat" engine/internal`
- File review: `engine/internal/workbench/manager.go`, `engine/internal/engine/engine.go`, `engine/internal/engine/context.go`
- Mode: local security code review for open-source readiness
- Branch: `chore/open-source-readiness-checklist`

## Issue 1: `workbench_id` path traversal risk in delete flow

- Status: Open
- Severity: High (potential destructive delete outside intended workbench directory)
- Area: Workbench ID validation / workbench lifecycle
- Expected: `workbench_id` should be constrained to a strict opaque token format and reject special path tokens such as `.` and `..`.
- Actual: Validation only rejects empty IDs and path separators. Special tokens (`.` / `..`) are not rejected, and delete uses `os.RemoveAll` on `filepath.Join(baseDir, id)`.

Evidence:
- Weak ID validation in path resolver:
  - `engine/internal/workbench/manager.go:968`
  - `engine/internal/workbench/manager.go:969`
- Destructive delete on resolved root:
  - `engine/internal/workbench/manager.go:334`
- RPC entrypoint only checks non-empty ID:
  - `engine/internal/engine/engine.go:498`
  - `engine/internal/engine/engine.go:501`

Impact:
- A crafted JSON-RPC request with `workbench_id:"."` can target the workbenches base directory.
- A crafted JSON-RPC request with `workbench_id:".."` can target the parent directory of the workbenches base path.
- Depending on filesystem permissions, this can delete data outside the intended sandbox.

Notes:
- The engine is stdio-based (not an exposed network service), but this is still relevant for threat models that include compromised local callers, plugin abuse, or malformed client traffic.

## Issue 2: `ContextProcess` bypasses egress consent gate

- Status: Open
- Severity: High (policy mismatch for model egress and consent)
- Area: Workbench Context / egress gating
- Expected: Context model calls should enforce the same scoped consent requirements as Workshop model calls.
- Actual: `ContextProcess` resolves provider/model and makes model calls without invoking `ensureConsent`.

Evidence:
- Context processing path reaches model call without consent gate:
  - `engine/internal/engine/context.go:255`
  - `engine/internal/engine/context.go:279`
  - `engine/internal/engine/context.go:343`
- Workshop paths explicitly enforce consent before model calls:
  - `engine/internal/engine/engine.go:760`
  - `engine/internal/engine/engine.go:836`
- Consent gate implementation (scoped by provider/model/scope hash):
  - `engine/internal/engine/engine.go:2603`

Impact:
- Context processing can perform provider egress before explicit scoped consent is granted for that workbench.
- Behavior diverges from expected safety policy and existing test expectations for consent-first model calls.

Notes:
- If this is intentional product behavior, consent policy and test cases should be updated to reflect a separate context consent model.
- If unintentional, `ContextProcess` should call `ensureConsent` (or an equivalent context-specific consent gate) before `processContextWithModel`.
