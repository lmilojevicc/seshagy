package sessionmgr

import (
	"encoding/json"
	"testing"
)

func TestItemsToJSONUsesModeToken(t *testing.T) {
	items := []Item{{Kind: KindSession, Name: "work", Target: "work"}}
	payload := ItemsToJSON(ModeSessions, items, IconSet{}, "")
	if payload.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", payload.SchemaVersion)
	}
	if !payload.Ok {
		t.Fatal("ok must be true")
	}
	if payload.Mode != "sessions" {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if len(payload.Items) != 1 || payload.Items[0].Kind != "session" {
		t.Fatalf("items = %#v", payload.Items)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if attached, ok := raw["items"].([]any)[0].(map[string]any)["attached"].(bool); !ok {
		t.Fatalf("attached must be emitted for session items: %#v", raw)
	} else if attached {
		t.Fatal("attached should be false")
	}
}
