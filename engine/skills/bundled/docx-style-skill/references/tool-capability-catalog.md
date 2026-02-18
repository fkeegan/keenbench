# DOCX Tool Capability Catalog

Supported operations with style parameters:
- `set_paragraphs`: each paragraph supports:
  - `runs[]` with run-level keys: `text`, `font_name`, `font_size`, `bold`, `italic`, `underline`, `font_color`, `highlight_color`
  - paragraph keys: `style`, `alignment`, `space_before`, `space_after`, `line_spacing`, `indent_left`, `indent_right`, `indent_first_line`
  - when `runs` is present, plain `text` is ignored.
- `append_paragraph`: same `runs` and paragraph keys as `set_paragraphs`.

Rules:
- Use only style keys supported by the worker.
- Ignore unsupported keys rather than inventing schema extensions.
