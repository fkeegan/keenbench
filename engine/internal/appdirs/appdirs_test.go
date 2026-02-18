package appdirs

import (
	"os"
	"testing"
)

func TestDataDirOverride(t *testing.T) {
	os.Setenv("KEENBENCH_DATA_DIR", "/tmp/keenbench-test")
	defer os.Unsetenv("KEENBENCH_DATA_DIR")
	path, err := DataDir()
	if err != nil {
		t.Fatalf("data dir: %v", err)
	}
	if path != "/tmp/keenbench-test" {
		t.Fatalf("expected override path, got %s", path)
	}

	workbenches := WorkbenchesDir(path)
	if workbenches != "/tmp/keenbench-test/workbenches" {
		t.Fatalf("expected workbenches dir, got %s", workbenches)
	}
}
