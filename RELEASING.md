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
   - `make version-check`
3. Update `CHANGELOG.md`:
   - Move relevant entries from `[Unreleased]` to the new version section.
4. Create annotated release tag:
   - `git tag -a v0.x.y -m "Release v0.x.y"`
   - `git push origin v0.x.y`
5. GitHub Actions automatically builds Linux and Windows artifacts and creates the GitHub Release from the tag.
6. Verify release assets are attached:
   - `KeenBench-linux-x86_64.AppImage` (or platform-matching arch variant)
   - `KeenBench-linux-<arch>.AppImage.sha256`
   - `KeenBench-windows-x64-setup.exe`
   - `KeenBench-windows-x64-setup.exe.sha256`
7. Verify the release notes and edit details in GitHub if needed.

## Release Notes Template

Auto-generated notes use `.github/release.yml` categories. If manual edits are needed, use this structure:

- Summary
- Highlights
- Breaking changes (if any)
- Migration notes (if any)
- Known issues
