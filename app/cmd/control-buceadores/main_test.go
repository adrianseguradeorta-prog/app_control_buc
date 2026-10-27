package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"controlbuceadores/internal/core"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("CB_TEST_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet("helper", flag.ExitOnError)
	os.Args = append([]string{"helper"}, args...)
	main()
	os.Exit(0)
}
func helper(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "CB_TEST_HELPER=1")
	return cmd
}
func waitReady(t *testing.T, cmd *exec.Cmd, base string) runtimeInfo {
	t.Helper()
	client := http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		b, e := os.ReadFile(filepath.Join(base, "data", "runtime.json"))
		if e == nil {
			var info runtimeInfo
			if json.Unmarshal(b, &info) == nil {
				r, e := client.Get("http://127.0.0.1:" + fmtInt(info.Port) + "/health")
				if e == nil {
					r.Body.Close()
					if r.StatusCode == 200 {
						return info
					}
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("servidor no arrancó")
	return runtimeInfo{}
}
func fmtInt(n int) string { return fmt.Sprintf("%d", n) }
func TestMainLifecycle(t *testing.T) {
	for _, mode := range []string{"stdin", "external", "web"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			occupied, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer occupied.Close()
			cfg := core.DefaultConfig()
			cfg.PreferredPort = occupied.Addr().(*net.TCPAddr).Port
			if e = core.SaveConfig(filepath.Join(base, "config.json"), cfg); e != nil {
				t.Fatal(e)
			}
			cmd := helper(t, "--base", base)
			stdin, e := cmd.StdinPipe()
			if e != nil {
				t.Fatal(e)
			}
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer cmd.Process.Kill()
			info := waitReady(t, cmd, base)
			if info.Port == cfg.PreferredPort {
				t.Fatal("puerto ocupado no sustituido")
			}
			duplicate := helper(t, "--base", base)
			if e = duplicate.Run(); e == nil {
				t.Fatal("segunda instancia permitida")
			}
			switch mode {
			case "stdin":
				io.WriteString(stdin, "CERRAR\n")
			case "external":
				if b, e := helper(t, "--base", base, "--close").CombinedOutput(); e != nil {
					t.Fatalf("--close: %v %s", e, b)
				}
			case "web":
				jar, _ := cookiejar.New(nil)
				client := http.Client{Jar: jar, Timeout: 2 * time.Second}
				u := "http://127.0.0.1:" + fmtInt(info.Port)
				resp, e := client.Get(u + "/login")
				if e != nil {
					t.Fatal(e)
				}
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				re := regexp.MustCompile(`name="csrf" value="([^"]+)"`)
				token := re.FindSubmatch(b)
				if len(token) < 2 {
					t.Fatal("CSRF ausente")
				}
				resp, e = client.PostForm(u+"/login", url.Values{"csrf": {string(token[1])}, "username": {"admin"}, "password": {"Zocotroco$01"}})
				if e != nil {
					t.Fatal(e)
				}
				b, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				token = re.FindSubmatch(b)
				if len(token) < 2 {
					t.Fatal("sesión no iniciada")
				}
				resp, e = client.PostForm(u+"/admin/cerrar", url.Values{"csrf": {string(token[1])}})
				if e != nil {
					t.Fatal(e)
				}
				resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Fatal(resp.StatusCode)
				}
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no se cerró")
			}
			if _, e = os.Stat(filepath.Join(base, "data", "runtime.json")); !os.IsNotExist(e) {
				t.Fatal("runtime obsoleto")
			}
		})
	}
}
func TestSelfTest(t *testing.T) {
	if e := selfTest(); e != nil {
		t.Fatal(e)
	}
}
func TestCloseMissingRuntime(t *testing.T) {
	if e := closeExisting(t.TempDir()); e == nil {
		t.Fatal("runtime inexistente aceptado")
	}
}
func TestAdministrativeRecoveryCLI(t *testing.T) {
	base := t.TempDir()
	if _, e := core.NewApp(base); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(t.TempDir(), "new-password.txt")
	os.WriteFile(file, []byte("replacement-password\n"), 0600)
	cmd := helper(t, "--base", base, "--reset-user", "admin", "--password-file", file)
	b, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("%v %s", e, b)
	}
	if strings.Contains(string(b), "replacement-password") {
		t.Fatal("contraseña expuesta")
	}
	repo, e := core.LoadOrCreate(filepath.Join(base, "data", "store.json"))
	if e != nil {
		t.Fatal(e)
	}
	if !core.VerifyPassword(repo.Data.Users[0], "replacement-password") {
		t.Fatal("contraseña no restablecida")
	}
}

func TestConcurrentServerStartsOnlyOne(t *testing.T) {
	base := t.TempDir()
	a := helper(t, "--base", base)
	b := helper(t, "--base", base)
	a.Stdout = io.Discard
	a.Stderr = io.Discard
	b.Stdout = io.Discard
	b.Stderr = io.Discard
	ia, _ := a.StdinPipe()
	ib, _ := b.StdinPipe()
	if e := a.Start(); e != nil {
		t.Fatal(e)
	}
	defer a.Process.Kill()
	if e := b.Start(); e != nil {
		t.Fatal(e)
	}
	defer b.Process.Kill()
	da := make(chan error, 1)
	db := make(chan error, 1)
	go func() { da <- a.Wait() }()
	go func() { db <- b.Wait() }()
	waitReady(t, a, base)
	var survivor chan error
	select {
	case e := <-da:
		if e == nil {
			t.Fatal("loser exited successfully")
		}
		survivor = db
	case e := <-db:
		if e == nil {
			t.Fatal("loser exited successfully")
		}
		survivor = da
	case <-time.After(3 * time.Second):
		t.Fatal("two servers stayed running")
	}
	io.WriteString(ia, "CERRAR\n")
	io.WriteString(ib, "CERRAR\n")
	select {
	case e := <-survivor:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("survivor did not close")
	}
}
