# Implementation Plan: OpenAI RPI Reasoning-Effort Runtime Settings

## Summary
Add provider-level runtime settings for RPI phase reasoning effort so OpenAI-backed runs can independently tune Research, Plan, and Implement effort for:
- `openai`
- `openai-codex`

Settings are persisted in `engine/internal/settings/settings.go` with backward-compatible defaults and backfill behavior.

## Scope
- Persist three provider-level fields:
  - `rpi_research_reasoning_effort`
  - `rpi_plan_reasoning_effort`
  - `rpi_implement_reasoning_effort`
- Keep schema version unchanged (`schema_version: 1`).
- Backfill legacy settings JSON to safe defaults.
- Enforce canonical literals for persisted values.

## Allowed Values
Accepted literals (canonical, lowercase):
- `none`
- `low`
- `medium`
- `high`
- `xhigh`

Default for all R/P/I fields is `medium`.

## Persistence and Backward Compatibility
1. Extend `ProviderSettings` with the three RPI reasoning-effort fields.
2. On `Load()`:
   - Keep existing provider backfill behavior.
   - For `openai` and `openai-codex`, default missing/empty values to `medium`.
   - Normalize non-canonical input to canonical lowercase.
   - Fallback invalid values to `medium`.
3. On `Save()`:
   - Apply the same backfill/normalization pass before writing JSON.
4. Non-OpenAI providers (`anthropic`, `google`) remain unchanged.

This preserves compatibility with existing settings files that only contain `enabled` and/or pre-date `openai-codex`.

## Runtime Consumption Plan
1. Research phase uses provider’s `rpi_research_reasoning_effort`.
2. Plan phase uses provider’s `rpi_plan_reasoning_effort`.
3. Implement phase uses provider’s `rpi_implement_reasoning_effort`.
4. If a value is missing/invalid at runtime, treat it as `medium`.
5. Apply only when provider is `openai` or `openai-codex`; other providers ignore these fields.

## Test Plan
### Unit tests (`engine/internal/settings/settings_test.go`)
- Default load sets OpenAI and OpenAI Codex R/P/I effort to `medium`.
- Round-trip persistence retains valid configured values.
- Legacy JSON backfills missing fields to `medium`.
- Legacy invalid values normalize/fallback to canonical defaults.
- Existing provider enable/disable behavior remains unchanged.

### Coverage gates
- Run: `cd engine && go test ./... -coverprofile=coverage.out`
- Required project gate: total Go coverage `>= 50%`.
- No fake AI mode for AI-interaction tests (per repo policy).

## Acceptance Criteria
- Settings JSON remains backward compatible without schema bump.
- OpenAI provider entries always expose valid canonical R/P/I effort values after load/save.
- Defaults/backfill behavior is deterministic and covered by tests.
