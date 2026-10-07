package security

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Name length limits (in runes).
const (
	MinNameLen = 2
	MaxNameLen = 16
)

// Errors returned by ValidateName. Their text is shown to players.
var (
	ErrNameTooShort = errors.New("too short. Even Bob has three letters")
	ErrNameTooLong  = errors.New("too long. HR's database only has 16 boxes")
	ErrNameChars    = errors.New("letters, numbers, spaces, '-', '_' and '.' only")
	ErrNameReserved = errors.New("that name is reserved. Nice try")
)

var reservedNames = map[string]bool{
	"bob": true, "admin": true, "root": true, "system": true, "server": true,
	"deadport": true, "manager": true, "moderator": true, "mod": true, "sysop": true,
}

// ValidateName checks a proposed display name and returns the normalized
// version (trimmed, internal whitespace collapsed).
func ValidateName(raw string) (string, error) {
	// Reject raw control characters/escapes outright instead of silently fixing them.
	if SanitizeText(raw) != strings.NewReplacer("\t", " ").Replace(raw) {
		return "", ErrNameChars
	}
	name := strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(name)
	if n < MinNameLen {
		return "", ErrNameTooShort
	}
	if n > MaxNameLen {
		return "", ErrNameTooLong
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", ErrNameChars
	}
	if reservedNames[strings.ToLower(name)] {
		return "", ErrNameReserved
	}
	return name, nil
}
