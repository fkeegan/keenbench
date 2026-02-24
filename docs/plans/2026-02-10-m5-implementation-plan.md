# Implementation Plan: M5 — Style/Asset Preservation + High-Fidelity PPTX Review Layout

## Status
Planned (not started, updated 2026-02-10)

## Goal
Add deterministic style/asset preservation capabilities for office derivative workflows and deliver full-positioned PPTX review rendering so the system can preserve and present visual structure with higher fidelity.

---

## Key References
- `docs/design/capabilities/file-operations.md`
- `docs/design/capabilities/workshop.md`
- `docs/design/design.md`
- `docs/design/adr/ADR-0008-local-tool-worker-for-file-operations.md`
- `docs/design/adr/ADR-0006-structured-error-codes-and-failure-taxonomy.md`
- `docs/test/test-plan.md`

---

## Current State Audit (Codebase)

| Area | Current State | Gap vs Target |
|---|---|---|
| Style/asset worker RPCs | `engine/tools/pyworker/worker.py` does not expose `*GetStyles` or `*CopyAssets` methods. | Entire style/asset introspection and copy surface is still missing. |
| Structured review RPCs | Engine does not expose `ReviewGetPptxContentDiff` (or positioned slide variants). | No structured PPTX review content contract exists yet. |
| Worker slide-content extraction | Worker has `PptxGetMap` + text extraction, but no positioned slide-content payload API for review rendering. | Missing geometry/z-order/text-run payloads required for positioned canvas rendering. |
| Review UI rendering path | `app/lib/screens/review_screen.dart` uses image previews for pptx/docx and grid preview for xlsx; no positioned slide canvas. | No high-fidelity structured slide renderer in UI yet. |
| Font resolution strategy | No explicit Review-side font resolution policy for positioned PPTX rendering. | Missing deterministic bundled/OS/fallback font pipeline and missing-font signaling. |
| Test coverage | No M5-specific tests for style/asset copy or positioned PPTX rendering behavior. | Add worker/engine/integration coverage before rollout. |

### Dependencies from M4
1. M4 Phase 4 (DOCX/PPTX structured fallback) should land first to establish baseline structured review contracts and fallback behavior.
2. M4 real-model test-policy cleanup should be in place before adding new AI-path coverage in M5.

---

## Scope

### In Scope
1. Style-introspection worker RPCs for xlsx/docx/pptx.
2. Asset-copy worker RPCs for xlsx/docx/pptx.
3. Workshop tool surfaces for explicit style/asset query and copy actions.
4. Validation and structured error mapping for unsupported assets/operations.
5. Upgrade PPTX review payloads from M4 layout-lite to full positioned slide content (absolute geometry + z-order + richer typography metadata).
6. Render side-by-side positioned PPTX slides in Review with deterministic scaling and graceful font fallback behavior.
7. Real-model AI-path tests for derivative workflows that preserve formatting/assets.

### Out of Scope
1. Full-fidelity clone of every office feature (animations/macros/exotic chart variants).
2. Remote rendering/editing backends.
3. Non-office formats.
4. Guaranteeing exact cross-platform font raster equivalence for every font family.

---

## Decisions Locked for M5
1. Preserve style/asset operations as explicit tools (`*GetStyles`, `*CopyAssets`), not implicit behavior in generic write operations.
2. Asset-copy operations are best-effort and must fail with structured errors when unsupported; no silent partial success.
3. Copy operations remain Draft-only and inherit existing sandbox guarantees.
4. Tests assert structural fidelity outcomes, not exact generated prose.
5. M5 upgrades PPTX review from M4 text/layout-lite payloads to positioned slide payloads while preserving M4 fallback behavior.
6. Font matching is best-effort: prefer bundled fonts, then OS fonts, then deterministic fallback families with explicit metadata flags.
7. Baseline comparison semantics for structured review stay aligned with existing draft-baseline behavior from M4.

---

## API / Contract Changes

### Engine Internal RPC to Pyworker
1. `XlsxGetStyles(path, sheet?)`
2. `XlsxCopyAssets(source_path, target_path, assets[])`
3. `DocxGetStyles(path)`
4. `DocxCopyAssets(source_path, target_path, assets[])`
5. `PptxGetStyles(path)`
6. `PptxCopyAssets(source_path, target_path, assets[])`
7. `PptxGetSlideContent(path, slide_index, detail="positioned")` returning normalized slide-space geometry and run-level style metadata.

### Workshop Tool Contracts
1. Add style query tools that return normalized descriptors.
2. Add asset copy tools that accept explicit asset lists and return operation summaries.

### Review Contracts
1. Extend `ReviewGetPptxContentDiff` payload to include positioned shapes (x/y/w/h, z-order, rotation where available), text frames/runs, and image refs.
2. Keep `ReviewGetPptxContentDiff` backward compatible with M4 layout-lite payload fields.
3. Retain fallback chain from M4 when positioned extraction/rendering is unavailable.

---

## Implementation Plan (By Area)

