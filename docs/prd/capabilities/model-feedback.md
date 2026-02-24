# PRD: Model Feedback (Product-Model-Fit)

## Status
Draft

## Version
v0.1

## Last Updated
2026-02-22

## Purpose
Treat the model as an internal customer of KeenBench tooling by collecting structured post-run feedback plus objective telemetry, then turning repeated pain points into a backlog intake stream.

## Scope
- In scope (v1): opt-in feedback mode, post-run survey for `WorkshopRunAgent`, local runtime record persistence, manual export to `docs/issues/model-feedback/`.
- Out of scope (v1): automatic issue creation, autonomous prioritization, feedback collection from non-agent workshop flow, UI controls.

## Key Concepts

### Runtime Feedback Record
A local markdown artifact written per `WorkshopRunAgent` run when feedback mode is enabled. It combines:
- objective run telemetry
- model feedback survey output
- collection status/errors

### Curated Intake Doc
A repo-tracked markdown document produced by manual export from runtime records. These docs are the intake surface for human triage.

## Functional Requirements

### v1
1. Feedback mode is disabled by default.
2. Feedback mode is enabled with `KEENBENCH_MODEL_FEEDBACK=1`.
3. When enabled, each `WorkshopRunAgent` invocation produces exactly one runtime feedback record.
4. A runtime record must include:
   - model/provider ids
   - run id and timestamps
   - phase completion status
   - tool usage/failure metrics
   - draft presence
   - run error metadata when applicable
5. On successful run completion, engine requests model feedback using a markdown template with required sections:
   - `What slowed me down`
   - `Hardest tool interaction`
   - `Highest-impact tooling change`
   - `Confidence (1-5)`
   - `Additional notes`
6. If feedback model call fails, runtime record is still written with status `model_call_failed`.
7. If markdown parsing fails, runtime record is still written with status `parse_failed`.
8. Feedback collection must never block or fail the user-facing workshop run.
9. Runtime records are stored under workbench metadata (not repo docs).
10. Provide manual export command that writes curated intake docs under `docs/issues/model-feedback/<YYYY-MM-DD>/`.
11. Export must support dedupe by source runtime record id.

## Non-Functional Requirements

1. No additional egress destinations beyond configured model providers.
2. No secrets are written to runtime feedback artifacts.
3. Runtime collection overhead should be bounded and non-disruptive.
4. Export output is deterministic for stable triage workflows.

## Failure Modes & Recovery

- Feedback model call fails: keep run successful, persist status-bearing runtime record.
- Feedback parse fails: keep run successful, persist raw feedback and parse error.
- Runtime storage write fails: log warning; do not fail workshop run.
- Export read/write errors: fail command with explicit error and leave existing docs unchanged.

## Success Criteria

- Collection coverage: >=95% of enabled `WorkshopRunAgent` runs produce a runtime record.
- Parsing success improves over time and is measurable via status counts.
- Repeated model pain points are visible in curated intake docs and used in backlog triage.

## Related Docs
- `docs/design/capabilities/model-feedback.md`
- `docs/prd/capabilities/workshop.md`
- `docs/design/capabilities/workshop.md`
- `docs/plans/2026-02-22-model-feedback-product-fit-implementation-plan.md`
