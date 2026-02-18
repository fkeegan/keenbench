# KeenBench — Test Plan: Publish and Discard

> Extracted from the [master test plan](../test-plan.md). This section can be used independently.

## Policy

**Every test that involves AI interaction MUST use real model API calls. No exceptions.**
No fake/mock AI clients. No `KEENBENCH_FAKE_OPENAI`. No conditional branching on fake vs real mode.
Assertions on AI output must be structural or numerical — never exact-text-match on prose.
See `CLAUDE.md` for the full testing policy.

## Test Environment

- Linux desktop (X11). E2E harness targets `flutter test integration_test -d linux`.
- Network access to: `api.openai.com`, `api.anthropic.com`, `generativelanguage.googleapis.com`.
- Valid API keys in `.env`:
  - `KEENBENCH_OPENAI_API_KEY` (required for all AI tests)
  - `KEENBENCH_ANTHROPIC_API_KEY` (required for multi-provider tests)
  - `KEENBENCH_GEMINI_API_KEY` (required for multi-provider tests)
- Clean app data directory per test run (`KEENBENCH_DATA_DIR` pointed to temp dir).
- Python venv with tool worker dependencies (`engine/tools/pyworker/.venv`).

## Conventions

- **Priority:** P0 = must-pass, P1 = important, P2 = nice-to-have.
- **IDs:** `TC-###`. No milestone prefix — test cases apply across milestones.
- **AI tests:** Marked with `[AI]` tag. These MUST use real model calls.
- **Steps format:** Each step is an atomic action followed by `Expected:` with the verifiable result.
- **Timeout convention:** AI-driven steps use 60-120s timeouts unless noted.

## UI Keys Reference

Key elements for this section:
- `AppKeys.reviewScreen`, `AppKeys.reviewPublishButton`, `AppKeys.reviewDiscardButton`
- `AppKeys.workbenchScreen`, `AppKeys.workbenchDraftBanner`
- `AppKeys.workbenchFileList`
- `AppKeys.workbenchMessageList`, `AppKeys.workbenchMessageRewindButton(<message_id>)`
- `AppKeys.workbenchCheckpointsButton`, `AppKeys.checkpointsScreen`, `AppKeys.checkpointsList`

## Publish/Discard Behavior

- **Publish:** Moves draft files to the published directory atomically. Creates a checkpoint. The draft is removed.
- **Discard:** Removes draft files entirely. Published files remain unchanged. The draft is removed.
- **Rewind across publish boundary:** Rewinding conversation to a point before a publish event restores Published files to the state for that point in history.
- After either action, the workbench returns to normal mode (composer visible, file mutations enabled).
- Published files live in `workbenches/<id>/published/`. Draft files live in `workbenches/<id>/draft/`.
- Engine API: `DraftGetState` returns `has_draft: true/false`.

---

## Test Cases

### 9. Publish and Discard

#### TC-070: Publish applies Draft atomically `[AI]`
- Priority: P0
- Preconditions: Draft exists with at least one ADDED file (e.g., `org_chart.md` from TC-040).
- Steps:
  1. Open the review screen.
     Expected: Review screen shows the changes.
  2. Click "Publish" (`AppKeys.reviewPublishButton`).
     Expected: The review screen closes. The workbench screen (`AppKeys.workbenchScreen`) reappears. The draft banner (`AppKeys.workbenchDraftBanner`) is NOT visible.
  3. Verify the file list now contains the published files.
     Expected: The newly added files (e.g., `org_chart.md`) appear in the workbench file list as regular files.
  4. Verify via engine API (`DraftGetState`).
     Expected: `has_draft` is `false`.
  5. Verify files exist in the published directory on disk.
     Expected: The files exist at `workbenches/<id>/published/<filename>` and are readable.

#### TC-071: Discard removes Draft, published unchanged `[AI]`
- Priority: P0
- Preconditions: Draft exists with ADDED files that have NOT been published before.
- Steps:
  1. Note the current published file list (before discard).
     Expected: The published directory does not contain the draft-added files.
  2. Open the review screen and click "Discard" (`AppKeys.reviewDiscardButton`).
     Expected: The workbench screen reappears. The draft banner is NOT visible.
  3. Verify the file list does NOT contain the discarded draft files.
     Expected: The files that were only in the draft (never published) are not in the file list.
  4. Verify via engine API (`DraftGetState`).
     Expected: `has_draft` is `false`.
  5. Verify the published directory on disk.
     Expected: The draft-only files do NOT exist in the published directory.

#### TC-072: Draft persists across app restart `[AI]`
- Priority: P1
- Preconditions: Draft exists.
- Steps:
  1. Note the draft state (draft banner visible, file names in draft).
     Expected: Draft banner is present.
  2. Quit the app and relaunch.
     Expected: Home screen appears.
  3. Open the workbench.
     Expected: The draft banner is still present OR the review screen auto-opens. The draft changes are preserved.
  4. Open review and verify the same changes are listed.
     Expected: The change list matches what was seen before the restart.
  5. Discard or publish to clean up.
     Expected: Draft is resolved.

#### TC-073: Publish creates a checkpoint `[AI]`
- Priority: P1
- Preconditions: Draft exists.
- Steps:
  1. Click "Publish" on the review screen.
     Expected: Draft is published. Workbench screen appears.
  2. Click the Checkpoints button (`AppKeys.workbenchCheckpointsButton`).
     Expected: The checkpoints screen (`AppKeys.checkpointsScreen`) opens.
  3. Verify the checkpoints list (`AppKeys.checkpointsList`).
     Expected: At least one checkpoint exists with reason "publish" or description referencing the publish action. The checkpoint has a timestamp.

#### TC-074: Rewind before publish restores published file content `[AI]`
- Priority: P0
- Preconditions: Workbench has a single text file `notes.txt` with known original content (for example, `Original baseline text`). No Draft exists.
- Steps:
  1. Extract `notes.txt` to a temp output folder via `WorkbenchFilesExtract` and record it as `source-notes.txt`.
     Expected: Extract succeeds. `source-notes.txt` content is the baseline reference.
  2. Send a workshop prompt that modifies `notes.txt` (for example: "Rewrite notes.txt as a concise executive summary with different wording"), wait for Draft creation, open review, and click Publish.
     Expected: Publish succeeds. Draft is cleared. A publish checkpoint/system event exists.
  3. Extract `notes.txt` again and save as `new-notes.txt`.
     Expected: `new-notes.txt` differs from `source-notes.txt`.
  4. In the message list (`AppKeys.workbenchMessageList`), click Rewind (`AppKeys.workbenchMessageRewindButton(<message_id>)`) on a message from before the publish-triggering turn and confirm.
     Expected: Rewind completes. Conversation is truncated to the selected point. Any Draft state reflects the rewound point.
  5. Extract `notes.txt` again and save as `old-notes.txt`.
     Expected: `old-notes.txt` is identical to `source-notes.txt` and differs from `new-notes.txt`.
