package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"keenbench/engine/internal/envutil"
	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
)

const (
	modelFeedbackEnvFlag = "KEENBENCH_MODEL_FEEDBACK"

	modelFeedbackStatusCollected       = "collected"
	modelFeedbackStatusModelCallFailed = "model_call_failed"
	modelFeedbackStatusParseFailed     = "parse_failed"
	modelFeedbackStatusSkipped         = "skipped"

	modelFeedbackVersion = 1

	modelFeedbackToolLogWindowMax = 5000

	modelFeedbackMaxSectionChars = 1800
)

type modelFeedbackPhaseStatus struct {
	Research  bool
	Plan      bool
	Implement bool
	Summary   bool
}

type modelFeedbackToolMetrics struct {
	ToolCallsTotal  int
	ToolCallsFailed int
	ToolElapsedMS   int64
	ToolCounts      map[string]int
}

type modelFeedbackSurvey struct {
	WhatSlowedMeDown       string
	HardestToolInteraction string
	HighestImpactingChange string
	Confidence             int
	AdditionalNotes        string
}

type modelFeedbackRuntimeRecord struct {
	RecordID         string
	RunID            string
	WorkbenchID      string
	ProviderID       string
	ModelID          string
	CollectionStatus string
	CollectedAt      time.Time
	RunStartedAt     time.Time
	RunEndedAt       time.Time
	RunElapsedMS     int64
	HasDraft         bool
	SummaryMessageID string
	SummaryText      string
	PhaseStatus      modelFeedbackPhaseStatus
	ToolMetrics      modelFeedbackToolMetrics
	RunErrorCode     string
	RunErrorPhase    string
	RunErrorSubphase string
	RunErrorDetail   string
	ModelError       string
	ParseError       string
	RawFeedback      string
	Survey           modelFeedbackSurvey
}

