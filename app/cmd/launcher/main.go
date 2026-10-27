package main

import (
	"controlbuceadores/internal/core"
	"controlbuceadores/internal/instance"
	"controlbuceadores/internal/update"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type running struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Version string `json:"version"`
}

func readRunning(base string) (running, error) {
	var r running
	b, e := os.ReadFile(filepath.Join(base, "data", "runtime.json"))
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(b, &r)
	return r, e
}
func endpoint(r running, path string) string {
	host := r.Host
	here, _ := os.Hostname()
	if host == "" || strings.EqualFold(host, here) {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d%s", host, r.Port, path)
}
func show(base string) bool {
	r, e := readRunning(base)
	if e != nil {
		return false
	}
	client := http.Client{Timeout: time.Second}
	resp, e := client.Get(endpoint(r, "/health"))
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	var h struct {
		OK bool `json:"ok"`
	}
	if json.NewDecoder(resp.Body).Decode(&h) != nil || !h.OK {
		return false
	}
	u := endpoint(r, "/")
	fmt.Println("El servidor ya está activo:", u)
	open(u)
	return true
}
func open(u string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	}
}
func main() {
	base, e := os.Executable()
	if e != nil {
		fmt.Println(e)
		os.Exit(1)
	}
	base = filepath.Dir(base)
	args := os.Args[1:]
	if len(args) > 1 && args[0] == "--base" {
		base = args[1]
		args = args[2:]
	}
	action := ""
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}
	if e = run(base, action); e != nil {
		fmt.Println("No se ha podido iniciar:", e)
		os.Exit(1)
	}
}
func run(base, action string) error {
	os.MkdirAll(filepath.Join(base, "logs"), 0700)
	f, e := os.OpenFile(filepath.Join(base, "logs", "launcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags|log.LUTC)
	if action == "cerrar" {
		r, e := readRunning(base)
		if e != nil {
			return e
		}
		here, _ := os.Hostname()
		if r.Host != "" && !strings.EqualFold(here, r.Host) {
			return fmt.Errorf("cierre desde Administración o desde el equipo servidor")
		}
		client := http.Client{Timeout: 3 * time.Second}
		resp, e := client.Get(endpoint(r, "/internal/shutdown?token="+r.Token))
		if e != nil {
			return e
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("cierre rechazado")
		}
		logger.Print("cierre solicitado")
		return nil
	}
	lock, e := instance.Acquire(filepath.Join(base, "updater", "deployment.lock"))
	if e != nil {
		for i := 0; i < 10; i++ {
			if show(base) {
				return nil
			}
			time.Sleep(time.Second)
		}
		return fmt.Errorf("otra instancia está arrancando o actualizando; vuelva a intentarlo")
	}
	defer lock.Close()
	guard, err := instance.Acquire(filepath.Join(base, "data", "server.lock"))
	if err != nil {
		if show(base) {
			return nil
		}
		return fmt.Errorf("datos ocupados por un servidor; no se actualiza")
	}
	guard.Close()
	// A direct server also holds deployment.lock; no data changes before ownership.
	state, e := update.ReadState(base)
	if e != nil {
		return e
	}
	if action == "version" {
		fmt.Printf("Launcher %s; aplicación %s; esquema %d\n", update.LauncherVersion, state.Current, core.SchemaVersion)
		return nil
	}
	if action == "prueba" {
		cmd := exec.Command(update.Program(base, state.Current), "--self-test")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	current := state.Current
	candidate := false
	if action == "rollback" {
		if state.Previous == "" {
			return fmt.Errorf("no hay versión anterior")
		}
		current = state.Previous
		candidate = true
	} else {
		var cfg update.Config
		b, e := os.ReadFile(filepath.Join(base, "updater", "config.json"))
		if e == nil {
			e = json.Unmarshal(b, &cfg)
		}
		if e == nil && cfg.ManifestURL != "" {
			fmt.Println("Versión instalada:", state.Current, "· Comprobando actualizaciones…")
			m, err := update.Check(cfg, update.Client(cfg))
			if err != nil {
				logger.Print("comprobación no disponible")
				fmt.Println("No se pudo comprobar el canal. Se iniciará la versión local.")
			} else if cmp, err := update.Compare(m.Version, state.Current); err == nil && cmp > 0 {
				fmt.Printf("Nueva versión: %s\n%s\nActualizar [s/N]: ", m.Version, m.Notes)
				answer := readAnswer()
				if strings.EqualFold(answer, "s") {
					if m.Schema != core.SchemaVersion {
						fmt.Println("Esta versión necesita una migración no soportada. Se conserva la versión local.")
						logger.Print("esquema no compatible rechazado")
					} else {
						_, err = update.Prepare(base, m, runtime.GOOS+"-"+runtime.GOARCH, update.Client(cfg))
						if err != nil {
							logger.Print("release rechazada")
							fmt.Println("Actualización rechazada. Se conserva la versión local.")
						} else {
							current = m.Version
							candidate = true
							logger.Print("descarga y SHA-256 verificados")
						}
					}
				}
			} else {
				fmt.Println("La versión local está actualizada.")
			}
		}
	}
	var backup string
	restore := func() error {
		if backup == "" {
			return nil
		}
		return core.RestoreBackup(backup, filepath.Join(base, "data", "store.json"), filepath.Join(base, "config.json"), filepath.Join(base, "data", "photos"))
	}
	if candidate {
		// Backup raw files, retaining fields unknown to this binary.
		store := filepath.Join(base, "data", "store.json")
		if _, e = os.Stat(store); e == nil {
			backup = filepath.Join(base, "data", "backups", "backup_preupdate_"+time.Now().UTC().Format("20060102_150405.000000000")+".zip")
			if e = core.CreateBackup(backup, store, filepath.Join(base, "config.json"), filepath.Join(base, "data", "photos"), "antes de actualización/rollback", state.Current); e != nil {
				return e
			}
			logger.Print("backup previo creado")
			if e = core.MigrateStoreFile(store); e != nil {
				logger.Print("migración rechazada sin modificar los datos")
				current = state.Current
				candidate = false
			}
			cmd := exec.Command(update.Program(base, current), "--base", base, "--validate")
			if e = cmd.Run(); e != nil {
				logger.Print("validación fallida; versión anterior conservada")
				if e = restore(); e != nil {
					return e
				}
				current = state.Current
				candidate = false
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	logger.Print("arranque de aplicación ", current)
	cmd, done, e := start(base, current)
	if e != nil {
		if candidate {
			logger.Print("ejecutable no arrancable; versión anterior conservada")
			if e = restore(); e != nil {
				return e
			}
			cmd, done, e = start(base, state.Current)
			candidate = false
		}
		if e != nil {
			return e
		}
	}
	if candidate {
		if e = ready(base, current, done); e != nil {
			logger.Print("health check fallido; rollback")
			stop(cmd, done)
			if backup != "" {
				if e = core.RestoreBackup(backup, filepath.Join(base, "data", "store.json"), filepath.Join(base, "config.json"), filepath.Join(base, "data", "photos")); e != nil {
					return fmt.Errorf("recuperación requiere backup local: %w", e)
				}
			}
			fmt.Println("La versión nueva no arrancó. Se recupera la anterior.")
			cmd, done, e = start(base, state.Current)
			if e != nil {
				return e
			}
		} else {
			next := update.State{Current: current, Previous: state.Current}
			if e = update.SaveState(base, next); e != nil {
				stop(cmd, done)
				if backup != "" {
					if restoreErr := core.RestoreBackup(backup, filepath.Join(base, "data", "store.json"), filepath.Join(base, "config.json"), filepath.Join(base, "data", "photos")); restoreErr != nil {
						return restoreErr
					}
				}
				logger.Print("activación fallida; se arranca versión anterior")
				cmd, done, e = start(base, state.Current)
				if e != nil {
					return e
				}
				return <-done
			}
			logger.Print("versión activada ", current)
		}
	}
	e = <-done
	logger.Print("servidor finalizado")
	return e
}
func readAnswer() string {
	var b [1]byte
	var out strings.Builder
	for {
		n, e := os.Stdin.Read(b[:])
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			if b[0] != '\r' {
				out.WriteByte(b[0])
			}
		}
		if e != nil {
			break
		}
	}
	return strings.TrimSpace(out.String())
}
func start(base, version string) (*exec.Cmd, chan error, error) {
	cmd := exec.Command(update.Program(base, version), "--base", base, "--managed")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if e := cmd.Start(); e != nil {
		return nil, nil, e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return cmd, done, nil
}
func stop(cmd *exec.Cmd, done chan error) {
	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}
func ready(base, version string, done chan error) error {
	deadline := time.Now().Add(12 * time.Second)
	c := http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		select {
		case e := <-done:
			done <- e
			return fmt.Errorf("proceso terminó antes del health check")
		default:
		}
		r, e := readRunning(base)
		if e == nil && r.Version == version {
			resp, e := c.Get(endpoint(r, "/health"))
			if e == nil {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 100))
				resp.Body.Close()
				if resp.StatusCode == 200 && strings.Contains(string(b), `"ok":true`) {
					return nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("health check agotado")
}
