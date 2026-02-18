# Governance

This project is maintained by the KeenBench maintainers.

## Decision Model

- Day-to-day technical decisions happen in pull requests.
- Maintainers make final merge decisions after review.
- Significant design changes should reference or add an ADR in `docs/design/adr/`.

## Maintainer Responsibilities

- Review and merge pull requests.
- Enforce the security and testing policies.
- Keep release notes and changelog entries current.
- Moderate issues/discussions under the Code of Conduct.

## Merge and Branch Protection Policy

Configure repository branch protection (for `main`) with these minimum rules:

- Require pull requests before merge.
- Require at least 1 approving review.
- Require status checks to pass:
  - `go-test`
  - `flutter-test`
  - `format-check`
- Dismiss stale approvals on new commits.
- Restrict direct pushes to maintainers.

## Release Authority

- Maintainers cut releases and create tags.
- Use `RELEASING.md` for versioning and release procedure.
