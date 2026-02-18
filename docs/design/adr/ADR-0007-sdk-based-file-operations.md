# ADR-0007: SDK-Based File Operations with Capability Routing (Historical, Superseded)

## Status
Superseded by ADR-0008 (2026-01-31)

Historical note:
- This ADR describes an older provider-routing approach and is retained for decision history only.
- Current architecture (ADR-0008 + current PRD/design docs) uses provider-agnostic local file operations and does not define a provider-based analysis-only restriction.
- The remainder of this document intentionally records superseded assumptions and is not normative for current behavior.

## Context (Historical)
KeenBench needs to read and write office documents (docx, odt, xlsx, pptx, pdf) within the Workbench. The three supported model providers offer different native capabilities for file operations:

- **Anthropic**: Claude Agent SDK with Document Skills provides native file read/write for office formats
- **OpenAI**: Codex SDK provides native file read/write capabilities
- **Google**: ADK provides File Search for reading, but lacks native file write/edit support

This capability asymmetry means that when Google/Gemini is the primary model, file editing operations cannot be performed natively. We need a strategy that:
1. Leverages provider SDKs rather than building custom file tools
2. Handles capability gaps gracefully
3. Maintains transparency about which model is performing operations

## Decision (Historical)
Use provider SDKs (Claude Agent SDK, OpenAI Codex SDK, Google ADK) for file operations with **capability routing** to handle provider asymmetry:

1. **SDK Abstraction Layer**: The Go engine wraps each provider's SDK behind a unified file operations interface.

2. **Capability Routing**: When the primary model lacks native file write capability (currently Google), file write operations are transparently delegated to a secondary model that has this capability.
   - Secondary model selection priority: Anthropic (preferred) > OpenAI
   - If no secondary model is configured, file editing is unavailable (historical behavior: Workshop analysis/Q&A only)

3. **Primary Model for Reasoning**: The user-selected model always handles reasoning, planning, and decision-making. Only the physical file operation is delegated.

4. **Transparency**: The system logs which model performed file operations in the audit trail. Users are informed in Settings that Google requires a secondary provider for file editing.

## Consequences (Historical)

### Positive
- **Pre-built tools**: Document Skills (Anthropic) and Codex tools (OpenAI) handle office format complexity (parsing, formatting preservation, TOC, styles)
- **Maintained by providers**: SDK updates handle format changes, edge cases, and improvements
- **Lazy loading**: SDKs implement intelligent loading for large files (chunk loading, TOC extraction)
- **Context management**: SDKs manage token budgets when processing large documents
- **Simpler engine code**: No need to implement docx/xlsx/pptx parsing and generation
- (Best-effort) `.odt` support may require conversion, but is still simpler than full bespoke document tooling.

### Negative
- **External dependency**: Reliance on provider SDK availability and stability
- **Capability asymmetry**: Google users need a secondary provider for full functionality
- **Coordination complexity**: File operation routing adds orchestration logic
- **Sandbox mapping**: Must map SDK sandbox semantics to Draft/Published model
- **Cost**: File operations via secondary model incur additional API costs

## Alternatives Considered (Historical)

### Alternative A: Custom File Tools (Build Our Own)
Build file read/write tools using libraries like Apache POI (Java), python-docx, openpyxl.

**Rejected because:**
- Significant implementation effort for office format handling
- Ongoing maintenance burden for format edge cases
- Would need to solve context management (large files) ourselves
- Duplicates work already done by provider SDKs

### Alternative B: Always Use a File-Capable Model
Force all file operations through Anthropic or OpenAI, regardless of primary model.

**Rejected because:**
- Undermines multi-model choice (user selected Google for a reason)
- Loses reasoning continuity (file decisions made by different model)
- More expensive (all file ops incur secondary model costs)
- Capability routing achieves similar result with better UX

### Alternative C: Google Read-Only, No Routing
Simply disable file editing when Google is primary, with no fallback.

**Rejected because:**
- Too limiting for users who want Gemini's context window advantages
- Doesn't leverage the multi-provider architecture
- Poor UX when user expects full functionality

## References
- PRD links:
  - `docs/prd/keenbench-prd.md` (FR7)
  - `docs/prd/capabilities/multi-model.md`
  - `docs/prd/capabilities/file-operations.md`
- Design links:
  - `docs/design/capabilities/file-operations.md`
  - `docs/design/capabilities/multi-model.md`
