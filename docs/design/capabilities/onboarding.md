# Design: Onboarding (First-Run Walkthrough)

## Status
Draft (v1)

## PRD References
- `docs/prd/capabilities/onboarding.md`
- `docs/prd/keenbench-prd.md` (First-Run Walkthrough)
- `docs/prd/milestones/v1.md` (first-run walkthrough acceptance criteria)
- Related:
  - `docs/design/capabilities/workbench.md` (scope + “copies; originals untouched”)
  - `docs/design/capabilities/draft-publish.md` (Draft vs Published)
  - `docs/design/capabilities/review-diff.md` (review mental model)
  - `docs/design/capabilities/multi-model.md` (provider keys)
  - `docs/design/capabilities/security-egress.md` (consent before first model call)
  - `docs/design/capabilities/accessibility.md` (walkthrough accessibility baseline)

## Summary
Onboarding is a first-run walkthrough designed to build user trust by teaching:
- the Workbench boundary (“copies; originals untouched”),
- Draft vs Published safety,
- review/publish workflow, and
- how to run a small, low-risk first job.

Key v1 choices (confirmed):
- The walkthrough appears on first launch, is skippable, and can be re-opened from Help.
- The walkthrough does **not** prompt for provider keys; keys are requested only when the user attempts their first model-powered action.
- A sample Workbench is provided using safe, bundled example files.
- Walkthrough is fully keyboard-navigable and screen reader labeled.

## Goals / Non-Goals

### Goals
- Minimize fear by making safety boundaries explicit before users run anything.
- Get users to a successful “first output” quickly (activation).
- Provide a safe sandbox to explore the UI (sample Workbench).
- Avoid adding configuration burden during onboarding.

### Non-Goals
- A full tutorial library or “learning center” in v1.
- Advanced templates, role-based training, or interactive multi-step courses.
- Forcing provider key setup before users can understand what the app does.

## User Experience

### Entry Condition
Show onboarding when:
- the app is launched and `onboarding.completed=false`, or
- the user selects “Help → Onboarding walkthrough…”

If the app is updated with major workflow changes, onboarding can re-trigger via a versioned key:
- `onboarding.last_completed_version < current_onboarding_version`.

**Version bump criteria**: Increment `current_onboarding_version` when:
- the walkthrough step sequence changes (steps added/removed/reordered),
- core safety concepts are renamed or redefined (e.g., "Draft" terminology changes), or
- a new mandatory consent or safety disclosure is introduced.

Do not bump for cosmetic changes, copy edits, or bug fixes.

### Walkthrough Structure (v1)
Use a dedicated onboarding flow (dialog or full-screen “wizard”) with 4–6 short steps:

1. **Welcome**
   - “KeenBench works inside a Workbench.”
   - Primary action: **Start walkthrough**
   - Secondary: **Skip**

2. **Scope Boundary (Workbench)**
   - Explain:
     - “AI can only access files in this Workbench.”
     - “Workbench contains copies; originals untouched.”
   - Link: “Learn more” (opens Workbench help section)

3. **Draft vs Published**
   - Simple diagram:
     - Published = final
     - Draft = where AI writes
   - Copy:
     - “Nothing changes until you Publish.”
     - “You can always Discard.”

4. **Review Before Publish**
   - Explain high-level review tiers:
     - text diffs
     - summaries + before/after previews for binaries
   - Note: “Review is offline; no extra model calls.”

5. **Create Sample Workbench**
   - Button: **Create sample Workbench**
   - On success: automatically open it (Workbench view).
   - Optional checkbox: “Open Workshop after creating sample” (default on).
   - Sample includes a small set of safe files (see “Sample Workbench Contents”).

6. **Try a Small Job**
   - Show a suggested prompt and a “Run in Workshop” button.
   - If no providers are configured:
     - show “Add a model provider to run your first job” with **Open Settings**.
     - after configuration, return to the sample Workbench with the suggested prompt pre-filled.

### Suggested First “Small Job” (v1)
The first job should be low-risk and easy to evaluate:
- Output format: a new Markdown file (e.g., `summary.md`) in Draft.
- Prompt (suggested):
  > “Create `summary.md` with: (1) a 5-bullet summary of the sample files, (2) 3 open questions, and (3) a recommended next step. Keep it under 250 words.”

This produces visible value while keeping review simple (one new file).

### Screen Reader Labels (Per Step)
Each walkthrough step must have:
- a clear heading announced on focus (e.g., "Step 2 of 6: Scope Boundary"),
- descriptive labels for all interactive elements,
- progress announced (e.g., "Step 2 of 6").

