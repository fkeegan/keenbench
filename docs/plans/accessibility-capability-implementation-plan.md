# Implementation Plan: Accessibility Capability (WCAG 2.1 AA Baseline)

## Status
In progress (started 2026-02-18)

## References
- `docs/prd/capabilities/accessibility.md`
- `docs/design/capabilities/accessibility.md`

## Scope (This Pass)
- Core v1 flows only:
  - Workbench create/add files and Workshop chat flow
  - Review/Diff publish/discard flow
  - Model selector and core error states
- Validation depth:
  - Automated widget/integration checks for keyboard/focus/semantics
  - Manual screen-reader smoke checklist updates
- Out of scope this pass:
  - Full app-wide accessibility hardening
  - Onboarding-specific accessibility implementation
  - OS-specific custom accessibility features beyond Flutter baseline

## Decisions Locked
1. Use cross-platform Flutter semantics/focus patterns only.
2. Shortcut profile (conservative):
   - Enter send / Shift+Enter newline
   - Ctrl/Cmd+L focus composer
   - Ctrl/Cmd+Enter send
   - Ctrl/Cmd+R open review
   - Ctrl/Cmd+Shift+P publish (review context)
   - Ctrl/Cmd+Shift+D discard
3. Announcements are transition-based (no per-token spam).
4. Error feedback uses an accessibility summary region plus announcements.
5. Text scaling support is clamped to 200% with best-effort layout stability.
6. Reduced-motion preference disables/minimizes non-essential UI animation.

## Public Interfaces / Types
- New app accessibility modules under `app/lib/accessibility/`:
  - `a11y_announcer.dart`
  - `a11y_focus.dart`
  - `skip_links.dart`
  - `a11y_shortcuts.dart`
- New UI keys in `app/lib/app_keys.dart` for:
  - skip links and skip targets
  - model selector semantics container
  - error summary region
  - review list/diff accessibility anchors

## Implementation Phases

### Phase 1 — Shared Infrastructure
Files:
- `app/lib/main.dart`
- `app/lib/accessibility/*`

Tasks:
1. Add announcer helper with de-duplication.
2. Add safe focus helpers and skip-link widget.
3. Add reusable shortcut intents + shortcut maps.
4. Wire app-level media behavior:
   - text scale clamp to 200%
   - disable animation tickers when system animation reduction is enabled

### Phase 2 — Workbench Accessibility
Files:
- `app/lib/screens/workbench_screen.dart`
- `app/lib/widgets/clutter_bar.dart`
- `app/lib/widgets/keenbench_app_bar.dart` (if required)
- `app/lib/app_keys.dart`

Tasks:
1. Add skip links:
   - Skip to main content
   - Skip to composer
2. Add focus targets for file list, main content, composer, model selector.
3. Add keyboard shortcuts/actions for core Workshop tasks.
4. Add semantics improvements:
   - model selector current value + provider context in options
   - file row semantic labels
   - status rows as live regions
   - clutter bar semantic description
5. Add error summary region with focus/announcement behavior.
6. Announce state transitions:
   - generating start/complete
   - tool start/complete
   - draft ready/review ready
   - model changed
   - clutter heavy transitions

### Phase 3 — Review Accessibility
Files:
- `app/lib/screens/review_screen.dart`
- `app/lib/app_keys.dart`

Tasks:
1. Add skip links:
   - Skip to file list
   - Skip to main content
2. Add focus anchors for change list and detail pane.
3. Add semantics:
   - change list items include path/type/kind/selected state
   - diff hunks announce ordinal context (`Hunk X of Y`)
   - preview navigation controls expose explicit labels/tooltips
4. Add review-level error summary + announcements.
5. Add review shortcut handling for publish/discard actions.

### Phase 4 — Contrast and Non-Color Indicators
Files:
- `app/lib/theme.dart`
- `app/lib/screens/workbench_screen.dart`
- `app/lib/screens/review_screen.dart`
- `app/lib/widgets/clutter_bar.dart`

Tasks:
1. Verify AA contrast on core surfaces and controls.
2. Keep explicit text/icon indicators for success/warning/error states.
3. Ensure focus visibility remains clear at all times.

### Phase 5 — Tests and Validation
Files:
- `app/test/accessibility_core_test.dart` (new)
- `app/test/workbench_review_checkpoint_test.dart`
- `app/test/settings_screen_test.dart` (if needed for selector semantics)
- `app/integration_test/e2e_accessibility_core_test.dart` (new, non-AI)
- `docs/test/test-plan.md`
- `docs/test/sections/*` (accessibility manual checklist)

Tasks:
1. Add widget tests for keyboard-only flow and focus traversal.
2. Add semantics assertions for model selector/status/diff regions.
3. Add error-region/focus restoration tests.
4. Add reduced-motion/text-scale behavior checks for core widgets.
5. Update manual screen-reader checklist matrix:
   - macOS VoiceOver, Windows NVDA primary
   - Windows Narrator, Linux Orca secondary

## Acceptance Criteria Mapping
- Keyboard-only completion for Workbench add files and Review publish/discard.
- Screen readers identify current model, draft state, review actions.
- Dynamic status/error announcements are emitted on state transitions.
- Core UI contrast and non-color status indicators meet WCAG 2.1 AA intent.

## Risks and Mitigations
1. Risk: Announcement noise during streaming.
   - Mitigation: De-duplicate and announce only phase/state transitions.
2. Risk: Focus loss after dialogs/errors.
   - Mitigation: Explicit focus anchors + restoration helpers.
3. Risk: Cross-platform accessibility behavior variance.
   - Mitigation: Use only Flutter-supported semantics/focus patterns.
4. Risk: Regressions in existing workflow tests.
   - Mitigation: Add focused widget tests and keep UI behavior otherwise unchanged.

## Execution Notes
- No engine API changes expected for this capability.
- Accessibility labels/announcements must avoid sensitive payloads.
- AI-policy reminder: Any AI-interaction tests must use real models only.
