package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUnknownFieldsSurviveWrites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "store.json")
	s := DefaultStore()
	b, _ := json.Marshal(s)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	raw["future_metadata"] = map[string]any{"secret_local": "keep-local"}
	raw["users"].([]any)[0].(map[string]any)["unknown_user_field"] = "retain"
	b, _ = json.Marshal(raw)
	os.WriteFile(p, b, 0600)
	r, e := LoadOrCreate(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Update(func(s *Store) error { s.Users[0].DisplayName = "Updated"; return nil }); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(p)
	json.Unmarshal(b, &raw)
	if raw["future_metadata"] == nil || raw["users"].([]any)[0].(map[string]any)["unknown_user_field"] != "retain" {
		t.Fatal("unknown fields lost")
	}
}
func TestVersionedMigrationPreservesUnknownAndIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "store.json")
	s := DefaultStore()
	s.SchemaVersion = 3
	b, _ := json.Marshal(s)
	var raw map[string]any
	json.Unmarshal(b, &raw)
	raw["unrecognized"] = []any{"retain", "verbatim"}
	b, _ = json.Marshal(raw)
	os.WriteFile(p, b, 0600)
	if e := MigrateStoreFile(p); e != nil {
		t.Fatal(e)
	}
	first, _ := os.ReadFile(p)
	if e := MigrateStoreFile(p); e != nil {
		t.Fatal(e)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Fatal("not idempotent")
	}
	json.Unmarshal(second, &raw)
	if raw["schema_version"] != float64(4) || raw["unrecognized"] == nil {
		t.Fatal("migration lost data")
	}
	raw["schema_version"] = 99
	b, _ = json.Marshal(raw)
	os.WriteFile(p, b, 0600)
	if e := MigrateStoreFile(p); e == nil {
		t.Fatal("unsupported migration accepted")
	}
	after, _ := os.ReadFile(p)
	if string(after) != string(b) {
		t.Fatal("failed migration wrote data")
	}
}
