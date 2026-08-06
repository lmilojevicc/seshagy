package completion

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxFieldBytes = 4096
	maxCandidates = 256
)

type Result struct {
	Directive  Directive
	Candidates []Candidate
}

func Encode(result Result) string {
	if result.Directive != NoFiles && result.Directive != Files && result.Directive != Dirs {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("v1\t")
	builder.WriteString(string(result.Directive))
	builder.WriteByte('\n')
	written := 0
	for _, candidate := range result.Candidates {
		if written >= maxCandidates || !validField(candidate.Value) ||
			!validField(candidate.Description) ||
			candidate.Value == "" {
			continue
		}
		builder.WriteString("c\t")
		builder.WriteString(candidate.Value)
		builder.WriteByte('\t')
		builder.WriteString(candidate.Description)
		builder.WriteByte('\n')
		written++
	}
	return builder.String()
}

func validField(value string) bool {
	if len(value) > maxFieldBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}
