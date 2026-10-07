package terminal

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// ROT13 rotates ASCII letters by 13.
func ROT13(s string) string { return Caesar(s, 13) }

// Caesar shifts ASCII letters by n positions (n may be negative).
func Caesar(s string, n int) string {
	n = ((n % 26) + 26) % 26
	b := []rune(s)
	for i, r := range b {
		switch {
		case r >= 'a' && r <= 'z':
			b[i] = 'a' + (r-'a'+rune(n))%26
		case r >= 'A' && r <= 'Z':
			b[i] = 'A' + (r-'A'+rune(n))%26
		}
	}
	return string(b)
}

// Reverse reverses a string rune by rune.
func Reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// Base64Encode encodes using standard base64.
func Base64Encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// Base64Decode decodes standard or URL base64, with or without padding.
func Base64Decode(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if out, err := enc.DecodeString(s); err == nil {
			return string(out), true
		}
	}
	return "", false
}

// HexEncode encodes bytes as lowercase hex.
func HexEncode(s string) string { return hex.EncodeToString([]byte(s)) }

// HexDecode decodes hex, ignoring spaces, colons and an optional 0x prefix.
func HexDecode(s string) (string, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	s = strings.NewReplacer(" ", "", ":", "", "\\x", "").Replace(s)
	if s == "" {
		return "", false
	}
	out, err := hex.DecodeString(s)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// Printable reports whether decoded text is safe and sensible to display.
func Printable(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f || r == 0xfffd {
			return false
		}
	}
	return true
}
