package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"keenbench/engine/internal/appdirs"
)

type indexEntry struct {
	Version          int             `json:"version"`
	RecordID         string          `json:"record_id"`
	RecordPath       string          `json:"record_path"`
	RunID            string          `json:"run_id"`
	WorkbenchID      string          `json:"workbench_id"`
	ProviderID       string          `json:"provider_id,omitempty"`
	ModelID          string          `json:"model_id,omitempty"`
	CollectionStatus string          `json:"collection_status"`
	CollectedAt      string          `json:"collected_at"`
	RunStartedAt     string          `json:"run_started_at,omitempty"`
	RunEndedAt       string          `json:"run_ended_at,omitempty"`
	RunElapsedMS     int64           `json:"run_elapsed_ms,omitempty"`
	HasDraft         bool            `json:"has_draft"`
	PhaseStatus      map[string]bool `json:"phase_status,omitempty"`
	ToolCallsTotal   int             `json:"tool_calls_total,omitempty"`
	ToolCallsFailed  int             `json:"tool_calls_failed,omitempty"`
	ToolElapsedMS    int64           `json:"tool_elapsed_ms,omitempty"`
	ToolCounts       map[string]int  `json:"tool_counts,omitempty"`
	RunErrorCode     string          `json:"run_error_code,omitempty"`
	RunErrorPhase    string          `json:"run_error_phase,omitempty"`
	RunErrorSubphase string          `json:"run_error_subphase,omitempty"`
	RunErrorDetail   string          `json:"run_error_detail,omitempty"`
	ModelError       string          `json:"model_error,omitempty"`
	ParseError       string          `json:"parse_error,omitempty"`
	SummaryMessageID string          `json:"summary_message_id,omitempty"`
	SummaryPreview   string          `json:"summary_preview,omitempty"`
}

type runtimeRecord struct {
	Entry   indexEntry
	Content string
	When    time.Time
}

func main() {
	var outDir string
	var dataDir string
	var since string
	var max int
	var force bool

	flag.StringVar(&outDir, "out", "", "Output directory for exported model feedback issue docs")
	flag.StringVar(&dataDir, "data-dir", "", "KeenBench data directory (defaults to appdirs.DataDir())")
	flag.StringVar(&since, "since", "", "Only export entries collected at or after YYYY-MM-DD")
	flag.IntVar(&max, "max", 0, "Maximum number of records to export (0 means no limit)")
	flag.BoolVar(&force, "force", false, "Overwrite/export even when source_record_id already exists")
	flag.Parse()

	if strings.TrimSpace(outDir) == "" {
		exitErr(errors.New("--out is required"))
	}
	resolvedDataDir := strings.TrimSpace(dataDir)
	if resolvedDataDir == "" {
		var err error
		resolvedDataDir, err = appdirs.DataDir()
		if err != nil {
			exitErr(fmt.Errorf("resolve data dir: %w", err))
		}
	}
	resolvedOut, err := filepath.Abs(outDir)
	if err != nil {
		exitErr(fmt.Errorf("resolve output dir: %w", err))
	}
	if err := os.MkdirAll(resolvedOut, 0o755); err != nil {
		exitErr(fmt.Errorf("create output dir: %w", err))
	}

	var sinceTime time.Time
	if strings.TrimSpace(since) != "" {
		sinceTime, err = time.Parse("2006-01-02", strings.TrimSpace(since))
		if err != nil {
			exitErr(fmt.Errorf("invalid --since date: %w", err))
		}
		sinceTime = sinceTime.UTC()
	}

	records, err := loadRuntimeRecords(resolvedDataDir)
	if err != nil {
		exitErr(err)
	}
	if len(records) == 0 {
		fmt.Println("No model feedback runtime records found.")
		return
	}

	exportedIDs := map[string]bool{}
	if !force {
		exportedIDs, err = loadExportedSourceIDs(resolvedOut)
		if err != nil {
			exitErr(err)
		}
	}

	exportedCount := 0
	skippedCount := 0
	for _, record := range records {
		if !sinceTime.IsZero() && record.When.Before(sinceTime) {
			skippedCount++
			continue
		}
		if !force && exportedIDs[record.Entry.RecordID] {
			skippedCount++
			continue
		}

		docPath, writeErr := writeIssueDoc(resolvedOut, record)
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "warn: failed to write issue doc for %s: %v\n", record.Entry.RecordID, writeErr)
			skippedCount++
			continue
		}
		exportedCount++
		exportedIDs[record.Entry.RecordID] = true
		fmt.Printf("Exported %s\n", docPath)
		if max > 0 && exportedCount >= max {
			break
		}
	}

	fmt.Printf("Export complete. exported=%d skipped=%d\n", exportedCount, skippedCount)
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func loadRuntimeRecords(dataDir string) ([]runtimeRecord, error) {
	workbenchesRoot := appdirs.WorkbenchesDir(dataDir)
	items, err := os.ReadDir(workbenchesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list workbenches: %w", err)
	}
	records := make([]runtimeRecord, 0)
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		indexPath := filepath.Join(workbenchesRoot, item.Name(), "meta", "workshop", "model_feedback", "index.jsonl")
		entries, err := readIndexEntries(indexPath)
		if err != nil {
			return nil, fmt.Errorf("read index %s: %w", indexPath, err)
		}
		for _, entry := range entries {
			recordPath := strings.TrimSpace(entry.RecordPath)
			if recordPath == "" {
				recordPath = filepath.Join(workbenchesRoot, entry.WorkbenchID, "meta", "workshop", "model_feedback", entry.RecordID+".md")
			}
			content, err := os.ReadFile(recordPath)
			if err != nil {
				return nil, fmt.Errorf("read runtime record %s: %w", recordPath, err)
			}
			when, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.CollectedAt))
			if err != nil {
				when = time.Unix(0, 0).UTC()
			}
			records = append(records, runtimeRecord{
				Entry:   entry,
				Content: string(content),
				When:    when.UTC(),
			})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].When.Before(records[j].When)
	})
	return records, nil
}

