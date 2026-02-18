# PRD: Draft & Publish

## Status
Draft

## Purpose
Ensure AI edits are safe, reviewable, and reversible before touching the Published state.

## Scope
- In scope (v1): Draft state per job, publish/discard, checkpoints + restore. **One Draft at a time.**
- In scope (v1.5): Draft forking via "Try with another model" for Workshop responses, concurrent Drafts.
- Out of scope: cross-Workbench merges, real-time multi-user editing, automatic conflict resolution.

## User Experience
- Clear Draft status indicator and actions: Publish, Discard (Review opens automatically when Draft appears).
- If a Draft exists, starting a new job prompts: Publish, Discard, or Cancel (v1).
- **File operations blocked while Draft exists**: Users cannot add, remove, or extract files from the Workbench while a Draft is in progress. They must Publish or Discard first.
- **Draft age reminder**: If a Draft is older than 24 hours, show a gentle reminder: "You have an open Draft from [date]. Review and publish or discard?"
- **Stale Draft auto-cleanup**: On Workbench open, if a Draft is older than 7 days with no activity, prompt: "A stale Draft was found (last modified [date]). Discard it?" with actions: **Discard** / **Keep**.
- In v1.5, prompt includes: continue, fork, or discard (Workshop only).
- Publish checkpoints are surfaced inline in Workshop chat with a Restore affordance.
- Restore actions are blocked while a Draft exists.
- The standalone Checkpoints screen remains the full history/manual checkpoint surface.

## Functional Requirements

### v1
1. AI reads and writes only inside Draft; Published is read-only.
2. Only one Draft at a time.
3. Publish applies Draft changes atomically and creates a checkpoint.
4. Discard removes Draft and restores Published as the active state.
5. Auto-checkpoint before each job execution (manual checkpoint optional).
6. Restore “goes back in time”: it reverts Published (and associated Workbench history like conversation/job records) to a selected checkpoint.
7. Review opens automatically when a Draft is created and when reopening with an existing Draft.
8. Publish checkpoints appear inline in Workshop chat with a Restore action.
9. Restore actions are blocked while a Draft exists.

### v1.5
10. "Try with another model" can create a parallel Draft labeled by model (Workshop only).
11. Concurrent Drafts are supported with clear labeling.

## Failure Modes & Recovery
- Publish fails mid-write: rollback to prior Published state and keep Draft intact.
- Draft missing or corrupted: block publish and offer discard or restore.
- Checkpoint creation fails: warn and block execution until resolved.
- Disk full/write error: pause Draft writes and prompt user to free space.
- Publish conflict detected (Published changed outside app): prompt to Discard Draft or Restore Published from checkpoint. Fork option deferred to v1.5.
- Restore attempted while Draft exists: block restore and guide user to Publish or Discard first.
- Crash during publish: Draft is preserved. User restarts app and can retry Publish or Discard. No automatic recovery in v1.

## Security & Privacy
- Draft and Published are contained within the Workbench sandbox.
- No changes are applied to Published without explicit user action.

## Acceptance Criteria
- Users can publish a Draft and see all changes applied at once.
- Users can discard a Draft without affecting Published.
- Checkpoints exist for each job and restore returns the Workbench to a prior state (“go back in time”).
- Review opens automatically when Draft appears (including reopen with an existing Draft).
- Publish checkpoints appear inline in Workshop chat with Restore controls.
- Restore controls are disabled while a Draft exists.
- Forked Drafts are clearly labeled and do not overwrite each other (Workshop only).

## Open Questions
~~Should v1 allow multiple concurrent Drafts or enforce one at a time?~~ → **Resolved**: One at a time in v1. Concurrent Drafts in v1.5.

~~How should publish conflicts be handled if multiple Drafts exist?~~ → **Resolved**: N/A for v1 (single Draft). In v1.5, Drafts are labeled and user chooses which to publish.

~~Should v1 allow file add/remove while a Draft exists?~~ → **Resolved**: No. File operations are blocked while a Draft exists. User must Publish or Discard first.
