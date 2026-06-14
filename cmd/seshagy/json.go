package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func stripJSONFlag(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		out = append(out, arg)
	}
	return out, jsonOutput
}

func encodeJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func modeUsage(flag string) string {
	return fmt.Sprintf("usage: seshagy %s [--json]", flag)
}

func joinUsage(parts ...string) string {
	return "usage: seshagy " + strings.Join(parts, " ")
}
