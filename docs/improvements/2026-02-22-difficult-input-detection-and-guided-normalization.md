# Improvement Plan: Difficult Input Detection + Guided Normalization

## Status
Draft (2026-02-22)

## Goal
Reduce analysis failures caused by poor/flattened/ambiguous source files by detecting difficult inputs early, surfacing clear user guidance, and enabling explicit normalization workflows.

This applies to CSV/XLSX first, but the design must generalize to other file types (DOCX/PDF/PPTX/text) where extraction quality can degrade model decisions.

---

## Key References (Impacted Design)
- `engine/internal/engine/workshop_tools.go`
- `engine/internal/engine/engine.go`
- `engine/tools/pyworker/worker.py`
- `docs/improvements/2026-02-22-xlsx-mixed-sql-file-read-improvement.md`
- `docs/issues/2026-02-22-rpi-research-recall-tool-result-payload-blowup.md`
- `docs/plans/2026-02-05-m2-improvements.md` (format/style reference)

---

## Scope

### In Scope
1. Add a cross-format difficult-input detector that computes quality signals and severity at map/describe time.
2. Add user-facing guidance when source quality is likely to cause high ambiguity or token/tool churn.
3. Add explicit, opt-in normalization profiles (starting with tabular/Jira-like exports) that create derivative `_normalized.*` files.
4. Preserve provenance by attaching normalization metadata and reversible mapping details.
5. Improve receipts/prompts to avoid large noisy metadata payloads when quality issues are detected.

### Out of Scope
- Silent automatic schema rewrites without user-visible signaling.
- One universal normalization strategy for all formats.
- Perfect semantic reconstruction of broken/flattened inputs.

---

## What “Done” Means (Acceptance Criteria)
1. The system can flag difficult inputs with deterministic quality metrics and severity labels.
2. For flagged files, the model receives compact actionable guidance before deep analysis begins.
3. Users can choose explicit normalization paths, and outputs are written as underscore-prefixed intermediates by default.
4. Normalization outputs include provenance metadata (`profile`, `rules_applied`, `columns_collapsed`, `lossy_fields`).
5. Tool receipts stay compact even for very wide schemas and avoid dumping full metadata when not required.
6. The same detector framework supports non-tabular formats with format-specific checks.

---

## Major Design Decisions
1. **Detect first, transform second**: quality diagnostics are always produced before normalization.
2. **No silent lossy merges**: any potentially lossy operation requires explicit profile/rule selection.
3. **Profile-based normalization**: strategy is chosen by named profile (for example `jira_flat_csv`) rather than implicit heuristics.
4. **Cross-format scorecard**: each file gets a quality summary with shared dimensions (structure, sparsity, duplication, extraction confidence).
5. **User-guided escalation**: when quality is poor, suggest better source export options and continue with best-effort only when accepted.

---

## Public API / Interface Changes
1. Extend `get_file_map` / `table_get_map` / `table_describe` outputs with `quality` block:
- `severity`: `ok | warning | high_risk`
- `signals`: list of triggered checks
- `recommendations`: compact next actions
2. Add optional `normalization_profiles` list to relevant map/describe responses.
3. Add normalization tool surface (initially tabular), for example:
- `table_normalize_export(path, profile, target_path, options)`
4. Add receipt/schema compaction behavior for wide metadata payloads.

---

## Implementation Plan (By Area)

### Engine (Go)
1. Introduce quality-aware receipt templates that summarize wide metadata instead of inlining full structures.
2. Add prompt guidance rules for flagged inputs:
- ask for/offer normalization first
- avoid repeated broad reads when severity is `high_risk`
3. Capture telemetry for detector triggers, normalization adoption, and fallback churn.

### Pyworker / Data Layer (Python)
1. Implement quality detectors for tabular files:
- duplicate-header families
- duplicate-slot ratio
- extreme width
- sparse high-cardinality field families
2. Implement first normalization profile(s) for flattened CSV exports (`jira_flat_csv`).
3. Emit provenance artifacts describing every transformation.
4. Add pluggable detector hooks for DOCX/PDF/PPTX/text extraction quality checks.

### Prompting / Tool Guidance
1. Update instructions to prefer quality assessment before heavy extraction on suspicious files.
2. Add deterministic rules for when to continue as-is versus suggest improved source files.
3. Ensure wording is explicit that normalization may be lossy and profile-dependent.

### Docs
1. Add user guidance for producing better source exports (for example Jira current-fields exports).
2. Document quality severity semantics and normalization profiles.
3. Document how intermediate underscore-prefixed files are used and cleaned up.

---

## Test Plan
1. Unit tests (Python):
- detector thresholds and signal outputs
- profile normalization behavior and provenance metadata
- failure modes on malformed inputs
2. Unit tests (Go):
- receipt compaction for flagged wide metadata
- prompt routing behavior when `quality.severity=high_risk`
3. Integration tests:
- flattened Jira CSV case: detect -> suggest -> normalize -> analyze
- non-normalized path still works with explicit warning
4. Manual validation:
- compare tool churn, payload size, and answer quality before/after detector+normalization flow

---

## Rollout / Migration Notes
1. Ship detection first behind a feature flag, then normalization profiles.
2. Keep all current tools fully functional during rollout.
3. Default to non-destructive behavior (suggestions + opt-in transforms).

---

## Open Questions
1. Which quality thresholds should be global vs profile-specific.
2. How to standardize quality scoring across non-tabular formats.
3. How to present normalization tradeoffs in UI without blocking power users.
4. Whether to auto-suggest export remediation text per source system (Jira, ServiceNow, etc.).
