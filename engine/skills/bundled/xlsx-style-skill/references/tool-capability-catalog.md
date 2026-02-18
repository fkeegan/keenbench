# XLSX Tool Capability Catalog

Supported operations with style parameters:
- `set_cells`: per-cell style via `cells[].style` with keys:
  - `font_name`, `font_size`, `font_bold`, `font_italic`, `font_color`
  - `fill_color`, `fill_pattern`
  - `number_format`
  - `h_align`, `v_align`, `wrap_text`
  - `border_top`, `border_bottom`, `border_left`, `border_right` as objects `{style, color}`
- `set_range`: range style via top-level `style` using the same keys as `set_cells`.
- `set_column_widths`: `sheet`, `columns[]` with `{column, width}`.
- `set_row_heights`: `sheet`, `rows[]` with `{row, height}`.
- `freeze_panes`: `sheet`, `row` and `column` (0-based first unfrozen row/column).

Rules:
- Do not emit unknown style keys.
- Keep style values valid for XLSX writer semantics.
