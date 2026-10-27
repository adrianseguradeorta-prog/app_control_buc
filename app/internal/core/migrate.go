package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateStoreFile requires exclusive deployment ownership and a pre-migration backup.
// Only the explicitly supported 3 -> 4 transition is performed. Unknown fields survive.
func MigrateStoreFile(path string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var raw map[string]json.RawMessage
	if e = json.Unmarshal(b, &raw); e != nil {
		return e
	}
	var schema int
	if e = json.Unmarshal(raw["schema_version"], &schema); e != nil {
		return e
	}
	if schema == SchemaVersion {
		return nil
	}
	if schema != 3 {
		return fmt.Errorf("no existe migración desde esquema %d", schema)
	}
	var s Store
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	if e = ValidateStore(s); e != nil {
		return e
	}
	raw["schema_version"] = json.RawMessage("4")
	b, e = json.MarshalIndent(raw, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".migration-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
