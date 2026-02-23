# RPI Research Can Blow Up Payload via `recall_tool_result` and Misreport Failure as Max-Turn Exhaustion (2026-02-22)

Source runs:
- Reproduction run: local app run with `KEENBENCH_DEBUG=1` and `WorkshopRegenerate`
- Engine log excerpt time window: `2026-02-22T21:58:30+01:00` to `2026-02-22T21:58:42+01:00`
- Workbench ID: `542e8560d044805c`
- Branch observed: `feat/model-feedback-product-fit`

## Issue 1: Research phase can inject multi-MB recalled tool output into next model turn

- Status: Open
- Severity: High (run instability / avoidable token-context blowup)
- Area: Workshop RPI research loop / tool result recall and payload control

Expected:
- Research should continue map/query-first with bounded reads.
- Large historical tool results should not be re-injected as full multi-MB payloads into subsequent model turns.
- If full recall is necessary, it should be size-bounded, chunked, or disallowed in research.

Actual:
- Model called `recall_tool_result` (`entry_id=12`) during research.
- Tool returned ~4.99 MB raw payload (`result_bytes` and `receipt_bytes` both `4991935`).
- Next API turn payload grew to ~5.09 MB (`payload_bytes_approx: 5090157`).

Evidence (from engine log):
- `workshop.tool_start` with tool `recall_tool_result`, `entry_id:13`, args `{"entry_id":12}`
- `workshop.tool_complete` with `result_bytes:4991935`, `receipt_bytes:4991935`
- Next request `workshop.research.agent_api_request` with `payload_bytes_approx:5090157`

Root cause:
- `recall_tool_result` is available in `ResearchTools`.
- `recall_tool_result` returns the full raw tool result from tool log with no size cap/chunking.
- That full recalled content is appended as a tool message and sent in subsequent turns.

Code references:
- `engine/internal/engine/workshop_tools.go` (research tool set includes `recall_tool_result`)
- `engine/internal/engine/workshop_tools.go` (`recallToolResult` returns `entry.Result` directly)
- `engine/internal/engine/engine.go` (`runAgentLoop` appends tool result receipt/content directly into messages)

## Issue 2: Empty stop response is surfaced as misleading max-turn exhaustion

- Status: Open
- Severity: Medium-High (diagnostic confusion / incorrect failure semantics)
- Area: Workshop RPI loop termination classification

Expected:
- If model returns `finish_reason=stop` with empty content, error should explicitly indicate empty terminal response (or be retried/handled), not max-turn exhaustion.

Actual:
- Turn 10 returned `finish_reason:"stop"` and `content_length:0`.
- Loop emitted warning `workshop.research.agent_max_turns_exhausted` and error:
  - `AGENT_LOOP_DETECTED`
  - detail: `agent reached maximum turn limit (60) without completing`
- This is misleading because the run ended after 11 turns, not after consuming 60 turns.

Evidence (from engine log):
- `workshop.research.agent_api_response` turn 10 with `finish_reason:"stop"`, `content_length:0`
- `workshop.research.agent_complete` with `total_turns:11`
- immediately followed by `workshop.research.agent_max_turns_exhausted`
- RPC error payload includes `AGENT_LOOP_DETECTED` + max-turn detail

Root cause:
- `runAgentLoop` treats `finalAssistantText == "" && fullResponse.Len()==0` as max-turn exhaustion regardless of actual turn index and stop reason.

Code references:
- `engine/internal/engine/engine.go` (`runAgentLoop`, post-loop `finalAssistantText`/`fullResponse` check)

## Not a root cause

- The new model-feedback feature did not cause the failure.
- `model_feedback.recorded ... status:"skipped"` was logged after failure as passive telemetry capture.

## Suggested fix direction

1. Research recall guardrails:
   - Option A: remove `recall_tool_result` from `ResearchTools`.
   - Option B: keep it but cap size and require chunked recall parameters.
   - Option C: keep but return bounded preview by default with explicit `offset/length` paging.
2. Loop termination correctness:
   - Add explicit empty-terminal-response error classification (distinct from max-turn exhaustion).
   - Include `turn`, `finish_reason`, and content/stream lengths in error detail.
3. Prompt/tool policy hardening:
   - Strengthen research prompt to forbid large full recalls and prefer bounded query windows.
4. Regression tests:
   - Reproduce large recall in research and assert payload cap behavior.
   - Assert empty `stop` response surfaces correct error code/detail.
