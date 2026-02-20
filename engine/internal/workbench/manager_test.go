package workbench

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilesAddSemantics(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fileA := filepath.Join(root, "a.txt")
	fileB := filepath.Join(root, "b.csv")
	fileLarge := filepath.Join(root, "large.txt")
	if err := os.WriteFile(fileA, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("a,b\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(fileLarge, []byte("x"), 0o600); err != nil {
		t.Fatalf("write large: %v", err)
	}
	if err := os.Truncate(fileLarge, maxSize+1); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	results, err := mgr.FilesAdd(wb.ID, []string{fileA, fileB, fileLarge, fileA})
	if err != nil {
		t.Fatalf("files add: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	files, err := mgr.FilesList(wb.ID)
	if err != nil {
		t.Fatalf("files list: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestFilesAddSymlinkRejected(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	target := filepath.Join(root, "real.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	results, err := mgr.FilesAdd(wb.ID, []string{link})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(results) != 1 || results[0].Status != "skipped" {
		t.Fatalf("expected symlink skipped")
	}
}

func TestFilesAddLimitRejectsBatch(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var paths []string
	for i := 0; i < maxFiles+1; i++ {
		p := filepath.Join(root, "file-"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(p, []byte("ok"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		paths = append(paths, p)
	}
	_, err = mgr.FilesAdd(wb.ID, paths)
	if err == nil {
		t.Fatalf("expected error when exceeding file limit")
	}
	files, err := mgr.FilesList(wb.ID)
	if err != nil {
		t.Fatalf("files list: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestFilesRemove(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fileA := filepath.Join(root, "a.txt")
	fileB := filepath.Join(root, "b.csv")
	if err := os.WriteFile(fileA, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("a,b\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{fileA, fileB}); err != nil {
		t.Fatalf("add: %v", err)
	}
	results, err := mgr.FilesRemove(wb.ID, []string{"a.txt"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(results) != 1 || results[0].Status != "removed" {
		t.Fatalf("expected removed result")
	}
	files, err := mgr.FilesList(wb.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 1 || files[0].Path != "b.csv" {
		t.Fatalf("expected only b.csv after removal")
	}
	publishedPath := filepath.Join(root, "workbenches", wb.ID, "published", "a.txt")
	if _, err := os.Stat(publishedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected removed file on disk")
	}
}

func TestFilesRemoveCleansTabularArtifacts(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	csvPath := filepath.Join(root, "sales.csv")
	if err := os.WriteFile(csvPath, []byte("region,amount\nwest,10\n"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{csvPath}); err != nil {
		t.Fatalf("add: %v", err)
	}

	tabularDir := filepath.Join(root, "workbenches", wb.ID, "meta", "tabular")
	if err := os.MkdirAll(tabularDir, 0o755); err != nil {
		t.Fatalf("mkdir tabular: %v", err)
	}
	key := tabularCacheKey("sales.csv")
	for _, suffix := range []string{".duckdb", ".duckdb.wal", ".meta.json", ".json", ".utf8.csv"} {
		path := filepath.Join(tabularDir, key+suffix)
		if err := os.WriteFile(path, []byte("artifact"), 0o600); err != nil {
			t.Fatalf("write artifact %s: %v", suffix, err)
		}
	}

	results, err := mgr.FilesRemove(wb.ID, []string{"sales.csv"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(results) != 1 || results[0].Status != "removed" {
		t.Fatalf("expected removed result, got %#v", results)
	}

	for _, suffix := range []string{".duckdb", ".duckdb.wal", ".meta.json", ".json", ".utf8.csv"} {
		path := filepath.Join(tabularDir, key+suffix)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s artifact to be removed", suffix)
		}
	}
}

func TestDeleteRemovesWorkbenchIncludingTabularArtifacts(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tabularDir := filepath.Join(root, "workbenches", wb.ID, "meta", "tabular")
	if err := os.MkdirAll(tabularDir, 0o755); err != nil {
		t.Fatalf("mkdir tabular: %v", err)
	}
	artifactPath := filepath.Join(tabularDir, tabularCacheKey("sales.csv")+".duckdb")
	if err := os.WriteFile(artifactPath, []byte("artifact"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	if err := mgr.Delete(wb.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	wbPath := filepath.Join(root, "workbenches", wb.ID)
	if _, err := os.Stat(wbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workbench directory removal, stat err=%v", err)
	}
}

func TestApplyDraftWriteCleansTabularArtifactsForCSV(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	csvPath := filepath.Join(root, "sales.csv")
	if err := os.WriteFile(csvPath, []byte("region,amount\nwest,10\n"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{csvPath}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}

	tabularDir := filepath.Join(root, "workbenches", wb.ID, "meta", "tabular")
	if err := os.MkdirAll(tabularDir, 0o755); err != nil {
		t.Fatalf("mkdir tabular: %v", err)
	}
	key := tabularCacheKey("sales.csv")
	for _, suffix := range []string{".duckdb", ".duckdb.wal", ".meta.json", ".json", ".utf8.csv"} {
		path := filepath.Join(tabularDir, key+suffix)
		if err := os.WriteFile(path, []byte("artifact"), 0o600); err != nil {
			t.Fatalf("write artifact %s: %v", suffix, err)
		}
	}

	if err := mgr.ApplyDraftWrite(wb.ID, "sales.csv", "region,amount\nwest,15\n"); err != nil {
		t.Fatalf("apply draft write: %v", err)
	}

	for _, suffix := range []string{".duckdb", ".duckdb.wal", ".meta.json", ".json", ".utf8.csv"} {
		path := filepath.Join(tabularDir, key+suffix)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s artifact to be removed", suffix)
		}
	}
}

func TestFilesRemoveBlockedByDraft(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fileA := filepath.Join(root, "a.txt")
	if err := os.WriteFile(fileA, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{fileA}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if _, err := mgr.FilesRemove(wb.ID, []string{"a.txt"}); err == nil {
		t.Fatalf("expected draft exists error")
	}
}

func TestFilesExtractAll(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fileA := filepath.Join(root, "a.txt")
	fileB := filepath.Join(root, "b.csv")
	if err := os.WriteFile(fileA, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("a,b\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{fileA, fileB}); err != nil {
		t.Fatalf("add: %v", err)
	}

	dest := filepath.Join(root, "extract")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	results, err := mgr.FilesExtract(wb.ID, dest, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, result := range results {
		if result.Status != "extracted" {
			t.Fatalf("expected extracted result, got %#v", result)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "a.txt")); err != nil {
		t.Fatalf("expected a.txt in destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "b.csv")); err != nil {
		t.Fatalf("expected b.csv in destination: %v", err)
	}
}

func TestFilesExtractBlockedByDraft(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fileA := filepath.Join(root, "a.txt")
	if err := os.WriteFile(fileA, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{fileA}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	dest := filepath.Join(root, "extract")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := mgr.FilesExtract(wb.ID, dest, []string{"a.txt"}); err == nil {
		t.Fatalf("expected draft exists error")
	}
}

func TestFilesExtractRenamesExistingDestination(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fileA := filepath.Join(root, "a.txt")
	if err := os.WriteFile(fileA, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{fileA}); err != nil {
		t.Fatalf("add: %v", err)
	}
	dest := filepath.Join(root, "extract")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "a.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	results, err := mgr.FilesExtract(wb.ID, dest, []string{"a.txt"})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(results) != 1 || results[0].Status != "extracted" || results[0].FinalPath != "a(1).txt" {
		t.Fatalf("expected renamed extract result, got %#v", results)
	}
	if _, err := os.Stat(filepath.Join(dest, "a(1).txt")); err != nil {
		t.Fatalf("expected renamed destination file: %v", err)
	}
}

func TestFilesExtractRepeatedUsesIncrementingSuffixes(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	filePath := filepath.Join(root, "file.xlsx")
	if err := os.WriteFile(filePath, []byte("sheet"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{filePath}); err != nil {
		t.Fatalf("add: %v", err)
	}
	dest := filepath.Join(root, "extract")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	expectedNames := []string{"file.xlsx", "file(1).xlsx", "file(2).xlsx"}
	for i, expected := range expectedNames {
		results, err := mgr.FilesExtract(wb.ID, dest, []string{"file.xlsx"})
		if err != nil {
			t.Fatalf("extract iteration %d: %v", i, err)
		}
		if len(results) != 1 || results[0].Status != "extracted" || results[0].FinalPath != expected {
			t.Fatalf("unexpected result on iteration %d: %#v", i, results)
		}
	}

	for _, expected := range expectedNames {
		if _, err := os.Stat(filepath.Join(dest, expected)); err != nil {
			t.Fatalf("expected extracted file %q: %v", expected, err)
		}
	}
}

func TestFilesExtractUsesFirstAvailableSuffix(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	filePath := filepath.Join(root, "file.xlsx")
	if err := os.WriteFile(filePath, []byte("sheet"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{filePath}); err != nil {
		t.Fatalf("add: %v", err)
	}
	dest := filepath.Join(root, "extract")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "file.xlsx"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "file(2).xlsx"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing suffix: %v", err)
	}

	results, err := mgr.FilesExtract(wb.ID, dest, []string{"file.xlsx"})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(results) != 1 || results[0].Status != "extracted" || results[0].FinalPath != "file(1).xlsx" {
		t.Fatalf("expected first available suffix, got %#v", results)
	}
}

func TestFilesExtractRenamesDotfileAndMultiDotNames(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dotfilePath := filepath.Join(root, ".env")
	if err := os.WriteFile(dotfilePath, []byte("KEY=value"), 0o600); err != nil {
		t.Fatalf("write dotfile: %v", err)
	}
	multiDotPath := filepath.Join(root, "archive.tar.gz")
	if err := os.WriteFile(multiDotPath, []byte("archive"), 0o600); err != nil {
		t.Fatalf("write multi-dot: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{dotfilePath, multiDotPath}); err != nil {
		t.Fatalf("add: %v", err)
	}
	dest := filepath.Join(root, "extract")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, ".env"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing dotfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "archive.tar.gz"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing multi-dot: %v", err)
	}

	dotfileResults, err := mgr.FilesExtract(wb.ID, dest, []string{".env"})
	if err != nil {
		t.Fatalf("extract dotfile: %v", err)
	}
	if len(dotfileResults) != 1 || dotfileResults[0].Status != "extracted" || dotfileResults[0].FinalPath != ".env(1)" {
		t.Fatalf("expected renamed dotfile, got %#v", dotfileResults)
	}

	multiDotResults, err := mgr.FilesExtract(wb.ID, dest, []string{"archive.tar.gz"})
	if err != nil {
		t.Fatalf("extract multi-dot: %v", err)
	}
	if len(multiDotResults) != 1 || multiDotResults[0].Status != "extracted" || multiDotResults[0].FinalPath != "archive.tar(1).gz" {
		t.Fatalf("expected renamed multi-dot, got %#v", multiDotResults)
	}
}

func TestWorkbenchDelete(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := mgr.Delete(wb.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "workbenches", wb.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workbench to be deleted")
	}
}

func TestWorkbenchForkCloneAllPreservesHistoryAndContext(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	source, err := mgr.Create("Source", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceFile := filepath.Join(root, "source.txt")
	if err := os.WriteFile(sourceFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if _, err := mgr.FilesAdd(source.ID, []string{sourceFile}); err != nil {
		t.Fatalf("files add: %v", err)
	}
	if _, err := mgr.CheckpointCreate(source.ID, "manual", "before fork"); err != nil {
		t.Fatalf("checkpoint create: %v", err)
	}
	conversationPath := filepath.Join(root, "workbenches", source.ID, "meta", "conversation.jsonl")
	if err := os.WriteFile(conversationPath, []byte("{\"id\":\"u-1\"}\n"), 0o600); err != nil {
		t.Fatalf("write conversation: %v", err)
	}
	contextDir := filepath.Join(root, "workbenches", source.ID, "meta", "context", "situation")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("mkdir context: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "context.md"), []byte("Current workbench context"), 0o600); err != nil {
		t.Fatalf("write context file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "source.json"), []byte("{\"mode\":\"text\"}"), 0o600); err != nil {
		t.Fatalf("write context source: %v", err)
	}
	consent := &Consent{
		Workshop: WorkshopConsent{
			ProviderID: "openai",
			ModelID:    "openai:gpt-5.2",
			ScopeHash:  "abc",
		},
	}
	if err := mgr.WriteConsent(source.ID, consent); err != nil {
		t.Fatalf("write consent: %v", err)
	}

	forked, err := mgr.Fork(source.ID, ForkModeCloneAll, "Source Fork", "u-1")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.ID == source.ID {
		t.Fatalf("expected different fork id")
	}
	if forked.ParentWorkbenchID != source.ID {
		t.Fatalf("expected parent_workbench_id %q, got %q", source.ID, forked.ParentWorkbenchID)
	}
	if forked.ForkMode != ForkModeCloneAll {
		t.Fatalf("expected fork mode %q, got %q", ForkModeCloneAll, forked.ForkMode)
	}
	if forked.ForkedFromMessageID != "u-1" {
		t.Fatalf("expected forked_from_message_id u-1, got %q", forked.ForkedFromMessageID)
	}
	if forked.ForkedAt == "" {
		t.Fatalf("expected forked_at to be set")
	}

	forkConversationPath := filepath.Join(root, "workbenches", forked.ID, "meta", "conversation.jsonl")
	conversationData, err := os.ReadFile(forkConversationPath)
	if err != nil {
		t.Fatalf("read fork conversation: %v", err)
	}
	if string(conversationData) != "{\"id\":\"u-1\"}\n" {
		t.Fatalf("expected conversation copy, got %q", string(conversationData))
	}
	forkContextPath := filepath.Join(root, "workbenches", forked.ID, "meta", "context", "situation", "context.md")
	if _, err := os.Stat(forkContextPath); err != nil {
		t.Fatalf("expected context copy: %v", err)
	}
	checkpoints, err := mgr.CheckpointsList(forked.ID)
	if err != nil {
		t.Fatalf("checkpoints list: %v", err)
	}
	if len(checkpoints) == 0 {
		t.Fatalf("expected copied checkpoints")
	}
	if _, err := os.Stat(filepath.Join(root, "workbenches", forked.ID, "meta", "egress_consent.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected consent not copied, got err=%v", err)
	}
	files, err := mgr.FilesList(forked.ID)
	if err != nil {
		t.Fatalf("files list: %v", err)
	}
	if len(files) != 1 || files[0].Path != "source.txt" {
		t.Fatalf("expected copied file manifest, got %#v", files)
	}
	if _, err := os.Stat(filepath.Join(root, "workbenches", forked.ID, "draft")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no draft in fork, got err=%v", err)
	}
}

func TestWorkbenchForkCloneFilesAndContextNoChatClearsHistory(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	source, err := mgr.Create("Source", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceFile := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(sourceFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if _, err := mgr.FilesAdd(source.ID, []string{sourceFile}); err != nil {
		t.Fatalf("files add: %v", err)
	}
	if _, err := mgr.CheckpointCreate(source.ID, "manual", "before fork"); err != nil {
		t.Fatalf("checkpoint create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "workbenches", source.ID, "meta", "conversation.jsonl"), []byte("{\"id\":\"u-1\"}\n"), 0o600); err != nil {
		t.Fatalf("write conversation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "workbenches", source.ID, "meta", "workshop_state.json"), []byte("{\"active_model_id\":\"openai:gpt-5.2\"}"), 0o600); err != nil {
		t.Fatalf("write workshop state: %v", err)
	}
	contextDir := filepath.Join(root, "workbenches", source.ID, "meta", "context", "situation")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("mkdir context: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "context.md"), []byte("kept context"), 0o600); err != nil {
		t.Fatalf("write context: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "workbenches", source.ID, "meta", "egress_consent.json"), []byte("{\"schema_version\":2}"), 0o600); err != nil {
		t.Fatalf("write consent: %v", err)
	}

	forked, err := mgr.Fork(source.ID, ForkModeCloneFilesAndContextNoChat, "", "")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.ParentWorkbenchID != source.ID {
		t.Fatalf("expected parent_workbench_id %q, got %q", source.ID, forked.ParentWorkbenchID)
	}
	if forked.ForkMode != ForkModeCloneFilesAndContextNoChat {
		t.Fatalf("expected fork mode %q, got %q", ForkModeCloneFilesAndContextNoChat, forked.ForkMode)
	}
	if !strings.Contains(strings.ToLower(forked.Name), strings.ToLower(source.Name)) {
		t.Fatalf("expected generated fork name to include source name, got %q", forked.Name)
	}

	forkRoot := filepath.Join(root, "workbenches", forked.ID)
	if _, err := os.Stat(filepath.Join(forkRoot, "meta", "conversation.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected conversation removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(forkRoot, "meta", "workshop_state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workshop_state removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(forkRoot, "meta", "checkpoints")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected checkpoints removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(forkRoot, "meta", "context", "situation", "context.md")); err != nil {
		t.Fatalf("expected context kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(forkRoot, "meta", "egress_consent.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected consent not copied, got err=%v", err)
	}
	files, err := mgr.FilesList(forked.ID)
	if err != nil {
		t.Fatalf("files list: %v", err)
	}
	if len(files) != 1 || files[0].Path != "notes.txt" {
		t.Fatalf("expected copied files, got %#v", files)
	}
}

func TestWorkbenchForkCloneFilesOnlyClearsContext(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	source, err := mgr.Create("Source", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceFile := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(sourceFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if _, err := mgr.FilesAdd(source.ID, []string{sourceFile}); err != nil {
		t.Fatalf("files add: %v", err)
	}
	contextDir := filepath.Join(root, "workbenches", source.ID, "meta", "context", "situation")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("mkdir context: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "context.md"), []byte("not copied"), 0o600); err != nil {
		t.Fatalf("write context: %v", err)
	}

	forked, err := mgr.Fork(source.ID, ForkModeCloneFilesOnly, "", "")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	forkRoot := filepath.Join(root, "workbenches", forked.ID)
	if _, err := os.Stat(filepath.Join(forkRoot, "meta", "context")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected context removed, got err=%v", err)
	}
	files, err := mgr.FilesList(forked.ID)
	if err != nil {
		t.Fatalf("files list: %v", err)
	}
	if len(files) != 1 || files[0].Path != "notes.txt" {
		t.Fatalf("expected copied files, got %#v", files)
	}
}

func TestWorkbenchForkBlockedByDraft(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	source, err := mgr.Create("Source", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := mgr.CreateDraft(source.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if _, err := mgr.Fork(source.ID, ForkModeCloneAll, "fork", ""); err == nil {
		t.Fatalf("expected draft exists error")
	}
}

func TestDraftLifecycle(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	src := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{src}); err != nil {
		t.Fatalf("add: %v", err)
	}
	state, err := mgr.CreateDraft(wb.ID)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if state == nil {
		t.Fatalf("expected draft state")
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "notes.txt", "updated"); err != nil {
		t.Fatalf("apply write: %v", err)
	}
	_, err = mgr.PublishDraft(wb.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	content, err := mgr.ReadFile(wb.ID, "published", "notes.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if content != "updated" {
		t.Fatalf("expected updated content, got %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, "workbenches", wb.ID, "draft")); err == nil {
		t.Fatalf("draft should be removed after publish")
	}
}

func TestCheckpointRestorePublishedOnly(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Checkpoint", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	src := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(src, []byte("v0"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{src}); err != nil {
		t.Fatalf("add: %v", err)
	}
	checkpointID, err := mgr.CheckpointCreate(wb.ID, "manual", "before update")
	if err != nil {
		t.Fatalf("checkpoint create: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "notes.txt", "v1"); err != nil {
		t.Fatalf("apply draft write: %v", err)
	}
	if _, err := mgr.PublishDraft(wb.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := mgr.CheckpointRestorePublished(wb.ID, checkpointID); err != nil {
		t.Fatalf("restore published: %v", err)
	}
	content, err := mgr.ReadFile(wb.ID, "published", "notes.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if content != "v0" {
		t.Fatalf("expected restored published content v0, got %q", content)
	}
}

func TestCreateDraftWithSourcePersistsMetadata(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	state, err := mgr.CreateDraftWithSource(wb.ID, "workshop", "agent")
	if err != nil {
		t.Fatalf("create draft with source: %v", err)
	}
	if state.SourceKind != "workshop" || state.SourceRef != "agent" {
		t.Fatalf("unexpected source metadata: %#v", state)
	}
	loaded, err := mgr.DraftState(wb.ID)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	if loaded.SourceKind != "workshop" || loaded.SourceRef != "agent" {
		t.Fatalf("expected source metadata to persist: %#v", loaded)
	}
}

func TestDraftStateLegacySourceCompatibility(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	legacyDraft := map[string]any{
		"draft_id":   "draft-legacy",
		"created_at": "2026-02-01T00:00:00Z",
		"source": map[string]any{
			"kind":   "delegate",
			"job_id": "job-123",
		},
	}
	draftPath := filepath.Join(root, "workbenches", wb.ID, "meta", "draft.json")
	if err := writeJSON(draftPath, legacyDraft); err != nil {
		t.Fatalf("write legacy draft: %v", err)
	}
	state, err := mgr.DraftState(wb.ID)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	if state == nil {
		t.Fatalf("expected draft state")
	}
	if state.SourceKind != "delegate" {
		t.Fatalf("expected source_kind delegate, got %q", state.SourceKind)
	}
	if state.SourceRef != "job-123" {
		t.Fatalf("expected source_ref job-123, got %q", state.SourceRef)
	}
}

func TestInvalidDraftPath(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = mgr.CreateDraft(wb.ID)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "../oops.txt", "bad"); err == nil {
		t.Fatalf("expected invalid path error")
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "nested/oops.txt", "bad"); err == nil {
		t.Fatalf("expected invalid path error for nested path")
	}
}

func TestReadFileInvalidPath(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := mgr.ReadFile(wb.ID, "published", "../nope.txt"); err == nil {
		t.Fatalf("expected invalid path error")
	}
	if _, err := mgr.ReadFile(wb.ID, "published", "nested/nope.txt"); err == nil {
		t.Fatalf("expected invalid path error for nested path")
	}
}

func TestConsentAndChangeSet(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	src := filepath.Join(root, "data.txt")
	if err := os.WriteFile(src, []byte("one"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{src}); err != nil {
		t.Fatalf("add: %v", err)
	}
	scope, err := mgr.ComputeScopeHash(wb.ID)
	if err != nil || scope == "" {
		t.Fatalf("scope hash failed")
	}
	consent := &Consent{Workshop: WorkshopConsent{ProviderID: "openai", ScopeHash: scope, ConsentedAt: "now"}}
	if err := mgr.WriteConsent(wb.ID, consent); err != nil {
		t.Fatalf("write consent: %v", err)
	}
	read, err := mgr.ReadConsent(wb.ID)
	if err != nil {
		t.Fatalf("read consent: %v", err)
	}
	if read.Workshop.ScopeHash != scope {
		t.Fatalf("expected scope hash to persist")
	}

	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "data.txt", "two"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	changes, err := mgr.ChangeSet(wb.ID)
	if err != nil {
		t.Fatalf("changeset: %v", err)
	}
	if len(changes) != 1 || changes[0].ChangeType != "modified" {
		t.Fatalf("expected modified change")
	}
}

func TestComputeScopeHashIgnoresContentChanges(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	src := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(src, []byte("one"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{src}); err != nil {
		t.Fatalf("add: %v", err)
	}
	hash1, err := mgr.ComputeScopeHash(wb.ID)
	if err != nil || hash1 == "" {
		t.Fatalf("scope hash failed")
	}
	publishedPath := filepath.Join(root, "workbenches", wb.ID, "published", "notes.txt")
	if err := os.WriteFile(publishedPath, []byte("two"), 0o600); err != nil {
		t.Fatalf("external write: %v", err)
	}
	newTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(publishedPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	hash2, err := mgr.ComputeScopeHash(wb.ID)
	if err != nil || hash2 == "" {
		t.Fatalf("scope hash failed")
	}
	if hash1 != hash2 {
		t.Fatalf("expected scope hash to ignore content changes")
	}

	extra := filepath.Join(root, "extra.txt")
	if err := os.WriteFile(extra, []byte("more"), 0o600); err != nil {
		t.Fatalf("write extra: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{extra}); err != nil {
		t.Fatalf("add extra: %v", err)
	}
	hash3, err := mgr.ComputeScopeHash(wb.ID)
	if err != nil || hash3 == "" {
		t.Fatalf("scope hash failed")
	}
	if hash3 == hash1 {
		t.Fatalf("expected scope hash to change when file list changes")
	}
}

func TestChangeSetDeletionDetected(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	src := filepath.Join(root, "data.txt")
	if err := os.WriteFile(src, []byte("one"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{src}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	draftPath := filepath.Join(root, "workbenches", wb.ID, "draft", "data.txt")
	if err := os.Remove(draftPath); err != nil {
		t.Fatalf("remove draft file: %v", err)
	}
	if _, err := mgr.ChangeSet(wb.ID); err == nil || !errors.Is(err, ErrDeletionDetected) {
		t.Fatalf("expected deletion detected error")
	}
}

func TestFilesAddOpaqueAllowed(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	filePath := filepath.Join(root, "blob.bin")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{filePath}); err != nil {
		t.Fatalf("add: %v", err)
	}
	files, err := mgr.FilesList(wb.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files list failed")
	}
	if !files[0].IsOpaque || files[0].FileKind != FileKindBinary {
		t.Fatalf("expected opaque binary entry")
	}
}

func TestManifestMigrationAddsMetadata(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	published := filepath.Join(root, "workbenches", wb.ID, "published")
	if err := os.WriteFile(filepath.Join(published, "notes.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	manifestPath := filepath.Join(root, "workbenches", wb.ID, "meta", "files.json")
	legacy := FileManifest{
		SchemaVersion: 1,
		Files: []FileEntry{{
			Path:       "notes.txt",
			Size:       5,
			ModifiedAt: time.Now().UTC().Format(time.RFC3339),
			AddedAt:    time.Now().UTC().Format(time.RFC3339),
		}},
	}
	if err := writeJSON(manifestPath, legacy); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	files, err := mgr.FilesList(wb.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files list failed")
	}
	if files[0].FileKind != FileKindText {
		t.Fatalf("expected migrated file kind %q, got %q", FileKindText, files[0].FileKind)
	}
	if files[0].MimeType != mimeTypeForExtension(".txt") {
		t.Fatalf("expected migrated mime type %q, got %q", mimeTypeForExtension(".txt"), files[0].MimeType)
	}
	if files[0].IsOpaque {
		t.Fatalf("expected migrated text file to be non-opaque")
	}
}

func TestManifestMigrationNormalizesLegacyExtensionFileKind(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	published := filepath.Join(root, "workbenches", wb.ID, "published")
	if err := os.WriteFile(filepath.Join(published, "notes.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	manifestPath := filepath.Join(root, "workbenches", wb.ID, "meta", "files.json")
	now := time.Now().UTC().Format(time.RFC3339)
	legacy := FileManifest{
		SchemaVersion: schema,
		Files: []FileEntry{{
			Path:       "notes.txt",
			Size:       5,
			ModifiedAt: now,
			AddedAt:    now,
			FileKind:   "txt",
			MimeType:   "application/octet-stream",
			IsOpaque:   true,
		}},
	}
	if err := writeJSON(manifestPath, legacy); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	files, err := mgr.FilesList(wb.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files list failed")
	}
	if files[0].FileKind != FileKindText {
		t.Fatalf("expected normalized file kind %q, got %q", FileKindText, files[0].FileKind)
	}
	if files[0].MimeType != mimeTypeForExtension(".txt") {
		t.Fatalf("expected normalized mime type %q, got %q", mimeTypeForExtension(".txt"), files[0].MimeType)
	}
	if files[0].IsOpaque {
		t.Fatalf("expected normalized text file to be non-opaque")
	}

	var migrated FileManifest
	if err := readJSON(manifestPath, &migrated); err != nil {
		t.Fatalf("read migrated manifest: %v", err)
	}
	if len(migrated.Files) != 1 {
		t.Fatalf("expected 1 migrated file entry, got %d", len(migrated.Files))
	}
	if migrated.Files[0].FileKind != FileKindText {
		t.Fatalf("expected persisted file kind %q, got %q", FileKindText, migrated.Files[0].FileKind)
	}
}

func TestDraftStagingCommit(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "workbenches"))
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Test", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	src := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := mgr.FilesAdd(wb.ID, []string{src}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "notes.txt", "draft"); err != nil {
		t.Fatalf("apply draft: %v", err)
	}
	staging := "draft.test.staging"
	if err := mgr.CreateDraftStaging(wb.ID, staging); err != nil {
		t.Fatalf("create staging: %v", err)
	}
	if err := mgr.ApplyWriteToArea(wb.ID, staging, "notes.txt", "staged"); err != nil {
		t.Fatalf("apply staging: %v", err)
	}
	if err := mgr.CommitDraftStaging(wb.ID, staging); err != nil {
		t.Fatalf("commit staging: %v", err)
	}
	content, err := mgr.ReadFile(wb.ID, "draft", "notes.txt")
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if content != "staged" {
		t.Fatalf("expected staged content, got %q", content)
	}
}

func TestDraftStagingIsolation(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root)
	if err := mgr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	wb, err := mgr.Create("Staging isolation", "openai:gpt-5.2")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := mgr.CreateDraft(wb.ID); err != nil {
		t.Fatalf("draft: %v", err)
	}
	if err := mgr.ApplyDraftWrite(wb.ID, "notes.txt", "base"); err != nil {
		t.Fatalf("write: %v", err)
	}

	stagingA := "draft.a.staging"
	stagingB := "draft.b.staging"
	if err := mgr.CreateDraftStaging(wb.ID, stagingA); err != nil {
		t.Fatalf("staging A: %v", err)
	}
	if err := mgr.CreateDraftStaging(wb.ID, stagingB); err != nil {
		t.Fatalf("staging B: %v", err)
	}
	if err := mgr.ApplyWriteToArea(wb.ID, stagingA, "notes.txt", "alpha"); err != nil {
		t.Fatalf("write staging A: %v", err)
	}
	if err := mgr.ApplyWriteToArea(wb.ID, stagingB, "notes.txt", "beta"); err != nil {
		t.Fatalf("write staging B: %v", err)
	}

	if err := mgr.CommitDraftStaging(wb.ID, stagingA); err != nil {
		t.Fatalf("commit A: %v", err)
	}
	if got, err := mgr.ReadFile(wb.ID, stagingB, "notes.txt"); err != nil {
		t.Fatalf("read staging B: %v", err)
	} else if got != "beta" {
		t.Fatalf("expected staging B to remain, got %q", got)
	}
}
