package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadExportedSourceIDs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "2026-02-22", "item.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "# Intake\n\n- source_record_id: `mf-123`\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	ids, err := loadExportedSourceIDs(root)
	if err != nil {
		t.Fatalf("load ids: %v", err)
	}
	if !ids["mf-123"] {
		t.Fatalf("expected mf-123 in exported ids, got: %+v", ids)
	}
}

func TestWriteIssueDoc(t *testing.T) {
	out := t.TempDir()
	record := runtimeRecord{
		Entry: indexEntry{
			RecordID:         "mf-abc",
			ModelID:          "openai:gpt-5.4",
			CollectionStatus: "collected",
			CollectedAt:      "2026-02-22T10:11:12Z",
			RunID:            "run-1",
			WorkbenchID:      "wb-1",
			ProviderID:       "openai",
			RecordPath:       "/tmp/runtime.md",
		},
		Content: "# Runtime\n",
		When:    time.Date(2026, 2, 22, 10, 11, 12, 0, time.UTC),
	}
	path, err := writeIssueDoc(out, record)
	if err != nil {
		t.Fatalf("write issue doc: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected issue doc at %s: %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read issue doc: %v", err)
	}
	if got := string(data); got == "" || !containsAll(got, "source_record_id", "mf-abc", "## Runtime Record") {
		t.Fatalf("unexpected issue doc content: %s", got)
	}
}

func containsAll(input string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(input, needle) {
			return false
		}
	}
	return true
}
