package toolworker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

type Fake struct {
	workdir string
}

func NewFake(workbenchesDir string) *Fake {
	return &Fake{workdir: strings.TrimSpace(workbenchesDir)}
}

func (f *Fake) Call(_ context.Context, method string, params any, result any) error {
	payload := map[string]any{}
	data, _ := json.Marshal(params)
	_ = json.Unmarshal(data, &payload)
	fullPath, _ := resolveFakePath(f.workdir, payload)

	switch method {
	case "WorkerGetInfo":
		return assignResult(result, map[string]any{"ok": true, "worker": "fake"})
	case "DocxApplyOps", "XlsxApplyOps", "PptxApplyOps", "DocxCopyAssets", "XlsxCopyAssets", "PptxCopyAssets":
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return err
		}
		content := []byte("fake ops applied")
		if err := os.WriteFile(fullPath, content, 0o600); err != nil {
			return err
		}
		return assignResult(result, map[string]any{"ok": true})
	case "DocxGetStyles":
		return assignResult(result, map[string]any{
			"format":      "docx",
			"styles":      []any{map[string]any{"asset_id": "paragraph_style:Normal", "type": "paragraph_style", "name": "Normal"}},
			"assets":      []any{map[string]any{"asset_id": "paragraph_style:Normal", "type": "paragraph_style", "name": "Normal"}},
			"style_count": 1,
		})
	case "XlsxGetStyles":
		return assignResult(result, map[string]any{
			"format":      "xlsx",
			"assets":      []any{map[string]any{"asset_id": "named_style:Normal", "type": "named_style", "name": "Normal"}},
			"sheets":      []any{"Sheet1"},
			"style_count": 1,
		})
	case "PptxGetStyles":
		return assignResult(result, map[string]any{
			"format":      "pptx",
			"assets":      []any{map[string]any{"asset_id": "text_style:0:0", "type": "text_style"}},
			"slide_count": 1,
		})
	case "DocxExtractText", "XlsxExtractText", "PptxExtractText", "PdfExtractText", "OdtExtractText":
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		return assignResult(result, map[string]any{"text": string(data)})
	case "DocxGetSectionContent":
		content, _ := os.ReadFile(fullPath)
		sectionIndex := fakeIndex(payload["section_index"])
		return assignResult(result, map[string]any{
			"section_index": sectionIndex,
			"section_count": 1,
			"section": map[string]any{
				"heading":    "Section 1",
				"level":      1,
				"paragraphs": []any{map[string]any{"index": 0, "text": string(content), "style": "Normal", "runs": []any{map[string]any{"index": 0, "text": string(content)}}}},
				"tables":     []any{},
				"images":     []any{},
			},
		})
	case "PptxGetSlideContent":
		content, _ := os.ReadFile(fullPath)
		slideIndex := fakeIndex(payload["slide_index"])
		return assignResult(result, map[string]any{
			"slide_index": slideIndex,
			"slide_count": 1,
			"slide": map[string]any{
				"index":       slideIndex,
				"title":       "Slide 1",
				"layout":      "Title and Content",
				"render_mode": "positioned",
				"positioned": map[string]any{
					"coordinate_space": "slide_ratio",
					"slide_size":       map[string]any{"width": 9144000, "height": 6858000, "unit": "emu"},
					"positioned_shapes": []any{
						map[string]any{
							"index":      0,
							"z_index":    0,
							"name":       "Body",
							"shape_type": "TEXT_BOX",
							"bounds": map[string]any{
								"x": 0.1, "y": 0.1, "width": 0.8, "height": 0.2, "unit": "slide_ratio",
							},
							"text_runs": []any{
								map[string]any{"index": 0, "text": string(content), "font_name": "Inter", "size_pt": 14},
							},
							"text_blocks": []any{
								map[string]any{"index": 0, "text": string(content)},
							},
						},
					},
				},
				"shapes": []any{
					map[string]any{
						"index":       0,
						"name":        "Body",
						"shape_type":  "TEXT_BOX",
						"left":        0,
						"top":         0,
						"width":       0,
						"height":      0,
						"text_blocks": []any{map[string]any{"index": 0, "text": string(content), "runs": []any{map[string]any{"index": 0, "text": string(content)}}}},
					},
				},
			},
		})
	case "PdfRenderPage", "DocxRenderPage", "OdtRenderPage":
		return assignResult(result, map[string]any{
			"bytes_base64": blankPNG(),
			"page_count":   1,
			"mime_type":    "image/png",
			"scaled_down":  false,
		})
	case "PptxRenderSlide":
		return assignResult(result, map[string]any{
			"bytes_base64": blankPNG(),
			"slide_count":  1,
			"mime_type":    "image/png",
			"scaled_down":  false,
		})
	case "XlsxRenderGrid":
		return assignResult(result, map[string]any{
			"sheets":    []string{"Sheet1"},
			"row_count": 2,
			"col_count": 2,
			"cells": []any{
				[]any{
					map[string]any{"value": "A1", "type": "string", "formula": nil},
					map[string]any{"value": "B1", "type": "string", "formula": nil},
				},
				[]any{
					map[string]any{"value": 1, "type": "number", "formula": nil},
					map[string]any{"value": 2, "type": "number", "formula": nil},
				},
			},
		})
	case "ImageRender":
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return assignResult(result, map[string]any{
				"bytes_base64": blankPNG(),
				"mime_type":    "image/png",
				"scaled_down":  false,
			})
		}
		return assignResult(result, map[string]any{
			"bytes_base64": base64.StdEncoding.EncodeToString(data),
			"mime_type":    "application/octet-stream",
			"scaled_down":  false,
		})
	case "ImageGetMetadata":
		info, err := os.Stat(fullPath)
		if err != nil {
			return err
		}
		width, height := decodeImageSize(fullPath)
		return assignResult(result, map[string]any{
			"format":     "unknown",
			"width":      width,
			"height":     height,
			"size_bytes": info.Size(),
		})
	case "TabularGetMap":
		cols, rows, err := fakeReadCSVTable(fullPath)
		if err != nil {
			return err
		}
		return assignResult(result, map[string]any{
			"format":              "csv",
			"delimiter":           ",",
			"quote_char":          "\"",
			"encoding_detected":   "utf-8",
			"encoding_confidence": 1.0,
			"has_header":          true,
			"row_count":           len(rows),
			"column_count":        len(cols),
			"columns":             fakeTabularColumns(cols),
			"chunks":              fakeTabularChunks(len(rows), 500),
		})
	case "TabularDescribe":
		cols, rows, err := fakeReadCSVTable(fullPath)
		if err != nil {
			return err
		}
		columnDefs := make([]map[string]any, 0, len(cols))
		for idx, col := range cols {
			columnDefs = append(columnDefs, map[string]any{
				"name":              col,
				"index":             idx,
				"inferred_type":     "string",
				"nullable":          true,
				"non_null_count":    len(rows),
				"distinct_estimate": len(rows),
			})
		}
		return assignResult(result, map[string]any{
			"row_count":    len(rows),
			"column_count": len(cols),
			"columns":      columnDefs,
		})
	case "TabularGetStats":
		cols, rows, err := fakeReadCSVTable(fullPath)
		if err != nil {
			return err
		}
		columnStats := make([]map[string]any, 0, len(cols))
		for _, col := range cols {
			columnStats = append(columnStats, map[string]any{
				"name":              col,
				"type":              "string",
				"non_null_count":    len(rows),
				"distinct_estimate": len(rows),
				"min_length":        0,
				"max_length":        0,
				"most_common":       []any{},
			})
		}
		return assignResult(result, map[string]any{
			"row_count": len(rows),
			"columns":   columnStats,
		})
	case "TabularReadRows":
		cols, rows, err := fakeReadCSVTable(fullPath)
		if err != nil {
			return err
		}
		rowStart := fakeIndex(payload["row_start"])
		if rowStart <= 0 {
			rowStart = 1
		}
		rowCount := fakeIndex(payload["row_count"])
		if rowCount <= 0 {
			rowCount = 100
		}
		offset := rowStart - 1
		if offset > len(rows) {
			offset = len(rows)
		}
		end := offset + rowCount
		if end > len(rows) {
			end = len(rows)
		}
		window := rows[offset:end]
		typedRows := make([][]any, 0, len(window))
		for _, row := range window {
			values := make([]any, 0, len(row))
			for _, cell := range row {
				values = append(values, cell)
			}
			typedRows = append(typedRows, values)
		}
		columnTypes := make([]string, 0, len(cols))
		for range cols {
			columnTypes = append(columnTypes, "string")
		}
		return assignResult(result, map[string]any{
			"columns":      cols,
			"column_types": columnTypes,
			"rows":         typedRows,
			"row_start":    rowStart,
			"row_count":    len(typedRows),
			"total_rows":   len(rows),
			"has_more":     end < len(rows),
		})
	case "TabularQuery":
		_, rows, err := fakeReadCSVTable(fullPath)
		if err != nil {
			return err
		}
		query, _ := payload["query"].(string)
		if strings.Contains(strings.ToLower(query), "count(") {
			return assignResult(result, map[string]any{
				"columns":          []string{"count"},
				"column_types":     []string{"integer"},
				"rows":             [][]any{{len(rows)}},
				"row_count":        1,
				"total_row_count":  1,
				"window_rows":      1,
				"window_offset":    0,
				"has_more":         false,
				"query_elapsed_ms": 1,
			})
		}
		return assignResult(result, map[string]any{
			"columns":          []string{},
			"column_types":     []string{},
			"rows":             [][]any{},
			"row_count":        0,
			"total_row_count":  0,
			"window_rows":      fakeIndex(payload["window_rows"]),
			"window_offset":    fakeIndex(payload["window_offset"]),
			"has_more":         false,
			"query_elapsed_ms": 1,
		})
	case "TabularExport":
		targetPath, _ := payload["target_path"].(string)
		if targetPath != "" {
			workbenchID, _ := payload["workbench_id"].(string)
			targetRoot, _ := payload["target_root"].(string)
			if targetRoot == "" {
				targetRoot = "draft"
			}
			target := filepath.Join(f.workdir, workbenchID, targetRoot, targetPath)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte("fake tabular export"), 0o600); err != nil {
				return err
			}
		}
		return assignResult(result, map[string]any{
			"target_path":  targetPath,
			"format":       payload["format"],
			"sheet":        payload["sheet"],
			"row_count":    0,
			"column_count": 0,
			"warnings":     []any{},
		})
	case "TabularUpdateFromExport":
		targetPath, _ := payload["target_path"].(string)
		if targetPath != "" {
			workbenchID, _ := payload["workbench_id"].(string)
			targetRoot, _ := payload["target_root"].(string)
			if targetRoot == "" {
				targetRoot = "draft"
			}
			target := filepath.Join(f.workdir, workbenchID, targetRoot, targetPath)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte("fake tabular update from export"), 0o600); err != nil {
				return err
			}
		}
		mode, _ := payload["mode"].(string)
		if mode == "" {
			mode = "replace_sheet"
		}
		sheet, _ := payload["sheet"].(string)
		if sheet == "" {
			sheet = "Sheet1"
		}
		return assignResult(result, map[string]any{
			"target_path":   targetPath,
			"sheet":         sheet,
			"mode":          mode,
			"row_count":     0,
			"column_count":  0,
			"written_range": "",
			"warnings":      []any{},
		})
	default:
		return errors.New("unsupported fake method")
	}
}