type modelFeedbackIndexEntry struct {
	Version          int               `json:"version"`
	RecordID         string            `json:"record_id"`
	RecordPath       string            `json:"record_path"`
	RunID            string            `json:"run_id"`
	WorkbenchID      string            `json:"workbench_id"`
	ProviderID       string            `json:"provider_id,omitempty"`
	ModelID          string            `json:"model_id,omitempty"`
	CollectionStatus string            `json:"collection_status"`
	CollectedAt      string            `json:"collected_at"`
	RunStartedAt     string            `json:"run_started_at,omitempty"`
	RunEndedAt       string            `json:"run_ended_at,omitempty"`
	RunElapsedMS     int64             `json:"run_elapsed_ms,omitempty"`
	HasDraft         bool              `json:"has_draft"`
	PhaseStatus      map[string]bool   `json:"phase_status,omitempty"`
	ToolCallsTotal   int               `json:"tool_calls_total,omitempty"`
	ToolCallsFailed  int               `json:"tool_calls_failed,omitempty"`
	ToolElapsedMS    int64             `json:"tool_elapsed_ms,omitempty"`
	ToolCounts       map[string]int    `json:"tool_counts,omitempty"`
	RunErrorCode     string            `json:"run_error_code,omitempty"`
	RunErrorPhase    string            `json:"run_error_phase,omitempty"`
	RunErrorSubphase string            `json:"run_error_subphase,omitempty"`
	RunErrorDetail   string            `json:"run_error_detail,omitempty"`
	ModelError       string            `json:"model_error,omitempty"`
	ParseError       string            `json:"parse_error,omitempty"`
	SummaryMessageID string            `json:"summary_message_id,omitempty"`
	SummaryPreview   string            `json:"summary_preview,omitempty"`
	Survey           map[string]any    `json:"survey,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type modelFeedbackCollectionInput struct {
	Ctx              context.Context
	WorkbenchID      string
	RunID            string
	ProviderID       string
	ModelID          string
	RunStartedAt     time.Time
	RunEndedAt       time.Time
	HasDraft         bool
	SummaryMessageID string
	SummaryText      string
	PhaseStatus      modelFeedbackPhaseStatus
	ToolLogSeqStart  int
	ToolLogSeqEnd    int
	RunError         *errinfo.ErrorInfo
	Client           LLMClient
	APIKey           string
}

func (e *Engine) shouldCollectModelFeedback() bool {
	return envutil.Bool(modelFeedbackEnvFlag)
}

func (e *Engine) collectModelFeedback(input modelFeedbackCollectionInput) {
	if !e.shouldCollectModelFeedback() {
		return
	}
	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	if e.now != nil {
		now = e.now().UTC()
	}
	record := modelFeedbackRuntimeRecord{
		RecordID:         fmt.Sprintf("mf-%d", now.UnixNano()),
		RunID:            strings.TrimSpace(input.RunID),
		WorkbenchID:      strings.TrimSpace(input.WorkbenchID),
		ProviderID:       strings.TrimSpace(input.ProviderID),
		ModelID:          strings.TrimSpace(input.ModelID),
		CollectionStatus: modelFeedbackStatusSkipped,
		CollectedAt:      now,
		RunStartedAt:     input.RunStartedAt.UTC(),
		RunEndedAt:       input.RunEndedAt.UTC(),
		HasDraft:         input.HasDraft,
		SummaryMessageID: strings.TrimSpace(input.SummaryMessageID),
		SummaryText:      strings.TrimSpace(input.SummaryText),
		PhaseStatus:      input.PhaseStatus,
	}
	if !record.RunStartedAt.IsZero() && !record.RunEndedAt.IsZero() {
		record.RunElapsedMS = record.RunEndedAt.Sub(record.RunStartedAt).Milliseconds()
		if record.RunElapsedMS < 0 {
			record.RunElapsedMS = 0
		}
	}
	if input.RunError != nil {
		record.RunErrorCode = strings.TrimSpace(input.RunError.ErrorCode)
		record.RunErrorPhase = strings.TrimSpace(input.RunError.Phase)
		record.RunErrorSubphase = strings.TrimSpace(input.RunError.Subphase)
		record.RunErrorDetail = strings.TrimSpace(input.RunError.Detail)
	}
	record.ToolMetrics = e.collectModelFeedbackToolMetrics(record.WorkbenchID, input.ToolLogSeqStart, input.ToolLogSeqEnd)

	canAskModel :=
		input.RunError == nil &&
			record.PhaseStatus.Summary &&
			record.ProviderID != "" &&
			record.ModelID != "" &&
			strings.TrimSpace(input.APIKey) != "" &&
			input.Client != nil
	if canAskModel {
		raw, modelErr := e.requestModelFeedback(ctx, record, input.Client, input.APIKey)
		record.RawFeedback = strings.TrimSpace(raw)
		if modelErr != nil {
			record.CollectionStatus = modelFeedbackStatusModelCallFailed
			record.ModelError = modelErr.Error()
		} else {
			survey, parseErr := parseModelFeedbackMarkdown(record.RawFeedback)
			if parseErr != nil {
				record.CollectionStatus = modelFeedbackStatusParseFailed
				record.ParseError = parseErr.Error()
			} else {
				record.CollectionStatus = modelFeedbackStatusCollected
				record.Survey = survey
			}
		}
	}

	if writeErr := e.writeModelFeedbackRecord(record); writeErr != nil {
		e.logger.Warn("model_feedback.write_failed", "workbench_id", record.WorkbenchID, "run_id", record.RunID, "error", writeErr.Error())
		return
	}
	e.logger.Info(
		"model_feedback.recorded",
		"workbench_id", record.WorkbenchID,
		"run_id", record.RunID,
		"record_id", record.RecordID,
		"status", record.CollectionStatus,
	)
}

func (e *Engine) requestModelFeedback(ctx context.Context, record modelFeedbackRuntimeRecord, client LLMClient, apiKey string) (string, error) {
	messages := buildModelFeedbackMessages(record)
	response, err := client.Chat(ctx, apiKey, providerModelName(record.ModelID), messages)
	if err != nil {
		return "", err
	}
	return response, nil
}

func buildModelFeedbackMessages(record modelFeedbackRuntimeRecord) []llm.Message {
	phaseLine := fmt.Sprintf(
		"research=%t, plan=%t, implement=%t, summary=%t",
		record.PhaseStatus.Research,
		record.PhaseStatus.Plan,
		record.PhaseStatus.Implement,
		record.PhaseStatus.Summary,
	)
	summary := strings.TrimSpace(record.SummaryText)
	if len(summary) > 700 {
		summary = summary[:700] + "..."
	}
	toolSummary := "(none)"
	if len(record.ToolMetrics.ToolCounts) > 0 {
		keys := make([]string, 0, len(record.ToolMetrics.ToolCounts))
		for k := range record.ToolMetrics.ToolCounts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s:%d", key, record.ToolMetrics.ToolCounts[key]))
		}
		toolSummary = strings.Join(parts, ", ")
	}
	runResult := "success"
	if record.RunErrorCode != "" {
		runResult = "failed"
	}
	system := strings.Join([]string{
		"You are providing post-run tooling feedback as the model that just completed the run.",
		"Respond ONLY in Markdown using exactly these H2 headings, in this order:",
		"## What slowed me down",
		"## Hardest tool interaction",
		"## Highest-impact tooling change",
		"## Confidence (1-5)",
		"## Additional notes",
		"Keep each section concise and concrete. Confidence must include one integer from 1 to 5.",
	}, "\n")
	user := strings.TrimSpace(fmt.Sprintf(
		"Run context:\n- run_id: %s\n- provider_id: %s\n- model_id: %s\n- run_result: %s\n- phases_completed: %s\n- tool_calls_total: %d\n- tool_calls_failed: %d\n- tool_elapsed_ms: %d\n- tool_counts: %s\n- has_draft: %t\n\nSummary text (if any):\n%s\n",
		record.RunID,
		record.ProviderID,
		record.ModelID,
		runResult,
		phaseLine,
		record.ToolMetrics.ToolCallsTotal,
		record.ToolMetrics.ToolCallsFailed,
		record.ToolMetrics.ToolElapsedMS,
		toolSummary,
		record.HasDraft,
		summary,
	))
	return []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

func parseModelFeedbackMarkdown(raw string) (modelFeedbackSurvey, error) {
	content := strings.ReplaceAll(raw, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	sections := make(map[string]string)
	current := ""
	var currentLines []string
	commit := func() {
		if current == "" {
			return
		}
		joined := strings.TrimSpace(strings.Join(currentLines, "\n"))
		sections[current] = clampFeedbackSection(joined)
	}
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if heading, ok := parseFeedbackHeading(line); ok {
			commit()
			current = heading
			currentLines = currentLines[:0]
			continue
		}
		if current != "" {
			currentLines = append(currentLines, rawLine)
		}
	}
	commit()

	missing := make([]string, 0)
	for _, key := range []string{"what_slowed", "hardest_tool", "highest_impact"} {
		if strings.TrimSpace(sections[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return modelFeedbackSurvey{}, fmt.Errorf("missing required feedback sections: %s", strings.Join(missing, ", "))
	}

	confidence := 3
	if section := sections["confidence"]; section != "" {
		if parsed, ok := parseFeedbackConfidence(section); ok {
			confidence = parsed
		}
	}
	return modelFeedbackSurvey{
		WhatSlowedMeDown:       sections["what_slowed"],
		HardestToolInteraction: sections["hardest_tool"],
		HighestImpactingChange: sections["highest_impact"],
		Confidence:             confidence,
		AdditionalNotes:        sections["additional_notes"],
	}, nil
}

func parseFeedbackHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "##") {
		return "", false
	}
	title := strings.TrimSpace(strings.TrimPrefix(line, "##"))
	if title == "" {
		return "", false
	}
	normalized := strings.ToLower(title)
	normalized = strings.TrimSuffix(normalized, ":")
	normalized = strings.ReplaceAll(normalized, "", "")
	normalized = strings.TrimSpace(normalized)
	switch normalized {
	case "what slowed me down":
		return "what_slowed", true
	case "hardest tool interaction":
		return "hardest_tool", true
	case "highest-impact tooling change", "highest impact tooling change":
		return "highest_impact", true
	case "confidence (1-5)":
		return "confidence", true
	case "additional notes":
		return "additional_notes", true
	default:
		return "", false
	}
}

var feedbackConfidencePattern = regexp.MustCompile(`([1-5])`)

func parseFeedbackConfidence(raw string) (int, bool) {
	match := feedbackConfidencePattern.FindStringSubmatch(raw)
	if len(match) != 2 {
		return 0, false
	}
	parsed, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	if parsed < 1 || parsed > 5 {
		return 0, false
	}
	return parsed, true
}

func clampFeedbackSection(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= modelFeedbackMaxSectionChars {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:modelFeedbackMaxSectionChars]) + "\n\n[Truncated]"
}

func (e *Engine) collectModelFeedbackToolMetrics(workbenchID string, seqStart, seqEnd int) modelFeedbackToolMetrics {
	metrics := modelFeedbackToolMetrics{ToolCounts: map[string]int{}}
	if strings.TrimSpace(workbenchID) == "" || seqEnd <= seqStart {
		return metrics
	}
	if seqEnd-seqStart > modelFeedbackToolLogWindowMax {
		seqStart = seqEnd - modelFeedbackToolLogWindowMax
	}
	path := e.toolLogPath(workbenchID)
	file, err := os.Open(path)
	if err != nil {
		return metrics
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry toolLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.ID <= seqStart || entry.ID > seqEnd {
			continue
		}
		metrics.ToolCallsTotal++
		metrics.ToolElapsedMS += entry.ElapsedMS
		toolName := strings.TrimSpace(entry.Tool)
		if toolName != "" {
			metrics.ToolCounts[toolName]++
		}
		if strings.TrimSpace(entry.Error) != "" {
			metrics.ToolCallsFailed++
		}
	}
	return metrics
}

func (e *Engine) writeModelFeedbackRecord(record modelFeedbackRuntimeRecord) error {
	if strings.TrimSpace(record.WorkbenchID) == "" {
		return errors.New("workbench id is required")
	}
	if strings.TrimSpace(record.RecordID) == "" {
		return errors.New("record id is required")
	}
	if strings.TrimSpace(record.CollectionStatus) == "" {
		record.CollectionStatus = modelFeedbackStatusSkipped
	}
	dir := e.modelFeedbackDir(record.WorkbenchID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	recordPath := filepath.Join(dir, record.RecordID+".md")
	rendered := renderModelFeedbackRecordMarkdown(record)
	if err := os.WriteFile(recordPath, []byte(rendered), 0o600); err != nil {
		return err
	}
	index := modelFeedbackIndexEntry{
		Version:          modelFeedbackVersion,
		RecordID:         record.RecordID,
		RecordPath:       recordPath,
		RunID:            record.RunID,
		WorkbenchID:      record.WorkbenchID,
		ProviderID:       record.ProviderID,
		ModelID:          record.ModelID,
		CollectionStatus: record.CollectionStatus,
		CollectedAt:      record.CollectedAt.Format(time.RFC3339),
		HasDraft:         record.HasDraft,
		RunElapsedMS:     record.RunElapsedMS,
		ToolCallsTotal:   record.ToolMetrics.ToolCallsTotal,
		ToolCallsFailed:  record.ToolMetrics.ToolCallsFailed,
		ToolElapsedMS:    record.ToolMetrics.ToolElapsedMS,
		ToolCounts:       record.ToolMetrics.ToolCounts,
		RunErrorCode:     record.RunErrorCode,
		RunErrorPhase:    record.RunErrorPhase,
		RunErrorSubphase: record.RunErrorSubphase,
		RunErrorDetail:   record.RunErrorDetail,
		ModelError:       record.ModelError,
		ParseError:       record.ParseError,
		SummaryMessageID: record.SummaryMessageID,
		SummaryPreview:   summaryPreview(record.SummaryText),
		PhaseStatus: map[string]bool{
			"research":  record.PhaseStatus.Research,
			"plan":      record.PhaseStatus.Plan,
			"implement": record.PhaseStatus.Implement,
			"summary":   record.PhaseStatus.Summary,
		},
	}
	if !record.RunStartedAt.IsZero() {
		index.RunStartedAt = record.RunStartedAt.Format(time.RFC3339)
	}
	if !record.RunEndedAt.IsZero() {
		index.RunEndedAt = record.RunEndedAt.Format(time.RFC3339)
	}
	if !isZeroSurvey(record.Survey) {
		index.Survey = map[string]any{
			"confidence":        record.Survey.Confidence,
			"what_slowed":       summaryPreview(record.Survey.WhatSlowedMeDown),
			"hardest_tool":      summaryPreview(record.Survey.HardestToolInteraction),
			"highest_impacting": summaryPreview(record.Survey.HighestImpactingChange),
		}
	}
	return appendModelFeedbackIndex(dir, index)
}

func (e *Engine) modelFeedbackDir(workbenchID string) string {
	return filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "workshop", "model_feedback")
}

func appendModelFeedbackIndex(dir string, entry modelFeedbackIndexEntry) error {
	path := filepath.Join(dir, "index.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func renderModelFeedbackRecordMarkdown(record modelFeedbackRuntimeRecord) string {
	var b strings.Builder
	b.WriteString("# Model Feedback Record\n\n")
	b.WriteString("## Metadata\n")
	b.WriteString(fmt.Sprintf("- Record ID: `%s`\n", record.RecordID))
	b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", safeFallback(record.RunID, "unknown")))
	b.WriteString(fmt.Sprintf("- Workbench ID: `%s`\n", safeFallback(record.WorkbenchID, "unknown")))
	b.WriteString(fmt.Sprintf("- Provider: `%s`\n", safeFallback(record.ProviderID, "unknown")))
	b.WriteString(fmt.Sprintf("- Model: `%s`\n", safeFallback(record.ModelID, "unknown")))
	b.WriteString(fmt.Sprintf("- Collection Status: `%s`\n", safeFallback(record.CollectionStatus, modelFeedbackStatusSkipped)))
	b.WriteString(fmt.Sprintf("- Collected At: `%s`\n", record.CollectedAt.Format(time.RFC3339)))
	if !record.RunStartedAt.IsZero() {
		b.WriteString(fmt.Sprintf("- Run Started: `%s`\n", record.RunStartedAt.Format(time.RFC3339)))
	}
	if !record.RunEndedAt.IsZero() {
		b.WriteString(fmt.Sprintf("- Run Ended: `%s`\n", record.RunEndedAt.Format(time.RFC3339)))
	}
	b.WriteString(fmt.Sprintf("- Run Elapsed (ms): `%d`\n", record.RunElapsedMS))
	b.WriteString(fmt.Sprintf("- Has Draft: `%t`\n", record.HasDraft))
	if record.SummaryMessageID != "" {
		b.WriteString(fmt.Sprintf("- Summary Message ID: `%s`\n", record.SummaryMessageID))
	}

	b.WriteString("\n## Objective Telemetry\n")
	b.WriteString(fmt.Sprintf("- Phases Completed: research=`%t`, plan=`%t`, implement=`%t`, summary=`%t`\n",
		record.PhaseStatus.Research,
		record.PhaseStatus.Plan,
		record.PhaseStatus.Implement,
		record.PhaseStatus.Summary,
	))
	b.WriteString(fmt.Sprintf("- Tool Calls Total: `%d`\n", record.ToolMetrics.ToolCallsTotal))
	b.WriteString(fmt.Sprintf("- Tool Calls Failed: `%d`\n", record.ToolMetrics.ToolCallsFailed))
	b.WriteString(fmt.Sprintf("- Tool Elapsed Total (ms): `%d`\n", record.ToolMetrics.ToolElapsedMS))
	if len(record.ToolMetrics.ToolCounts) > 0 {
		keys := make([]string, 0, len(record.ToolMetrics.ToolCounts))
		for key := range record.ToolMetrics.ToolCounts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString("- Tool Counts by Name:\n")
		for _, key := range keys {
			b.WriteString(fmt.Sprintf("  - `%s`: %d\n", key, record.ToolMetrics.ToolCounts[key]))
		}
	}
	if record.RunErrorCode != "" {
		b.WriteString("- Run Error:\n")
		b.WriteString(fmt.Sprintf("  - code: `%s`\n", record.RunErrorCode))
		b.WriteString(fmt.Sprintf("  - phase: `%s`\n", safeFallback(record.RunErrorPhase, "n/a")))
		b.WriteString(fmt.Sprintf("  - subphase: `%s`\n", safeFallback(record.RunErrorSubphase, "n/a")))
		if record.RunErrorDetail != "" {
			b.WriteString(fmt.Sprintf("  - detail: %s\n", strings.TrimSpace(record.RunErrorDetail)))
		}
	}
	if summary := strings.TrimSpace(record.SummaryText); summary != "" {
		b.WriteString("\n## Run Summary Text\n")
		b.WriteString(summary)
		b.WriteString("\n")
	}

	b.WriteString("\n## Model Feedback\n")
	if !isZeroSurvey(record.Survey) {
		b.WriteString("### What slowed me down\n")
		b.WriteString(nonEmptyOrPlaceholder(record.Survey.WhatSlowedMeDown))
		b.WriteString("\n\n")
		b.WriteString("### Hardest tool interaction\n")
		b.WriteString(nonEmptyOrPlaceholder(record.Survey.HardestToolInteraction))
		b.WriteString("\n\n")
		b.WriteString("### Highest-impact tooling change\n")
		b.WriteString(nonEmptyOrPlaceholder(record.Survey.HighestImpactingChange))
		b.WriteString("\n\n")
		b.WriteString("### Confidence (1-5)\n")
		b.WriteString(fmt.Sprintf("%d\n\n", record.Survey.Confidence))
		b.WriteString("### Additional notes\n")
		b.WriteString(nonEmptyOrPlaceholder(record.Survey.AdditionalNotes))
		b.WriteString("\n")
	} else {
		b.WriteString("_Feedback survey was not collected successfully._\n")
	}

	if strings.TrimSpace(record.ModelError) != "" {
		b.WriteString("\n## Model Collection Error\n")
		b.WriteString(strings.TrimSpace(record.ModelError))
		b.WriteString("\n")
	}
	if strings.TrimSpace(record.ParseError) != "" {
		b.WriteString("\n## Parse Error\n")
		b.WriteString(strings.TrimSpace(record.ParseError))
		b.WriteString("\n")
	}
	if strings.TrimSpace(record.RawFeedback) != "" {
		b.WriteString("\n## Raw Feedback Response\n")
		b.WriteString("```markdown\n")
		b.WriteString(strings.TrimSpace(record.RawFeedback))
		b.WriteString("\n```\n")
	}
	return b.String()
}

func safeFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func nonEmptyOrPlaceholder(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return strings.TrimSpace(value)
}

func summaryPreview(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= 200 {
		return trimmed
	}
	return trimmed[:200] + "..."
}

func isZeroSurvey(s modelFeedbackSurvey) bool {
	return strings.TrimSpace(s.WhatSlowedMeDown) == "" &&
		strings.TrimSpace(s.HardestToolInteraction) == "" &&
		strings.TrimSpace(s.HighestImpactingChange) == "" &&
		strings.TrimSpace(s.AdditionalNotes) == "" &&
		s.Confidence == 0
}

func loadModelFeedbackIndex(dataDir string) ([]modelFeedbackIndexEntry, error) {
	workbenchesRoot := filepath.Join(dataDir, "workbenches")
	items, err := os.ReadDir(workbenchesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	entries := make([]modelFeedbackIndexEntry, 0)
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		indexPath := filepath.Join(workbenchesRoot, item.Name(), "meta", "workshop", "model_feedback", "index.jsonl")
		fileEntries, readErr := readModelFeedbackIndexFile(indexPath)
		if readErr != nil {
			return nil, readErr
		}
		entries = append(entries, fileEntries...)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CollectedAt < entries[j].CollectedAt
	})
	return entries, nil
}

func readModelFeedbackIndexFile(path string) ([]modelFeedbackIndexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]modelFeedbackIndexEntry, 0)
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var entry modelFeedbackIndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if strings.TrimSpace(entry.RecordID) == "" {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}
