package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
	"keenbench/engine/internal/toolworker"
	"keenbench/engine/internal/workbench"
)

// WorkshopTools defines the tools available to the model during workshop sessions.
var WorkshopTools = []llm.Tool{
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "list_files",
			Description: "List all files in the workbench with their metadata (path, type, size). Use this first to see what files are available.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "get_file_info",
			Description: "Get detailed information about a specific file. For xlsx returns sheet names and dimensions. For pdf returns page count. For images returns dimensions.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "File path in the workbench"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "get_file_map",
			Description: `Get a structural map of a file showing its internal layout without reading content. Use this before read_file to understand file structure:
- xlsx: sheets with used ranges, data islands, chunk boundaries, and flags (charts, merged cells, formulas)
- docx: sections by heading with char counts, tables, images
- pptx: slides with titles, layouts, media flags
- pdf: page count, table of contents, page chunks
- csv: tabular schema/chunk map (also available via table_get_map)
- text: line count, char count, line chunks
The map tells you what regions exist and how large they are. Then use read_file with specific coordinates to read content.`,
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "File path in the workbench"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "read_file",
			Description: `Read content from a file. For large files, use get_file_map first to see the structural map, then read specific regions:
- xlsx: specify sheet name and optional range (e.g. A1:E50)
- docx: specify section heading or index to read a specific section
- pptx: specify slide_index (0-based) to read a specific slide
- pdf: specify pages range (e.g. "1-5") to read specific pages
- text: specify line_start and line_count for large files
The map in the file context shows available regions, sizes, and chunk boundaries. Always check the map before reading to avoid requesting more data than needed.`,
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "File path in the workbench"},
					"sheet": {"type": "string", "description": "Sheet name (xlsx only)"},
					"range": {"type": "string", "description": "Cell range like A1:D100 (xlsx only, optional)"},
					"section": {"type": "string", "description": "Section heading text or index (docx only)"},
					"slide_index": {"type": "integer", "description": "Slide index, 0-based (pptx only)"},
					"pages": {"type": "string", "description": "Page range like '1-5' (pdf only)"},
					"line_start": {"type": "integer", "description": "Starting line number, 1-indexed (text only)"},
					"line_count": {"type": "integer", "description": "Number of lines to read (text only)"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_get_map",
			Description: "Get a structural map of a CSV file. Returns column names/types, row count, chunk boundaries, and encoding metadata.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "CSV file path in the workbench"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_describe",
			Description: "Get detailed per-column metadata for a CSV file, including inferred types, nullability, and distinct-value estimates.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "CSV file path in the workbench"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_stats",
			Description: "Get summary statistics for CSV columns. Use columns to scope specific fields, or omit for all columns.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "CSV file path in the workbench"},
					"columns": {
						"type": "array",
						"description": "Optional list of column names",
						"items": {"type": "string"}
					}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_read_rows",
			Description: "Read rows from a CSV file by position. Use this to browse tabular data without writing SQL.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "CSV file path in the workbench"},
					"row_start": {"type": "integer", "description": "First row to read (1-based, excluding header)"},
					"row_count": {"type": "integer", "description": "Number of rows to read"},
					"columns": {
						"type": "array",
						"description": "Optional list of columns to include",
						"items": {"type": "string"}
					}
				},
				"required": ["path", "row_start", "row_count"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_query",
			Description: "Run a read-only SQL query (DuckDB syntax) against a CSV file. Use table name `data`. Double-quote column names with spaces or special chars. Use window options for pagination.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "CSV file path in the workbench"},
					"query": {"type": "string", "description": "Read-only SQL query. Use table name data."},
					"window_rows": {"type": "integer", "description": "Optional max rows for this response window"},
					"window_offset": {"type": "integer", "description": "Optional window offset"}
				},
				"required": ["path", "query"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_export",
			Description: "Export CSV table data or query results to a stand-alone Draft file as csv or xlsx.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Source CSV file path in the workbench"},
					"query": {"type": "string", "description": "Optional read-only SQL query. If omitted, exports full table."},
					"target_path": {"type": "string", "description": "Destination path in Draft"},
					"format": {"type": "string", "enum": ["csv", "xlsx"], "description": "Export format"},
					"sheet": {"type": "string", "description": "Optional sheet name for xlsx exports"}
				},
				"required": ["path", "target_path", "format"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "table_update_from_export",
			Description: "Write CSV table data/query results into an existing Draft xlsx workbook/sheet.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Source CSV file path in the workbench"},
					"query": {"type": "string", "description": "Optional read-only SQL query. If omitted, uses full table."},
					"target_path": {"type": "string", "description": "Destination .xlsx path in Draft"},
					"sheet": {"type": "string", "description": "Target worksheet name"},
					"mode": {"type": "string", "enum": ["replace_sheet", "append_rows", "write_range"], "description": "Update mode"},
					"start_cell": {"type": "string", "description": "Optional top-left target cell for write_range mode (default A1)"},
					"include_header": {"type": "boolean", "description": "Whether to include CSV header row in written output"},
					"create_workbook_if_missing": {"type": "boolean", "description": "Create workbook if target does not exist"},
					"create_sheet_if_missing": {"type": "boolean", "description": "Create sheet if target sheet does not exist"},
					"clear_target_range": {"type": "boolean", "description": "For write_range mode, clear destination range before writing"}
				},
				"required": ["path", "target_path", "sheet", "mode"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "write_text_file",
			Description: "Write or create a text file (md, txt, csv, json, etc.). Overwrites if file exists.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "File path to write"},
					"content": {"type": "string", "description": "File content"}
				},
				"required": ["path", "content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "xlsx_operations",
			Description: "Create or modify an Excel file. Can copy from existing file or create new. Operations: ensure_sheet, set_range, set_cells, summarize_by_category, set_column_widths, set_row_heights, freeze_panes.",
			Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"path": {"type": "string", "description": "Target filename (no directory prefix, e.g. report.xlsx)"},
						"create_new": {"type": "boolean", "description": "If true, creates a new file (otherwise modifies existing)"},
						"copy_from": {"type": "string", "description": "Source filename to copy from when creating new (no directory prefix)"},
						"operations": {
							"type": "array",
							"description": "List of operations to apply",
							"items": {
								"type": "object",
								"properties": {
									"op": {"type": "string", "enum": ["ensure_sheet", "set_range", "set_cells", "summarize_by_category", "set_column_widths", "set_row_heights", "freeze_panes"], "description": "Operation type"},
									"sheet": {"type": "string", "description": "Target sheet name"},
									"start": {"type": "string", "description": "Starting cell for set_range (e.g. A1)"},
									"values": {
										"type": "array",
										"description": "2D array of values for set_range",
										"items": {"type": "array", "items": {"type": "string"}}
									},
									"style": {
										"type": "object",
										"description": "Optional style object for style-capable xlsx write operations",
										"properties": {
											"font_name": {"type": "string"},
											"font_size": {"type": "number"},
											"font_bold": {"type": "boolean"},
											"font_italic": {"type": "boolean"},
											"font_color": {"type": "string"},
											"fill_color": {"type": "string"},
											"fill_pattern": {"type": "string"},
											"number_format": {"type": "string"},
											"h_align": {"type": "string"},
											"v_align": {"type": "string"},
											"wrap_text": {"type": "boolean"},
											"border_top": {
												"type": "object",
												"properties": {
													"style": {"type": "string"},
													"color": {"type": "string"}
												}
											},
											"border_bottom": {
												"type": "object",
												"properties": {
													"style": {"type": "string"},
													"color": {"type": "string"}
												}
											},
											"border_left": {
												"type": "object",
												"properties": {
													"style": {"type": "string"},
													"color": {"type": "string"}
												}
											},
											"border_right": {
												"type": "object",
												"properties": {
													"style": {"type": "string"},
													"color": {"type": "string"}
												}
											}
										},
										"additionalProperties": true
									},
									"source_sheets": {
										"type": "array",
										"description": "Source sheet names for summarize_by_category (for example [\"Q1\",\"Q2\",\"Q3\",\"Q4\"])",
										"items": {"type": "string"}
									},
									"category_col": {"type": "string", "description": "Category column for summarize_by_category (default B; accepts letter or 1-based index)"},
									"amount_col": {"type": "string", "description": "Amount column for summarize_by_category (default C; accepts letter or 1-based index)"},
									"category_header": {"type": "string", "description": "Optional header label for the category column (default Category)"},
									"columns": {
										"type": "array",
										"description": "Column width updates for set_column_widths",
										"items": {
											"type": "object",
											"properties": {
												"column": {"type": "string", "description": "Column identifier (e.g. A, B, C or 1-based index)"},
												"width": {"type": "number", "description": "Column width in Excel character units"}
											},
											"required": ["column", "width"]
										}
									},
									"rows": {
										"type": "array",
										"description": "Row height updates for set_row_heights",
										"items": {
											"type": "object",
											"properties": {
												"row": {"type": "integer", "description": "1-based row index"},
												"height": {"type": "number", "description": "Row height in points"}
											},
											"required": ["row", "height"]
										}
									},
									"row": {"type": "integer", "description": "First unfrozen row (0-based) for freeze_panes"},
									"column": {"type": "integer", "description": "First unfrozen column (0-based) for freeze_panes"},
									"cells": {
										"type": "array",
										"description": "Array of {cell, value} for set_cells",
										"items": {
											"type": "object",
											"properties": {
												"cell": {"type": "string"},
												"value": {"description": "Cell value", "anyOf": [{"type": "string"}, {"type": "number"}, {"type": "boolean"}]},
												"type": {"type": "string", "description": "Optional value type hint"},
												"style": {
													"type": "object",
													"description": "Optional style object for this cell",
													"properties": {
														"font_name": {"type": "string"},
														"font_size": {"type": "number"},
														"font_bold": {"type": "boolean"},
														"font_italic": {"type": "boolean"},
														"font_color": {"type": "string"},
														"fill_color": {"type": "string"},
														"fill_pattern": {"type": "string"},
														"number_format": {"type": "string"},
														"h_align": {"type": "string"},
														"v_align": {"type": "string"},
														"wrap_text": {"type": "boolean"},
														"border_top": {
															"type": "object",
															"properties": {
																"style": {"type": "string"},
																"color": {"type": "string"}
															}
														},
														"border_bottom": {
															"type": "object",
															"properties": {
																"style": {"type": "string"},
																"color": {"type": "string"}
															}
														},
														"border_left": {
															"type": "object",
															"properties": {
																"style": {"type": "string"},
																"color": {"type": "string"}
															}
														},
														"border_right": {
															"type": "object",
															"properties": {
																"style": {"type": "string"},
																"color": {"type": "string"}
															}
														}
													},
													"additionalProperties": true
												}
											},
											"required": ["cell", "value"]
										}
									}
							},
							"required": ["op"]
						}
					}
				},
				"required": ["path", "operations"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "docx_operations",
			Description: "Create or modify a Word document. Operations: set_paragraphs, append_paragraph, replace_text.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Target file path"},
					"create_new": {"type": "boolean", "description": "If true, creates a new file"},
					"copy_from": {"type": "string", "description": "Source file to copy from when creating new"},
					"operations": {
						"type": "array",
						"description": "List of operations to apply",
						"items": {
								"type": "object",
								"properties": {
									"op": {"type": "string", "enum": ["set_paragraphs", "append_paragraph", "replace_text"], "description": "Operation type"},
									"paragraphs": {
										"type": "array",
										"description": "Array of {text, style} for set_paragraphs",
										"items": {
											"type": "object",
											"properties": {
												"text": {"type": "string"},
												"style": {"type": "string"},
												"runs": {
													"type": "array",
													"description": "Optional run-level styled text for this paragraph",
													"items": {
														"type": "object",
														"properties": {
															"text": {"type": "string"},
															"font_name": {"type": "string"},
															"font_size": {"type": "number"},
															"bold": {"type": "boolean"},
															"italic": {"type": "boolean"},
															"underline": {"type": "boolean"},
															"font_color": {"type": "string"},
															"highlight_color": {"type": "string"}
														}
													}
												},
												"alignment": {"type": "string", "description": "Paragraph alignment (left, center, right, justify)"},
												"space_before": {"type": "number", "description": "Space before paragraph in points"},
												"space_after": {"type": "number", "description": "Space after paragraph in points"},
												"line_spacing": {"type": "number", "description": "Line spacing multiplier"},
												"indent_left": {"type": "number", "description": "Left indent in inches"},
												"indent_right": {"type": "number", "description": "Right indent in inches"},
												"indent_first_line": {"type": "number", "description": "First-line indent in inches"}
											}
										}
									},
									"text": {"type": "string", "description": "Text content for append_paragraph"},
									"style": {"type": "string", "description": "Paragraph style (Normal, Heading1, etc.)"},
									"runs": {
										"type": "array",
										"description": "Optional run-level styled text for append_paragraph",
										"items": {
											"type": "object",
											"properties": {
												"text": {"type": "string"},
												"font_name": {"type": "string"},
												"font_size": {"type": "number"},
												"bold": {"type": "boolean"},
												"italic": {"type": "boolean"},
												"underline": {"type": "boolean"},
												"font_color": {"type": "string"},
												"highlight_color": {"type": "string"}
											}
										}
									},
									"alignment": {"type": "string", "description": "Paragraph alignment (left, center, right, justify)"},
									"space_before": {"type": "number", "description": "Space before paragraph in points"},
									"space_after": {"type": "number", "description": "Space after paragraph in points"},
									"line_spacing": {"type": "number", "description": "Line spacing multiplier"},
									"indent_left": {"type": "number", "description": "Left indent in inches"},
									"indent_right": {"type": "number", "description": "Right indent in inches"},
									"indent_first_line": {"type": "number", "description": "First-line indent in inches"},
									"search": {"type": "string", "description": "Text to find for replace_text"},
									"replace": {"type": "string", "description": "Replacement text for replace_text"},
									"match_case": {"type": "boolean", "description": "Whether replace_text matching is case-sensitive"}
								},
								"required": ["op"]
							}
						}
				},
				"required": ["path", "operations"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "pptx_operations",
			Description: "Create or modify a PowerPoint presentation. Operations: add_slide, set_slide_text, append_bullets.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Target file path"},
					"create_new": {"type": "boolean", "description": "If true, creates a new file"},
					"copy_from": {"type": "string", "description": "Source file to copy from when creating new"},
					"operations": {
						"type": "array",
						"description": "List of operations to apply",
						"items": {
								"type": "object",
								"properties": {
									"op": {"type": "string", "enum": ["add_slide", "set_slide_text", "append_bullets"], "description": "Operation type"},
									"layout": {"type": "string", "description": "Slide layout for add_slide"},
									"title": {"type": "string", "description": "Slide title"},
									"body": {"type": "string", "description": "Slide body text"},
									"title_runs": {
										"type": "array",
										"description": "Optional styled runs for title text",
										"items": {
											"type": "object",
											"properties": {
												"text": {"type": "string"},
												"font_name": {"type": "string"},
												"font_size": {"type": "number"},
												"bold": {"type": "boolean"},
												"italic": {"type": "boolean"},
												"underline": {"type": "boolean"},
												"font_color": {"type": "string"},
												"highlight_color": {"type": "string"}
											}
										}
									},
									"body_runs": {
										"type": "array",
										"description": "Optional styled runs for body text",
										"items": {
											"type": "object",
											"properties": {
												"text": {"type": "string"},
												"font_name": {"type": "string"},
												"font_size": {"type": "number"},
												"bold": {"type": "boolean"},
												"italic": {"type": "boolean"},
												"underline": {"type": "boolean"},
												"font_color": {"type": "string"},
												"highlight_color": {"type": "string"}
											}
										}
									},
									"alignment": {"type": "string", "description": "Paragraph alignment (left, center, right)"},
									"space_before": {"type": "number", "description": "Space before paragraph in points"},
									"space_after": {"type": "number", "description": "Space after paragraph in points"},
									"line_spacing": {"type": "number", "description": "Line spacing multiplier"},
									"index": {"type": "integer", "description": "Target slide index (0-based)"},
									"bullets": {"type": "array", "items": {"type": "string"}, "description": "Bullet points for append_bullets"}
								},
								"required": ["op"]
						}
					}
				},
				"required": ["path", "operations"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "xlsx_get_styles",
			Description: "Query style metadata from an Excel file (sheet styles, formats, and related descriptors). Use this for derivative fidelity tasks.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Excel file path"},
					"sheet": {"type": "string", "description": "Optional sheet name to scope style query"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "xlsx_copy_assets",
			Description: "Copy styles/assets from a source Excel file into a target Excel file in Draft. Assets are explicit selectors returned by style tools or known identifiers.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"source_path": {"type": "string", "description": "Source Excel file path"},
					"target_path": {"type": "string", "description": "Target Excel file path"},
					"assets": {
						"type": "array",
						"description": "Asset selectors to copy (string selectors or objects such as {type, id, name})",
						"items": {"type": "string"}
					}
				},
				"required": ["source_path", "target_path", "assets"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "docx_get_styles",
			Description: "Query style metadata from a Word document (paragraph/character styles and related descriptors). Use this for derivative fidelity tasks.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Word document path"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "docx_copy_assets",
			Description: "Copy styles/assets from a source Word document into a target Word document in Draft. Assets are explicit selectors returned by style tools or known identifiers.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"source_path": {"type": "string", "description": "Source Word document path"},
					"target_path": {"type": "string", "description": "Target Word document path"},
					"assets": {
						"type": "array",
						"description": "Asset selectors to copy (string selectors or objects such as {type, id, name})",
						"items": {"type": "string"}
					}
				},
				"required": ["source_path", "target_path", "assets"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "pptx_get_styles",
			Description: "Query style metadata from a PowerPoint presentation (masters/layout/style descriptors). Use this for derivative fidelity tasks.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "PowerPoint file path"}
				},
				"required": ["path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "pptx_copy_assets",
			Description: "Copy styles/assets from a source PowerPoint file into a target PowerPoint file in Draft. Assets are explicit selectors returned by style tools or known identifiers.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"source_path": {"type": "string", "description": "Source PowerPoint file path"},
					"target_path": {"type": "string", "description": "Target PowerPoint file path"},
					"assets": {
						"type": "array",
						"description": "Asset selectors to copy (string selectors or objects such as {type, id, name})",
						"items": {"type": "string"}
					}
				},
				"required": ["source_path", "target_path", "assets"]
			}`),
		},
	},
	{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "recall_tool_result",
			Description: "Retrieve the full result of a previous tool call from the tool log. Use when you need to re-examine data from an earlier tool call referenced in a receipt.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"entry_id": {"type": "integer", "description": "The tool log entry ID from a previous receipt"}
				},
				"required": ["entry_id"]
			}`),
		},
	},
}

