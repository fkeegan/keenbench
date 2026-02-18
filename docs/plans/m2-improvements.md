# Implementation Plan: M2 Improvements — Deletes + Chat Markdown/Copy

## Status
Draft (2026-02-05)

## Goal
Add missing UX-critical improvements:
- Delete Workbenches.
- Remove files from a Workbench.
- Selectable chat text + copy button.
- Render assistant responses as markdown.

Keep PRDs and design docs in sync, and audit implementation against this plan after completion.

---

## Branch
- Create branch `m2-improvements`.

---

## Key References (Impacted Design)
- `docs/design/capabilities/workbench.md`
- `docs/prd/capabilities/workbench.md`
- `docs/design/capabilities/workshop.md`
- `docs/prd/capabilities/workshop.md`
- `docs/prd/keenbench-prd.md`
- `docs/design/style-guide.md`

---

## Scope

### In Scope
1. **Delete Workbench** (engine + UI) with explicit confirmation.
2. **Remove Workbench files** (engine + UI) with explicit confirmation.
3. **Chat UX improvements**: selectable text, copy button, markdown rendering for assistant replies.
4. **Docs updates** for PRDs and design.
5. **Audit**: compare implementation vs plan and record gaps.

### Out of Scope
- Draft deletes inside Review (file deletion in Draft remains unsupported).
- Any changes to Draft/Publish rules beyond blocking deletes when Draft exists.

---

## What “Done” Means (Acceptance Criteria)
1. A user can delete a Workbench from the Home screen; deletion is blocked while a Draft exists.
2. A user can remove a Workbench file from the Workbench file list; removal is blocked while a Draft exists.
3. Assistant messages render markdown with selectable text and a small icon-only copy button (links disabled).
4. User messages are selectable.
5. All impacted PRD/design docs are updated.
6. An implementation-vs-plan audit is recorded at the end.

---

## Major Design Decisions
1. **Deletes are blocked when a Draft exists** (publish or discard first).
2. **Copy button is icon-only and always visible** on assistant messages.
3. **Markdown rendering is assistant-only**, links are disabled.
4. UI changes follow the warm minimal style guide (4px spacing, subtle borders/shadows, Inter + JetBrains Mono).

---

## Public API / Interface Changes
New RPCs:
- `WorkbenchFilesRemove({workbench_id, workbench_paths[]}) -> {remove_results[]}`
- `WorkbenchDelete({workbench_id}) -> {}`

New data type:
- `workbench.RemoveResult` (`path`, `status`, `reason?`)

---

## Implementation Plan (By Area)

### Engine
1. Add `Manager.Delete` to remove Workbench directories (blocked if Draft exists).
2. Add `Manager.FilesRemove` to delete files, update manifest, and return per-file results.
3. Add engine RPCs `WorkbenchFilesRemove` and `WorkbenchDelete`.
4. Emit clutter update after file removal; clear session consent on Workbench delete.

### UI (Flutter)
1. Home screen: Workbench tile overflow menu with **Delete Workbench** action + confirm dialog.
2. Workbench screen: add remove icon button per file row + confirm dialog.
3. Workbench chat:
   - User messages: `SelectableText`.
   - Assistant messages: `MarkdownBody(selectable: true)`.
   - Add small icon-only copy button (16–18px icon, 28–32px hit target).
4. Add AppKeys for delete actions and copy button.

### Docs
1. Update Workbench PRD/design to include delete Workbench and confirm remove file behavior.
2. Update Workshop PRD/design to include selectable chat, copy button, markdown rendering.
3. Update main PRD SR4 to note deletes blocked while Draft exists.

---

## Test Plan
1. `cd engine && go test ./...`
2. `cd app && flutter test`
3. Engine unit tests:
   - `FilesRemove` removes file + updates manifest.
   - `FilesRemove` blocked when Draft exists.
   - `Delete` removes Workbench directory.
4. Manual checks:
   - Delete Workbench from Home (confirm + refresh).
   - Remove file from Workbench list (confirm + refresh).
   - Chat markdown renders; selectable text and copy button work.

---

## Rollout / Migration Notes
- No data migration required; deletes affect only local Workbench storage.
- Confirm dialogs guard destructive actions; Draft blocks prevent unsafe deletes.

---

## Audit (Post-Implementation)
After completion, compare code + docs against this plan and record:
- Any skipped items
- Any added-but-unplanned changes
- Follow-up gaps
