# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog.

## [Unreleased]

### Added

### Changed

### Fixed

## [0.1.6] - 2026-03-10

### Changed

- Updated OpenAI model IDs to `gpt-5.4` for both the standard API and Codex endpoint (replaces deprecated `gpt-5.2` / `gpt-5.3-codex`).
- Shortened model display names in the selector dropdown (removed brand prefixes and provider parentheticals).
- Widened model selector to 280px and reduced font to 13px (`small`) per the style guide.

### Fixed

- Codex provider no longer fails with a 400 error caused by passing `gpt-4.5` (unsupported on the ChatGPT Codex backend).

## [0.1.5] - 2026-02-25

### Added

- Workbench rename flow across engine and UI.
- OS drag-and-drop file add support in the Workbench sidebar.

### Fixed

- Linux GNOME dock/taskbar app mapping now resolves KeenBench name and icon correctly for local dev runs and packaged builds.

## [0.1.1] - 2026-02-23

### Added

- Release automation now attaches Linux AppImage artifacts and SHA-256 checksums to GitHub Releases on `v*.*.*` tags.

### Changed

- Release documentation now includes binary asset verification steps for Linux AppImage releases.

## [0.1.0] - 2026-02-20

### Added

- Open-source readiness checklist (`docs/plans/implemented/2026-02-16-open-source-readiness-checklist.md`)
- OSS governance baseline docs (`LICENSE`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `SUPPORT.md`, `GOVERNANCE.md`)
- CI workflows split into default checks and secret-gated AI E2E runs

### Changed

- README updated with OSS policy, CI tiers, and contributor links

## [0.0.0] - 2026-02-16

### Added

- Initial changelog scaffold
