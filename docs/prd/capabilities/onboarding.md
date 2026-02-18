# PRD: Onboarding & First-Run Walkthrough

## Status
Draft

## Purpose
Help new users understand scope boundaries, Draft vs Published, and the review workflow before they trust AI changes.

## Scope
- In scope (v1): first-run walkthrough, sample Workbench, scope explanation, quick "run a small job" prompt.
- Out of scope: full tutorial library, advanced templates, role-based training.

## User Experience
- On first launch, offer a guided walkthrough.
- Provide a sample Workbench with example files.
- Explain: Workbench scope, Draft vs Published, and review/publish flow.
- Highlight that Workbench contains copies of files; originals are untouched.
- End with a small, low-risk job prompt to build confidence.
- If no provider is configured, prompt the user to add one to enable the small job.
- Allow skipping and re-opening from Help.

## Functional Requirements
1. First-run walkthrough appears on initial launch and can be skipped.
2. Sample Workbench is available and uses safe, non-sensitive example files.
3. Walkthrough explains: Workbench copies files, Draft vs Published, and how review works.
4. Walkthrough ends with a prompt to run a small job.
5. Walkthrough can be re-opened from Help.

## Accessibility
- Walkthrough is fully keyboard navigable.
- Screen reader labels are present for all steps and actions.

## Acceptance Criteria
- New users see a walkthrough on first run or can explicitly skip it.
- Users can open the sample Workbench and run a small job (after configuring at least one provider if needed).
- The walkthrough can be re-opened later from Help.
