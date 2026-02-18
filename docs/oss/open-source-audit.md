# Open Source Audit Notes (2026-02-16)

## Security and Secrets Audit

Commands run from repo root:

```bash
rg -n --hidden -I -S "(sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{36}|xox[baprs]-[A-Za-z0-9-]{20,}|AIza[0-9A-Za-z_-]{35}|AKIA[0-9A-Z]{16}|BEGIN (EC|RSA|OPENSSH|DSA)? ?PRIVATE KEY)" --glob '!.git' --glob '!engine/tools/pyworker/.venv/**'

git log --all --patch --no-color -- . ':(exclude)engine/tools/pyworker/.venv/**' | \
  rg -n "(sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{36}|xox[baprs]-[A-Za-z0-9-]{20,}|AIza[0-9A-Za-z_-]{35}|AKIA[0-9A-Z]{16}|BEGIN (EC|RSA|OPENSSH|DSA)? ?PRIVATE KEY)"
```

Result:

- No high-confidence secrets found in working tree.
- No high-confidence secrets found in scanned commit diffs.

## `.env` Handling

- `.env` is ignored in `.gitignore`.
- `.env` is not tracked.
- `.env.example` is present with variable names only.

## Docs Hygiene Actions

- Redacted machine-specific absolute paths in issue/plan docs.
- Added explicit document visibility decisions in `docs/oss/doc-visibility.md`.

## Test Data Check

- Existing fixture set reviewed under `engine/testdata/`.
- `engine/testdata/real/cuentas_octubre_2024_anonymized_draft.xlsx` is retained as anonymized real-world fixture.
- If publication risk changes, replace with synthetic fixture and update test docs.
