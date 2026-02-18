# PRD: Accessibility

## Status
Draft

## Purpose
Ensure v1 core flows are usable with assistive technologies and meet WCAG 2.1 AA targets.

## Scope
- In scope (v1): Workbench creation/add files, Workshop chat, review/publish, model selector, error states.
- In scope (v1.5): full accessibility audit with documented compliance gaps.
- Out of scope: custom OS-level accessibility tooling.

## User Experience
- All primary workflows are keyboard operable.
- Screen readers can announce status changes (job running, review ready, publish complete).
- Visual cues (color, icons) have text or ARIA equivalents.

## Functional Requirements
1. Keyboard navigation covers all primary workflows without traps.
2. Focus order is logical and visible at all times.
3. Screen reader labels exist for buttons, inputs, and review/diff controls.
4. Status updates (errors, job completion, publish success) are announced to assistive tech.
5. Color contrast meets WCAG 2.1 AA for text and UI controls.
6. Non-color indicators are present for status states (success, warning, error).

## Acceptance Criteria
- Users can complete Workbench add files and review/publish using only the keyboard.
- Screen readers can identify the current model, Draft status, and review actions.
- Contrast checks meet WCAG 2.1 AA for core UI surfaces.
