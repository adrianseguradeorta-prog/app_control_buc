package main

import (
	"archive/zip"
	"bytes"
	"controlbuceadores/internal/core"
	"controlbuceadores/internal/update"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestLauncherHelper(t *testing.T) {
	if os.Getenv("CB_LAUNCHER_HELPER") == "1" {
		e := run(os.Getenv("CB_BASE"), os.Getenv("CB_ACTION"))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		os.Exit(0)
	}
}
func TestRealUpdateOfflineAndRollback(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Mis Aplicaciones", "Control Buceadores", "NO TOCAR")
	os.MkdirAll(base, 0700)
	build := func(v string) string {
		p := filepath.Join(root, "server-"+v)
		if runtime.GOOS == "windows" {
			p += ".exe"
		}
		c := exec.Command("go", "build", "-buildvcs=false", "-ldflags", "-X main.appVersion="+v, "-o", p, "../control-buceadores")
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("build %v %s", e, b)
		}
		return p
	}
	old := build("8.2.0")
	newbin := build("8.3.0")
	seed, e := core.NewApp(base)
	if e != nil {
		t.Fatal(e)
	}
	seed.Repo.Update(func(s *core.Store) error {
		s.Divers = []core.Diver{{ID: "TEST", Name: "Synthetic Person", Active: true, QualBasic: true}}
		s.Dives = []core.Dive{{ID: 1, DiverID: "TEST", Date: "2026-01-01", Qualification: "Autónomo", DiveType: "Autónomo Aire", IBCode: "61001", Gas: "AIRE", MaxDepth: 26, BottomTime: 20, Valid: true}}
		s.NextDiveID = 2
		return nil
	})
	os.MkdirAll(seed.PhotosDir, 0700)
	os.WriteFile(filepath.Join(seed.PhotosDir, "sentinel.png"), []byte("LOCAL-ONLY-PHOTO"), 0600)
	install := func(v, bin string) {
		dir := filepath.Dir(update.Program(base, v))
		os.MkdirAll(dir, 0700)
		b, _ := os.ReadFile(bin)
		os.WriteFile(update.Program(base, v), b, 0700)
	}
	install("8.2.0", old)
	update.SaveState(base, update.State{Current: "8.2.0"})
	var uploads atomic.Int32
	var requests atomic.Int32
	var bad atomic.Bool
	asset := func(v string, broken bool) []byte {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		f, _ := z.Create(filepath.Base(update.Program(base, v)))
		data, _ := os.ReadFile(newbin)
		if broken {
			data = []byte("not executable")
		}
		f.Write(data)
		f, _ = z.Create("release.json")
		json.NewEncoder(f).Encode(map[string]any{"version": v, "architecture": runtime.GOOS + "-" + runtime.GOARCH, "schema_version": 4})
		z.Close()
		return b.Bytes()
	}
	goodAsset := asset("8.3.0", false)
	brokenAsset := asset("8.4.0", true)
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "GET" || len(body) > 0 {
			uploads.Add(1)
			http.Error(w, "upload forbidden", 400)
			return
		}
		v := "8.3.0"
		data := goodAsset
		if bad.Load() {
			v = "8.4.0"
			data = brokenAsset
		}
		if r.URL.Path == "/asset" {
			w.Write(data)
			return
		}
		h := sha256.Sum256(data)
		json.NewEncoder(w).Encode(update.Manifest{Format: 1, Application: "control-buceadores", Version: v, Schema: 4, Minimum: "1.0.0", Architectures: map[string]update.Asset{runtime.GOOS + "-" + runtime.GOARCH: {File: srv.URL + "/asset", SHA256: hex.EncodeToString(h[:])}}})
	}))
	defer srv.Close()
	cert := filepath.Join(root, "ca.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600)
	cfg := update.Config{CAFile: cert, ManifestURL: srv.URL + "/update.json", TimeoutSeconds: 1}
	cb, _ := json.Marshal(cfg)
	os.MkdirAll(filepath.Join(base, "updater"), 0700)
	os.WriteFile(filepath.Join(base, "updater", "config.json"), cb, 0600)
	launch := func(answer string) (*exec.Cmd, chan error) {
		exe, _ := os.Executable()
		c := exec.Command(exe, "-test.run=^TestLauncherHelper$")
		c.Env = append(os.Environ(), "CB_LAUNCHER_HELPER=1", "CB_BASE="+base, "SSL_CERT_FILE="+cert)
		in, _ := c.StdinPipe()
		if e = c.Start(); e != nil {
			t.Fatal(e)
		}
		if answer != "" {
			io.WriteString(in, answer+"\n")
		}
		done := make(chan error, 1)
		go func() { done <- c.Wait() }()
		t.Cleanup(func() { c.Process.Kill() })
		return c, done
	}
	waitVersion := func(v string) {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			r, e := readRunning(base)
			if e == nil && r.Version == v {
				client := http.Client{Timeout: time.Second}
				resp, e := client.Get(endpoint(r, "/health"))
				if e == nil {
					resp.Body.Close()
					if resp.StatusCode == 200 {
						return
					}
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("server did not start", v)
	}
	shutdown := func(done chan error) {
		r, e := readRunning(base)
		if e != nil {
			t.Fatal(e)
		}
		resp, e := http.Get(endpoint(r, "/internal/shutdown?token="+r.Token))
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		select {
		case e := <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown failed")
		}
	}
	verifyData := func() {
		r, e := core.LoadOrCreate(seed.Repo.Path)
		if e != nil || len(r.Data.Divers) != 1 || len(r.Data.Dives) != 1 || len(r.Data.Users) != 1 || len(r.Data.ReportTemplates) != 1 {
			t.Fatal("data lost", e)
		}
		b, _ := os.ReadFile(filepath.Join(seed.PhotosDir, "sentinel.png"))
		if string(b) != "LOCAL-ONLY-PHOTO" {
			t.Fatal("photos lost")
		}
	}
	// Real old server startup, offline (no channel yet).
	os.Rename(filepath.Join(base, "updater", "config.json"), filepath.Join(base, "updater", "saved.json"))
	_, done := launch("")
	waitVersion("8.2.0")
	verifyData()
	shutdown(done)
	os.Rename(filepath.Join(base, "updater", "saved.json"), filepath.Join(base, "updater", "config.json"))
	_, done = launch("s")
	waitVersion("8.3.0")
	deadline := time.Now().Add(3 * time.Second)
	for {
		st, _ := update.ReadState(base)
		if st.Current == "8.3.0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not activated")
		}
		time.Sleep(20 * time.Millisecond)
	}
	verifyData()
	jar, _ := cookiejar.New(nil)
	client := http.Client{Jar: jar}
	r, _ := readRunning(base)
	u := endpoint(r, "")
	resp, e := client.Get(u + "/login")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	token := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(b)
	resp, e = client.PostForm(u+"/login", url.Values{"csrf": {string(token[1])}, "username": {"admin"}, "password": {"Zocotroco$01"}})
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	for _, p := range []string{"/buceadores", "/inmersiones", "/mantenimiento", "/informe/mensual"} {
		resp, e = client.Get(u + p)
		if e != nil || resp.StatusCode != 200 {
			t.Fatal("screen failed", p, e)
		}
		resp.Body.Close()
	}
	shutdown(done)
	backups, _ := filepath.Glob(filepath.Join(seed.BackupsDir, "backup_preupdate_*.zip"))
	if len(backups) < 1 {
		t.Fatal("missing preupdate backup")
	}
	for _, p := range backups {
		if _, e = core.ValidateBackup(p); e != nil {
			t.Fatal(e)
		}
	}
	// Same version never requests an asset; a second launcher reuses the server.
	before := requests.Load()
	_, done = launch("")
	waitVersion("8.3.0")
	_, secondDone := launch("")
	select {
	case e := <-secondDone:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second launcher did not recognize running server")
	}
	shutdown(done)
	if requests.Load() != before+1 {
		t.Fatal("same version unexpectedly downloaded")
	}
	// Verified but unlaunchable update must fall back without changing active state.
	bad.Store(true)
	_, done = launch("s")
	waitVersion("8.3.0")
	verifyData()
	shutdown(done)
	st, _ := update.ReadState(base)
	if st.Current != "8.3.0" {
		t.Fatal("bad update activated")
	}
	// No Internet after an update: restart installed version.
	srv.Close()
	_, done = launch("")
	waitVersion("8.3.0")
	verifyData()
	shutdown(done)
	if uploads.Load() != 0 {
		t.Fatal("data uploaded")
	}
	if requests.Load() < 3 {
		t.Fatal("update not exercised")
	}
}
