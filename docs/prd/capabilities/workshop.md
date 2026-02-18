# PRD: Workshop Mode

## Status
Draft

## Version
v0.3

## Last Updated
2026-02-13

## Purpose
Provide an interactive, chat-based collaboration mode for step-by-step work with tight feedback loops and incremental Draft application.

## Scope
- In scope (v1): chat interface, Draft application, message-level undo, conversation persistence, model switching, streaming responses.
- In scope (v1.5): "Try with another model" fork for parallel responses (requires concurrent Drafts).
- Out of scope: real-time collaboration, voice input, image generation within chat.

## Key Concepts

### Draft Revisions (Workshop-Internal)
Draft Revisions are Workshop-internal snapshots of Draft state tied to conversation points. They enable message-level undo to restore both chat and Draft together ("go back in time"). Draft Revisions are distinct from Checkpoints:
- **Draft Revisions**: Workshop-internal, snapshot Draft state, enable message-level undo. Never exposed in the Checkpoints UI.
- **Checkpoints**: System-wide, snapshot Published state, enable restore to prior approved states.

When a user Publishes a Draft, Draft Revisions for that Draft are pruned (they're no longer meaningful since the Draft is gone). When a user Discards a Draft, Draft Revisions are also pruned.

## User Experience
- Chat-like interface scoped to the active Workbench.
- User sends a prompt (Enter to send; Shift+Enter for newline).
- AI responds with text; messages are selectable and include a copy icon button; assistant responses render markdown (links are disabled). The AI may either answer read-only (analysis/Q&A) or perform write operations in Draft, depending on user intent and task needs.
- Review opens automatically when a Draft appears (new Draft creation) and when a Workbench is reopened with an existing Draft.
- Users review the Draft and publish or discard.
- Publish checkpoints are shown inline in the Workshop chat timeline with a Restore action.
- Restore from chat is blocked while a Draft exists; users must publish or discard first.
- The standalone Checkpoints screen remains the full history/manual checkpoint management surface.
- Workbench chrome uses icon actions: a history-style Checkpoints button in the top bar and a Settings gear in the bottom-left of the file panel.
- Conversation is linear in v1 (no branching).
- Model selector is always visible; switching is instant.
- Conversation persists per Workbench and restores on reopen.
- Users can revert to a previous message (undo/regenerate).

## Functional Requirements

### v1
1. Workshop chat is scoped to the active Workbench; AI can only read/write Workbench files.
2. AI responses support streaming for perceived responsiveness.
3. The AI drives a tool-use workflow that can read files, apply edits, and create Draft changes directly. For analysis-only requests, it may return without creating Draft changes.
4. Users review the Draft and publish or discard.
5. Review opens automatically when a Draft is created and when reopening a Workbench that already has a Draft.
6. Publish checkpoints are surfaced inline in Workshop chat with a Restore affordance.
7. Restore actions are blocked while a Draft exists, with clear guidance.
8. Conversation is saved per Workbench and restored when the Workbench is reopened.
9. Message-level undo: users can revert the conversation to a previous message.
   - Reverting discards all messages after the selected point.
   - Users can regenerate a response from a reverted point.
10. Model switching: users can switch models at any time.
   - Switch is immediate (no confirmation dialog).
   - New model picks up full conversation history and continues.
   - Switch persists as the Workbench default model.
11. Conversation is linear; no branching or parallel threads in v1.
12. Workbench chrome uses icon-only Settings/Checkpoints actions with tooltips (Settings bottom-left, Checkpoints top bar).
14. For tabular text workflows (CSV), Workshop supports map-first + query-first execution (schema/stats/query/export) so large datasets can be processed without script-generation detours.

### v1.5
15. "Try with another model" creates a parallel response from a different model.
16. User can compare parallel responses and choose which to keep.
17. Parallel responses may create concurrent Drafts (labeled by model).

## Draft Interaction
- Workshop changes go through the standard Draft workflow.
- Only one Draft at a time in v1; when a Draft exists, Workshop input is blocked until Publish or Discard.
- Draft creation (or reopening with an existing Draft) routes users directly into Review.
- If a Draft exists from a prior job or Workshop session, starting a new job prompts: Publish, Discard, or Cancel.
- Publishing and discarding Drafts follows the Draft & Publish rules.

## Conversation Limits
- No hard limit on conversation length in v1.
- Clutter Bar reflects conversation history weight alongside file weight.
- When context is constrained, older messages may be summarized or truncated (implementation detail).

## RPI Workflow Intelligence

Workshop agent execution follows a phased RPI workflow (Research → Plan → Implement), with a final Summary response.

### Requirements

17. **Phase orchestration**: Each Workshop run shall execute in order: Research, Plan, Implement (item loop), then Summary.

18. **Fresh phase context**: Research, Plan, and each Implement item shall run with fresh model context; phase state is transferred via internal artifacts, not accumulated conversation history.

19. **State artifacts**: The engine shall persist internal artifacts under `meta/workshop/_rpi/`:
   - `research.md`
   - `plan.md`

20. **Plan tracking**: `plan.md` shall use markdown checklist items (`- [ ]`, `- [x]`, `- [!]`) and be engine-updated after each implement item.

21. **Resumability**: A subsequent `WorkshopRunAgent` invocation shall resume from saved `_rpi` state (skip completed phases/items and continue from first pending work).

22. **State invalidation**: `_rpi` state shall be cleared when prior output is invalidated (new user message, regenerate, rewind/undo).

23. **Phase-scoped tools**:
   - Research: read-only subset.
   - Plan: minimal read/recall subset.
   - Implement: full workshop tool set.

24. **Summary-only chat output**: Only Summary phase text shall stream/persist as the user-visible assistant message; R/P/I outputs are internal artifacts.

25. **Progress notifications**: The engine shall emit phase lifecycle notifications and implement-item progress updates for UI status display.

26. **Plan inflation guard**: Implement phase may append newly discovered plan items, but total item count shall be capped to `2x` original plan size.

## Failure Modes & Recovery
- Model unavailable: surface error, allow retry or switch model.
- Rate limit / timeout: pause, allow retry or switch model.
- Network interruption: conversation is saved up to the last successful exchange; user can resume.
- Streaming interrupted: partial response is preserved; user can regenerate.
- Auto-apply fails: assistant responds with a clear limitation + suggested alternative; Draft remains unchanged.
- Implement item failure: retry once, then mark item failed (`[!]`) with reason and continue remaining items.

## Security & Privacy
- Workshop operates only within the Workbench sandbox.
- Model calls only to configured providers.
- Conversation history is stored locally per Workbench.

## Acceptance Criteria
- Users can have a multi-turn conversation scoped to Workbench files.
- Draft changes are created when the AI performs write operations; analysis-only requests may produce no Draft changes. Users publish or discard Draft changes.
- Review opens automatically after Draft creation and on Workbench reopen when a Draft already exists.
- Publish checkpoints appear inline in chat and include a Restore action.
- Restore from chat is blocked while a Draft exists.
- Conversation is restored when reopening a Workbench.
- Users can revert to a previous message and regenerate.
- Model switch is seamless; new model continues from conversation history.
- Streaming responses feel responsive (first token appears promptly).
- "Try with another model" creates labeled parallel outputs (v1.5).
- Workshop execution follows Research → Plan → Implement → Summary with resumable `_rpi` artifacts.
- Only Summary text appears as assistant chat output; R/P/I phase chatter is not persisted to conversation.
- UI shows phase status and implement progress notifications while agent work is in flight.
- Large CSV workflows can complete using table tools (describe/stats/query/export) without requiring generated helper scripts.
- Implement-phase retries mark failed checklist items and continue remaining work.

## Open Questions
None currently.

## Resolved Questions
None currently.
