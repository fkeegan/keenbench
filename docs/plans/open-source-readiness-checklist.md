# Open Source Readiness Checklist

## Status
Completed (2026-02-16)

## Objective
Open-source `keenbench` with an MIT license while preserving a clean path for a separate proprietary enterprise offering in another repo.

## Scope
- In scope: legal baseline, repo/document hygiene, CI/test policy, contribution model, release process.
- Out of scope: feature roadmap changes and large refactors unrelated to OSS publication.

## Phase 1: Legal + Licensing
- [x] Add `LICENSE` (MIT) at repo root.
- [x] Add `NOTICE` only if needed by included dependencies or bundled assets.
- [x] Add a short trademark statement in `README.md` (code is MIT; project/product name and logos remain protected).
- [x] Record enterprise split rule in docs:
  - MIT repo contains only OSS features.
  - Enterprise features live in separate private repo with separate commercial terms.
  - No shared private code copied back into OSS repo unless intentionally relicensed.

## Phase 2: Security + Repo Hygiene
- [x] Rotate and remove any accidentally committed credentials (none detected in scan; rotate externally if any prior exposure is known).
- [x] Run a secret scan on full history (`gitleaks` or equivalent).
- [x] Keep `.env` out of git-tracked secrets; provide `.env.example` with required variable names only.
- [x] Review for internal URLs, internal org names, or private incident references.
- [x] Confirm no private datasets/sample files are published.

## Phase 3: Documentation Triage
- [x] Keep and polish public entry docs:
  - `README.md`
  - `docs/design/style-guide.md`
  - `docs/test/test-plan.md` (with real-model testing policy notes)
- [x] Review each docs area and choose `keep public`, `redact`, or `remove`:
  - `docs/design/`
  - `docs/plans/`
  - `docs/prd/`
  - `docs/issues/`
  - `docs/research/`
- [x] Remove or redact any doc that exposes non-public strategy or sensitive operations.
- [x] Ensure no doc points contributors to off-limits/private material.

## Phase 4: Community + Governance Files
- [x] Add `CONTRIBUTING.md` (setup, test matrix, PR expectations).
- [x] Add `CODE_OF_CONDUCT.md`.
- [x] Add `SECURITY.md` (vulnerability reporting path and response SLA).
- [x] Add issue templates (bug, feature) and PR template under `.github/`.
- [x] Define maintainer review/merge rules (who can approve, branch protections).

## Phase 5: CI and Testing Strategy
- [x] Add CI workflow for fast checks on every PR:
  - formatting/lint checks
  - Go unit tests + coverage gate
  - Flutter unit/widget tests
- [x] Split AI-dependent tests into dedicated workflows that require configured secrets/API keys.
- [x] Document exactly which jobs run by default vs. which require maintainer secrets.
- [x] Publish CI status badges in `README.md`.

## Phase 6: Releases + Operations
- [x] Adopt versioning policy (`v0.x` until API/UX stabilizes).
- [x] Add `CHANGELOG.md` and release note template.
- [x] Define release checklist (test pass, tag, artifacts, notes).
- [x] Add support guidance (`SUPPORT.md` or section in `README.md`).

## Recommended Doc Visibility Defaults
- `docs/design/`: keep mostly public; redact prompt internals if needed.
- `docs/plans/`: keep implementation plans that help contributors; redact business-sensitive details.
- `docs/prd/`: selective publish; redact market/strategy material.
- `docs/issues/`: review carefully; often contains internal context.
- `docs/research/`: selective publish; remove private references.

## Launch Sequence (Practical)
1. Finalize license + legal files.
2. Complete security/doc triage.
3. Land CI and governance files.
4. Run a full dry-run from a clean machine using only public docs.
5. Make repo public and monitor first external issues/PRs for one week.

## Evidence Links
- Audit notes: `docs/oss/open-source-audit.md`
- Doc visibility decisions: `docs/oss/doc-visibility.md`
- Governance baseline: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `GOVERNANCE.md`
- CI workflows: `.github/workflows/ci.yml`
