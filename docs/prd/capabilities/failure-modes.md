# PRD: Failure Modes & Recovery

## Status
Draft

## Purpose
Provide predictable, recoverable behavior when jobs fail so users can trust the system.

## Scope
- In scope: model errors, rate limits, file I/O failures, cancellations, checkpoint restore, disk space issues.
- Out of scope: full OS crash recovery, data recovery after disk failure.

## User Experience
- Clear error banner with next actions (retry, switch model, restore checkpoint, view report).
- Job report includes failure reason, last completed step, and partial outputs.
- Status report includes audit trail fields (model, steps completed, files touched, checkpoints created, estimate vs actual usage when available).
- Draft state is preserved unless the user discards it.

## Functional Requirements
1. Errors are classified (model, network, file I/O, user cancel) and logged.
2. Published state is never modified on failure.
3. Draft state is preserved by default after failures.
4. Users can retry from the last safe step or restore a checkpoint.
5. Failures produce a status report with partial results when available.
6. Failure reports include: model involved, steps completed, files changed, checkpoints created.

## Failure Modes & Recovery

### Workshop
- Model unavailable: retry with backoff; offer to switch provider.
- Rate limit / timeout: pause, resume, or switch model.
- File read/parse failure: list affected files and continue if possible.
- Write failure / disk full: halt Draft writes and prompt user to free space.
- User cancellation: stop cleanly and keep Draft.

## Security & Privacy
- Failures never expand file scope or send extra data.
- In production, error logs exclude raw file contents and raw prompts by default. In debug mode, raw prompt/file excerpts may be logged for development triage and must be off by default.

## Acceptance Criteria
- Failed jobs never alter Published state.
- Users always get a clear error reason and a recovery action.
- Draft is preserved on failure unless explicitly discarded.
- Failure reports include the audit trail fields needed to understand what ran and what changed.

## Open Questions
~~Should v1 auto-retry transient errors, or always ask the user?~~ → **Resolved**: Auto-retry transient errors (3 attempts with exponential backoff). If the problem persists, show error to user and offer option to switch provider.

~~How should "resume from last step" work across different providers?~~ → **Resolved**: Resume logic is provider-agnostic. The system tracks the last completed step and resumes from there regardless of which provider is used.
