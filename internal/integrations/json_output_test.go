package integrations

import (
	"encoding/json"
	"testing"
)

func TestScanToJSONGolden(t *testing.T) {
	recs := []Recommendation{
		{
			Target:         TargetPi,
			Label:          "Pi",
			Commands:       []string{"pi"},
			ConfigDir:      "~/.pi",
			InstallPath:    "~/.pi/hooks/seshagy-report.sh",
			AgentAvailable: true,
			State:          StatusNotInstalled,
			Version:        0,
			Installable:    true,
			Reason:         "agent on PATH",
			Authority:      LifecycleAuthority,
		},
		{
			Target:         TargetCursor,
			Label:          "Cursor Agent",
			Commands:       []string{"cursor-agent"},
			AgentAvailable: false,
			State:          StatusNotInstalled,
			Installable:    false,
			Reason:         "cursor-agent not on PATH",
			Authority:      LifecycleAuthority,
		},
	}

	got, err := json.MarshalIndent(ScanToJSON(recs), "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	want := `{
  "ok": true,
  "schema_version": 1,
  "integrations": [
    {
      "target": "pi",
      "label": "Pi",
      "commands": [
        "pi"
      ],
      "config_dir": "~/.pi",
      "install_path": "~/.pi/hooks/seshagy-report.sh",
      "agent_available": true,
      "state": "not-installed",
      "installable": true,
      "reason": "agent on PATH",
      "authority": "lifecycle"
    },
    {
      "target": "cursor",
      "label": "Cursor Agent",
      "commands": [
        "cursor-agent"
      ],
      "agent_available": false,
      "state": "not-installed",
      "installable": false,
      "reason": "cursor-agent not on PATH",
      "authority": "lifecycle"
    }
  ]
}`

	if string(got) != want {
		t.Fatalf("ScanToJSON() mismatch:\n%s", got)
	}
}
