# PRD: Clutter Bar

## Status
Draft

## Purpose
Provide users with an intuitive visual indicator of context pressure so they understand when the Workbench is approaching capacity limits without needing to understand tokens or context windows.

## Scope
- In scope (v1): visual indicator, model-aware calculation, static thresholds based on best-guess defaults, warning messaging, context compression trigger.
- Out of scope (v1): threshold learning/tuning based on telemetry, automatic remediation suggestions.
- Future (v2+): threshold calibration based on observed behavior and telemetry.

## User Experience
- The Clutter Bar is visible in the Workbench UI (Workshop, Review/Diff, Checkpoints) but not on global screens like Settings.
- Displays a simple visual meter from "light" to "heavy."
- When clutter is high, a warning appears: "Workbench is cluttered — performance may be degraded."
- The Clutter Bar is informational only; it does not block actions.

### Visual States
| State | Visual | Meaning |
|-------|--------|---------|
| Light | Green / low fill | Plenty of context headroom |
| Moderate | Yellow / medium fill | Context is filling up |
| Heavy | Red / high fill | Near capacity; performance may degrade |

## What Contributes to Clutter

### File Count
More files = more clutter. Each file adds baseline overhead regardless of size.

### File Weight
Files are weighted by processing complexity:
| File Type | Weight Factor |
|-----------|---------------|
| Small text files (<10 KB) | Low |
| Large text files (>100 KB) | Medium |
| Documents (.docx, .odt, .pdf) | Medium-High |
| Spreadsheets (.xlsx) | High |
| Presentations (.pptx) | High |
| Images | Medium (description/OCR overhead) |
| Unsupported/binary | Low (metadata only) |

Note: Code files (.js, .py, etc.) are treated as text files for weighting purposes.

### Conversation History
Workshop conversation messages accumulate and add to clutter. Longer conversations = higher clutter.

### Context Items
Active Workbench context items (company-wide, department, situation, document style) contribute to clutter. Each context item adds a bounded, predictable weight:
| Context Type | Weight |
|--------------|--------|
| Situation (direct injection) | Low (~300 tokens) |
| Company-wide skill | Low (~500 tokens) |
| Department skill | Low (~500 tokens) |
| Document style skill | Low-Medium (~500–800 tokens) |

Context item clutter is small relative to files and conversation but is included for accuracy. See `docs/prd/capabilities/workbench-context.md`.

## Threshold Defaults (v1)

The following thresholds are best-guess starting points. They assume a typical model context window of ~128K tokens.

| Clutter Level | Threshold (approximate) |
|---------------|------------------------|
| Light | < 40% of estimated context capacity |
| Moderate | 40–70% of estimated context capacity |
| Heavy | > 70% of estimated context capacity |

**Estimation formula (simplified):**
```
clutter_score = (file_count_weight + total_file_weight + conversation_weight + context_items_weight) / model_context_estimate
```

- `file_count_weight`: base cost per file (e.g., 500 tokens per file)
- `total_file_weight`: sum of individual file weights based on type and size
- `conversation_weight`: estimated tokens from Workshop history
- `context_items_weight`: sum of active context item weights (see Context Items section above)
- `model_context_estimate`: approximate context window for the active model

**Note:** These defaults are static in v1. Calibration based on observed performance is deferred to v2+.

## Model Awareness

The Clutter Bar is **model-aware**: it uses the active model's context window size in the calculation.

- When the user switches models, the Clutter Bar recalculates immediately.
- A Workbench that is "light" with a 200K context model may become "moderate" or "heavy" with a 32K context model.
- No additional user action is required; the Clutter Bar reflects the current model's capacity.

## Context Compression

When context limits are approached, the system automatically applies **context compression** to stay within bounds.

**How it works:**
- Older conversation history is summarized to reduce token usage.
- Less-relevant file content may be summarized or excluded from active context.
- Compression is automatic and transparent to users.
- The Clutter Bar reflects the post-compression state.

**User experience:**
- Users are not prompted or interrupted when compression occurs.
- A non-blocking system note may be appended to the Workshop transcript to indicate compression occurred (no modal/prompt).
- If compression is applied, the system continues to function normally.
- The Clutter Bar may show reduced clutter after compression.

**Note:** Context compression is an implementation strategy (similar to Claude Code / Codex). The Clutter Bar abstracts this from users—they see context pressure, not compression mechanics.

## Functional Requirements

### v1
1. Clutter Bar is visible in the Workbench UI (Workshop, Review/Diff, Checkpoints).
2. Clutter score is calculated from file count, file weights, conversation history, and active context items.
3. Clutter calculation uses the active model's context window size (model-aware).
4. When the user switches models, the Clutter Bar recalculates immediately.
5. Visual state (light/moderate/heavy) is derived from the clutter score against static thresholds.
6. When clutter is heavy, display a warning message.
7. Clutter Bar is informational only; no actions are blocked.
8. Clutter updates in real-time as files are added/removed and conversation grows.
9. Context compression is applied automatically when context limits are approached.
10. The Clutter Bar reflects the post-compression state.

### Future (v2+)
11. Thresholds are calibrated based on telemetry and observed degradation patterns.

## Failure Modes & Recovery
- Clutter calculation fails: hide the bar or show "unavailable" rather than blocking.
- Model context estimate unavailable: use a conservative default estimate.

## Security & Privacy
- Clutter calculation is local; no data is sent externally.
- File contents are not transmitted for clutter estimation.

## Acceptance Criteria
- Clutter Bar is visible in the Workbench UI (Workshop, Review/Diff, Checkpoints).
- Clutter Bar updates as files are added/removed and conversation grows.
- Clutter Bar recalculates when the user switches models.
- Heavy clutter displays a warning message.
- Clutter Bar does not block any user actions.
- Visual states (light/moderate/heavy) are clearly distinguishable.
- Context compression occurs transparently without user prompts.
- Clutter Bar reflects post-compression state.

## Open Questions
None currently. Threshold calibration deferred to v2+.
