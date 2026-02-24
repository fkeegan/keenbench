# AGENTS.md — `docs/plans` Guide

This file governs `docs/plans/` and all children (including `docs/plans/implemented/`).

## Current Map (2026-02-24)

Active/pending plans in `docs/plans/`:

- `2026-01-31-m1-implementation-plan.md`
- `2026-02-04-m1-agentic-fix.md`
- `2026-02-05-m2-improvements.md`
- `2026-02-10-m5-implementation-plan.md`
- `2026-02-11-m4-fixes-feb-11.md`
- `2026-02-16-m4-implementation-plan.md`
- `2026-02-18-accessibility-capability-implementation-plan.md`
- `2026-02-18-openai-rpi-reasoning-effort-settings-plan.md`
- `2026-02-18-openai-subscription-oauth-implementation-plan.md`
- `2026-02-18-rpi-agent-workflow-implementation-plan.md`
- `2026-02-22-model-feedback-product-fit-implementation-plan.md`

Implemented/completed plans in `docs/plans/implemented/`:

- `2026-01-29-m0-implementation-plan.md`
- `2026-01-30-m0-logging-observability-plan.md`
- `2026-02-05-m2-implementation-plan.md`
- `2026-02-05-m2.1-review-auto-summary-checkpoints.md`
- `2026-02-05-m2.2-workbench-extract-and-ui-polish.md`
- `2026-02-06-m3-implementation-plan.md`
- `2026-02-12-workbench-context-implementation-plan.md`
- `2026-02-16-document-styling-v1-workshop-implementation-plan.md`
- `2026-02-16-open-source-readiness-checklist.md`
- `2026-02-17-table-update-from-export-implementation-plan.md`
- `2026-02-19-mistral-provider-implementation-plan.md`

## Naming and Date Rules

- Filename pattern: `YYYY-MM-DD-<slug>.md`
- Use date only (no timestamp).
- Date source priority:
1. Explicit status date in the file.
2. Explicit date in title/body.
3. Git first-add date (`git log --diff-filter=A --follow --format=%ad --date=short -- <file>`).

## Required Plan Format

Every plan document must include:

1. `# <Plan title>`
2. `## Status`
3. A single status line in the format: `<Status> (YYYY-MM-DD)` or `<Status> (<note>, YYYY-MM-DD)`

Allowed status values:

- `Draft`
- `Planned`
- `In Progress`
- `Proposed`
- `Implemented`
- `Completed`

After `## Status`, include either `## Summary` or `## Goal` before detailed sections.

## Lifecycle Rules

- Keep non-final plans in `docs/plans/`.
- Keep final plans in `docs/plans/implemented/`.
- When a plan is finished:
1. Update status to `Implemented` or `Completed` with the completion date.
2. Move the file to `docs/plans/implemented/`.
3. Update all references to the old path.

## Reference Hygiene

After any rename/move, update links across docs:

- `rg -n "<old-plan-path>" docs README.md CHANGELOG.md`
- Replace all matches with the new path.

Do not leave stale plan-path references in docs.

## Consistency Checklist

Before finalizing docs changes in this subtree:

1. Every plan filename matches `YYYY-MM-DD-<slug>.md`.
2. Every plan file (except `README.md` and this file) has `## Status`.
3. Status matches folder location:
- `Implemented`/`Completed` in `implemented/`
- `Draft`/`Planned`/`In Progress`/`Proposed` in root
4. No references remain to old plan paths.
