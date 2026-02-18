# Proposal v2 Prompting Guidance

## Purpose
Define how the model produces **Proposal schema v2** with structured ops for office files, plus
direct writes for text/code files. This guidance is used by `WorkshopProposeChanges`.
Note: in Workshop, these proposal artifacts are used by the compatibility flow when draft writes are needed. The model returns `no_changes: true` when no file edits are needed; there is no explicit user-facing “Propose changes” button.
The model must explicitly signal when **no file edits** are required.

## Output Contract (v2)
Return **only JSON** (no markdown). The object must follow:
```
{
  "schema_version": 2,
  "summary": "...",
  "no_changes": false,
  "writes": [{"path":"file.md","content":"..."}],
  "ops": [{
    "path":"report.docx",
    "kind":"docx",
    "summary":"...",
    "ops":[{"op":"set_paragraphs","paragraphs":[{"text":"...","style":"Heading1"}]}]
  }],
  "warnings":[]
}
```

### Rules
- `summary` is required and non-empty.
- If no file edits are needed, set `no_changes: true` and leave `writes`/`ops` empty.
- Otherwise **either** `writes` **or** `ops` (or both) must be present.
- **No deletes**. Do not include `delete`/`deletes` keys anywhere.
- Paths are **flat** (no folders).
- **Writes** allowed only for text/code extensions:
  `.md .txt .csv .json .xml .yaml .yml .html .js .ts .py .java .go .rb .rs .c .cpp .h .css .sql`
- **Ops** allowed only for `.docx`, `.xlsx`, `.pptx`.
  - `kind` must be one of `docx|xlsx|pptx` and match the file extension.
- Limits:
  - Max writes: **10**
  - Max ops per file: **100**
  - Max ops per proposal: **500**

## Allowed Ops

### DOCX
- `set_paragraphs`: replace body with ordered paragraphs  
  `{"op":"set_paragraphs","paragraphs":[{"text":"Summary","style":"Heading1"},{"text":"Notes","style":"Normal"}]}`
- `append_paragraph`: append paragraph  
  `{"op":"append_paragraph","text":"Next steps...","style":"Normal"}`
- `replace_text`: search/replace plain text  
  `{"op":"replace_text","search":"Old","replace":"New","match_case":false}`

### XLSX
- `ensure_sheet`: create sheet if missing  
  `{"op":"ensure_sheet","sheet":"Summary"}`
- `set_cells`: set specific cells  
  `{"op":"set_cells","sheet":"Summary","cells":[{"cell":"A1","value":"Metric","type":"string"}]}`
- `set_range`: set 2D range  
  `{"op":"set_range","sheet":"Summary","start":"A2","values":[["Q1",120],["Q2",140]]}`

### PPTX
- `add_slide`: add a slide with title/body  
  `{"op":"add_slide","layout":"title_and_content","title":"Overview","body":"Highlights"}`
- `set_slide_text`: replace title/body on existing slide  
  `{"op":"set_slide_text","index":0,"title":"Updated","body":"New body"}`
- `append_bullets`: append bullets  
  `{"op":"append_bullets","index":1,"bullets":["Point A","Point B"]}`

## Error Handling / Fallback
If unsure, prefer a **no-changes** proposal:
```
{"schema_version":2,"summary":"No draft changes.","no_changes":true,"writes":[],"ops":[],"warnings":[]}
```

## References
- `docs/design/capabilities/workshop.md`
- `docs/design/capabilities/file-operations.md`
- `docs/plans/m1-implementation-plan.md`