var ResearchTools []llm.Tool

var PlanTools []llm.Tool

func init() {
	ResearchTools = selectWorkshopTools(
		"list_files",
		"get_file_info",
		"get_file_map",
		"read_file",
		"table_get_map",
		"table_describe",
		"table_stats",
		"table_read_rows",
		"table_query",
		"recall_tool_result",
		"xlsx_get_styles",
		"docx_get_styles",
		"pptx_get_styles",
	)
	PlanTools = selectWorkshopTools("read_file", "recall_tool_result")
}

func selectWorkshopTools(names ...string) []llm.Tool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	filtered := make([]llm.Tool, 0, len(names))
	for _, tool := range WorkshopTools {
		if _, ok := set[tool.Function.Name]; !ok {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

// ToolHandler executes tool calls for a workshop session.
type ToolHandler struct {
	engine      *Engine
	workbenchID string
	ctx         context.Context
	focusHints  map[string]map[string]any
}

// NewToolHandler creates a new tool handler for the given workbench.
func NewToolHandler(e *Engine, workbenchID string, ctx context.Context) *ToolHandler {
	return &ToolHandler{
		engine:      e,
		workbenchID: workbenchID,
		ctx:         ctx,
		focusHints:  make(map[string]map[string]any),
	}
}

// FocusHints returns collected per-path review focus hints from successful
// office write tool calls. If the same path is edited multiple times, the last
// successful tool call wins.
func (h *ToolHandler) FocusHints() map[string]map[string]any {
	if len(h.focusHints) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(h.focusHints))
	for path, hint := range h.focusHints {
		if path == "" || len(hint) == 0 {
			continue
		}
		copyHint := make(map[string]any, len(hint))
		for key, value := range hint {
			copyHint[key] = value
		}
		out[path] = copyHint
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *ToolHandler) setFocusHint(path string, hint map[string]any) {
	if strings.TrimSpace(path) == "" || len(hint) == 0 {
		return
	}
	if h.focusHints == nil {
		h.focusHints = make(map[string]map[string]any)
	}
	copyHint := make(map[string]any, len(hint))
	for key, value := range hint {
		copyHint[key] = value
	}
	h.focusHints[path] = copyHint
}

func (h *ToolHandler) resolvePptxFocusHint(path string, ops []map[string]any) map[string]any {
	if hint := buildPptxFocusHint(ops); hint != nil {
		return hint
	}
	if !hasAddSlideOperation(ops) {
		return nil
	}
	lastSlideIndex, ok := h.readPptxLastSlideIndex(path)
	if !ok {
		return nil
	}
	return map[string]any{"slide_index": lastSlideIndex}
}

func hasAddSlideOperation(ops []map[string]any) bool {
	for _, op := range ops {
		name, _ := op["op"].(string)
		if !strings.EqualFold(strings.TrimSpace(name), "add_slide") {
			continue
		}
		return true
	}
	return false
}

func (h *ToolHandler) readPptxLastSlideIndex(path string) (int, bool) {
	if h.engine.toolWorker == nil {
		return 0, false
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         path,
		"root":         "draft",
	}
	var resp struct {
		SlideCount int `json:"slide_count"`
	}
	if err := h.engine.toolWorker.Call(h.ctx, "PptxGetMap", params, &resp); err != nil {
		return 0, false
	}
	if resp.SlideCount <= 0 {
		return 0, false
	}
	return resp.SlideCount - 1, true
}

// Execute runs a tool call and returns the result as a string.
func (h *ToolHandler) Execute(call llm.ToolCall) (string, error) {
	switch call.Function.Name {
	case "list_files":
		return h.listFiles()
	case "get_file_info":
		return h.getFileInfo(call.Function.Arguments)
	case "get_file_map":
		return h.getFileMap(call.Function.Arguments)
	case "read_file":
		return h.readFile(call.Function.Arguments)
	case "table_get_map":
		return h.tableGetMap(call.Function.Arguments)
	case "table_describe":
		return h.tableDescribe(call.Function.Arguments)
	case "table_stats":
		return h.tableStats(call.Function.Arguments)
	case "table_read_rows":
		return h.tableReadRows(call.Function.Arguments)
	case "table_query":
		return h.tableQuery(call.Function.Arguments)
	case "table_export":
		return h.tableExport(call.Function.Arguments)
	case "table_update_from_export":
		return h.tableUpdateFromExport(call.Function.Arguments)
	case "write_text_file":
		return h.writeTextFile(call.Function.Arguments)
	case "xlsx_operations":
		return h.xlsxOperations(call.Function.Arguments)
	case "docx_operations":
		return h.docxOperations(call.Function.Arguments)
	case "pptx_operations":
		return h.pptxOperations(call.Function.Arguments)
	case "xlsx_get_styles":
		return h.xlsxGetStyles(call.Function.Arguments)
	case "xlsx_copy_assets":
		return h.xlsxCopyAssets(call.Function.Arguments)
	case "docx_get_styles":
		return h.docxGetStyles(call.Function.Arguments)
	case "docx_copy_assets":
		return h.docxCopyAssets(call.Function.Arguments)
	case "pptx_get_styles":
		return h.pptxGetStyles(call.Function.Arguments)
	case "pptx_copy_assets":
		return h.pptxCopyAssets(call.Function.Arguments)
	case "recall_tool_result":
		return h.recallToolResult(call.Function.Arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
	}
}

func (h *ToolHandler) listFiles() (string, error) {
	files, err := h.engine.workbenches.DraftFilesList(h.workbenchID)
	if err != nil {
		return "", err
	}
	type fileInfo struct {
		Path     string `json:"path"`
		Kind     string `json:"kind"`
		Size     int64  `json:"size_bytes"`
		MimeType string `json:"mime_type"`
		IsOpaque bool   `json:"is_opaque,omitempty"`
	}
	result := make([]fileInfo, 0, len(files))
	for _, f := range files {
		result = append(result, fileInfo{
			Path:     f.Path,
			Kind:     f.FileKind,
			Size:     f.Size,
			MimeType: f.MimeType,
			IsOpaque: f.IsOpaque,
		})
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (h *ToolHandler) getFileInfo(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	files, err := h.engine.workbenches.DraftFilesList(h.workbenchID)
	if err != nil {
		return "", err
	}

	var fileEntry *workbench.FileEntry
	for _, f := range files {
		if f.Path == args.Path {
			fileEntry = &f
			break
		}
	}
	if fileEntry == nil {
		return "", fmt.Errorf("file not found: %s", args.Path)
	}

	result := map[string]any{
		"path":      fileEntry.Path,
		"kind":      fileEntry.FileKind,
		"size":      fileEntry.Size,
		"mime_type": fileEntry.MimeType,
	}

	// Get additional info based on file type
	area := "published"
	if ds, _ := h.engine.workbenches.DraftState(h.workbenchID); ds != nil {
		area = "draft"
	}

	switch fileEntry.FileKind {
	case workbench.FileKindXlsx:
		if h.engine.toolWorker != nil {
			var resp struct {
				Sheets []struct {
					Name     string `json:"name"`
					RowCount int    `json:"row_count"`
					ColCount int    `json:"col_count"`
				} `json:"sheets"`
			}
			params := map[string]any{
				"workbench_id": h.workbenchID,
				"path":         args.Path,
				"root":         area,
			}
			if err := h.engine.toolWorker.Call(h.ctx, "XlsxGetInfo", params, &resp); err == nil {
				result["sheets"] = resp.Sheets
			}
		}
	case workbench.FileKindPdf:
		if h.engine.toolWorker != nil {
			var resp struct {
				PageCount int `json:"page_count"`
			}
			params := map[string]any{
				"workbench_id": h.workbenchID,
				"path":         args.Path,
				"root":         area,
			}
			if err := h.engine.toolWorker.Call(h.ctx, "PdfGetInfo", params, &resp); err == nil {
				result["page_count"] = resp.PageCount
			}
		}
	case workbench.FileKindImage:
		if h.engine.toolWorker != nil {
			var resp struct {
				Width  int    `json:"width"`
				Height int    `json:"height"`
				Format string `json:"format"`
			}
			params := map[string]any{
				"workbench_id": h.workbenchID,
				"path":         args.Path,
				"root":         area,
			}
			if err := h.engine.toolWorker.Call(h.ctx, "ImageGetMetadata", params, &resp); err == nil {
				result["width"] = resp.Width
				result["height"] = resp.Height
				result["format"] = resp.Format
			}
		}
	}

	b, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// smallTextFileThreshold is the maximum size in bytes for a text file to be
// considered "small" enough to inline fully in the map-first context.
const smallTextFileThreshold = 4000

func isCSVWorkbenchPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".csv")
}

func (h *ToolHandler) getFileMap(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	files, err := h.engine.workbenches.DraftFilesList(h.workbenchID)
	if err != nil {
		return "", err
	}

	var fileEntry *workbench.FileEntry
	for _, f := range files {
		if f.Path == args.Path {
			fileEntry = &f
			break
		}
	}
	if fileEntry == nil {
		return "", fmt.Errorf("file not found: %s", args.Path)
	}

	area := "published"
	if ds, _ := h.engine.workbenches.DraftState(h.workbenchID); ds != nil {
		area = "draft"
	}

	switch fileEntry.FileKind {
	case workbench.FileKindXlsx:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		var resp json.RawMessage
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if err := h.engine.toolWorker.Call(h.ctx, "XlsxGetMap", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindDocx:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		var resp json.RawMessage
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if err := h.engine.toolWorker.Call(h.ctx, "DocxGetMap", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindPptx:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		var resp json.RawMessage
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if err := h.engine.toolWorker.Call(h.ctx, "PptxGetMap", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindPdf:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		var resp json.RawMessage
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if err := h.engine.toolWorker.Call(h.ctx, "PdfGetMap", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindText:
		if isCSVWorkbenchPath(args.Path) {
			if h.engine.toolWorker == nil {
				return "", toolworker.ErrUnavailable
			}
			var resp json.RawMessage
			params := map[string]any{
				"workbench_id": h.workbenchID,
				"path":         args.Path,
				"root":         area,
			}
			if err := h.engine.toolWorker.Call(h.ctx, "TabularGetMap", params, &resp); err != nil {
				return "", err
			}
			return string(resp), nil
		}
		// For text files, try pyworker TextGetMap first; fall back to local computation.
		if h.engine.toolWorker != nil {
			var resp json.RawMessage
			params := map[string]any{
				"workbench_id": h.workbenchID,
				"path":         args.Path,
				"root":         area,
			}
			if err := h.engine.toolWorker.Call(h.ctx, "TextGetMap", params, &resp); err == nil {
				return string(resp), nil
			}
		}
		// Fallback: compute locally
		content, err := h.engine.workbenches.ReadFile(h.workbenchID, area, args.Path)
		if err != nil {
			return "", err
		}
		return computeTextMap(content), nil

	default:
		if fileEntry.IsOpaque {
			return fmt.Sprintf(`{"error":"opaque file, no structural map available for %s"}`, args.Path), nil
		}
		return fmt.Sprintf(`{"error":"no structural map available for file kind: %s"}`, fileEntry.FileKind), nil
	}
}

// computeTextMap generates a text file map locally (line_count, char_count, chunks).
func computeTextMap(content string) string {
	const chunkLines = 200
	lines := strings.Split(content, "\n")
	lineCount := len(lines)
	charCount := len(content)

	type chunk struct {
		Index int    `json:"index"`
		Lines string `json:"lines"`
	}
	var chunks []chunk
	idx := 0
	ln := 1
	for ln <= lineCount {
		end := ln + chunkLines - 1
		if end > lineCount {
			end = lineCount
		}
		chunks = append(chunks, chunk{Index: idx, Lines: fmt.Sprintf("%d-%d", ln, end)})
		idx++
		ln = end + 1
	}

	result := map[string]any{
		"line_count": lineCount,
		"char_count": charCount,
		"chunks":     chunks,
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func (h *ToolHandler) validateCSVPath(path, fieldName string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return workshopValidationError(fieldName + " is required")
	}
	kind, _ := workbench.FileKindForPath(path)
	if kind != workbench.FileKindText || !isCSVWorkbenchPath(path) {
		return workshopValidationError(fieldName + " must be a .csv file")
	}
	return nil
}

func (h *ToolHandler) tableGetMap(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
	}
	return h.callJSONWorker("TabularGetMap", params)
}

func (h *ToolHandler) tableDescribe(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
	}
	return h.callJSONWorker("TabularDescribe", params)
}

func (h *ToolHandler) tableStats(argsJSON string) (string, error) {
	var args struct {
		Path    string   `json:"path"`
		Columns []string `json:"columns"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
	}
	if len(args.Columns) > 0 {
		params["columns"] = args.Columns
	}
	return h.callJSONWorker("TabularGetStats", params)
}

func (h *ToolHandler) tableReadRows(argsJSON string) (string, error) {
	var args struct {
		Path     string   `json:"path"`
		RowStart int      `json:"row_start"`
		RowCount int      `json:"row_count"`
		Columns  []string `json:"columns"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}
	if args.RowStart <= 0 {
		return "", workshopValidationError("row_start must be >= 1")
	}
	if args.RowCount <= 0 {
		return "", workshopValidationError("row_count must be >= 1")
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
		"row_start":    args.RowStart,
		"row_count":    args.RowCount,
	}
	if len(args.Columns) > 0 {
		params["columns"] = args.Columns
	}
	return h.callJSONWorker("TabularReadRows", params)
}

func (h *ToolHandler) tableQuery(argsJSON string) (string, error) {
	var args struct {
		Path         string `json:"path"`
		Query        string `json:"query"`
		WindowRows   *int   `json:"window_rows"`
		WindowOffset *int   `json:"window_offset"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", workshopValidationError("query is required")
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
		"query":        args.Query,
	}
	if args.WindowRows != nil {
		if *args.WindowRows <= 0 {
			return "", workshopValidationError("window_rows must be >= 1")
		}
		params["window_rows"] = *args.WindowRows
	}
	if args.WindowOffset != nil {
		if *args.WindowOffset < 0 {
			return "", workshopValidationError("window_offset must be >= 0")
		}
		params["window_offset"] = *args.WindowOffset
	}
	return h.callJSONWorker("TabularQuery", params)
}

func (h *ToolHandler) tableExport(argsJSON string) (string, error) {
	var args struct {
		Path       string `json:"path"`
		Query      string `json:"query"`
		TargetPath string `json:"target_path"`
		Format     string `json:"format"`
		Sheet      string `json:"sheet"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.TargetPath) == "" {
		return "", workshopValidationError("target_path is required")
	}
	format := strings.ToLower(strings.TrimSpace(args.Format))
	if format != "csv" && format != "xlsx" {
		return "", workshopValidationError("format must be csv or xlsx")
	}
	if ext := strings.ToLower(strings.TrimSpace(filepath.Ext(args.TargetPath))); ext != "."+format {
		return "", workshopValidationError("target_path extension must match format")
	}
	if _, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent"); err != nil {
		return "", fmt.Errorf("failed to ensure draft: %w", err)
	}
	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
		"target_path":  strings.TrimSpace(args.TargetPath),
		"target_root":  "draft",
		"format":       format,
	}
	if strings.TrimSpace(args.Query) != "" {
		params["query"] = strings.TrimSpace(args.Query)
	}
	if strings.TrimSpace(args.Sheet) != "" {
		params["sheet"] = strings.TrimSpace(args.Sheet)
	}
	if h.engine.toolWorker == nil {
		return "", workshopMappedToolError(toolworker.ErrUnavailable, "TabularExport unavailable")
	}
	var resp struct {
		TargetPath  string   `json:"target_path"`
		Format      string   `json:"format"`
		RowCount    int      `json:"row_count"`
		ColumnCount int      `json:"column_count"`
		Warnings    []string `json:"warnings"`
		Sheet       string   `json:"sheet"`
	}
	if err := h.engine.toolWorker.Call(h.ctx, "TabularExport", params, &resp); err != nil {
		return "", workshopMappedToolError(err, "TabularExport failed")
	}

	targetPath := strings.TrimSpace(resp.TargetPath)
	if targetPath == "" {
		targetPath = strings.TrimSpace(args.TargetPath)
	}
	if format == "xlsx" && targetPath != "" {
		sheet := strings.TrimSpace(resp.Sheet)
		if sheet == "" {
			sheet = strings.TrimSpace(args.Sheet)
		}
		if sheet == "" {
			sheet = "Sheet1"
		}
		h.setFocusHint(targetPath, map[string]any{
			"sheet":     sheet,
			"row_start": 0,
			"col_start": 0,
		})
	}

	out, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("failed to encode tabular export result: %w", err)
	}
	return string(out), nil
}

func parseWrittenRangeTopLeftCell(writtenRange string) (int, int, bool) {
	trimmed := strings.TrimSpace(writtenRange)
	if trimmed == "" {
		return 0, 0, false
	}
	if bang := strings.LastIndex(trimmed, "!"); bang >= 0 {
		trimmed = strings.TrimSpace(trimmed[bang+1:])
	}
	if colon := strings.Index(trimmed, ":"); colon >= 0 {
		trimmed = strings.TrimSpace(trimmed[:colon])
	}
	trimmed = strings.ReplaceAll(trimmed, "$", "")
	return parseCellRef(trimmed)
}

func (h *ToolHandler) tableUpdateFromExport(argsJSON string) (string, error) {
	var args struct {
		Path                    string `json:"path"`
		Query                   string `json:"query"`
		TargetPath              string `json:"target_path"`
		Sheet                   string `json:"sheet"`
		Mode                    string `json:"mode"`
		StartCell               string `json:"start_cell"`
		IncludeHeader           *bool  `json:"include_header"`
		CreateWorkbookIfMissing *bool  `json:"create_workbook_if_missing"`
		CreateSheetIfMissing    *bool  `json:"create_sheet_if_missing"`
		ClearTargetRange        *bool  `json:"clear_target_range"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := h.validateCSVPath(args.Path, "path"); err != nil {
		return "", err
	}

	targetPath := strings.TrimSpace(args.TargetPath)
	if targetPath == "" {
		return "", workshopValidationError("target_path is required")
	}
	if strings.ToLower(strings.TrimSpace(filepath.Ext(targetPath))) != ".xlsx" {
		return "", workshopValidationError("target_path must be a .xlsx file")
	}

	sheet := strings.TrimSpace(args.Sheet)
	if sheet == "" {
		return "", workshopValidationError("sheet is required")
	}

	mode := strings.ToLower(strings.TrimSpace(args.Mode))
	switch mode {
	case "replace_sheet", "append_rows", "write_range":
	default:
		return "", workshopValidationError("mode must be replace_sheet, append_rows, or write_range")
	}

	startCell := strings.TrimSpace(args.StartCell)
	if mode == "write_range" {
		if startCell == "" {
			startCell = "A1"
		}
		if _, _, ok := parseCellRef(startCell); !ok {
			return "", workshopValidationError("start_cell must be an A1-style cell reference")
		}
	}

	if args.ClearTargetRange != nil && mode != "write_range" {
		return "", workshopValidationError("clear_target_range is only supported for mode write_range")
	}

	if _, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent"); err != nil {
		return "", fmt.Errorf("failed to ensure draft: %w", err)
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         "draft",
		"target_path":  targetPath,
		"target_root":  "draft",
		"sheet":        sheet,
		"mode":         mode,
	}
	if query := strings.TrimSpace(args.Query); query != "" {
		params["query"] = query
	}
	if mode == "write_range" {
		params["start_cell"] = startCell
	}
	if args.IncludeHeader != nil {
		params["include_header"] = *args.IncludeHeader
	}
	if args.CreateWorkbookIfMissing != nil {
		params["create_workbook_if_missing"] = *args.CreateWorkbookIfMissing
	}
	if args.CreateSheetIfMissing != nil {
		params["create_sheet_if_missing"] = *args.CreateSheetIfMissing
	}
	if args.ClearTargetRange != nil {
		params["clear_target_range"] = *args.ClearTargetRange
	}

	if h.engine.toolWorker == nil {
		return "", workshopMappedToolError(toolworker.ErrUnavailable, "TabularUpdateFromExport unavailable")
	}
	var resp struct {
		TargetPath   string   `json:"target_path"`
		Sheet        string   `json:"sheet"`
		Mode         string   `json:"mode"`
		RowCount     int      `json:"row_count"`
		ColumnCount  int      `json:"column_count"`
		WrittenRange string   `json:"written_range"`
		Warnings     []string `json:"warnings"`
	}
	if err := h.engine.toolWorker.Call(h.ctx, "TabularUpdateFromExport", params, &resp); err != nil {
		return "", workshopMappedToolError(err, "TabularUpdateFromExport failed")
	}

	focusPath := strings.TrimSpace(resp.TargetPath)
	if focusPath == "" {
		focusPath = targetPath
	}
	focusSheet := strings.TrimSpace(resp.Sheet)
	if focusSheet == "" {
		focusSheet = sheet
	}
	rowStart := 0
	colStart := 0
	if row, col, ok := parseWrittenRangeTopLeftCell(resp.WrittenRange); ok {
		rowStart = row - 1
		colStart = col - 1
	}
	h.setFocusHint(focusPath, map[string]any{
		"sheet":     focusSheet,
		"row_start": rowStart,
		"col_start": colStart,
	})

	out, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("failed to encode tabular update result: %w", err)
	}
	return string(out), nil
}

func (h *ToolHandler) readFile(argsJSON string) (string, error) {
	var args struct {
		Path       string `json:"path"`
		Sheet      string `json:"sheet"`
		Range      string `json:"range"`
		Section    string `json:"section"`
		SlideIndex *int   `json:"slide_index"`
		Pages      string `json:"pages"`
		LineStart  *int   `json:"line_start"`
		LineCount  *int   `json:"line_count"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	files, err := h.engine.workbenches.DraftFilesList(h.workbenchID)
	if err != nil {
		return "", err
	}

	var fileEntry *workbench.FileEntry
	for _, f := range files {
		if f.Path == args.Path {
			fileEntry = &f
			break
		}
	}
	if fileEntry == nil {
		return "", fmt.Errorf("file not found: %s", args.Path)
	}

	area := "published"
	if ds, _ := h.engine.workbenches.DraftState(h.workbenchID); ds != nil {
		area = "draft"
	}

	switch fileEntry.FileKind {
	case workbench.FileKindText:
		// Support line_start/line_count via pyworker TextReadLines
		if args.LineStart != nil || args.LineCount != nil {
			if h.engine.toolWorker != nil {
				params := map[string]any{
					"workbench_id": h.workbenchID,
					"path":         args.Path,
					"root":         area,
				}
				if args.LineStart != nil {
					params["line_start"] = *args.LineStart
				}
				if args.LineCount != nil {
					params["line_count"] = *args.LineCount
				}
				var resp json.RawMessage
				if err := h.engine.toolWorker.Call(h.ctx, "TextReadLines", params, &resp); err == nil {
					return string(resp), nil
				}
				// Fall through to full read on error
			}
		}
		content, err := h.engine.workbenches.ReadFile(h.workbenchID, area, args.Path)
		if err != nil {
			return "", err
		}
		return content, nil

	case workbench.FileKindXlsx:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if args.Sheet != "" {
			params["sheet"] = args.Sheet
		}
		if args.Range != "" {
			params["range"] = args.Range
		}
		// Use XlsxReadRange if sheet/range specified, otherwise XlsxExtractText
		method := "XlsxExtractText"
		if args.Sheet != "" {
			method = "XlsxReadRange"
		}
		var resp json.RawMessage
		if err := h.engine.toolWorker.Call(h.ctx, method, params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindDocx:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if args.Section != "" {
			params["section"] = args.Section
		}
		var resp json.RawMessage
		if err := h.engine.toolWorker.Call(h.ctx, "DocxExtractText", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindOdt:
		text, err := h.engine.extractText(h.ctx, h.workbenchID, area, fileEntry.FileKind, args.Path)
		if err != nil {
			return "", err
		}
		return text, nil

	case workbench.FileKindPptx:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if args.SlideIndex != nil {
			params["slide_index"] = *args.SlideIndex
		}
		var resp json.RawMessage
		if err := h.engine.toolWorker.Call(h.ctx, "PptxExtractText", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindPdf:
		if h.engine.toolWorker == nil {
			return "", toolworker.ErrUnavailable
		}
		params := map[string]any{
			"workbench_id": h.workbenchID,
			"path":         args.Path,
			"root":         area,
		}
		if args.Pages != "" {
			params["pages"] = args.Pages
		}
		var resp json.RawMessage
		if err := h.engine.toolWorker.Call(h.ctx, "PdfExtractText", params, &resp); err != nil {
			return "", err
		}
		return string(resp), nil

	case workbench.FileKindImage:
		return fmt.Sprintf("Image file: %s (use get_file_info for metadata)", args.Path), nil

	default:
		if fileEntry.IsOpaque {
			return fmt.Sprintf("Binary/opaque file: %s (content not readable)", args.Path), nil
		}
		return "", fmt.Errorf("unsupported file kind: %s", fileEntry.FileKind)
	}
}

// sanitizeFlatPath strips any directory prefix from a path, keeping only the
// filename. The workbench is flat so agents should only pass filenames, but
// LLMs sometimes prefix with "draft/" or a subdirectory.
func sanitizeFlatPath(path string) string {
	return filepath.Base(path)
}

func (h *ToolHandler) writeTextFile(argsJSON string) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	args.Path = sanitizeFlatPath(args.Path)

	// Validate path extension is a text type
	kind, _ := workbench.FileKindForPath(args.Path)
	if kind != workbench.FileKindText && kind != "" {
		return "", fmt.Errorf("write_text_file only supports text files, got kind: %s", kind)
	}

	// Ensure draft exists
	ds, err := h.engine.workbenches.DraftState(h.workbenchID)
	if err != nil || ds == nil {
		_, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent")
		if err != nil {
			return "", fmt.Errorf("failed to create draft: %w", err)
		}
	}

	if err := h.engine.workbenches.ApplyDraftWrite(h.workbenchID, args.Path, args.Content); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(args.Content), args.Path), nil
}

func (h *ToolHandler) xlsxOperations(argsJSON string) (string, error) {
	var args struct {
		Path       string           `json:"path"`
		CreateNew  bool             `json:"create_new"`
		CopyFrom   string           `json:"copy_from"`
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	args.Path = sanitizeFlatPath(args.Path)
	if args.CopyFrom != "" {
		args.CopyFrom = sanitizeFlatPath(args.CopyFrom)
	}
	if len(args.Operations) == 0 {
		return "", fmt.Errorf("operations is required")
	}

	if h.engine.toolWorker == nil {
		return "", toolworker.ErrUnavailable
	}

	// Ensure draft exists
	ds, err := h.engine.workbenches.DraftState(h.workbenchID)
	if err != nil || ds == nil {
		_, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent")
		if err != nil {
			return "", fmt.Errorf("failed to create draft: %w", err)
		}
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         args.Path,
		"ops":          args.Operations,
		"root":         "draft",
	}
	if args.CreateNew {
		params["create_new"] = true
	}
	if args.CopyFrom != "" {
		params["copy_from"] = args.CopyFrom
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := h.engine.toolWorker.Call(h.ctx, "XlsxApplyOps", params, &resp); err != nil {
		return "", fmt.Errorf("xlsx operation failed: %w", err)
	}
	if hint := buildXlsxFocusHint(args.Operations); hint != nil {
		h.setFocusHint(args.Path, hint)
	}

	action := "Modified"
	if args.CreateNew {
		action = "Created"
	}
	return fmt.Sprintf("%s %s with %d operations", action, args.Path, len(args.Operations)), nil
}

func (h *ToolHandler) docxOperations(argsJSON string) (string, error) {
	var args struct {
		Path       string           `json:"path"`
		CreateNew  bool             `json:"create_new"`
		CopyFrom   string           `json:"copy_from"`
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	args.Path = sanitizeFlatPath(args.Path)
	if args.CopyFrom != "" {
		args.CopyFrom = sanitizeFlatPath(args.CopyFrom)
	}
	if len(args.Operations) == 0 {
		return "", fmt.Errorf("operations is required")
	}
	normalizeDocxOperationAliases(args.Operations)

	if h.engine.toolWorker == nil {
		return "", toolworker.ErrUnavailable
	}

	// Ensure draft exists
	ds, err := h.engine.workbenches.DraftState(h.workbenchID)
	if err != nil || ds == nil {
		_, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent")
		if err != nil {
			return "", fmt.Errorf("failed to create draft: %w", err)
		}
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         args.Path,
		"ops":          args.Operations,
		"root":         "draft",
	}
	if args.CreateNew {
		params["create_new"] = true
	}
	if args.CopyFrom != "" {
		params["copy_from"] = args.CopyFrom
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := h.engine.toolWorker.Call(h.ctx, "DocxApplyOps", params, &resp); err != nil {
		return "", fmt.Errorf("docx operation failed: %w", err)
	}
	if hint := buildDocxFocusHint(args.Operations); hint != nil {
		h.setFocusHint(args.Path, hint)
	}

	action := "Modified"
	if args.CreateNew {
		action = "Created"
	}
	return fmt.Sprintf("%s %s with %d operations", action, args.Path, len(args.Operations)), nil
}

func (h *ToolHandler) pptxOperations(argsJSON string) (string, error) {
	var args struct {
		Path       string           `json:"path"`
		CreateNew  bool             `json:"create_new"`
		CopyFrom   string           `json:"copy_from"`
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	args.Path = sanitizeFlatPath(args.Path)
	if args.CopyFrom != "" {
		args.CopyFrom = sanitizeFlatPath(args.CopyFrom)
	}
	if len(args.Operations) == 0 {
		return "", fmt.Errorf("operations is required")
	}
	normalizePptxOperationAliases(args.Operations)

	if h.engine.toolWorker == nil {
		return "", toolworker.ErrUnavailable
	}

	// Ensure draft exists
	ds, err := h.engine.workbenches.DraftState(h.workbenchID)
	if err != nil || ds == nil {
		_, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent")
		if err != nil {
			return "", fmt.Errorf("failed to create draft: %w", err)
		}
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         args.Path,
		"ops":          args.Operations,
		"root":         "draft",
	}
	if args.CreateNew {
		params["create_new"] = true
	}
	if args.CopyFrom != "" {
		params["copy_from"] = args.CopyFrom
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := h.engine.toolWorker.Call(h.ctx, "PptxApplyOps", params, &resp); err != nil {
		return "", fmt.Errorf("pptx operation failed: %w", err)
	}
	if hint := h.resolvePptxFocusHint(args.Path, args.Operations); hint != nil {
		h.setFocusHint(args.Path, hint)
	}

	action := "Modified"
	if args.CreateNew {
		action = "Created"
	}
	return fmt.Sprintf("%s %s with %d operations", action, args.Path, len(args.Operations)), nil
}

func normalizeDocxOperationAliases(ops []map[string]any) {
	for _, op := range ops {
		if op == nil {
			continue
		}
		if _, ok := op["search"]; ok {
			continue
		}
		if legacy, ok := op["find"]; ok {
			op["search"] = legacy
		}
	}
}

func normalizePptxOperationAliases(ops []map[string]any) {
	for _, op := range ops {
		if op == nil {
			continue
		}
		if _, ok := op["index"]; ok {
			continue
		}
		if legacy, ok := op["slide_index"]; ok {
			op["index"] = legacy
		}
	}
}

func (h *ToolHandler) ensureDraftForCopyTools() error {
	ds, err := h.engine.workbenches.DraftState(h.workbenchID)
	if err == nil && ds != nil {
		return nil
	}
	if _, err := h.engine.workbenches.CreateDraftWithSource(h.workbenchID, "workshop", "agent"); err != nil {
		return fmt.Errorf("failed to create draft: %w", err)
	}
	return nil
}

func (h *ToolHandler) workshopReadRoot() string {
	ds, err := h.engine.workbenches.DraftState(h.workbenchID)
	if err == nil && ds != nil {
		return "draft"
	}
	return "published"
}

func workshopValidationError(detail string) error {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "invalid arguments"
	}
	return fmt.Errorf("%s: %s", errinfo.CodeValidationFailed, detail)
}

func workshopMappedToolError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	if mapped := mapToolWorkerError(errinfo.PhaseWorkshop, err); mapped != nil {
		detail := strings.TrimSpace(mapped.Detail)
		if detail == "" {
			detail = strings.TrimSpace(fallback)
		}
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%s: %s", mapped.ErrorCode, detail)
	}
	if strings.TrimSpace(fallback) == "" {
		return err
	}
	return fmt.Errorf("%s: %w", fallback, err)
}

func validateOfficeToolPath(path, expectedKind, fieldName string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return workshopValidationError(fieldName + " is required")
	}
	kind, _ := workbench.FileKindForPath(path)
	if kind != expectedKind {
		return workshopValidationError(fmt.Sprintf("%s must be %s", fieldName, expectedKind))
	}
	return nil
}

func validateAssetSelectors(assets []any) error {
	if len(assets) == 0 {
		return workshopValidationError("assets is required")
	}
	for i, asset := range assets {
		switch typed := asset.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				return workshopValidationError(fmt.Sprintf("assets[%d] must be non-empty", i))
			}
		case map[string]any:
			typeName, _ := typed["type"].(string)
			id, _ := typed["id"].(string)
			name, _ := typed["name"].(string)
			if strings.TrimSpace(typeName) == "" && strings.TrimSpace(id) == "" && strings.TrimSpace(name) == "" {
				return workshopValidationError(fmt.Sprintf("assets[%d] must include type, id, or name", i))
			}
		default:
			return workshopValidationError(fmt.Sprintf("assets[%d] must be string or object", i))
		}
	}
	return nil
}

func (h *ToolHandler) callJSONWorker(method string, params map[string]any) (string, error) {
	if h.engine.toolWorker == nil {
		return "", workshopMappedToolError(toolworker.ErrUnavailable, method+" unavailable")
	}
	var resp json.RawMessage
	if err := h.engine.toolWorker.Call(h.ctx, method, params, &resp); err != nil {
		return "", workshopMappedToolError(err, method+" failed")
	}
	if len(resp) == 0 {
		empty, _ := json.Marshal(map[string]any{"ok": true})
		return string(empty), nil
	}
	return string(resp), nil
}

func (h *ToolHandler) xlsxGetStyles(argsJSON string) (string, error) {
	var args struct {
		Path  string `json:"path"`
		Sheet string `json:"sheet"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := validateOfficeToolPath(args.Path, workbench.FileKindXlsx, "path"); err != nil {
		return "", err
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
	}
	if sheet := strings.TrimSpace(args.Sheet); sheet != "" {
		params["sheet"] = sheet
	}
	return h.callJSONWorker("XlsxGetStyles", params)
}

func (h *ToolHandler) xlsxCopyAssets(argsJSON string) (string, error) {
	var args struct {
		SourcePath string `json:"source_path"`
		TargetPath string `json:"target_path"`
		Assets     []any  `json:"assets"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := validateOfficeToolPath(args.SourcePath, workbench.FileKindXlsx, "source_path"); err != nil {
		return "", err
	}
	if err := validateOfficeToolPath(args.TargetPath, workbench.FileKindXlsx, "target_path"); err != nil {
		return "", err
	}
	if err := validateAssetSelectors(args.Assets); err != nil {
		return "", err
	}
	if err := h.ensureDraftForCopyTools(); err != nil {
		return "", err
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"source_path":  strings.TrimSpace(args.SourcePath),
		"target_path":  strings.TrimSpace(args.TargetPath),
		"assets":       args.Assets,
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
	}
	return h.callJSONWorker("XlsxCopyAssets", params)
}

func (h *ToolHandler) docxGetStyles(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := validateOfficeToolPath(args.Path, workbench.FileKindDocx, "path"); err != nil {
		return "", err
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
	}
	return h.callJSONWorker("DocxGetStyles", params)
}

func (h *ToolHandler) docxCopyAssets(argsJSON string) (string, error) {
	var args struct {
		SourcePath string `json:"source_path"`
		TargetPath string `json:"target_path"`
		Assets     []any  `json:"assets"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := validateOfficeToolPath(args.SourcePath, workbench.FileKindDocx, "source_path"); err != nil {
		return "", err
	}
	if err := validateOfficeToolPath(args.TargetPath, workbench.FileKindDocx, "target_path"); err != nil {
		return "", err
	}
	if err := validateAssetSelectors(args.Assets); err != nil {
		return "", err
	}
	if err := h.ensureDraftForCopyTools(); err != nil {
		return "", err
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"source_path":  strings.TrimSpace(args.SourcePath),
		"target_path":  strings.TrimSpace(args.TargetPath),
		"assets":       args.Assets,
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
	}
	return h.callJSONWorker("DocxCopyAssets", params)
}

func (h *ToolHandler) pptxGetStyles(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := validateOfficeToolPath(args.Path, workbench.FileKindPptx, "path"); err != nil {
		return "", err
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"path":         strings.TrimSpace(args.Path),
		"root":         h.workshopReadRoot(),
	}
	return h.callJSONWorker("PptxGetStyles", params)
}

func (h *ToolHandler) pptxCopyAssets(argsJSON string) (string, error) {
	var args struct {
		SourcePath string `json:"source_path"`
		TargetPath string `json:"target_path"`
		Assets     []any  `json:"assets"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if err := validateOfficeToolPath(args.SourcePath, workbench.FileKindPptx, "source_path"); err != nil {
		return "", err
	}
	if err := validateOfficeToolPath(args.TargetPath, workbench.FileKindPptx, "target_path"); err != nil {
		return "", err
	}
	if err := validateAssetSelectors(args.Assets); err != nil {
		return "", err
	}
	if err := h.ensureDraftForCopyTools(); err != nil {
		return "", err
	}

	params := map[string]any{
		"workbench_id": h.workbenchID,
		"source_path":  strings.TrimSpace(args.SourcePath),
		"target_path":  strings.TrimSpace(args.TargetPath),
		"assets":       args.Assets,
		"root":         "draft",
		"source_root":  "draft",
		"target_root":  "draft",
	}
	return h.callJSONWorker("PptxCopyAssets", params)
}

func (h *ToolHandler) recallToolResult(argsJSON string) (string, error) {
	var args struct {
		EntryID int `json:"entry_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", workshopValidationError("invalid arguments")
	}
	if args.EntryID <= 0 {
		return "", workshopValidationError("entry_id must be >= 1")
	}
	entry, err := h.engine.readToolLogEntry(h.workbenchID, args.EntryID)
	if err != nil {
		return "", fmt.Errorf("recall failed: %w", err)
	}
	return entry.Result, nil
}

const RPIResearchSystemPrompt = `You are KeenBench in RESEARCH phase.

Goal:
- Explore the available files and user request.
- Build an accurate understanding of structure, constraints, and edge cases.
- Do not modify any files.

Rules:
1. Use only available read-only tools.
2. Follow MAP-FIRST workflow: inspect structure first, then targeted reads/queries.
3. Use recall_tool_result only when a receipt is insufficient.
4. Keep findings concrete: file structure, schema, data patterns, assumptions, and risks.
5. End with a clear research summary as your final text response.`

const RPIPlanSystemPrompt = `You are KeenBench in PLAN phase.

Goal:
- Convert the research into a concrete execution checklist.
- Do not execute file edits in this phase.

Output requirements:
- Return markdown only.
- Use this exact high-level structure:

# Execution Plan

## Task
<one-line summary>

## Items
- [ ] 1. <Label> — <Description>
- [ ] 2. <Label> — <Description>

## Notes
- <note>

Rules:
1. Items must be atomic and ordered.
2. Use checkbox format exactly: "- [ ] N. Label — Description".
3. Include only actionable implementation items.
4. Do not mark items complete.
5. Do not execute the plan.`

const RPIImplementSystemPrompt = `You are KeenBench in IMPLEMENT phase.

You must complete exactly ONE plan item in this run.

Current plan item:
%s

Current plan state:
%s

Rules:
1. Focus only on the current item above.
2. Use tools to perform edits and verification as needed.
3. Do not modify plan.md directly. The engine updates checklist states.
4. After completing work, verify key outputs (read-back/checks).
5. If you discover additional required items, include them ONLY at the end of your final text response in this exact format:
   - [ ] N. Label — Description
6. Do not edit/remove/reorder existing items in your response.
7. Finish with a concise implementation summary for this item.
8. The file manifest below shows all current workbench files and their structure. Do NOT call list_files or get_file_info to discover files — use the manifest. Only call list_files after creating a new file to confirm it exists.
9. For xlsx files, the manifest map shows existing sheet names and dimensions. Do NOT call ensure_sheet for sheets that already appear in the manifest — they already exist.
10. Prefer table_update_from_export to write query results into existing xlsx workbooks/sheets. Use table_export for stand-alone csv/xlsx outputs.
11. Keep SQL queries efficient: use GROUP BY/aggregation instead of SELECT * followed by manual processing. Double-quote column names with special characters in DuckDB.`

const RPISummarySystemPrompt = `You are KeenBench in SUMMARY phase.

Create the final user-visible summary of completed work.

Completed plan:
%s

Current file manifest:
%s

Requirements:
1. Summarize completed outcomes clearly.
2. Call out any failed items and reasons.
3. List created/modified files using exact manifest names.
4. Keep the response concise and user-focused.`

// AgentSystemPrompt returns the system prompt for the agentic workshop.
const AgentSystemPrompt = `You are KeenBench, an AI assistant that helps users work with files.

YOU MUST USE TOOLS to accomplish tasks. Do not just describe what you would do - actually call the tools to do it.

Available tools:
- list_files: See all available files with their types and sizes
- get_file_info: Get lightweight metadata about a file (sheets for xlsx, pages for pdf, etc.)
- get_file_map: Get structural map of a file showing internal layout, regions, and chunk boundaries
- read_file: Read file content with optional region selectors (sheet/range, section, slide_index, pages, line_start/line_count)
- table_get_map: Get structural map of a CSV file with schema, chunks, and encoding metadata
- table_describe: Get detailed column metadata for a CSV file
- table_stats: Get summary stats for CSV columns
- table_read_rows: Read CSV rows by position
- table_query: Run read-only SQL against CSV data (table name is data)
- table_export: Export CSV data/query results to stand-alone Draft files (csv/xlsx)
- table_update_from_export: Write CSV data/query results into existing Draft xlsx workbook/sheet
- write_text_file: Create or update text files
- xlsx_operations: Create or modify Excel files (use copy_from to copy an existing file)
- docx_operations: Create or modify Word documents
- pptx_operations: Create or modify PowerPoint presentations
- xlsx_get_styles / docx_get_styles / pptx_get_styles: Inspect style descriptors for fidelity-sensitive derivative tasks
- xlsx_copy_assets / docx_copy_assets / pptx_copy_assets: Copy style/layout/media assets between same-format files in Draft
- recall_tool_result: Retrieve the full result of a previous tool call by entry ID (use sparingly)

MAP-FIRST WORKFLOW:
For structured files (xlsx, docx, pptx, pdf), CSV files, and large text files, follow this approach:
1. The file manifest below shows what files exist with structural maps already included.
2. Use the map to understand the file layout (sheets, sections, slides, pages, chunks, or table schema/chunks).
3. Use read_file or table tools with specific coordinates/query windows to read only the data you need.
4. For large files, read in chunks rather than requesting all content at once.
5. Do NOT ask the user to upload files that appear in the manifest - they are already here.

TOOL RESULTS AND CONTEXT:
- Tool results are NOT kept in full in conversation context. You receive a compact receipt with shape info and a data preview.
- Each receipt references a tool log entry ID. Use recall_tool_result(entry_id) only if you need the full data.
- Prefer making decisions from receipts. Most tasks can be completed without recalling full results.
- For data-to-file workflows, use table_export for stand-alone outputs and table_update_from_export for existing xlsx workbook/sheet targets. Do NOT recall large results just to pass them to write_text_file.

TASK COMPLETION:
- ALWAYS complete the entire task in a single run. Do NOT stop partway to describe remaining steps.
- If a task involves multiple files, sheets, sections, or entities, process ALL of them before responding.
- Never end a message with "next steps" or "what's still pending". If there is more work, keep calling tools until it is done.
- The only acceptable final response is one that confirms ALL requested work is complete.
- If the task is genuinely too large for one run (50+ turns), complete as much as possible and clearly state what remains.

CRITICAL RULES:
1. ALWAYS call tools to accomplish tasks - never just describe what you would do
2. For file modifications, ALWAYS use the appropriate tool (xlsx_operations, docx_operations, etc.)
3. To copy and modify a file, use create_new=true with copy_from pointing to the source file
4. For xlsx: use read_file with sheet and range (e.g. A1:E50) based on the map
5. For docx: use read_file with section parameter to read specific sections
6. For pptx: use read_file with slide_index to read specific slides
7. For pdf: use read_file with pages parameter (e.g. "1-5") to read specific pages
8. For text: use read_file with line_start and line_count for large files
9. For CSV analysis tasks, prefer table_* tools over line-based read_file.
10. For CSV analysis, start with table_get_map to see schema. Use table_stats for summaries. Prefer aggregation queries (GROUP BY, SUM, COUNT, AVG) over SELECT *. Use window_rows to control result size (default 100, use 10-25 for exploration). Use table_export for stand-alone files and table_update_from_export when writing into existing xlsx workbook/sheet.
11. All modifications go to a draft - users will review before publishing
12. PDF and image files are read-only
13. For docx replace_text use "search"/"replace"; for pptx set_slide_text/append_bullets use "index" for slide number
14. For CSV merge/join tasks, NEVER return header-only output when source files contain rows. If join keys are missing or mismatched, perform a deterministic best-effort merge (for example, assign rows from the second file in order / round-robin) and include a row for each primary-source record.
15. For requests to edit an existing file, keep the same filename/path unless the user explicitly asks for a new file.
16. For DOCX template-fill tasks, replace ALL placeholders that exist in the document, including placeholders inside table cells (for example {{items_table}}). Never leave unresolved {{...}} tokens when source data exists.
17. For PPTX slide-creation tasks that require action items/bullets, ensure the body/content placeholder actually contains the requested items (not title-only output). Use append_bullets or multiline body text and verify with read_file on the target slide if needed.
18. For XLSX summary/aggregation tasks, use xlsx_operations with op="summarize_by_category" so totals are computed from source-sheet data deterministically. Do not hand-calculate, invent, or guess totals.
19. For derivative fidelity tasks (preserve formatting/theme/layout/branding), prefer style/asset tools: query styles first (xlsx_get_styles/docx_get_styles/pptx_get_styles), then copy required assets with matching *_copy_assets tools before content edits.
20. Style/asset copy tools are format-specific: xlsx->xlsx, docx->docx, pptx->pptx.

DuckDB SQL PATTERNS (table_query uses DuckDB):
- Column names with spaces/special chars require double-quotes: SELECT "Issue Type", "Component/s" FROM data
- CASE: SELECT CASE WHEN "Status"='Done' THEN 'Closed' ELSE 'Open' END AS norm FROM data
- Concatenate skipping NULLs: SELECT concat_ws(char(10), "Comment", "Comment_1", "Comment_2") FROM data
- String aggregation: SELECT "App", string_agg(DISTINCT "Label", ', ') AS labels FROM data GROUP BY "App"
- Write results to xlsx: use table_export(path, target_path, format="xlsx", sheet="SheetName", query="SELECT ...") for stand-alone outputs, or table_update_from_export(...) for existing workbook/sheet targets

When asked to translate or modify an Excel file:
1. Review the map to see sheet names, ranges, and data islands
2. Call read_file with the sheet name and range to see content chunk by chunk
3. Call xlsx_operations with create_new=true, copy_from=original_file, and operations to modify cells

When asked to create a copy of a file:
1. Use xlsx_operations (or docx_operations/pptx_operations) with create_new=true and copy_from=source_path

Be direct. Use tools immediately. Don't ask questions you can answer with tools. Complete the entire task — never stop to describe what you would do next.`
