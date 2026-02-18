# ADR-0003: JSON-RPC over stdio for UI↔Engine IPC

## Status
Accepted

## Context
KeenBench uses a Flutter desktop UI and a Go engine as **separate processes** for responsiveness and isolation (`docs/design/design.md`). The UI must send commands to the engine and receive:
- Request/response results (create Workbench, start jobs, publish, etc.)
- Streaming/progress updates (token streaming, job stage updates, diff/preview readiness)

Constraints:
- Cross-platform packaging (macOS/Windows/Linux).
- Avoid fragile local networking assumptions (ports, firewalls, loopback restrictions).
- Keep the protocol debuggable and evolvable (versioning, backward-compat).

The primary IPC options identified in `docs/design/design.md` were:
- gRPC over loopback
- JSON-RPC over stdio

## Decision
Use **JSON-RPC 2.0 over stdio**:
- The Flutter UI spawns the Go engine as a child process.
- The UI sends JSON-RPC requests on the engine’s stdin.
- The engine writes JSON-RPC responses and notifications on stdout.
- Events/progress are delivered as JSON-RPC **notifications** (no `id`).

Framing (v1):
- Use a line-delimited JSON envelope (one JSON-RPC message per line).
- Enforce message size limits and reject malformed frames.

Versioning (v1):
- The engine exposes `EngineGetInfo() -> {engine_version, api_version}`.
- Requests include `api_version`; the engine rejects incompatible versions with a clear error.

## Consequences

### Positive
- Simple packaging: no loopback ports, no service discovery, fewer firewall issues.
- Easy to debug: plain JSON messages can be logged in debug builds.
- No codegen pipeline required for Dart/Go.
- Natural fit for event streams via notifications.

### Negative
- We lose strong typing and schema enforcement that gRPC can provide.
- Requires careful message framing, buffering, and backpressure handling.
- Larger payloads (e.g., previews) must be served via file paths/handles rather than inline JSON blobs.

## Alternatives Considered
- **gRPC over loopback**: stronger typing and tooling, but adds networking surface area and packaging complexity.
- **Embed Go engine as a library**: tighter coupling and more complex build/distribution across platforms.
- **WebSocket/HTTP local server**: similar drawbacks to loopback networking plus more moving parts.

## References
- `docs/design/design.md` (Process Boundary + IPC)
- `docs/prd/keenbench-prd.md` (safety requirements; audit trail needs streaming/progress)
