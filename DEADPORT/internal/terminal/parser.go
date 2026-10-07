package terminal

import (
	"errors"
	"strings"
)

// Parse limits keep pathological input cheap.
const (
	MaxLineLen    = 512
	MaxArgs       = 64
	MaxPipeStages = 6
)

// Parse errors (user-facing).
var (
	ErrUnterminatedQuote = errors.New("unterminated quote. Bob does this too")
	ErrRedirect          = errors.New("read-only filesystem: redirection is disabled. Bob made it that way after The Incident")
	ErrChaining          = errors.New("command chaining is disabled. One thing at a time, like a real IT department")
	ErrSubshell          = errors.New("subshells are disabled. This is not a real shell, it just plays one on TV")
	ErrTooLong           = errors.New("command too long. Management has a 512 character attention span")
	ErrTooManyStages     = errors.New("too many pipes. This is a terminal, not plumbing school")
	ErrEmptyStage        = errors.New("empty pipeline stage. Nothing piped into nothing is still nothing")
)

// Pipeline is a parsed command line: one or more stages joined by '|'.
type Pipeline [][]string

// Parse tokenizes a command line. It understands single quotes, double
// quotes, backslash escapes and pipes. Redirection, chaining (; && ||),
// background jobs and subshells are rejected rather than interpreted.
func Parse(line string) (Pipeline, error) {
	if len(line) > MaxLineLen {
		return nil, ErrTooLong
	}
	var (
		stages  Pipeline
		args    []string
		cur     strings.Builder
		inTok   bool
		quote   rune
		escaped bool
	)
	flushTok := func() {
		if inTok {
			args = append(args, cur.String())
			cur.Reset()
			inTok = false
		}
	}
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if escaped {
			cur.WriteRune(r)
			inTok = true
			escaped = false
			continue
		}
		if quote != 0 {
			switch {
			case r == quote:
				quote = 0
			case r == '\\' && quote == '"' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\'):
				escaped = true
			default:
				cur.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\\':
			escaped = true
			inTok = true
		case '\'', '"':
			quote = r
			inTok = true
		case ' ', '\t':
			flushTok()
		case '|':
			if i+1 < len(runes) && runes[i+1] == '|' {
				return nil, ErrChaining
			}
			flushTok()
			if len(args) == 0 {
				return nil, ErrEmptyStage
			}
			stages = append(stages, args)
			args = nil
			if len(stages) >= MaxPipeStages {
				return nil, ErrTooManyStages
			}
		case '>', '<':
			return nil, ErrRedirect
		case ';', '&':
			return nil, ErrChaining
		case '`':
			return nil, ErrSubshell
		case '$':
			if i+1 < len(runes) && runes[i+1] == '(' {
				return nil, ErrSubshell
			}
			cur.WriteRune(r)
			inTok = true
		default:
			cur.WriteRune(r)
			inTok = true
		}
		if len(args) > MaxArgs {
			return nil, ErrTooLong
		}
	}
	if quote != 0 {
		return nil, ErrUnterminatedQuote
	}
	if escaped {
		cur.WriteRune('\\')
	}
	flushTok()
	if len(args) == 0 {
		if len(stages) > 0 {
			return nil, ErrEmptyStage
		}
		return nil, nil
	}
	stages = append(stages, args)
	return stages, nil
}
