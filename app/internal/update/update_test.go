package update

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersions(t *testing.T) {
	for _, v := range []struct {
		a, b string
		want int
	}{{"1.9.0", "1.10.0", -1}, {"2.0.0", "1.99.0", 1}, {"1.0.0", "1.0.0", 0}} {
		got, e := Compare(v.a, v.b)
		if e != nil || got != v.want {
			t.Fatal(got, e)
		}
	}
	if _, e := Compare("../../data", "1.0.0"); e == nil {
		t.Fatal("unsafe version")
	}
}
func TestOfflineAndTimeout(t *testing.T) {
	for _, handler := range []http.HandlerFunc{func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 503) }, func(w http.ResponseWriter, r *http.Request) { time.Sleep(50 * time.Millisecond) }} {
		s := httptest.NewTLSServer(handler)
		c := s.Client()
		c.Timeout = 10 * time.Millisecond
		if _, e := Check(Config{ManifestURL: s.URL}, c); e == nil {
			t.Fatal("expected unavailable")
		}
		s.Close()
	}
}
func TestReleaseValidation(t *testing.T) {
	for _, kind := range []string{"ok", "sha", "corrupt", "path", "architecture", "version", "missing", "data", "future-launcher"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			zw := zip.NewWriter(&b)
			w, _ := zw.Create("server")
			w.Write([]byte("verified test artifact"))
			meta := map[string]any{"version": "9.0.0", "architecture": "linux-amd64", "schema_version": 4}
			if kind == "architecture" {
				meta["architecture"] = "windows-arm64"
			}
			if kind == "version" {
				meta["version"] = "8.0.0"
			}
			if kind != "missing" {
				w, _ = zw.Create("release.json")
				json.NewEncoder(w).Encode(meta)
			}
			if kind == "path" {
				w, _ = zw.Create("../data/store.json")
				w.Write([]byte("bad"))
			}
			if kind == "data" {
				w, _ = zw.Create("data/store.json")
				w.Write([]byte("bad"))
			}
			zw.Close()
			data := b.Bytes()
			if kind == "corrupt" {
				data = []byte("not zip")
			}
			h := sha256.Sum256(data)
			hash := hex.EncodeToString(h[:])
			if kind == "sha" {
				hash = hex.EncodeToString(make([]byte, 32))
			}
			var manifest Manifest
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/manifest" {
					json.NewEncoder(w).Encode(manifest)
				} else {
					w.Write(data)
				}
			}))
			defer s.Close()
			manifest = Manifest{Format: 1, Application: "control-buceadores", Version: "9.0.0", Schema: 4, Minimum: "1.0.0", Architectures: map[string]Asset{"linux-amd64": {File: s.URL + "/asset", SHA256: hash}}}
			if kind == "future-launcher" {
				manifest.Minimum = "99.0.0"
			}
			m, e := Check(Config{ManifestURL: s.URL + "/manifest"}, s.Client())
			if kind == "future-launcher" {
				if e == nil {
					t.Fatal("launcher obsolete accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			base := t.TempDir()
			if e = SaveState(base, State{Current: "8.0.0"}); e != nil {
				t.Fatal(e)
			}
			_, e = Prepare(base, m, "linux-amd64", s.Client())
			if kind == "ok" && e != nil {
				t.Fatal(e)
			}
			if kind != "ok" && e == nil {
				t.Fatal("unsafe release accepted")
			}
			st, e := ReadState(base)
			if e != nil || st.Current != "8.0.0" {
				t.Fatal("active changed before health check")
			}
			entries, _ := os.ReadDir(filepath.Join(base, "updater"))
			if len(entries) != 0 {
				t.Fatal("temporary files retained")
			}
		})
	}
}
func TestRejectPlaintextAndCredentialURL(t *testing.T) {
	for _, u := range []string{"http://example.com/x", "https://user:secret@example.com/x", "file:///store.json"} {
		if e := validURL(u); e == nil {
			t.Fatal("unsafe URL")
		}
	}
}
