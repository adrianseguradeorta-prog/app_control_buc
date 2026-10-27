// package builds releases from an explicit allowlist, never from the installation folder.
package main

import (
	"archive/zip"
	"controlbuceadores/internal/update"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	v := flag.String("version", "8.2.0", "stable version")
	out := flag.String("out", "../release-output", "output outside data")
	public := flag.String("public-base", "", "HTTPS release asset base URL")
	flag.Parse()
	if _, e := update.Compare(*v, *v); e != nil {
		panic(e)
	}
	if e := build(*v, *out, strings.TrimRight(*public, "/")); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func build(v, out, public string) error {
	if e := os.MkdirAll(out, 0700); e != nil {
		return e
	}
	stage, e := os.MkdirTemp("", "cb-build-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	m := update.Manifest{Format: 1, Application: "control-buceadores", Version: v, Schema: 4, Minimum: update.LauncherVersion, Published: time.Now().UTC().Format(time.RFC3339), Notes: "Versión portátil con launcher y actualizaciones verificadas.", Architectures: map[string]update.Asset{}}
	sums := ""
	for _, arch := range []string{"amd64", "arm64"} {
		key := "windows-" + arch
		server := filepath.Join(stage, "server-"+arch+".exe")
		launcher := filepath.Join(stage, "launcher-"+arch+".exe")
		for _, target := range []struct {
			path, pkg string
			flags     string
		}{{server, "./cmd/control-buceadores", "-s -w -X main.appVersion=" + v}, {launcher, "./cmd/launcher", "-s -w"}} {
			cmd := exec.Command("go", "build", "-buildvcs=false", "-trimpath", "-ldflags", target.flags, "-o", target.path, target.pkg)
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=windows", "GOARCH="+arch)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if e = cmd.Run(); e != nil {
				return e
			}
		}
		sb, e := os.ReadFile(server)
		if e != nil {
			return e
		}
		lb, e := os.ReadFile(launcher)
		if e != nil {
			return e
		}
		meta, _ := json.Marshal(map[string]any{"version": v, "architecture": key, "schema_version": 4})
		assetName := "control-buceadores-" + v + "-" + key + ".zip"
		assetPath := filepath.Join(out, assetName)
		if e = zipFiles(assetPath, map[string][]byte{"server.exe": sb, "release.json": meta}); e != nil {
			return e
		}
		data, e := os.ReadFile(assetPath)
		if e != nil {
			return e
		}
		h := sha256.Sum256(data)
		sum := hex.EncodeToString(h[:])
		m.Architectures[key] = update.Asset{File: public + "/" + assetName, SHA256: sum}
		sums += sum + "  " + assetName + "\n"
		state, _ := json.Marshal(update.State{Current: v})
		cfg := []byte(`{"manifest_url":"https://github.com/adrianseguradeorta-prog/app_control_buc/releases/latest/download/update.json","timeout_seconds":5,"channel":"stable","owner":"adrianseguradeorta-prog","repository":"app_control_buc"}`)

		files := map[string][]byte{"CONTROL_BUCEADORES/EJECUTAR.bat": []byte(bat), "CONTROL_BUCEADORES/NO TOCAR/launcher.exe": lb, "CONTROL_BUCEADORES/NO TOCAR/current/active.json": state, "CONTROL_BUCEADORES/NO TOCAR/updater/config.json": cfg, "CONTROL_BUCEADORES/NO TOCAR/versions/" + v + "/server.exe": sb, "CONTROL_BUCEADORES/NO TOCAR/versions/" + v + "/release.json": meta}
		if e = zipFiles(filepath.Join(out, "CONTROL_BUCEADORES_PORTABLE_"+key+".zip"), files); e != nil {
			return e
		}
		portableName := "CONTROL_BUCEADORES_PORTABLE_" + key + ".zip"
		portableBytes, e := os.ReadFile(filepath.Join(out, portableName))
		if e != nil {
			return e
		}
		portableSum := sha256.Sum256(portableBytes)
		sums += hex.EncodeToString(portableSum[:]) + "  " + portableName + "\n"
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if e = os.WriteFile(filepath.Join(out, "update.json"), b, 0600); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(out, "SHA256SUMS.txt"), []byte(sums), 0600)
}
func zipFiles(path string, files map[string][]byte) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	w := zip.NewWriter(f)
	for n, b := range files {
		a, e := w.Create(n)
		if e != nil {
			f.Close()
			return e
		}
		if _, e = a.Write(b); e != nil {
			f.Close()
			return e
		}
	}
	if e = w.Close(); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}

const bat = `@echo off
setlocal
pushd "%~dp0"
if errorlevel 1 exit /b 1
"%~dp0NO TOCAR\launcher.exe" %*
set "RESULT=%ERRORLEVEL%"
popd
if not "%RESULT%"=="0" pause
exit /b %RESULT%
`
