package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"controlbuceadores/internal/core"
	"controlbuceadores/internal/instance"
)

var appVersion = "8.2.0"

type runtimeInfo struct {
	Host    string `json:"host"`
	Version string `json:"version"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

func main() {
	pending := flag.Bool("pending-activation", false, "bloquear usuarios hasta confirmar la versión")
	managed := flag.Bool("managed", false, "iniciado por launcher con bloqueo de despliegue")
	validate := flag.Bool("validate", false, "validar datos sin escribir ni arrancar")
	version := flag.Bool("version", false, "mostrar versión")
	baseFlag := flag.String("base", "", "directorio NO TOCAR (desarrollo/pruebas)")
	resetUser := flag.String("reset-user", "", "usuario para recuperación administrativa con servidor detenido")
	passwordFile := flag.String("password-file", "", "archivo local con contraseña nueva; no se registra su contenido")
	closeFlag := flag.Bool("close", false, "cerrar servidor existente")
	testFlag := flag.Bool("self-test", false, "autoprueba rápida")
	flag.Parse()
	base := *baseFlag
	if base == "" {
		exe, _ := os.Executable()
		base = filepath.Dir(exe)
	}
	if *version {
		fmt.Printf("%s schema=%d\n", appVersion, core.SchemaVersion)
		return
	}
	if *validate {
		r, e := core.LoadOrCreateReadOnly(filepath.Join(base, "data", "store.json"))
		if e != nil {
			fatal(e)
		}
		if e = core.ValidateStore(r); e != nil {
			fatal(e)
		}
		fmt.Println("Datos válidos")
		return
	}
	if *testFlag {
		if err := selfTest(); err != nil {
			fmt.Fprintln(os.Stderr, "PRUEBA FALLIDA:", err)
			os.Exit(1)
		}
		fmt.Println("PRUEBAS CODEX SOURCE: OK")
		return
	}
	if *closeFlag {
		if err := closeExisting(base); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Servidor cerrado.")
		return
	}
	if !*managed {
		l, e := instance.Acquire(filepath.Join(base, "updater", "deployment.lock"))
		if e != nil {
			fatal(fmt.Errorf("servidor activo o iniciándose; utilice el launcher"))
		}
		defer l.Close()
	}
	lock, e := instance.Acquire(filepath.Join(base, "data", "server.lock"))
	if e != nil {
		fatal(fmt.Errorf("otra instancia está utilizando estos datos"))
	}
	defer lock.Close()
	app, err := core.NewApp(base)
	if err != nil {
		fatal(err)
	}
	if *resetUser != "" || *passwordFile != "" {
		if *resetUser == "" || *passwordFile == "" {
			fatal(fmt.Errorf("use --reset-user y --password-file juntos"))
		}
		b, e := os.ReadFile(*passwordFile)
		if e != nil {
			fatal(e)
		}
		password := strings.TrimRight(string(b), "\r\n")
		if e = app.ResetPassword(*resetUser, password); e != nil {
			fatal(e)
		}
		fmt.Println("Contraseña actualizada; copia de seguridad creada. Elimine de forma segura el archivo de contraseña.")
		return
	}
	hostName, _ := os.Hostname()
	app.Config.AllowedHosts = append(app.Config.AllowedHosts, hostName)
	if interfaces, e := net.InterfaceAddrs(); e == nil {
		for _, address := range interfaces {
			if ipnet, ok := address.(*net.IPNet); ok {
				app.Config.AllowedHosts = append(app.Config.AllowedHosts, ipnet.IP.String())
			}
		}
	}
	app.Initializing = *pending
	app.Version = appVersion
	listener, err := core.ListenAvailable(app.Config.BindAddress, app.Config.PreferredPort)
	if err != nil {
		fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	app.Server = &http.Server{Addr: fmt.Sprintf("%s:%d", app.Config.BindAddress, port), Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	_ = os.MkdirAll(app.DataDir, 0755)
	host, _ := os.Hostname()
	ri := runtimeInfo{Port: port, Token: app.ShutdownToken, PID: os.Getpid(), Host: host, Version: appVersion}
	b, _ := json.MarshalIndent(ri, "", "  ")
	if err = os.WriteFile(app.RuntimePath, b, 0600); err != nil {
		fatal(err)
	}
	defer os.Remove(app.RuntimePath)
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	fmt.Println("Control de Buceadores")
	fmt.Println("Servidor:", url)
	fmt.Println("Escriba CERRAR y pulse INTRO para detenerlo.")
	go openBrowser(url)
	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			if strings.EqualFold(strings.TrimSpace(s.Text()), "CERRAR") {
				_, _ = http.Get(fmt.Sprintf("http://127.0.0.1:%d/internal/shutdown?token=%s", port, app.ShutdownToken))
				return
			}
		}
	}()
	err = app.Server.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func closeExisting(base string) error {
	b, e := os.ReadFile(filepath.Join(base, "data", "runtime.json"))
	if e != nil {
		return fmt.Errorf("no se encontró un servidor activo")
	}
	var r runtimeInfo
	if e = json.Unmarshal(b, &r); e != nil {
		return e
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/internal/shutdown?token=%s", r.Port, r.Token), nil)
	c := http.Client{Timeout: 3 * time.Second}
	resp, e := c.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("el servidor rechazó el cierre")
	}
	return nil
}
func openBrowser(u string) {
	time.Sleep(350 * time.Millisecond)
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		c = exec.Command("open", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	_ = c.Start()
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
func selfTest() error {
	for _, o := range core.IBOptions {
		if core.IBCodeFor(o.Label) != o.Code {
			return fmt.Errorf("Dato IB %s", o.Label)
		}
	}
	if _, e := core.ChoosePort("127.0.0.1", 24567); e != nil {
		return e
	}
	return nil
}
