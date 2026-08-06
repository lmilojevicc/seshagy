package completion

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEncodeRejectsInvalidRecordsAndPreservesShellMetacharacters(t *testing.T) {
	hostile := `space quote' double" $(touch nope) ` + "`cmd`" + ` ; * ? [x] -leading :colon \\`
	invalidUTF8 := string([]byte{0xff, 0xfe})
	output := Encode(Result{Directive: NoFiles, Candidates: []Candidate{
		{Value: hostile, Description: "literal"},
		{Value: "tab\there", Description: "bad"},
		{Value: "line\nhere", Description: "bad"},
		{Value: "escape\x1bhere", Description: "bad"},
		{Value: invalidUTF8, Description: "bad"},
		{Value: strings.Repeat("x", maxFieldBytes+1), Description: "bad"},
	}})
	if !strings.HasPrefix(output, "v1\tnofiles\n") || !strings.Contains(output, hostile) {
		t.Fatalf("valid hostile record did not round-trip: %q", output)
	}
	for _, rejected := range []string{"tab\there", "line\nhere", "escape\x1bhere", invalidUTF8} {
		if strings.Contains(output, rejected) {
			t.Fatalf("invalid record leaked into output: %q", rejected)
		}
	}
	if !utf8.ValidString(output) {
		t.Fatal("protocol output is invalid UTF-8")
	}
}

func TestEncodeBoundsCandidateCountAndRequiresDirective(t *testing.T) {
	values := make([]Candidate, maxCandidates+20)
	for i := range values {
		values[i] = Candidate{Value: strings.Repeat("x", i+1)}
	}
	output := Encode(Result{Directive: Files, Candidates: values})
	if got := strings.Count(output, "\nc\t"); got != maxCandidates {
		t.Fatalf("encoded %d candidates, want %d", got, maxCandidates)
	}
	if output := Encode(Result{Directive: "bogus"}); output != "" {
		t.Fatalf("invalid directive output = %q", output)
	}
}
