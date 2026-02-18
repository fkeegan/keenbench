package diff

import "testing"

func TestTextDiffLines(t *testing.T) {
	before := "alpha\nbeta\n"
	after := "alpha\ngamma\n"
	hunks := TextDiff(before, after)
	if len(hunks) == 0 {
		t.Fatalf("expected hunks")
	}
	lines := hunks[0].Lines
	if len(lines) == 0 {
		t.Fatalf("expected lines")
	}
	foundAdded := false
	foundRemoved := false
	for _, line := range lines {
		if line.Type == LineAdded {
			foundAdded = true
		}
		if line.Type == LineRemoved {
			foundRemoved = true
		}
	}
	if !foundAdded || !foundRemoved {
		t.Fatalf("expected added and removed lines")
	}
}