Step-specific labels:
| Step | Heading | Key Labels |
|------|---------|------------|
| Welcome | "Welcome to KeenBench" | "Start walkthrough, button", "Skip walkthrough, button" |
| Scope Boundary | "Your files stay safe in the Workbench" | "Learn more about Workbench scope, link" |
| Draft vs Published | "AI writes to Draft, you review and publish" | (informational; no interactive elements) |
| Review Before Publish | "Review changes before publishing" | (informational; no interactive elements) |
| Create Sample | "Try it with a sample Workbench" | "Create sample Workbench, button", "Open Workshop after creating, checkbox, checked" |
| Try a Small Job | "Run your first job" | "Run in Workshop, button", "Open Settings, button" (conditional) |

### Skip / Reopen
- If the user skips, onboarding is marked as dismissed but can be re-opened from Help.
- If the user completes all steps, onboarding is marked completed.

## Architecture

### UI Responsibilities (Flutter)
- Detect first run / onboarding version and drive the walkthrough UI.
- Create the sample Workbench on user action and open it.
- Provide the suggested first prompt and route into Workshop with pre-filled input.
- Handle the "no providers configured" gate by routing to Settings and back.
- Ensure onboarding screens are accessible (keyboard + screen reader).
- When routing to Settings from the walkthrough, persist walkthrough state (current step, sample Workbench ID if created) in UI session state.
- On return from Settings, restore walkthrough state and pre-fill the suggested prompt in the Workshop composer.
- If the user navigates away from the walkthrough entirely (closes app, opens a different Workbench), walkthrough state is discarded and onboarding resumes from step 1 on next trigger.

### Engine Responsibilities (Go)
- Provide global settings persistence (including onboarding flags).
- Create Workbenches and add files (reused from Workbench capability).
- Enforce all usual Workbench invariants even for sample content (sandbox, limits).

### IPC / API Surface
Onboarding can be UI-owned, but engine-backed settings keep state consistent across platforms.

**Commands**
- `SettingsGet() -> {settings}` (includes onboarding flags)
- `SettingsSetOnboarding({completed, dismissed, version}) -> {}`

Sample Workbench creation uses existing Workbench APIs:
- `WorkbenchCreate(name) -> {workbench_id}`
- `WorkbenchFilesAdd(workbench_id, source_paths[]) -> {add_results}`

## Data & Storage

### Global Settings
Store onboarding state in the engine-managed settings file (outside Workbenches):
`settings.json` (conceptual)
- `onboarding`: `{completed, dismissed, last_completed_version, completed_at?}`

### Sample Workbench Assets
Sample files are bundled with the app and materialized locally at creation time.

Recommended approach (v1):
- Bundle sample files as **Flutter assets**.
- UI writes them to a temp directory, then calls `WorkbenchFilesAdd` with those paths.

Rationale:
- avoids shipping duplicate asset systems in both UI and engine,
- keeps the Workbench add path consistent (same validations).

### Sample Workbench Contents (v1)
Keep the sample small and text-first so first-time review is straightforward, but include a tiny CSV to demonstrate "real inputs":
- `README.md`: explains that the Workbench contains copies and this is safe sample content.
- `brief.md`: a 1-page problem statement + goal + constraints.
- `notes.md`: messy bullet notes that benefit from summarization.
- `data.csv`: a small table (e.g., 10–20 rows) with a couple of columns to reference in the summary.

The sample Workbench is named **"Sample Workbench"** (not user-configurable). If a Workbench with this name already exists, append a numeric suffix (e.g., "Sample Workbench 2").

## Error Handling & Recovery
- Sample creation fails (disk full / permission):
  - show an actionable error: "Couldn't create sample Workbench: [reason]. You can continue without it or try again."
  - offer two actions: **Try again** and **Continue without sample**.
  - if user continues, skip to step 6 ("Try a Small Job") with a modified message: "Create a new Workbench to get started" and a **Create Workbench** button instead of the sample-specific prompt.
- User tries "Run small job" without providers:
  - route to Settings; do not "half-start" Workshop.
- Consent required:
  - handled by `docs/design/capabilities/security-egress.md` (consent before first model call).

## Security & Privacy
- Sample Workbench contains only non-sensitive, bundled content.
- Onboarding itself makes no model calls until the user explicitly runs the small job (and consents).

## Telemetry (If Any)
v1: none.

Future (opt-in):
- onboarding completion rate
- activation funnel (“created workbench + ran job within 10 minutes”)

## Open Questions
- None currently.

## Self-Review (Design Sanity)
- Matches PRD: first-run walkthrough, sample Workbench, scope + Draft vs Published explanation, small job prompt, skip + reopen.
- Avoids configuration fatigue by deferring provider keys until the user chooses to run a job.
- Keeps the sample workflow low risk (adds a single Markdown file) and easy to review.
- Reuses existing add/sandboxing paths so onboarding doesn’t become a special security case.
