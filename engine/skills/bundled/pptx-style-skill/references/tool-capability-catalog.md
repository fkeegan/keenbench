# PPTX Tool Capability Catalog

Supported operations with style parameters:
- `add_slide`: supports `title_runs[]` and `body_runs[]` with run-level keys:
  - `text`, `font_name`, `font_size`, `bold`, `italic`, `underline`, `font_color`
  - paragraph keys on the op: `alignment`, `space_before`, `space_after`, `line_spacing`
  - when `title_runs`/`body_runs` are present, plain `title`/`body` is ignored.
- `set_slide_text`: same run and paragraph style keys as `add_slide`.
- `append_bullets`: supports paragraph keys `alignment`, `space_before`, `space_after`, `line_spacing`.

Rules:
- Keep generated style keys within the supported writer schema.
- `highlight_color` is not supported by `python-pptx` and should not be emitted.
