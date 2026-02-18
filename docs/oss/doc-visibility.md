# Documentation Visibility Decisions

Date: 2026-02-16

## Summary

Documentation is retained publicly with selective redaction of machine-specific paths.

## Decisions by Area

- `docs/design/`: Keep public.
  - Rationale: required for architecture and UI contribution context.
- `docs/plans/`: Keep public with light hygiene redactions.
  - Rationale: useful implementation history for contributors.
- `docs/prd/`: Keep public.
  - Rationale: product intent and capability scope help external contributors.
- `docs/issues/`: Keep public with path redactions.
  - Rationale: bug archaeology is useful; local machine paths were removed.
- `docs/research/`: Keep public.
  - Rationale: informs design tradeoffs and future contributors.

## Redaction Rules Applied

- Replace host-specific absolute paths with generic placeholders.
- Keep file-level technical evidence while removing personal workstation details.

## Ongoing Rule

Before publishing new docs:

- Remove absolute user paths (`/home/<user>/...`, `/Users/<user>/...`).
- Avoid internal/private incident references.
- Avoid embedding credentials or environment-specific secrets.