func readIndexEntries(path string) ([]indexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	entries := make([]indexEntry, 0)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var entry indexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if strings.TrimSpace(entry.RecordID) == "" {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func loadExportedSourceIDs(outDir string) (map[string]bool, error) {
	ids := map[string]bool{}
	pattern := regexp.MustCompile(`(?m)^- source_record_id:\s*` + "`?" + `([^` + "`" + `\s]+)` + "`?" + `$`)
	err := filepath.WalkDir(outDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		match := pattern.FindStringSubmatch(string(data))
		if len(match) == 2 {
			ids[strings.TrimSpace(match[1])] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan exported docs: %w", err)
	}
	return ids, nil
}

func writeIssueDoc(outDir string, record runtimeRecord) (string, error) {
	datePart := record.When.Format("2006-01-02")
	if strings.TrimSpace(datePart) == "" || datePart == "0001-01-01" {
		datePart = "unknown-date"
	}
	dayDir := filepath.Join(outDir, datePart)
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		return "", err
	}
	modelSlug := slugify(record.Entry.ModelID)
	if modelSlug == "" {
		modelSlug = "unknown-model"
	}
	fileName := fmt.Sprintf("%s-%s-%s.md", datePart, modelSlug, shortSlug(record.Entry.RecordID, 20))
	path := filepath.Join(dayDir, fileName)
	content := renderIssueDoc(record)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func renderIssueDoc(record runtimeRecord) string {
	entry := record.Entry
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Model Feedback Intake - %s\n\n", entry.RecordID))
	b.WriteString("Source runtime record metadata:\n")
	b.WriteString(fmt.Sprintf("- source_record_id: `%s`\n", entry.RecordID))
	b.WriteString(fmt.Sprintf("- collected_at: `%s`\n", entry.CollectedAt))
	b.WriteString(fmt.Sprintf("- run_id: `%s`\n", fallback(entry.RunID, "unknown")))
	b.WriteString(fmt.Sprintf("- workbench_id: `%s`\n", fallback(entry.WorkbenchID, "unknown")))
	b.WriteString(fmt.Sprintf("- provider_id: `%s`\n", fallback(entry.ProviderID, "unknown")))
	b.WriteString(fmt.Sprintf("- model_id: `%s`\n", fallback(entry.ModelID, "unknown")))
	b.WriteString(fmt.Sprintf("- collection_status: `%s`\n", fallback(entry.CollectionStatus, "unknown")))
	b.WriteString(fmt.Sprintf("- runtime_record_path: `%s`\n", fallback(entry.RecordPath, "unknown")))
	b.WriteString("\n## Intake Candidate\n")
	b.WriteString("- Candidate backlog title: `Model feedback: " + fallback(entry.ModelID, "unknown-model") + " / " + fallback(entry.CollectionStatus, "unknown-status") + "`\n")
	if strings.TrimSpace(entry.SummaryPreview) != "" {
		b.WriteString("- Summary preview: " + strings.TrimSpace(entry.SummaryPreview) + "\n")
	}
	if strings.TrimSpace(entry.RunErrorCode) != "" {
		b.WriteString("- Run error: `" + strings.TrimSpace(entry.RunErrorCode) + "`\n")
	}
	if strings.TrimSpace(entry.ModelError) != "" {
		b.WriteString("- Model collection error: " + strings.TrimSpace(entry.ModelError) + "\n")
	}
	if strings.TrimSpace(entry.ParseError) != "" {
		b.WriteString("- Parse error: " + strings.TrimSpace(entry.ParseError) + "\n")
	}
	b.WriteString("\n## Runtime Record\n\n")
	b.WriteString(record.Content)
	if !strings.HasSuffix(record.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func slugify(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, ":", "-")
	normalized = strings.ReplaceAll(normalized, "/", "-")
	clean := regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(normalized, "-")
	clean = strings.Trim(clean, "-")
	return clean
}

func shortSlug(value string, max int) string {
	s := slugify(value)
	if s == "" {
		return "record"
	}
	if max > 0 && len(s) > max {
		return s[:max]
	}
	return s
}
