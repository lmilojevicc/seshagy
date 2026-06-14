package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const jsonSchemaVersion = 1

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func parseDeleteItemArgs(args []string) (line string, jsonOutput bool) {
	if len(args) > 0 && args[len(args)-1] == "--json" {
		jsonOutput = true
		args = args[:len(args)-1]
	}
	return strings.Join(args, " "), jsonOutput
}

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

func envelopeSuccess(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload)+2)
	for k, v := range payload {
		out[k] = v
	}
	out["schema_version"] = jsonSchemaVersion
	out["ok"] = true
	return out
}

func encodeSuccess(v any) error {
	switch payload := v.(type) {
	case map[string]any:
		return encodeJSON(envelopeSuccess(payload))
	case map[string]string:
		out := make(map[string]any, len(payload)+2)
		for k, val := range payload {
			out[k] = val
		}
		return encodeJSON(envelopeSuccess(out))
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return err
		}
		return encodeJSON(envelopeSuccess(out))
	}
}

type jsonErrorResponse struct {
	SchemaVersion int    `json:"schema_version"`
	Ok            bool   `json:"ok"`
	Error         string `json:"error"`
	Code          string `json:"code"`
}

func encodeJSONError(err error) error {
	msg := err.Error()
	code := "error"
	if strings.HasPrefix(msg, "usage:") {
		code = "usage"
	}
	return encodeJSON(jsonErrorResponse{
		SchemaVersion: jsonSchemaVersion,
		Ok:            false,
		Error:         msg,
		Code:          code,
	})
}

func modeUsage(flag string) string {
	return fmt.Sprintf("usage: seshagy %s [--json]", flag)
}

func joinUsage(parts ...string) string {
	return "usage: seshagy " + strings.Join(parts, " ")
}
