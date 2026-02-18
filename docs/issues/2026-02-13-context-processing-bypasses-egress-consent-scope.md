# Context Processing Bypasses Egress Consent Scope (2026-02-13)

Source runs:
- Reproduction run: `make run FLUTTER_BIN=<flutter-dtd-driver-wrapper>` (local, 2026-02-13 12:58-13:16)
- Engine log: `~/.config/keenbench/logs/engine.log`
- Test section: `docs/test/sections/13-workbench-context.md`
- Mode: manual system run via `$keenbench-ai-manual`
- Reproduction workbench: `WB Context - Consent Scope 2026-02-13-R2` (`workbench_id: 52d4912c277d0d93`)

## Issue 1: `ContextProcess` succeeds on fresh workbench before consent flow

- Status: Open
- Severity: High (egress-consent policy mismatch for context processing)
- Area: Workbench Context / egress gating
- Expected: In TC-171, processing Company context before any Workshop consent should fail with `EGRESS_CONSENT_REQUIRED` (or equivalent consent-required error), and the category should remain empty.
- Actual: `ContextProcess` succeeded and activated `company-context` on a fresh workbench with a scoped file, before any `WorkshopSendUserMessage`, `EgressGetConsentStatus`, or `EgressGrantWorkshopConsent` call for that workbench.

Evidence:
- Fresh workbench + no prior context:
  - `~/.config/keenbench/logs/engine.log:13371` (`ContextList` returns all categories `status:"empty"`)
- Company process request on fresh workbench:
  - `~/.config/keenbench/logs/engine.log:13374` (`ContextProcess`, id `20`, category `company-context`)
- Successful response (unexpected for TC-171 precondition):
  - `~/.config/keenbench/logs/engine.log:13377` (`ContextProcess` response with `status:"active"`)
- No consent/workshop call for this workbench id:
  - `rg -n 'WorkshopSendUserMessage|EgressGetConsentStatus|EgressGrantWorkshopConsent' ~/.config/keenbench/logs/engine.log | rg '52d4912c277d0d93'` -> no matches
- UI evidence (MCP screenshot capture names):
  - `manual/tc171-new-workbench-opened`
  - `manual/tc171-after-add-file`
  - `manual/tc171-after-company-process-attempt`

Notes:
- This may indicate either:
  - the test-plan expectation for TC-171 is outdated, or
  - context processing currently bypasses the intended consent gate.
- The behavior was reproduced after relaunch in a clean, newly created workbench during the same session.
