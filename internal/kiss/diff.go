package kiss

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	updateDiffContext       = 3
	updateDiffMaxLines      = 120
	updateDiffMaxCells      = 2_000_000
	updateDiffMaxLineLength = 240
)

type diffOp struct {
	kind    byte
	oldLine int
	newLine int
	text    string
}

type entryDiff struct {
	Text      string
	Note      string
	Truncated bool
}

func buildEntryDiff(currentName, targetName string, current, target []byte) entryDiff {
	currentLines := splitDiffLines(current)
	targetLines := splitDiffLines(target)
	cellCount := (len(currentLines) + 1) * (len(targetLines) + 1)
	if cellCount > updateDiffMaxCells {
		return entryDiff{
			Note: fmt.Sprintf("Entry diff omitted: %d current lines and %d target lines exceed inline diff limit.", len(currentLines), len(targetLines)),
		}
	}

	ops := lineDiffOps(currentLines, targetLines)
	if len(ops) == 0 {
		return entryDiff{}
	}
	lines, truncated := unifiedDiffLines(currentName, targetName, ops)
	return entryDiff{Text: strings.Join(lines, "\n"), Truncated: truncated}
}

func splitDiffLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lineDiffOps(current, target []string) []diffOp {
	n := len(current)
	m := len(target)
	dp := make([]int, (n+1)*(m+1))
	at := func(i, j int) int {
		return i*(m+1) + j
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if current[i] == target[j] {
				dp[at(i, j)] = dp[at(i+1, j+1)] + 1
				continue
			}
			if dp[at(i+1, j)] >= dp[at(i, j+1)] {
				dp[at(i, j)] = dp[at(i+1, j)]
			} else {
				dp[at(i, j)] = dp[at(i, j+1)]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && current[i] == target[j]:
			ops = append(ops, diffOp{kind: ' ', oldLine: i + 1, newLine: j + 1, text: current[i]})
			i++
			j++
		case j < m && (i == n || dp[at(i, j+1)] > dp[at(i+1, j)]):
			ops = append(ops, diffOp{kind: '+', newLine: j + 1, text: target[j]})
			j++
		default:
			ops = append(ops, diffOp{kind: '-', oldLine: i + 1, text: current[i]})
			i++
		}
	}
	return ops
}

func unifiedDiffLines(currentName, targetName string, ops []diffOp) ([]string, bool) {
	intervals := diffHunkIntervals(ops)
	if len(intervals) == 0 {
		return nil, false
	}

	lines := []string{
		"--- current/" + currentName,
		"+++ target/" + targetName,
	}
	truncated := false
	appendLine := func(line string) bool {
		if len(lines) >= updateDiffMaxLines {
			truncated = true
			return false
		}
		lines = append(lines, line)
		return true
	}

	for _, interval := range intervals {
		oldStart, oldCount, newStart, newCount := hunkRange(ops, interval[0], interval[1])
		if !appendLine(fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldCount, newStart, newCount)) {
			break
		}
		for _, op := range ops[interval[0]:interval[1]] {
			if !appendLine(string(op.kind) + truncateDiffLine(op.text)) {
				break
			}
		}
		if truncated {
			break
		}
	}
	return lines, truncated
}

func diffHunkIntervals(ops []diffOp) [][2]int {
	var intervals [][2]int
	for i := 0; i < len(ops); i++ {
		if ops[i].kind == ' ' {
			continue
		}
		start := i - updateDiffContext
		if start < 0 {
			start = 0
		}
		end := i + 1
		contextAfter := 0
		for end < len(ops) {
			if ops[end].kind == ' ' {
				contextAfter++
				if contextAfter > updateDiffContext {
					break
				}
			} else {
				contextAfter = 0
			}
			end++
		}
		if len(intervals) > 0 && start <= intervals[len(intervals)-1][1] {
			intervals[len(intervals)-1][1] = end
		} else {
			intervals = append(intervals, [2]int{start, end})
		}
		i = end - 1
	}
	return intervals
}

func hunkRange(ops []diffOp, start, end int) (int, int, int, int) {
	oldStart, newStart := 0, 0
	oldCount, newCount := 0, 0
	for _, op := range ops[start:end] {
		if op.kind != '+' {
			oldCount++
			if oldStart == 0 {
				oldStart = op.oldLine
			}
		}
		if op.kind != '-' {
			newCount++
			if newStart == 0 {
				newStart = op.newLine
			}
		}
	}
	if oldStart == 0 {
		oldStart = previousOldLine(ops, start) + 1
	}
	if newStart == 0 {
		newStart = previousNewLine(ops, start) + 1
	}
	return oldStart, oldCount, newStart, newCount
}

func previousOldLine(ops []diffOp, index int) int {
	for i := index - 1; i >= 0; i-- {
		if ops[i].oldLine > 0 {
			return ops[i].oldLine
		}
	}
	return 0
}

func previousNewLine(ops []diffOp, index int) int {
	for i := index - 1; i >= 0; i-- {
		if ops[i].newLine > 0 {
			return ops[i].newLine
		}
	}
	return 0
}

func truncateDiffLine(line string) string {
	if utf8.RuneCountInString(line) <= updateDiffMaxLineLength {
		return line
	}
	runes := []rune(line)
	return string(runes[:updateDiffMaxLineLength]) + "... [line truncated]"
}

func markdownFenceFor(content string) string {
	maxRun := 0
	currentRun := 0
	for _, r := range content {
		if r == '`' {
			currentRun++
			if currentRun > maxRun {
				maxRun = currentRun
			}
			continue
		}
		currentRun = 0
	}
	if maxRun < 3 {
		return "```"
	}
	return strings.Repeat("`", maxRun+1)
}