### Phase 1: Pyworker Style/Asset + Positioned Slide Methods
Files:
- `engine/tools/pyworker/worker.py`

Status: Pending.

Tasks:
1. Implement `*GetStyles` methods with normalized JSON descriptors.
2. Implement `*CopyAssets` methods for supported asset classes per format.
3. Implement positioned `PptxGetSlideContent` extraction with deterministic shape ordering and normalized coordinates.
4. Include run-level typography metadata sufficient for Review-side rendering choices.
5. Add deterministic response envelopes and structured failures.

Acceptance:
1. Worker methods return stable descriptors for the same file.
2. Unsupported asset types return structured errors without corrupting target files.
3. Positioned slide extraction is deterministic across repeated runs on the same input.

### Phase 2: Engine Tool Surfaces and Validation
Files:
- `engine/internal/engine/workshop_tools.go`
- `engine/internal/engine/engine.go`

Status: Pending.

Tasks:
1. Register new tool schemas for style query/copy operations.
2. Route calls through tool handler with argument validation.
3. Map worker errors to ADR-0006 error codes and actions.
4. Upgrade `ReviewGetPptxContentDiff` to consume positioned slide payloads while preserving M4-compatible fields.
5. Preserve baseline semantics and `baseline_missing` behavior introduced in M4.

Acceptance:
1. New tools are callable from Workshop and bounded to Draft.
2. Error handling is consistent with existing taxonomy.
3. `ReviewGetPptxContentDiff` remains backward compatible for clients expecting M4 payload shape.

### Phase 3: Review UI Positioned PPTX Canvas
Files:
- `app/lib/screens/review_screen.dart`
- `app/lib/models/models.dart`

Status: Pending.

Tasks:
1. Render paired baseline/draft slides on a scaled canvas using positioned shape payloads.
2. Apply deterministic font resolution strategy (bundled -> OS -> fallback) and expose missing-font indicator where needed.
3. Preserve existing fallback path (layout-lite/text diff/metadata) when positioned render data is incomplete.

Acceptance:
1. PPTX review supports side-by-side positioned rendering without external raster renderer dependency.
2. Missing fonts or unsupported shapes degrade gracefully without breaking review flow.

### Phase 4: Workshop UX and Prompt Guidance
Files:
- `engine/internal/engine/workshop_tools.go`
- `app/lib/state/workbench_state.dart`
- `app/lib/screens/workbench_screen.dart`

Status: Pending.

Tasks:
1. Update system guidance so the model prefers style/asset tools for derivative fidelity.
2. Surface non-blocking progress for style/asset operations in Workshop.

Acceptance:
1. Agent traces show explicit style/asset tool usage for derivative tasks.
2. User can see operation progress during long copy tasks.

### Phase 5: Tests and Validation
Files (non-exhaustive):
- `engine/internal/engine/*_test.go`
- `engine/internal/toolworker/*_test.go`
- `app/integration_test/*.dart`

Status: Pending.

Tasks:
1. Add worker + engine tests for style descriptors and asset copy behavior.
2. Add worker + engine tests for positioned PPTX payload extraction and compatibility fallback behavior.
3. Add integration tests for derivative creation preserving styles/assets.
4. Add Review integration tests for positioned PPTX rendering with graceful degradation on missing fonts/shapes.
5. Run AI-path tests with real models only; keep assertions structural.

Acceptance:
1. Derivative files preserve targeted style/asset invariants in supported scenarios.
2. Positioned PPTX review rendering is stable across fixture runs.
3. AI-path tests comply with real-model-only policy.

---

## Acceptance Criteria (M5)
| Criterion | Status (2026-02-10) | Notes |
|---|---|---|
| Agent can query style/asset metadata for xlsx/docx/pptx files. | Pending | `*GetStyles` RPCs not implemented yet. |
| Agent can copy supported styles/assets into derivative office files in Draft. | Pending | `*CopyAssets` RPCs not implemented yet. |
| Unsupported copy requests fail with explicit structured errors. | Pending | Depends on `*CopyAssets` implementation and error mapping. |
| PPTX review supports positioned side-by-side rendering with deterministic fallback behavior. | Pending | Requires new worker payload + engine RPC + Review UI canvas path. |
| End-to-end derivative workflows show measurable style/asset preservation improvements. | Pending | Requires feature implementation and integration tests. |

---

## Risks and Mitigations
1. Risk: format-specific edge cases produce partial fidelity.
   - Mitigation: explicit capability boundaries and fail-fast errors per asset class.
2. Risk: high complexity in cross-format asset behavior.
   - Mitigation: limit v1 support matrix and expand iteratively behind deterministic tests.
3. Risk: test flakiness in AI-driven derivative tasks.
   - Mitigation: structural assertions and fixed fixture corpus with known invariants.
4. Risk: cross-platform font availability differences affect positioned PPTX text appearance.
   - Mitigation: explicit font resolution order, missing-font indicators, and fixture tests that assert structure/geometry rather than pixel-perfect glyph rasterization.
