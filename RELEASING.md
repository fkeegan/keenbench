# Releasing

## Versioning Policy

- Use `v0.x.y` while APIs/workflows are still stabilizing.
- Breaking changes are allowed in `0.x`, but must be called out clearly in release notes.
- Move to `v1.0.0` once core workflows and compatibility expectations are stable.

## Release Checklist

1. Confirm branch is up to date and CI is green.
2. Run local verification:
   - `make fmt`
   - `make test`
3. Update `CHANGELOG.md`:
   - Move relevant entries from `[Unreleased]` to the new version section.
4. Create release tag:
   - `git tag v0.x.y`
   - `git push origin v0.x.y`
5. Create GitHub release using the release template.
6. Attach build artifacts (if applicable) and publish release notes.

## Release Notes Template

Use this structure in each release:

- Summary
- Highlights
- Breaking changes (if any)
- Migration notes (if any)
- Known issues
