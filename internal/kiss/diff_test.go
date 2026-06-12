package kiss

import (
	"strings"
	"testing"
)

func TestEntryDiffTruncatesLargeOutput(t *testing.T) {
	var current strings.Builder
	var target strings.Builder
	for i := 0; i < 200; i++ {
		current.WriteString("old line\n")
		target.WriteString("new line\n")
	}
	diff := buildEntryDiff("SKILL.md", "SKILL.md", []byte(current.String()), []byte(target.String()))
	if !diff.Truncated {
		t.Fatalf("expected large diff to be truncated")
	}
	if got := len(strings.Split(diff.Text, "\n")); got > updateDiffMaxLines {
		t.Fatalf("expected at most %d diff lines, got %d", updateDiffMaxLines, got)
	}
}

func TestEntryDiffMarkdownFenceExceedsContentBackticks(t *testing.T) {
	diff := buildEntryDiff("SKILL.md", "SKILL.md", []byte("old\n"), []byte("```go\nnew\n```\n"))
	fence := markdownFenceFor(diff.Text)
	if fence != "````" {
		t.Fatalf("expected four-backtick fence, got %q for diff:\n%s", fence, diff.Text)
	}
}
