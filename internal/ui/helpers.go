package ui

import "strings"

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + "…"
}

func boolToString(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "yes"
	}
	return "no"
}

// wrapText wraps s to lines of at most maxLen characters, breaking on word boundaries.
func wrapText(s string, maxLen int) []string {
	if maxLen <= 0 || len(s) <= maxLen {
		return []string{s}
	}
	var lines []string
	current := ""
	for _, word := range strings.Fields(s) {
		if current == "" {
			current = word
		} else if len(current)+1+len(word) <= maxLen {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
