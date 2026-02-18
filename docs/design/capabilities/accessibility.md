# Design: Accessibility (WCAG 2.1 AA Baseline)

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/accessibility.md`
- `docs/prd/keenbench-prd.md` (Accessibility)
- `docs/prd/milestones/v1.md` (accessibility acceptance criteria)
- Related:
  - `docs/design/capabilities/workbench.md`
  - `docs/design/capabilities/workshop.md`
  - `docs/design/capabilities/review-diff.md`
  - `docs/design/capabilities/clutter-bar.md`
  - `docs/design/capabilities/onboarding.md`

## Summary
Accessibility in v1 targets WCAG 2.1 AA for core flows with a single, cross-platform standard (macOS/Windows/Linux) implemented via Flutter desktop capabilities:
- full keyboard operability (no traps),
- logical focus order + visible focus,
- screen reader semantics for controls and status changes,
- sufficient color contrast and non-color indicators for state.

v1.5 adds a full audit with documented compliance gaps.

Key v1 choices (confirmed):
- Prioritize parity across operating systems by relying on Flutter semantics and patterns that are feasible across all three platforms.
- No OS-specific “extra” features in v1; implement what can be applied consistently.

## Goals / Non-Goals

### Goals
- Ensure users can complete v1 core workflows with keyboard-only.
- Ensure screen readers can interpret and announce:
  - current model,
  - Draft status,
  - job progress, errors, and completion states.
- Ensure contrast and state indicators meet WCAG 2.1 AA.
- Ensure asynchronous UI updates (streaming, progress, publish) are announced appropriately.

### Non-Goals
- Custom OS-level accessibility tooling or plugins beyond standard Flutter support.
- Full formal audit and certification in v1 (deferred to v1.5).
- Per-OS feature matrices; v1 aims for a single bar that applies everywhere.

## User Experience (Accessibility Requirements)

### Keyboard Navigation (Core)
All primary workflows must be keyboard operable:
- Workbench create/open, add/remove files
- Workshop chat (compose, send, review/discard, undo, open review)
- Review/Diff (file list navigation, diff hunks, preview controls, deletion confirmations, publish/discard)
- Settings (provider keys, model defaults)

Rules:
- No keyboard traps (ESC closes modals; focus never disappears).
- Focus order follows visual order and remains stable.
- Focus is always visible (strong focus ring).

### Skip Links
Provide skip links at the start of the focus order for major screens:
- "Skip to main content" (bypasses navigation/sidebar)
- "Skip to composer" (Workshop: jumps to message input)
- "Skip to file list" (Review: jumps to changed files list)

Skip links are visually hidden until focused, then displayed prominently.

### Screen Readers (Semantics + Announcements)
All interactive elements have:
- role (button, text field, checkbox, list item),
- accessible name (label),
- state (selected/disabled/expanded).

Status announcements required:
- “Job running”, “Paused”, “Canceled”, “Review ready”, “Publish complete”, “Error”.
- Model change announcements in Workshop.
- Clutter Bar state changes when entering `Heavy`.

Approach:
- Use Flutter `Semantics` labels for controls.
- Use an announcement mechanism for dynamic updates (e.g., “live region”-like behavior) driven by app state changes.

### Color Contrast & Non-Color Indicators
- Meet WCAG 2.1 AA contrast for text and UI controls.
- Never rely on color alone for meaning:
  - Clutter Bar includes text labels (`Light/Moderate/Heavy`).
  - Status banners include text and an icon.
  - Errors/warnings/success states include explicit copy.

### Motion & Scaling (Reasonable Cross-Platform Defaults)
Not explicit PRD requirements, but feasible and cross-platform:
- Respect system text scaling (Flutter text scale factor) **up to 200%**.
- At high scale factors (150%+):
  - UI may reflow (e.g., toolbar items wrap, sidebars collapse).
  - Text truncation uses ellipsis with full text available via tooltip or focus.
- Above 200%, behavior is best-effort; document known issues in release notes.
- **Respect `prefers-reduced-motion`**: when the system preference is set, disable or minimize:
  - streaming text animations (show final text immediately or in larger chunks),
  - progress bar animations (use discrete steps instead of smooth transitions),
  - transition animations between screens (use instant cuts).
- Avoid critical information conveyed only via animation.

## Component-Specific Notes

### Workbench
- File list rows must expose: file name, type, size, supported/opaque, and change status (A/M/D).
- Import button is always reachable; drag-and-drop is not the only path.

### Workshop
- Transcript supports linear reading order for screen readers.
- Streaming responses:
  - avoid re-announcing the entire message on each token update,
  - announce when streaming starts ("Generating response") and ends ("Response complete").
  - **Focus behavior**: focus remains on the composer during streaming. After streaming completes, focus stays on composer (user's next action is typically to type). Screen reader users hear the completion announcement without forced focus change.
- Draft actions (Review / Publish / Discard) are reachable and clearly labeled.

### Model Selector
- Selector is a dropdown or combobox with proper ARIA role.
- Current model is announced on focus: "Current model: [model name], dropdown".
- Model options are announced with provider context: "[model name], [provider]".
- Model change triggers announcement: "Model changed to [model name]".
- Keyboard: arrow keys navigate options, Enter selects, Escape closes without change.

### Review / Diff
- File list selection changes update main pane and are announced (“Showing diff for …”).
- Diff hunks are keyboard reachable and labeled (e.g., “Hunk 2 of 5”).
- Previews (PDF/PPTX/XLSX/images) have keyboard controls for page/slide/sheet navigation and zoom.

### Onboarding
- Walkthrough is fully keyboard navigable, with clear step titles.
- “Skip” and “Finish” are always reachable.

## Architecture

### UI Responsibilities (Flutter)
- Implement semantics labels for all controls and dynamic state.
- Centralize focus management:
  - predictable focus on screen transitions,
  - restore focus after modals, publish/discard completion, and errors.
- Provide keyboard shortcuts for frequent actions (v1 recommendation):
  - focus composer
  - send
  - open review
  - publish
  - discard

### Engine Responsibilities (Go)
The engine does not manage UI semantics, but it must provide:
- stable enum values for phases/states so UI can announce meaningful changes,
- structured errors that allow UI to present consistent recovery actions (ADR-0006).

## Verification (v1)

### Testing Matrix
| Platform | Screen Reader | Priority |
|----------|---------------|----------|
| macOS | VoiceOver | Primary |
| Windows | NVDA | Primary |
| Windows | Narrator | Secondary |
| Linux | Orca | Secondary |

### Test Frequency
- **Per release**: full keyboard-only and screen reader smoke test on primary screen readers.
- **Per feature**: developer self-test with VoiceOver or NVDA before merge.

### Checklist (must pass on all platforms)
- Keyboard-only completion of:
  - Workbench add files
  - Workshop send message + review/publish/discard
  - Review + publish/discard
- Screen reader smoke test:
  - identify current model, Draft status, and primary actions
  - hear announcements for job completion and errors
  - navigate file list and diff hunks without mouse
- Contrast verification for core UI surfaces and controls (use automated tooling + spot check).

## Error Handling & Recovery (Accessibility-Specific)
- Errors must move focus to the error region (or announce it) without forcing a modal unless required.
- When an error is dismissed, focus returns to the element that triggered the action (or the nearest sensible anchor).
- **Error message format**:
  - Use a dedicated error summary region with `role="alert"` for immediate announcement.
  - For field-level errors (e.g., invalid API key), associate the error with the field via `aria-describedby`.
  - Error text must include: what went wrong and what the user can do (e.g., "Invalid API key. Check the key and try again.").

## Security & Privacy
- Screen reader labels must not include sensitive content (API keys, file contents).
- Announcements should avoid reading large file excerpts; announce file names and actions only.

## Telemetry (If Any)
v1: none.

v1.5:
- Accessibility audit report with documented gaps (no user data).
- **Gap remediation**: Critical gaps (keyboard traps, missing labels for primary actions) are fixed in v1.5. Non-critical gaps are documented and prioritized for v2.

## Open Questions
- None currently.

## Self-Review (Design Sanity)
- Matches PRD: keyboard operability, focus order/visibility, screen reader labels + announcements, contrast and non-color indicators.
- Keeps scope realistic for v1 by relying on Flutter patterns that can apply across macOS/Windows/Linux without OS-specific feature matrices.
- Integrates with other capability designs (Clutter Bar labels, transcript system events, job state announcements) rather than treating accessibility as an afterthought.
