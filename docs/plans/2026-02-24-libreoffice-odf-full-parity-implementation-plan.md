# LibreOffice ODF Full-Parity Implementation Plan

## Status
Completed (2026-02-24)

## Summary
Bring OpenDocument formats to parity with existing Office support across worker capabilities, engine proposal/apply/review flows, and Flutter review/workbench UI handling.

Target parity:
- ODT parity with DOCX flows.
- ODS parity with XLSX flows.
- ODP parity with PPTX flows.

## Goal
Deliver explicit ODF method/tool coverage so ODF files can be read, mapped, previewed, diffed, styled, asset-copied, and edited through the same end-to-end pathways currently used by OOXML formats.

## Public Interface Changes
1. Workbench file kinds and MIME detection:
- Add `ods` and `odp` file kinds.
- Classify `.ods` and `.odp` with OpenDocument MIME types.

2. Worker JSON-RPC methods:
- ODT: `OdtApplyOps`, `OdtGetStyles`, `OdtCopyAssets`, `OdtGetMap`, `OdtGetSectionContent`.
- ODS: `OdsApplyOps`, `OdsExtractText`, `OdsGetInfo`, `OdsReadRange`, `OdsRenderGrid`, `OdsGetMap`, `OdsGetStyles`, `OdsCopyAssets`.
- ODP: `OdpApplyOps`, `OdpExtractText`, `OdpRenderSlide`, `OdpGetMap`, `OdpGetStyles`, `OdpCopyAssets`, `OdpGetSlideContent`.

3. Engine review JSON-RPC methods:
- `ReviewGetOdsPreviewGrid`.
- `ReviewGetOdpPreviewSlide`.
- `ReviewGetOdtContentDiff`.
- `ReviewGetOdpContentDiff`.

4. Workshop tool APIs:
- Add explicit tool names:
  - `odt_operations`, `ods_operations`, `odp_operations`
  - `odt_get_styles`, `ods_get_styles`, `odp_get_styles`
  - `odt_copy_assets`, `ods_copy_assets`, `odp_copy_assets`

## Implementation Slices
1. Worker capability parity
- Add generic LibreOffice conversion helpers and ODF conversion-backed delegation.
- Register new ODF methods in `METHODS` with correct read/write mode.

2. Workbench and engine type/routing parity
- Add ODS/ODP kinds and MIME coverage.
- Extend proposal validation and apply dispatch to allow ODF ops.
- Extend extraction/map/context/review switches to include ODS/ODP and structured ODT/ODP content diff paths.

3. Workshop tools parity
- Add ODF operations/style/copy tool definitions, execute routing, validators, and worker calls.
- Ensure read/map/info paths recognize ODS/ODP and structured ODT map.

4. Flutter review/workbench parity
- Route ODS preview grid and ODP slide preview.
- Add structured diff path for ODT/ODP where available.
- Remove ODT read-only treatment and add ODS/ODP tool-state labels.

5. Test and docs parity
- Add/update unit tests for proposal validation, apply dispatch, workshop tool routing, and review RPCs.
- Update test plan docs from ODT read-only assumptions to ODF parity expectations.

## Test Cases
1. Worker compile check
- `python3 -m py_compile engine/tools/pyworker/worker.py`

2. Engine tests (targeted and full package)
- Proposal validation accepts/rejects ODF op kinds correctly.
- Apply dispatch calls `OdtApplyOps`/`OdsApplyOps`/`OdpApplyOps`.
- Review RPCs for ODS/ODP and structured ODT/ODP return expected shapes.

3. Flutter tests
- Review screen chooses ODS/ODP RPCs correctly.
- Workbench file rows no longer classify ODT as read-only.

## Assumptions and Defaults
1. ODF write/style/asset fidelity is implemented via conversion-backed delegation to existing OOXML logic.
2. Existing OOXML RPC/tool names remain backward-compatible.
3. ADR-0006 error taxonomy and worker error mapping behavior are preserved.

## Implementation Outcome
Delivered end-to-end ODF parity across worker, engine, workshop tools, context/style skill selection, and Flutter review/workbench UI.

Implemented artifacts:
- Worker: added ODT/ODS/ODP apply/read/map/preview/style/asset methods with conversion-backed delegation.
- Engine: added ODS/ODP file kinds, proposal/apply validation/dispatch parity, extraction/map routing parity, and ODF review RPCs.
- Workshop tools: added ODF operations/style/copy tools and file read/map/info parity paths.
- Context: added ODF-aware style-skill routing and bundled ODF style skills.
- UI: enabled ODF review routing and removed ODT read-only treatment in Workbench.
- Tests: added targeted Go and Flutter coverage for ODF proposal/tool/review paths.
- Docs: updated master and section test plans for ODF parity semantics and added ODT/ODS/ODP draft test cases.

## Step-by-Step Reflection
1. Worker slice
- Result: parity methods added and registered with correct read/write modes.
- Self-check: validated Python syntax and corrected staging path handling to avoid draft-path side effects in read delegations.

2. Engine routing slice
- Result: ODF kinds and proposal/apply/review routing paths implemented.
- Self-check: ensured all switches (extract/map/preview/content diff) were covered to avoid format-specific dead paths.

3. Workshop tools slice
- Result: explicit ODF tools shipped with handler routing and validation.
- Self-check: compared tool registration, dispatcher cases, and guidance text to prevent partial exposure (tool listed but not executable, or vice versa).

4. UI slice
- Result: ODF review/preview RPC selection and read-only badge behavior aligned with engine capabilities.
- Self-check: verified kind-gating helpers for word-processing/spreadsheet/slides so ODF follows existing OOXML UI behavior.

5. Verification and docs slice
- Result: targeted + full tests passed; docs updated to remove stale ODT read-only assumptions and include ODF parity cases.
- Self-check: reconciled delegated doc edits manually to remove over-broad wording and ensure traceability rows match concrete test cases.
