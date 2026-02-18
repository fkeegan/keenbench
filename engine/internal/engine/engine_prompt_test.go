package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWorkshopChatIncludesManifestAndInstruction(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Prompt Test"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	notesPath := filepath.Join(dataDir, "notes.txt")
	docxPath := filepath.Join(dataDir, "report.docx")
	binPath := filepath.Join(dataDir, "unknown.bin")
	if err := os.WriteFile(notesPath, []byte("hello notes"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if err := os.WriteFile(docxPath, []byte("docx text"), 0o600); err != nil {
		t.Fatalf("write docx: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("opaque"), 0o600); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{notesPath, docxPath, binPath},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	messages, errInfo := eng.buildChatMessages(ctx, workbenchID)
	if errInfo != nil {
		t.Fatalf("build messages: %v", errInfo)
	}
	if len(messages) == 0 {
		t.Fatalf("expected messages")
	}
	if messages[0].Role != "system" {
		t.Fatalf("expected system message first, got %s", messages[0].Role)
	}
	content := messages[0].Content
	if !strings.Contains(content, "Do not ask the user to upload") {
		t.Fatalf("expected upload suppression instruction")
	}
	if !strings.Contains(content, "notes.txt") || !strings.Contains(content, "hello notes") {
		t.Fatalf("expected text file content in prompt")
	}
	if !strings.Contains(content, "report.docx") || !strings.Contains(content, "docx text") {
		t.Fatalf("expected docx content in prompt")
	}
	if !strings.Contains(content, "unknown.bin") || !strings.Contains(content, workshopContentUnavailable) {
		t.Fatalf("expected opaque placeholder in prompt")
	}
}

func TestProposalPromptIncludesManifestAndPayloads(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Proposal Prompt"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	notesPath := filepath.Join(dataDir, "notes.txt")
	if err := os.WriteFile(notesPath, []byte("proposal notes"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{notesPath},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	prompt, errInfo := eng.buildProposalPrompt(ctx, workbenchID)
	if errInfo != nil {
		t.Fatalf("build proposal prompt: %v", errInfo)
	}
	if !strings.Contains(prompt, "Workbench files:") || !strings.Contains(prompt, "Manifest:") {
		t.Fatalf("expected manifest in proposal prompt")
	}
	if !strings.Contains(prompt, "Files listed are already available") {
		t.Fatalf("expected upload suppression instruction in proposal prompt")
	}
	if !strings.Contains(prompt, "notes.txt") || !strings.Contains(prompt, "proposal notes") {
		t.Fatalf("expected file payload in proposal prompt")
	}
}

func TestWorkshopChatTruncatesLargeFileContext(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Truncate Prompt"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	var builder strings.Builder
	for i := 0; i < maxContextLinesPerFile+10; i++ {
		builder.WriteString("line ")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString("\n")
	}
	notesPath := filepath.Join(dataDir, "big_notes.txt")
	if err := os.WriteFile(notesPath, []byte(builder.String()), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{notesPath},
	})); errInfo != nil {
		t.Fatalf("add files: %v", errInfo)
	}

	messages, errInfo := eng.buildChatMessages(ctx, workbenchID)
	if errInfo != nil {
		t.Fatalf("build messages: %v", errInfo)
	}
	if len(messages) == 0 {
		t.Fatalf("expected messages")
	}
	if !strings.Contains(messages[0].Content, workshopContentTruncated) {
		t.Fatalf("expected truncated notice in prompt")
	}
}

func TestWorkshopChatTrimsConversation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	os.Setenv("KEENBENCH_DATA_DIR", dataDir)
	os.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	defer os.Unsetenv("KEENBENCH_FAKE_TOOL_WORKER")

	eng, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	createResp, errInfo := eng.WorkbenchCreate(ctx, mustJSON(t, map[string]any{"name": "Trim Conversation"}))
	if errInfo != nil {
		t.Fatalf("create: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)

	totalMessages := maxConversationMessages + 4
	for i := 0; i < totalMessages; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		entry := conversationMessage{
			Type:      "test_message",
			MessageID: fmt.Sprintf("m-%d", i),
			Role:      role,
			Text:      fmt.Sprintf("msg %d", i),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := eng.appendConversation(workbenchID, entry); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	messages, errInfo := eng.buildChatMessages(ctx, workbenchID)
	if errInfo != nil {
		t.Fatalf("build messages: %v", errInfo)
	}
	if got, want := len(messages), 1+maxConversationMessages; got != want {
		t.Fatalf("expected %d messages, got %d", want, got)
	}
	last := messages[len(messages)-1].Content
	if !strings.Contains(last, fmt.Sprintf("msg %d", totalMessages-1)) {
		t.Fatalf("expected last message to be preserved")
	}
}

func TestProposalSystemPromptIncludesXlsxStylingOps(t *testing.T) {
	for _, op := range []string{"set_column_widths", "set_row_heights", "freeze_panes"} {
		if !strings.Contains(proposalSystemPrompt, op) {
			t.Fatalf("expected proposal prompt to include %s", op)
		}
	}
}

func TestValidateOpEntryXlsxStylingOps(t *testing.T) {
	tests := []struct {
		name    string
		op      map[string]any
		wantErr string
	}{
		{
			name: "set_column_widths valid",
			op: map[string]any{
				"op":    "set_column_widths",
				"sheet": "Summary",
				"columns": []any{
					map[string]any{"column": "A", "width": 25.0},
					map[string]any{"column": 2.0, "width": 14},
				},
			},
		},
		{
			name: "set_column_widths missing width",
			op: map[string]any{
				"op":    "set_column_widths",
				"sheet": "Summary",
				"columns": []any{
					map[string]any{"column": "A"},
				},
			},
			wantErr: "set_column_widths requires columns entries with column and width",
		},
		{
			name: "set_row_heights valid",
			op: map[string]any{
				"op":    "set_row_heights",
				"sheet": "Summary",
				"rows": []any{
					map[string]any{"row": 1.0, "height": 20.0},
				},
			},
		},
		{
			name: "set_row_heights invalid row index",
			op: map[string]any{
				"op":    "set_row_heights",
				"sheet": "Summary",
				"rows": []any{
					map[string]any{"row": 0.0, "height": 20.0},
				},
			},
			wantErr: "set_row_heights requires rows entries with row and height",
		},
		{
			name: "freeze_panes valid row and column",
			op: map[string]any{
				"op":     "freeze_panes",
				"sheet":  "Summary",
				"row":    1.0,
				"column": 0.0,
			},
		},
		{
			name: "freeze_panes valid row only",
			op: map[string]any{
				"op":    "freeze_panes",
				"sheet": "Summary",
				"row":   1.0,
			},
		},
		{
			name: "freeze_panes missing row and column",
			op: map[string]any{
				"op":    "freeze_panes",
				"sheet": "Summary",
			},
			wantErr: "freeze_panes requires row or column",
		},
		{
			name: "freeze_panes invalid column",
			op: map[string]any{
				"op":     "freeze_panes",
				"sheet":  "Summary",
				"column": -1.0,
			},
			wantErr: "freeze_panes requires column to be >= 0",
		},
	}

	for _, tc := range tests {
		err := validateOpEntry("xlsx", tc.op)
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: expected error containing %q, got %v", tc.name, tc.wantErr, err)
		}
	}
}

func TestBuildXlsxFocusHintTreatsSheetLevelStylingOpsAsTouchedContext(t *testing.T) {
	tests := []struct {
		name string
		op   map[string]any
	}{
		{
			name: "set_column_widths",
			op: map[string]any{
				"op":    "set_column_widths",
				"sheet": "Summary",
				"columns": []any{
					map[string]any{"column": "A", "width": 25.0},
				},
			},
		},
		{
			name: "set_row_heights",
			op: map[string]any{
				"op":    "set_row_heights",
				"sheet": "Summary",
				"rows": []any{
					map[string]any{"row": 1.0, "height": 20.0},
				},
			},
		},
		{
			name: "freeze_panes",
			op: map[string]any{
				"op":     "freeze_panes",
				"sheet":  "Summary",
				"row":    1.0,
				"column": 0.0,
			},
		},
	}

	for _, tc := range tests {
		hint := buildXlsxFocusHint([]map[string]any{tc.op})
		if hint == nil {
			t.Fatalf("%s: expected focus hint", tc.name)
		}
		if hint["sheet"] != "Summary" {
			t.Fatalf("%s: expected sheet Summary, got %v", tc.name, hint["sheet"])
		}
		if _, ok := hint["row_start"]; ok {
			t.Fatalf("%s: expected sheet-level hint without row bounds, got %v", tc.name, hint)
		}
	}
}
