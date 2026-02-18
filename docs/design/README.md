# Design Docs

## Document Versioning
Starting now, design docs should include:
- `## Status`
- `## Version` (semantic doc version, for example `v0.3`)
- `## Last Updated` (ISO date, `YYYY-MM-DD`)

Bump version when design behavior/contracts change. Update only the date for editorial-only edits.

## How to Use This Folder
Keep design documentation split by **scope** so each doc stays small enough to read and review independently.

Recommended layers:
- **Overview**: `docs/design/design.md`
- **Per-capability designs**: `docs/design/capabilities/` (one doc per capability)
- **Architecture Decision Records (ADRs)**: `docs/design/adr/` (one doc per “one-way door” decision)

## Current Docs (Start Here)
- Overview: `docs/design/design.md`
- Test Framework: `docs/design/test-framework-design.md`
- Capabilities:
  - Workbench: `docs/design/capabilities/workbench.md`
  - Workbench Context: `docs/design/capabilities/workbench-context.md`
  - Workshop: `docs/design/capabilities/workshop.md`
  - Multi-Model: `docs/design/capabilities/multi-model.md`
  - Draft / Publish: `docs/design/capabilities/draft-publish.md`
  - Review / Diff: `docs/design/capabilities/review-diff.md`
  - Network Egress & Upload Guardrails: `docs/design/capabilities/security-egress.md`
  - Checkpoints: `docs/design/capabilities/checkpoints.md`
  - Clutter Bar: `docs/design/capabilities/clutter-bar.md`
  - Failure Modes & Recovery: `docs/design/capabilities/failure-modes.md`
  - Onboarding: `docs/design/capabilities/onboarding.md`
  - Accessibility: `docs/design/capabilities/accessibility.md`
  - File Operations (Tabular Text): `docs/design/capabilities/file-operations-tabular-text.md`
  - Document Styling: `docs/design/capabilities/document-styling.md`
- ADRs:
  - `docs/design/adr/ADR-0001-snapshot-store-for-checkpoints-and-drafts.md`
  - `docs/design/adr/ADR-0002-in-app-previews-for-review.md`
  - `docs/design/adr/ADR-0003-json-rpc-over-stdio-for-ui-engine-ipc.md`
  - `docs/design/adr/ADR-0004-encrypted-local-secrets-store-for-provider-keys.md`
  - `docs/design/adr/ADR-0005-file-operation-batches-as-atomic-unit.md`
  - `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`
  - `docs/design/adr/ADR-0007-sdk-based-file-operations.md`

## Writing Guidelines
- Prefer links to PRDs instead of repeating requirements.
- Put “why” (tradeoffs/decisions) in ADRs; put “how” (implementation) in per-capability docs.
- Keep capability docs narrowly scoped (UI + engine API + storage + edge cases).
