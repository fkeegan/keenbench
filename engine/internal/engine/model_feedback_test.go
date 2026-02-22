package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseModelFeedbackMarkdownValid(t *testing.T) {
	raw := `## What slowed me down
Tool retries because read and write ops were mixed.

## Hardest tool interaction
Choosing between table_query and read_file for csv chunks.

## Highest-impact tooling change
Expose clearer tool-level constraints in each phase.

## Confidence (1-5)
4

## Additional notes
This run needed two retries.`
	survey, err := parseModelFeedbackMarkdown(raw)
	if err != nil {
		t.Fatalf("parse feedback: %v", err)
	}
	if survey.Confidence != 4 {
		t.Fatalf("expected confidence 4, got %d", survey.Confidence)
	}
	if survey.WhatSlowedMeDown == "" || survey.HardestToolInteraction == "" || survey.HighestImpactingChange == "" {
		t.Fatalf("expected required survey sections to be populated: %+v", survey)
	}
}

func TestParseModelFeedbackMarkdownMissingRequiredSection(t *testing.T) {
	raw := `## What slowed me down
Retries

## Highest-impact tooling change
Better docs`
	if _, err := parseModelFeedbackMarkdown(raw); err == nil {
		t.Fatalf("expected parse error when required section is missing")
	}
}

func TestParseModelFeedbackMarkdownConfidenceDefaults(t *testing.T) {
	raw := `## What slowed me down
Long iterations.

## Hardest tool interaction
Finding the right table operation.

## Highest-impact tooling change
More explicit examples.

## Confidence (1-5)
high confidence
`
	survey, err := parseModelFeedbackMarkdown(raw)
	if err != nil {
		t.Fatalf("parse feedback: %v", err)
	}
	if survey.Confidence != 3 {
		t.Fatalf("expected confidence fallback to 3, got %d", survey.Confidence)
	}
}

func TestCollectModelFeedbackToolMetrics(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KEENBENCH_DATA_DIR", dataDir)
	t.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")

	eng, err := New()
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	workbenchID := "wb-feedback-tools"
	logPath := eng.toolLogPath(workbenchID)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir tool log dir: %v", err)
	}
	entries := []toolLogEntry{
		{ID: 1, Tool: "read_file", ElapsedMS: 5},
		{ID: 2, Tool: "table_query", ElapsedMS: 7},
		{ID: 3, Tool: "write_file", ElapsedMS: 11, Error: "write denied"},
	}
	data := make([]byte, 0)
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal log entry: %v", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(logPath, data, 0o600); err != nil {
		t.Fatalf("write tool log: %v", err)
	}

	metrics := eng.collectModelFeedbackToolMetrics(workbenchID, 0, 3)
	if metrics.ToolCallsTotal != 3 {
		t.Fatalf("expected 3 tool calls, got %d", metrics.ToolCallsTotal)
	}
	if metrics.ToolCallsFailed != 1 {
		t.Fatalf("expected 1 failed tool call, got %d", metrics.ToolCallsFailed)
	}
	if metrics.ToolElapsedMS != 23 {
		t.Fatalf("expected total elapsed 23ms, got %d", metrics.ToolElapsedMS)
	}
	if metrics.ToolCounts["read_file"] != 1 || metrics.ToolCounts["table_query"] != 1 || metrics.ToolCounts["write_file"] != 1 {
		t.Fatalf("unexpected tool counts: %+v", metrics.ToolCounts)
	}
}

func TestWriteModelFeedbackRecord(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KEENBENCH_DATA_DIR", dataDir)
	t.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")

	eng, err := New()
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	now := time.Date(2026, 2, 22, 10, 11, 12, 0, time.UTC)
	record := modelFeedbackRuntimeRecord{
		RecordID:         "mf-unit-1",
		RunID:            "run-123",
		WorkbenchID:      "wb-unit-1",
		ProviderID:       ProviderOpenAI,
		ModelID:          ModelOpenAIID,
		CollectionStatus: modelFeedbackStatusCollected,
		CollectedAt:      now,
		RunStartedAt:     now.Add(-2 * time.Second),
		RunEndedAt:       now,
		RunElapsedMS:     2000,
		HasDraft:         true,
		SummaryMessageID: "a-1",
		SummaryText:      "Summary text",
		PhaseStatus: modelFeedbackPhaseStatus{
			Research:  true,
			Plan:      true,
			Implement: true,
			Summary:   true,
		},
		ToolMetrics: modelFeedbackToolMetrics{
			ToolCallsTotal:  3,
			ToolCallsFailed: 1,
			ToolElapsedMS:   123,
			ToolCounts: map[string]int{
				"read_file":  2,
				"write_file": 1,
			},
		},
		Survey: modelFeedbackSurvey{
			WhatSlowedMeDown:       "Need better receipts.",
			HardestToolInteraction: "Choosing table tools.",
			HighestImpactingChange: "Explicit phase constraints.",
			Confidence:             4,
			AdditionalNotes:        "N/A",
		},
	}

	if err := eng.writeModelFeedbackRecord(record); err != nil {
		t.Fatalf("write model feedback record: %v", err)
	}

	recordPath := filepath.Join(eng.modelFeedbackDir(record.WorkbenchID), record.RecordID+".md")
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("expected markdown record at %s: %v", recordPath, err)
	}
	indexPath := filepath.Join(eng.modelFeedbackDir(record.WorkbenchID), "index.jsonl")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var parsed modelFeedbackIndexEntry
	lines := splitNonEmptyLines(string(indexData))
	if len(lines) != 1 {
		t.Fatalf("expected 1 index line, got %d", len(lines))
	}
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if parsed.RecordID != record.RecordID {
		t.Fatalf("expected record_id %q, got %q", record.RecordID, parsed.RecordID)
	}
	if parsed.CollectionStatus != modelFeedbackStatusCollected {
		t.Fatalf("expected status %q, got %q", modelFeedbackStatusCollected, parsed.CollectionStatus)
	}
	if parsed.ToolCallsTotal != 3 || parsed.ToolCallsFailed != 1 {
		t.Fatalf("unexpected tool metrics in index: %+v", parsed)
	}
}

func splitNonEmptyLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
