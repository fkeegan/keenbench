# Model Feedback Intake

This directory stores curated intake docs exported from runtime model feedback records.

Runtime records are written by the engine (when `KEENBENCH_MODEL_FEEDBACK=1`) under:
- `workbenches/<workbench_id>/meta/workshop/model_feedback/`

Use the export command to generate intake docs:

```bash
make feedback-intake
```

Or run directly:

```bash
cd engine
 go run ./cmd/keenbench-model-feedback-export --out ../docs/issues/model-feedback
```

Useful flags:
- `--since YYYY-MM-DD`
- `--max N`
- `--force`
- `--data-dir /path/to/keenbench-data`

Each exported markdown file includes `source_record_id` so repeated exports can dedupe existing intake docs by default.
