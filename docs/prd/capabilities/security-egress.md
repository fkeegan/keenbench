# PRD: Network Egress & Upload Guardrails

## Status
Draft

## Purpose
Ensure external data access and file uploads are explicit, reviewable, and within user-approved scope.

## Scope
- In scope (v1): model calls only to configured providers (official endpoints), explicit user confirmation before sending Workbench content to provider(s) (one-time per Workbench/job, not per-call), and audit logging of egress activity.
- Out of scope (v1): background web browsing, URL fetching/external retrieval, connectors (Drive/Notion/email/calendar), automatic provider fallback to unconfigured services.
- Future: URL fetching with per-fetch confirmation, and “send excerpt/summary only” options for uploads.

## User Experience
- Default behavior: only configured model endpoints are reachable.
- v1 does not support URL fetching/external retrieval; if requested, the app explains the limitation and asks the user to paste relevant content instead.
- Before the first model call in a Workbench (Workshop), show a confirmation dialog listing:
  - provider(s) and model(s), and
  - in-scope Workbench files and sizes.
- Subsequent model calls do not re-prompt unless provider(s) or file scope changes.
- Users can cancel without losing Draft state.

### Consent Persistence
- **Workshop**: Consent includes a "Don't ask me again for this Workbench" checkbox (default: checked). If checked, consent persists across app restarts. If unchecked, consent applies only to the current session and will be requested again on next app start.


## Functional Requirements
1. Network egress is restricted to configured model providers by default.
2. URL fetching/external retrieval is not available in v1; any attempt is blocked with a clear user-facing message.
3. Model calls require explicit user confirmation before sending Workbench content (one-time per Workbench/job, not per-call).
4. Confirmation lists provider(s)/model(s) and in-scope files and sizes.
5. Egress actions are recorded in the job audit trail (provider(s), time, and in-scope files and/or scope hash).

## Failure Modes & Recovery
- External retrieval requested: show “not supported in v1” and allow the user to proceed without it.
- Upload failure: pause execution, allow retry or cancel.
- Provider endpoint unreachable: surface error and offer model switch.

## Security & Privacy
- No hidden external calls.
- No implicit upload of files outside the Workbench without user confirmation.
- In production, audit logs do not store raw file contents or raw prompts by default (they record provider(s)/model(s) and file scope). In debug mode, raw prompt/file excerpts may be logged for development triage and must be off by default.

## Acceptance Criteria
- Users explicitly confirm before the app sends Workbench content to provider(s) in Workshop (per Workbench).
- The confirmation lists provider(s)/model(s) and in-scope files and sizes.
- Jobs never make external requests to unconfigured providers.
