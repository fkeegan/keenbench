# Design: <Capability Name>

## Status
Draft

## PRD References
- `docs/prd/capabilities/<capability>.md`
- (Optional) `docs/prd/keenbench-prd.md` sections: <...>

## Summary
- What this capability is and why it exists.

## Goals / Non-Goals
### Goals
- …

### Non-Goals
- …

## User Experience
- Primary user flow
- UI states (empty, loading, error, success)
- Accessibility considerations (keyboard, screen reader, contrast)

## Architecture
### UI Responsibilities (Flutter)
- …

### Engine Responsibilities (Go)
- …

### IPC / API Surface
- RPCs (request/response)
- Events (progress, streaming, completion, errors)

## Data & Storage
- New/updated entities (Workbench metadata, Draft state, job records, checkpoints)
- On-disk layout changes (if any)
- Migration/back-compat notes

## Algorithms / Logic
- Diffing/summarization/estimation logic relevant to this capability
- Performance constraints and limits

## Error Handling & Recovery
- Failure modes and user-facing actions
- Safe cancellation/resume behavior (if applicable)

## Security & Privacy
- Scope enforcement
- Network egress / upload confirmations (if applicable)
- Audit trail fields

## Telemetry (If Any)
- Metrics captured
- Opt-in/opt-out behavior

## Open Questions
- …

