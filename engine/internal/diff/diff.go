package diff

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type Line struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
}

type Hunk struct {
	Lines []Line `json:"lines"`
}

const (
	LineContext = "context"
	LineAdded   = "added"
	LineRemoved = "removed"
)

func TextDiff(before, after string) []Hunk {
	dmp := diffmatchpatch.New()
	beforeChars, afterChars, lineArray := dmp.DiffLinesToChars(before, after)
	diffs := dmp.DiffMain(beforeChars, afterChars, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	var lines []Line
	oldLine := 1
	newLine := 1
	for _, diff := range diffs {
		chunkLines := strings.Split(diff.Text, "\n")
		if len(chunkLines) > 0 && chunkLines[len(chunkLines)-1] == "" {
			chunkLines = chunkLines[:len(chunkLines)-1]
		}
		for _, line := range chunkLines {
			switch diff.Type {
			case diffmatchpatch.DiffEqual:
				lines = append(lines, Line{Type: LineContext, Text: line, OldLine: oldLine, NewLine: newLine})
				oldLine++
				newLine++
			case diffmatchpatch.DiffDelete:
				lines = append(lines, Line{Type: LineRemoved, Text: line, OldLine: oldLine})
				oldLine++
			case diffmatchpatch.DiffInsert:
				lines = append(lines, Line{Type: LineAdded, Text: line, NewLine: newLine})
				newLine++
			}
		}
	}
	return []Hunk{{Lines: lines}}
}

const MaxDiffLines = 5000

func TextDiffWithLimit(before, after string, maxLines int) ([]Hunk, bool) {
	if maxLines <= 0 {
		maxLines = MaxDiffLines
	}
	if lineCount(before)+lineCount(after) > maxLines {
		return nil, true
	}
	return TextDiff(before, after), false
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}
