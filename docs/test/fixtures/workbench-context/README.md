# Workbench Context Fixtures (Soap Opera: KeenBench)

These fixtures support manual QA for Workbench Context. The scenario is intentionally self-referential:

- **Company:** KeenBench (fictional startup)
- **Product:** KeenBench (this app)

## Cast + Departments (Fictional)

- Mira Kwon — CEO (Ops; also covers Sales/Growth leadership)
- Ethan Park — CTO (Engineering)
- Priya Desai — Sales Rep (Sales/Growth)
- Luis Alvarez — Lead Engineer (Engineering)

Departments used in context:

- Engineering: Ethan Park + Luis Alvarez
- Ops: Mira Kwon
- Sales/Growth: Mira Kwon + Priya Desai

## Files (Committed)

Text blocks to copy/paste into context:

- `keenbench_company_context_v1.txt`
- `keenbench_company_context_v2.txt`
- `keenbench_situation_alpha.txt`
- `keenbench_situation_beta.txt`
- `keenbench_document_style_v1.txt`

Workbench files used to create Drafts:

- `keenbench_weekly_notes.txt`
- `keenbench_launch_plan.csv`

## Generated Files

Some test cases require binary fixtures (DOCX/XLSX/PPTX/PDF/PNG/BIN) and large payload files.
Generate them with:

```bash
engine/tools/pyworker/.venv/bin/python scripts/testdata/generate_workbench_context_fixtures.py
```

Outputs:

- Binary fixtures written into this directory (same folder as this README).
- Large payloads written into `artifacts/testdata/` (gitignored).

