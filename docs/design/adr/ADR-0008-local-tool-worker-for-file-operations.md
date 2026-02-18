# ADR-0008: Local Tool Worker for File Operations

## Status
Accepted (2026-01-31)

## Context
KeenBench must read and write office files inside the Workbench while maintaining Draft-only safety. Remote tool containers are explicitly disallowed. Provider SDK file tooling requires uploading files to external services, which violates the local-only constraint.

We need a local execution path that:
- Operates entirely on the user's machine
- Keeps all edits confined to Workbench Draft
- Avoids fragile raw OOXML edits
- Is feasible to package for Linux first, macOS next

## Decision
Use a **local Python tool worker** for file operations:
- Engine spawns a long-lived Python worker at startup.
- JSON-RPC 2.0 over stdio for tool calls.
- Worker executes deterministic operations using mature libraries:
  - `python-docx` (docx)
  - `openpyxl` (xlsx)
  - `python-pptx` (pptx)
  - `pypdf` (pdf read-only)
- Models generate structured ops; the worker applies them to files in `draft/`.

This supersedes ADR-0007 (SDK-based file operations).

## Consequences

### Positive
- Local-only file edits; no uploads to provider tooling.
- Clear Draft boundary: worker reads/writes only in `draft/`.
- Deterministic edits using stable libraries.
- Compatible with existing JSON-RPC patterns.

### Negative
- Packaging complexity (Python runtime + native deps like lxml).
- Per-OS build artifacts required (Linux first, macOS next).
- Feature coverage depends on library capabilities.

## Alternatives Considered
- **Provider SDKs + capability routing (ADR-0007)**: rejected due to remote file tooling.
- **LibreOffice UNO**: high fidelity but heavy dependency and complex automation.
- **Java/POI or .NET/OpenXML**: viable but higher runtime and build complexity.

## References
- `docs/design/capabilities/file-operations.md`
- `docs/prd/capabilities/file-operations.md`
- `docs/design/adr/ADR-0007-sdk-based-file-operations.md`
