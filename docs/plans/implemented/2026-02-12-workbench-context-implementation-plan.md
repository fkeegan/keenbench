# Implementation Plan: Workbench Context

## Status
Completed (2026-02-12)

## Source Docs
- `docs/prd/capabilities/workbench-context.md`
- `docs/design/capabilities/workbench-context.md`
- `docs/prd/keenbench-prd.md`
- `docs/design/design.md`
- Agent Skills open standard: `https://agentskills.io/specification`

## Objective
Implement Workbench-level persistent context with four fixed categories (company, department, situation, document style), including synchronous processing, direct editing, always-inject runtime prompt integration, Draft blocking, and Clutter Bar impact.

## Locked Decisions
1. One context item per category (max 4 total).
2. Processing model call is synchronous and uses active Workbench model/provider path.
3. Skill categories must generate Agent Skills artifacts (`SKILL.md` + references), and situation uses `context.md`.
4. Runtime uses always-inject for all active context artifacts.
5. Direct edit path is intentionally permissive (no hard skill validation gate).
6. All context mutations are blocked while Draft exists.

## Implementation Plan

### 1. Engine RPC + Storage
1. Add RPC handlers:
   - `ContextList`
   - `ContextGet`
   - `ContextProcess`
   - `ContextGetArtifact`
   - `ContextUpdateDirect`
   - `ContextDelete`
2. Register RPC methods in engine bootstrap (`engine/cmd/keenbench-engine/main.go`).
3. Persist artifacts under:
   - `workbenches/<id>/meta/context/<category>/`
   - `source.json`
   - `source_file/` for file-mode source copy
4. Enforce category/path validation and traversal-safe writes.

### 2. Processing + Validation
1. Implement category-specific processing prompts and JSON artifact contract.
2. Implement single repair retry when model output fails hard validation.
3. Validate skill hard requirements for processed output:
   - `SKILL.md` required for skill categories
   - YAML frontmatter present
   - required `name` and `description`
   - `name` constraints + category match
   - referenced files exist and are path-safe
4. Implement file-mode extraction reuse via existing pyworker pipeline with context-specific staging.
5. Preserve raw source metadata and source file copy.

### 3. Runtime Injection + Clutter
1. Build context injection block from active artifacts:
   - `<workbench-situation>`
   - `<workbench-skill name="...">` with inlined references
2. Inject Workbench context into:
   - Workshop chat system prompt
   - agent/tools system prompt path
   - proposal prompt path
3. Include context token estimate in Clutter computation and events:
   - `context_items_weight`
   - `context_share`
   - `context_warning` (`>= 0.35`)

### 4. Flutter State + UI
1. Add context models and clutter fields in `app/lib/models/models.dart`.
2. Add Workbench state methods for context RPC CRUD/process/artifact fetch.
3. Subscribe to `ContextChanged` notifications and refresh context + clutter.
4. Add Workbench chrome entry action (`Add Context`) and context warning text.
5. Implement context overview screen:
   - four category cards
   - add/reprocess flow (text or file+note)
   - retry/cancel on process failure
   - inspect view with direct-edit toggle/save
   - overwrite warning before reprocess when manually edited
   - delete action
6. Keep context actions disabled while Draft exists with consistent tooltip language.

### 5. Test Plan
1. Engine tests:
   - context list/get/direct-edit/delete lifecycle
   - draft-blocked mutation checks
   - runtime injection presence in chat/proposal paths
   - clutter context-share warning behavior
2. Flutter widget tests:
   - context action opens context overview
   - high-context clutter warning renders
3. Full verification:
   - `cd engine && go test ./...`
   - `cd app && flutter test`

## Risks + Mitigations
1. Risk: context route missing `WorkbenchState` provider.
   - Mitigation: pass existing `WorkbenchState` into pushed context route via `ChangeNotifierProvider.value`.
2. Risk: malformed direct-edit artifacts.
   - Mitigation: direct-edit path remains permissive by design; reprocess path retains hard validation and repair retry.
3. Risk: oversized context crowding prompt budget.
   - Mitigation: Clutter includes runtime context share and exposes warning at `>= 35%`.

## Final Implementation-vs-Plan Audit
1. Engine context RPC surface: shipped.
2. Context storage layout + source metadata/file copy: shipped.
3. Processing pipeline + repair retry + skill hard validation: shipped.
4. Always-inject runtime context integration: shipped.
5. Clutter context metrics and warning: shipped.
6. Flutter Workbench context UX (add/edit/inspect/direct-edit/delete): shipped.
7. Draft blocking behavior for context mutations: shipped.
8. Automated tests for new invariants: shipped.
