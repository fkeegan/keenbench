# 14. Accessibility Core (WCAG 2.1 AA Baseline)

## Workbench Keyboard-Only Flow

### TC-A11Y-01
- Priority: P0
- Preconditions: Workbench has at least one file and no draft.
- Steps:
  1. Navigate to Workbench and use skip links to jump to main content and composer.
     Expected: Skip links are reachable via keyboard and move focus to target regions.
  2. Use keyboard only to compose and send a message.
     Expected: Message sends without mouse interaction.
  3. Trigger review/discard actions when draft is present.
     Expected: Review and discard controls are keyboard reachable and visible when focused.

## Screen Reader Announcements

### TC-A11Y-02
- Priority: P0
- Preconditions: VoiceOver (macOS) or NVDA (Windows) enabled.
- Steps:
  1. Start an assistant run from Workbench.
     Expected: Screen reader announces generation/status transitions (start/complete, tool activity, draft ready).
  2. Change model from the Workbench selector.
     Expected: Current model and model-change announcement are spoken.
  3. Trigger an actionable error (for example provider disabled).
     Expected: Error summary region is announced and focusable.

## Review Keyboard + Semantics

### TC-A11Y-03
- Priority: P0
- Preconditions: A draft exists with at least one changed file.
- Steps:
  1. Open Review and use skip links to jump between file list and detail pane.
     Expected: Focus moves to the requested regions.
  2. Navigate changed files with keyboard and select a file.
     Expected: File selection updates the pane and announces "Showing diff for ...".
  3. Navigate diff/preview controls (previous/next page or slide) using keyboard.
     Expected: Controls are reachable and labeled for assistive tech.

## Platform Matrix Smoke

### TC-A11Y-04
- Priority: P1
- Preconditions: Run once per release.
- Steps:
  1. Execute TC-A11Y-01 through TC-A11Y-03 on VoiceOver (macOS) and NVDA (Windows).
     Expected: No keyboard traps and no missing primary labels.
  2. Spot-check on Narrator (Windows) and Orca (Linux).
     Expected: Core navigation and announcements remain usable; document gaps for v1.5 audit.
