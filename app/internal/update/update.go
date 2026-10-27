// Package update downloads program-only assets. It never uploads local files.
package update

import (
	"archive/zip"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const LauncherVersion = "1.0.0"

var versionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type Asset struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Format        int              `json:"manifest_version"`
	Application   string           `json:"application"`
	Version       string           `json:"version"`
	Schema        int              `json:"schema_version"`
	Minimum       string           `json:"minimum_launcher_version"`
	Published     string           `json:"published_at"`
	Notes         string           `json:"notes"`
	Architectures map[string]Asset `json:"architectures"`
}
type Config struct {
	CAFile         string `json:"ca_file,omitempty"`
	ManifestURL    string `json:"manifest_url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Owner          string `json:"owner"`
	Repository     string `json:"repository"`
	Channel        string `json:"channel"`
}
type State struct {
	Current  string `json:"current"`
	Previous string `json:"previous"`
}

func Compare(a, b string) (int, error) {
	if !versionRE.MatchString(a) || !versionRE.MatchString(b) {
		return 0, fmt.Errorf("versión no válida")
	}
	aa := strings.Split(a, ".")
	bb := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, e := strconv.ParseUint(aa[i], 10, 32)
		if e != nil {
			return 0, e
		}
		y, e := strconv.ParseUint(bb[i], 10, 32)
		if e != nil {
			return 0, e
		}
		if x < y {
			return -1, nil
		}
		if x > y {
			return 1, nil
		}
	}
	return 0, nil
}
func validURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("se requiere HTTPS sin credenciales")
	}
	return nil
}
func Client(c Config) *http.Client {
	n := c.TimeoutSeconds
	if n <= 0 || n > 20 {
		n = 5
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if c.CAFile != "" {
		roots, e := x509.SystemCertPool()
		if e != nil {
			roots = x509.NewCertPool()
		}
		b, e := os.ReadFile(c.CAFile)
		if e != nil || !roots.AppendCertsFromPEM(b) {
			return &http.Client{Timeout: time.Duration(n) * time.Second, Transport: rejectTransport{}}
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Transport: transport, Timeout: time.Duration(n) * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("demasiadas redirecciones")
		}
		return validURL(r.URL.String())
	}}
}
func Check(c Config, client *http.Client) (Manifest, error) {
	var m Manifest
	if e := validURL(c.ManifestURL); e != nil {
		return m, e
	}
	r, e := client.Get(c.ManifestURL)
	if e != nil {
		return m, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return m, fmt.Errorf("canal no disponible (%d)", r.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if e != nil || len(b) > 1<<20 {
		return m, fmt.Errorf("manifest incompleto o demasiado grande")
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return m, e
	}
	if m.Format != 1 || m.Application != "control-buceadores" {
		return m, fmt.Errorf("manifest no compatible")
	}
	if _, e = Compare(m.Version, m.Version); e != nil {
		return m, e
	}
	cmp, e := Compare(LauncherVersion, m.Minimum)
	if e != nil || cmp < 0 {
		return m, fmt.Errorf("actualice manualmente el launcher; versión mínima %s", m.Minimum)
	}
	return m, nil
}
func StatePath(base string) string { return filepath.Join(base, "current", "active.json") }
func ReadState(base string) (State, error) {
	var s State
	b, e := os.ReadFile(StatePath(base))
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	if !versionRE.MatchString(s.Current) || s.Previous != "" && !versionRE.MatchString(s.Previous) {
		return s, fmt.Errorf("puntero de versión inválido")
	}
	return s, e
}
func SaveState(base string, s State) error {
	if !versionRE.MatchString(s.Current) || s.Previous != "" && !versionRE.MatchString(s.Previous) {
		return fmt.Errorf("versión inválida")
	}
	p := StatePath(base)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".active-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
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
	return os.Rename(name, p)
}
func Program(base, version string) string {
	name := "server.exe"
	if runtime.GOOS != "windows" {
		name = "server"
	}
	return filepath.Join(base, "versions", version, name)
}

// Prepare only accepts a program and release metadata; data can never be in a release ZIP.
func Prepare(base string, m Manifest, arch string, client *http.Client) (string, error) {
	if !versionRE.MatchString(m.Version) {
		return "", fmt.Errorf("versión inválida")
	}
	asset, ok := m.Architectures[arch]
	if !ok {
		return "", fmt.Errorf("arquitectura no publicada")
	}
	sum, e := hex.DecodeString(asset.SHA256)
	if e != nil || len(sum) != 32 {
		return "", fmt.Errorf("SHA-256 inválido")
	}
	if e = validURL(asset.File); e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Join(base, "updater"), 0700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(filepath.Join(base, "updater"), "download-*.zip")
	if e != nil {
		return "", e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	r, e := client.Get(asset.File)
	if e != nil {
		f.Close()
		return "", e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		f.Close()
		return "", fmt.Errorf("descarga no disponible")
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(r.Body, (128<<20)+1))
	ce := f.Close()
	if e != nil {
		return "", e
	}
	if ce != nil {
		return "", ce
	}
	if n > 128<<20 || r.ContentLength >= 0 && n != r.ContentLength {
		return "", fmt.Errorf("descarga incompleta")
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), asset.SHA256) {
		return "", fmt.Errorf("SHA-256 incorrecto")
	}
	zr, e := zip.OpenReader(tmp)
	if e != nil {
		return "", e
	}
	defer zr.Close()
	stage, e := os.MkdirTemp(filepath.Join(base, "updater"), "stage-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(stage)
	want := filepath.Base(Program(base, m.Version))
	seen := map[string]bool{}
	for _, z := range zr.File {
		if z.Name != want && z.Name != "release.json" || seen[z.Name] || z.Mode()&os.ModeSymlink != 0 || z.UncompressedSize64 > 128<<20 {
			return "", fmt.Errorf("contenido de release no permitido: %s", z.Name)
		}
		seen[z.Name] = true
		rc, e := z.Open()
		if e != nil {
			return "", e
		}
		b, e := io.ReadAll(io.LimitReader(rc, (128<<20)+1))
		rc.Close()
		if e != nil || len(b) > 128<<20 {
			return "", fmt.Errorf("archivo corrupto")
		}
		if e = os.WriteFile(filepath.Join(stage, z.Name), b, 0700); e != nil {
			return "", e
		}
	}
	if !seen[want] || !seen["release.json"] {
		return "", fmt.Errorf("release incompleta")
	}
	var info struct {
		Version      string `json:"version"`
		Architecture string `json:"architecture"`
		Schema       int    `json:"schema_version"`
	}
	b, e := os.ReadFile(filepath.Join(stage, "release.json"))
	if e != nil {
		return "", e
	}
	if e = json.Unmarshal(b, &info); e != nil || info.Version != m.Version || info.Architecture != arch || info.Schema != m.Schema {
		return "", fmt.Errorf("metadatos incoherentes")
	}
	if strings.HasPrefix(arch, "windows-") {
		exe, e := pe.Open(filepath.Join(stage, want))
		if e != nil {
			return "", e
		}
		machine := exe.Machine
		exe.Close()
		if arch == "windows-amd64" && machine != pe.IMAGE_FILE_MACHINE_AMD64 || arch == "windows-arm64" && machine != pe.IMAGE_FILE_MACHINE_ARM64 {
			return "", fmt.Errorf("arquitectura del ejecutable incorrecta")
		}
	}
	dst := filepath.Join(base, "versions", m.Version)
	if e = os.MkdirAll(filepath.Dir(dst), 0700); e != nil {
		return "", e
	}
	if _, e = os.Stat(dst); !os.IsNotExist(e) {
		return "", fmt.Errorf("la versión ya existe; no se sobrescribe")
	}
	if e = os.Rename(stage, dst); e != nil {
		return "", e
	}
	return Program(base, m.Version), nil
}

type rejectTransport struct{}

func (rejectTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("certificado de canal no válido")
}
