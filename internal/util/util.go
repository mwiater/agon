// internal/util/util.go
package util

import (
	"os"
	"strings"
	"unicode/utf8"
)

// WriteFile writes data to a file with 0o644 permissions.
func WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// TruncateRunes truncates a string to a maximum number of runes,
// appending an ellipsis if truncated.
func TruncateRunes(text string, maxRunes int) string {
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxRunes]) + "…"
}

// TruncateToWidth truncates each line of a string to a specified width in runes.
func TruncateToWidth(text string, width int) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if utf8.RuneCountInString(line) > width {
			lines[i] = TruncateRunes(line, width)
		}
	}
	return strings.Join(lines, "\n")
}

// WrapToWidth wraps the given text to a specified width, breaking long words.
func WrapToWidth(text string, width int) string {
	if width <= 0 {
		return text
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			out = append(out, "")
			continue
		}
		var cur strings.Builder
		runeCount := 0
		words := strings.Fields(line)
		for wi, w := range words {
			space := 0
			if wi > 0 {
				space = 1
			}
			wLen := utf8.RuneCountInString(w)
			if runeCount+space+wLen <= width {
				if wi > 0 {
					cur.WriteByte(' ')
					runeCount++
				}
				cur.WriteString(w)
				runeCount += wLen
				continue
			}
			if runeCount > 0 {
				out = append(out, cur.String())
				cur.Reset()
				runeCount = 0
			}
			if wLen <= width {
				cur.WriteString(w)
				runeCount = wLen
			} else {
				r := []rune(w)
				for start := 0; start < len(r); start += width {
					end := start + width
					if end > len(r) {
						end = len(r)
					}
					out = append(out, string(r[start:end]))
				}
			}
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
		} else if len(words) == 0 {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

// Min returns the smaller of two integers.
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// BoolToInt converts a boolean to an integer (1 for true, 0 for false).
func BoolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
