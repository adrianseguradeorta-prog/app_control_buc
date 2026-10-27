package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIBCodes(t *testing.T) {
	want := map[string]string{"Autónomo Aire": "61001", "SDS Aire": "62001", "SDFS Heliox": "62002", "Autónomo Nitrox": "62003", "Autónomo Heliox/Trimix": "62004", "Semicerrado Nitrox (Buceo técnico)": "62005", "Semicerrado Trimix": "62006", "Cerrado Oxígeno": "63001", "Semicerrado Nitrox (Caza de minas)": "64001", "Control en Cámara": "66001", "Tratamiento O2": "66002"}
	for label, code := range want {
		if got := IBCodeFor(label); got != code {
			t.Fatalf("%s: %s != %s", label, got, code)
		}
	}
}

func TestMaintenanceSummary(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	d := Diver{ID: "B1", QualBasic: true, QualSDSAir: true, QualSDSHeliox: true}
	dives := []Dive{{DiverID: "B1", Date: "2026-08-01", Qualification: "Autónomo CA aire/nitrox básico", IBCode: "61001", Gas: "AIRE", MaxDepth: 26, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-07-01", Qualification: "Autónomo CA aire/nitrox básico", IBCode: "61001", Gas: "AIRE", MaxDepth: 10, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-06-01", Qualification: "Autónomo CA aire/nitrox básico", IBCode: "61001", Gas: "AIRE", MaxDepth: 12, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-08-02", Qualification: "SDS aire", IBCode: "62001", Gas: "AIRE", SurfaceSupply: true, MaxDepth: 10, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-07-02", Qualification: "SDS aire", IBCode: "62001", Gas: "AIRE", SurfaceSupply: true, MaxDepth: 10, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-08-03", Qualification: "SDS HELIOX", IBCode: "62002", Gas: "AIRE", SurfaceSupply: true, MaxDepth: 20, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-07-03", Qualification: "SDS HELIOX", IBCode: "62002", Gas: "AIRE", SurfaceSupply: true, MaxDepth: 20, BottomTime: 20, Valid: true}, {DiverID: "B1", Date: "2026-06-03", Qualification: "SDS HELIOX", IBCode: "62002", Gas: "HELIOX", SurfaceSupply: true, MaxDepth: 45, BottomTime: 20, Valid: true}}
	qs := MaintenanceFor(d, dives, now)
	for _, q := range qs {
		if q.Status != "OK" {
			t.Fatalf("%s = %s (%s)", q.Label, q.Status, q.Detail)
		}
	}
}

func TestStoreAndBackup(t *testing.T) {
	dir := t.TempDir()
	repo, e := LoadOrCreate(filepath.Join(dir, "data", "store.json"))
	if e != nil {
		t.Fatal(e)
	}
	if repo.Data.UserByUsername("admin") == nil {
		t.Fatal("admin missing")
	}
	cfg := DefaultConfig()
	cfgp := filepath.Join(dir, "config.json")
	if e = SaveConfig(cfgp, cfg); e != nil {
		t.Fatal(e)
	}
	zip := filepath.Join(dir, "backup.zip")
	if e = CreateBackup(zip, repo.Path, cfgp, filepath.Join(dir, "data", "photos"), "test", "test"); e != nil {
		t.Fatal(e)
	}
	if _, e = ValidateBackup(zip); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(zip); e != nil {
		t.Fatal(e)
	}
}

func TestPortSelection(t *testing.T) {
	p, e := ChoosePort("127.0.0.1", 28654)
	if e != nil || p < 28654 {
		t.Fatalf("port: %d %v", p, e)
	}
}
