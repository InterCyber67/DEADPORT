// Package security contains small, dependency-free helpers that keep player
// input from doing anything surprising: text sanitization (no terminal escape
// injection), display-name validation and lightweight token-bucket rate limits.
package security

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SanitizeText removes everything that could manipulate a terminal:
// ANSI/VT escape sequences, C0/C1 control characters, DEL, bidi overrides and
// zero-width characters. Newlines and tabs are converted to spaces. The result
// is safe to render inside any player's terminal.
func SanitizeText(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == 0x1b: // ESC: skip the whole escape sequence
			i = skipEscape(runes, i)
			continue
		case r == 0x9b: // 8-bit CSI
			i = skipCSI(runes, i+1)
			continue
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// control characters are dropped
		case isInvisibleFormatting(r):
			// bidi overrides / zero width characters are dropped
		case !unicode.IsPrint(r) && r != ' ':
			// anything else non-printable is dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// skipEscape returns the index of the last rune belonging to the escape
// sequence that starts at runes[i] (which is ESC).
func skipEscape(runes []rune, i int) int {
	if i+1 >= len(runes) {
		return i
	}
	switch runes[i+1] {
	case '[': // CSI
		return skipCSI(runes, i+2)
	case ']', 'P', '_', '^', 'X': // OSC, DCS, APC, PM, SOS: terminated by BEL or ST
		for j := i + 2; j < len(runes); j++ {
			if runes[j] == 0x07 {
				return j
			}
			if runes[j] == 0x1b && j+1 < len(runes) && runes[j+1] == '\\' {
				return j + 1
			}
		}
		return len(runes) - 1
	default: // two-character escape
		return i + 1
	}
}

// skipCSI skips parameter/intermediate bytes until a final byte (0x40-0x7e).
func skipCSI(runes []rune, j int) int {
	for ; j < len(runes); j++ {
		if runes[j] >= 0x40 && runes[j] <= 0x7e {
			return j
		}
	}
	return len(runes) - 1
}

func isInvisibleFormatting(r rune) bool {
	switch {
	case r >= 0x200b && r <= 0x200f: // zero width + LRM/RLM
		return true
	case r >= 0x202a && r <= 0x202e: // bidi embedding/override
		return true
	case r >= 0x2060 && r <= 0x206f: // word joiner, invisible operators, bidi isolates
		return true
	case r == 0xfeff: // BOM / ZWNBSP
		return true
	}
	return false
}

// Truncate shortens s to at most max runes.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

// CleanLine sanitizes, trims and length-limits a line of player input.
func CleanLine(s string, max int) string {
	return strings.TrimSpace(Truncate(collapseSpaces(SanitizeText(s)), max))
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