func (f *Fake) Close() error {
	return nil
}

func (f *Fake) HealthCheck(_ context.Context) error {
	return nil
}

func resolveFakePath(workdir string, params map[string]any) (string, error) {
	if workdir == "" {
		return "", errors.New("missing workbenches dir")
	}
	wb, _ := params["workbench_id"].(string)
	root, _ := params["root"].(string)
	if root == "" {
		root = "draft"
	}
	path, _ := params["path"].(string)
	return filepath.Join(workdir, wb, root, path), nil
}

func assignResult(dest any, src any) error {
	if dest == nil {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func fakeReadCSVTable(path string) ([]string, [][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return []string{}, [][]string{}, nil
	}
	columns := records[0]
	rows := [][]string{}
	if len(records) > 1 {
		rows = records[1:]
	}
	return columns, rows, nil
}

func fakeTabularColumns(columns []string) []map[string]any {
	result := make([]map[string]any, 0, len(columns))
	for idx, name := range columns {
		result = append(result, map[string]any{
			"name":          name,
			"index":         idx,
			"inferred_type": "string",
		})
	}
	return result
}

func fakeTabularChunks(rowCount, chunkSize int) []map[string]any {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	if rowCount == 0 {
		return []map[string]any{}
	}
	chunks := make([]map[string]any, 0, (rowCount/chunkSize)+1)
	index := 0
	start := 1
	for start <= rowCount {
		end := start + chunkSize - 1
		if end > rowCount {
			end = rowCount
		}
		chunks = append(chunks, map[string]any{
			"index": index,
			"rows":  fmt.Sprintf("%d-%d", start, end),
		})
		index++
		start = end + 1
	}
	return chunks
}

func blankPNG() string {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 220, G: 220, B: 220, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodeImageSize(path string) (int, int) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func fakeIndex(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return 0
}
