package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Session struct {
	UserID, CSRF string
	Created      time.Time
}

type App struct {
	requestMu                                                        sync.Mutex
	Repo                                                             *Repository
	Config                                                           Config
	BaseDir, DataDir, PhotosDir, BackupsDir, ConfigPath, RuntimePath string
	Version                                                          string
	mu                                                               sync.Mutex
	Sessions                                                         map[string]Session
	Server                                                           *http.Server
	ShutdownToken                                                    string
}

func NewApp(baseDir string) (*App, error) {
	dataDir := filepath.Join(baseDir, "data")
	cfgPath := filepath.Join(baseDir, "config.json")
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	repo, err := LoadOrCreate(filepath.Join(dataDir, "store.json"))
	if err != nil {
		return nil, err
	}
	if len(repo.Data.Users) == 0 {
		repo.Data.Users = []User{NewUser("admin", "Administrador", "Zocotroco$01", true, true)}
		if err := repo.Save(); err != nil {
			return nil, err
		}
	}
	if len(repo.Data.ReportTemplates) == 0 {
		d := DefaultStore()
		repo.Data.ReportTemplates = d.ReportTemplates
		repo.Data.DefaultReportTemplateID = d.DefaultReportTemplateID
		if err := repo.Save(); err != nil {
			return nil, err
		}
	}
	if err := SaveConfig(cfgPath, cfg); err != nil {
		return nil, err
	}
	return &App{Repo: repo, Config: cfg, BaseDir: baseDir, DataDir: dataDir, PhotosDir: filepath.Join(dataDir, "photos"), BackupsDir: filepath.Join(dataDir, "backups"), ConfigPath: cfgPath, RuntimePath: filepath.Join(dataDir, "runtime.json"), Version: "8.2.0", Sessions: map[string]Session{}, ShutdownToken: randomID()}, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.root)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	})
	mux.HandleFunc("/login", a.login)
	mux.HandleFunc("/logout", a.logout)
	mux.HandleFunc("/registro", a.register)
	mux.HandleFunc("/cuenta", a.auth(a.account, false))
	mux.HandleFunc("/inicio", a.auth(a.home, false))
	mux.HandleFunc("/buceadores", a.auth(a.divers, false))
	mux.HandleFunc("/buceador/nuevo", a.auth(a.diverNew, false))
	mux.HandleFunc("/buceador/", a.auth(a.diverRoutes, false))
	mux.HandleFunc("/inmersiones", a.auth(a.dives, false))
	mux.HandleFunc("/inmersion/nueva", a.auth(a.diveNew, false))
	mux.HandleFunc("/inmersion/", a.auth(a.diveRoutes, false))
	mux.HandleFunc("/mantenimiento", a.auth(a.maintenance, false))
	mux.HandleFunc("/mantenimiento/", a.auth(a.maintenanceDetail, false))
	mux.HandleFunc("/informes", a.auth(a.reports, false))
	mux.HandleFunc("/informe/mensual", a.auth(a.reportMonthly, false))
	mux.HandleFunc("/informe/todas", a.auth(a.reportAll, false))
	mux.HandleFunc("/informe/buceador/", a.auth(a.reportDiver, false))
	mux.HandleFunc("/admin/importar", a.auth(a.importExcel, true))
	mux.HandleFunc("/admin/usuarios", a.auth(a.adminUsers, true))
	mux.HandleFunc("/admin/formato/guardar", a.auth(a.saveReportTemplate, true))
	mux.HandleFunc("/admin/backup", a.auth(a.backupNow, true))
	mux.HandleFunc("/admin/restaurar", a.auth(a.restoreBackup, true))
	mux.HandleFunc("/admin/cerrar", a.auth(a.shutdownWeb, true))
	mux.HandleFunc("/internal/shutdown", a.internalShutdown)
	mux.HandleFunc("/internal/status", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() || r.URL.Query().Get("token") != a.ShutdownToken {
			http.Error(w, "forbidden", 403)
			return
		}
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/photos/", a.auth(func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/photos/", http.FileServer(http.Dir(a.PhotosDir))).ServeHTTP(w, r)
	}, false))
	return securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requestMu.Lock()
		defer a.requestMu.Unlock()
		host := r.Host
		if h, _, e := net.SplitHostPort(host); e == nil {
			host = h
		}
		ip := net.ParseIP(host)
		allowed := host == "localhost" || ip != nil && ip.IsLoopback()
		for _, h := range a.Config.AllowedHosts {
			if strings.EqualFold(h, host) {
				allowed = true
			}
		}
		if !allowed {
			http.Error(w, "Host no permitido", 403)
			return
		}
		if origin := r.Header.Get("Origin"); r.Method == "POST" && origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host || u.Scheme != "http" {
				http.Error(w, "Origen no permitido", 403)
				return
			}
		}

		if r.Method != "GET" && r.Method != "POST" {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Método no admitido", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil && r.Method == "POST" && !strings.Contains(err.Error(), "isn't multipart/form-data") {
			http.Error(w, "Formulario no válido o demasiado grande", 400)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		mux.ServeHTTP(w, r)
	}))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (a *App) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.session(r); ok {
		http.Redirect(w, r, "/inicio", 302)
	} else {
		http.Redirect(w, r, "/login", 302)
	}
}

func (a *App) session(r *http.Request) (Session, bool) {
	c, e := r.Cookie("cb_session")
	if e != nil {
		return Session{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.Sessions[c.Value]
	if ok && time.Since(s.Created) > 12*time.Hour {
		delete(a.Sessions, c.Value)
		ok = false
	}
	return s, ok
}
func (a *App) currentUser(r *http.Request) (User, bool) {
	s, ok := a.session(r)
	if !ok {
		return User{}, false
	}
	var u User
	found := false
	a.Repo.Read(func(st Store) {
		if p := st.UserByID(s.UserID); p != nil {
			u = *p
			found = true
		}
	})
	return u, found
}
func (a *App) auth(h http.HandlerFunc, admin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.currentUser(r)
		if !ok {
			http.Redirect(w, r, "/login", 302)
			return
		}
		if u.Status != "approved" {
			http.Error(w, "Usuario pendiente de aprobación", 403)
			return
		}
		if admin && !u.IsAdmin {
			http.Error(w, "Se requiere administrador", 403)
			return
		}
		h(w, r)
	}
}
func (a *App) csrfOK(r *http.Request) bool {
	s, ok := a.session(r)
	return ok && r.FormValue("csrf") == s.CSRF
}
func (a *App) csrf(r *http.Request) string { s, _ := a.session(r); return s.CSRF }

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func publicCSRF(w http.ResponseWriter, r *http.Request) string {
	token := randToken()
	http.SetCookie(w, &http.Cookie{Name: "cb_public_csrf", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return token
}
func publicCSRFOK(r *http.Request) bool {
	c, e := r.Cookie("cb_public_csrf")
	return e == nil && c.Value != "" && r.FormValue("csrf") == c.Value
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		a.page(w, r, "Acceso", strings.Replace(`<div class="card narrow"><h2>Acceso</h2><form method="post"><label>Usuario</label><input name="username" required><label>Contraseña</label><input type="password" name="password" required><button>Entrar</button></form><p><a href="/registro">Solicitar una cuenta</a></p></div>`, `<form method="post">`, `<form method="post"><input type="hidden" name="csrf" value="`+publicCSRF(w, r)+`">`, 1))
		return
	}
	if !publicCSRFOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	_ = r.ParseForm()
	var u User
	found := false
	a.Repo.Read(func(st Store) {
		if p := st.UserByUsername(r.FormValue("username")); p != nil {
			u = *p
			found = true
		}
	})
	if !found || !VerifyPassword(u, r.FormValue("password")) {
		http.Error(w, "Credenciales incorrectas. Si esta base procede de V7, migre primero los hashes legacy o use una base nueva.", 401)
		return
	}
	if u.Status != "approved" {
		http.Error(w, "Cuenta pendiente de aprobación", 403)
		return
	}
	t := randToken()
	s := Session{UserID: u.ID, CSRF: randToken(), Created: time.Now()}
	a.mu.Lock()
	a.Sessions[t] = s
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "cb_session", Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/inicio", 302)
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST", 405)
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	if c, e := r.Cookie("cb_session"); e == nil {
		a.mu.Lock()
		delete(a.Sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "cb_session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", 302)
}
func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		a.page(w, r, "Registro", strings.Replace(`<div class="card narrow"><h2>Solicitud de cuenta</h2><form method="post"><label>Usuario</label><input name="username" required><label>Nombre visible</label><input name="display_name" required><label>Contraseña</label><input type="password" name="password" required minlength="8"><button>Solicitar</button></form></div>`, `<form method="post">`, `<form method="post"><input type="hidden" name="csrf" value="`+publicCSRF(w, r)+`">`, 1))
		return
	}
	if !publicCSRFOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	_ = r.ParseForm()
	user := strings.TrimSpace(r.FormValue("username"))
	if user == "" || len(r.FormValue("password")) < 8 || strings.TrimSpace(r.FormValue("display_name")) == "" {
		http.Error(w, "Datos incompletos", 400)
		return
	}
	err := a.Repo.Update(func(st *Store) error {
		if st.UserByUsername(user) != nil {
			return fmt.Errorf("usuario ya existe")
		}
		st.Users = append(st.Users, NewUser(user, strings.TrimSpace(r.FormValue("display_name")), r.FormValue("password"), false, false))
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.page(w, r, "Solicitud enviada", `<div class="card"><h2>Solicitud registrada</h2><p>Un administrador debe aprobarla antes del primer acceso.</p><a class="btn" href="/login">Volver</a></div>`)
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	var s Store
	a.Repo.Read(func(x Store) { s = x })
	body := fmt.Sprintf(`<div class="grid kpis"><div class="kpi"><span>Buceadores activos</span><b>%d</b></div><div class="kpi"><span>Inmersiones</span><b>%d</b></div><div class="kpi"><span>Usuarios pendientes</span><b>%d</b></div></div><div class="card"><h2>%s</h2><p>Aplicación de control de buceadores, inmersiones, Dato IB, mantenimiento e informes.</p></div>`, countActive(s.Divers), len(s.Dives), countPending(s.Users), template.HTMLEscapeString(a.Config.UnitName))
	a.page(w, r, "Inicio", body)
}
func countActive(ds []Diver) int {
	n := 0
	for _, d := range ds {
		if d.Active {
			n++
		}
	}
	return n
}
func countPending(us []User) int {
	n := 0
	for _, u := range us {
		if u.Status == "pending" {
			n++
		}
	}
	return n
}

func (a *App) divers(w http.ResponseWriter, r *http.Request) {
	var ds []Diver
	a.Repo.Read(func(s Store) { ds = append(ds, s.Divers...) })
	sort.Slice(ds, func(i, j int) bool { return ds[i].Name < ds[j].Name })
	var b strings.Builder
	b.WriteString(`<div class="actions"><h2>Buceadores</h2><a class="btn" href="/buceador/nuevo">Nuevo buceador</a></div><div class="card table-scroll"><table><tr><th>ID</th><th>Empleo</th><th>Nombre</th><th>Autónomo</th><th>SDS Aire</th><th>SDS HELIOX</th><th>Activo</th><th></th></tr>`)
	for _, d := range ds {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td><a href="/buceador/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td><a href="/buceador/%s/editar">Editar</a></td></tr>`, esc(d.ID), esc(d.Rank), url.PathEscape(d.ID), esc(d.Name), yn(d.QualBasic), yn(d.QualSDSAir), yn(d.QualSDSHeliox), yn(d.Active), url.PathEscape(d.ID))
	}
	b.WriteString(`</table></div>`)
	a.page(w, r, "Buceadores", b.String())
}
func yn(v bool) string {
	if v {
		return "SI"
	}
	return "NO"
}
func esc(s string) string { return template.HTMLEscapeString(s) }

func (a *App) diverNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		a.diverForm(w, r, Diver{Active: true}, "Nuevo buceador", "/buceador/nuevo")
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	_ = r.ParseMultipartForm(8 << 20)
	if _, _, err := photoData(r); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	d := diverFromForm(r)
	if err := validateDiver(d); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err := a.Repo.Update(func(s *Store) error {
		if s.DiverByID(d.ID) != nil {
			return fmt.Errorf("ID ya existente")
		}
		s.Divers = append(s.Divers, d)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := a.savePhoto(r, &d); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/buceadores", 302)
}

func diverFromForm(r *http.Request) Diver {
	return Diver{ID: strings.TrimSpace(r.FormValue("id")), Rank: strings.TrimSpace(r.FormValue("rank")), Name: strings.TrimSpace(r.FormValue("name")), EntryDate: r.FormValue("entry_date"), QualBasic: r.FormValue("qual_basic") == "on", QualSDSAir: r.FormValue("qual_sds_air") == "on", QualSDSHeliox: r.FormValue("qual_sds_heliox") == "on", Active: r.FormValue("active") == "on", Notes: r.FormValue("notes")}
}
func photoData(r *http.Request) ([]byte, string, error) {
	f, h, e := r.FormFile("photo")
	if e == http.ErrMissingFile || e == http.ErrNotMultipart {
		return nil, "", nil
	}
	if e != nil {
		return nil, "", e
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(h.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil, "", fmt.Errorf("foto: formato no admitido")
	}
	b, e := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if e != nil {
		return nil, "", e
	}
	if len(b) > 8<<20 {
		return nil, "", fmt.Errorf("foto demasiado grande")
	}

	cfg, kind, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 12000 || cfg.Height > 12000 || int64(cfg.Width)*int64(cfg.Height) > 30000000 {
		return nil, "", fmt.Errorf("imagen no válida")
	}
	if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
		return nil, "", fmt.Errorf("imagen dañada")
	}
	if kind == "png" && ext != ".png" || kind == "jpeg" && ext != ".jpg" && ext != ".jpeg" {
		return nil, "", fmt.Errorf("extensión de imagen incorrecta")
	}
	return b, ext, nil
}
func (a *App) savePhoto(r *http.Request, d *Diver) error {
	b, ext, e := photoData(r)
	if e != nil || b == nil {
		return e
	}
	if e = os.MkdirAll(a.PhotosDir, 0755); e != nil {
		return e
	}
	name := randomID() + ext
	p := filepath.Join(a.PhotosDir, name)
	if e = os.WriteFile(p, b, 0600); e != nil {
		return e
	}
	e = a.Repo.Update(func(s *Store) error {
		old := s.DiverByID(d.ID)
		if old == nil {
			return fmt.Errorf("buceador no existe")
		}
		old.Photo = name
		return nil
	})
	if e != nil {
		os.Remove(p)
		return e
	}
	return nil
}

func (a *App) diverRoutes(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/buceador/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 {
		return
	}
	id, _ := url.PathUnescape(parts[0])
	var d Diver
	found := false
	a.Repo.Read(func(s Store) {
		if x := s.DiverByID(id); x != nil {
			d = *x
			found = true
		}
	})
	if !found {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		a.diverView(w, r, d)
		return
	}
	switch parts[1] {
	case "editar":
		a.diverEdit(w, r, d)
	case "eliminar":
		a.diverDelete(w, r, d)
	default:
		http.NotFound(w, r)
	}
}
func (a *App) diverView(w http.ResponseWriter, r *http.Request, d Diver) {
	var ds []Dive
	a.Repo.Read(func(s Store) {
		for _, x := range s.Dives {
			if x.DiverID == d.ID {
				ds = append(ds, x)
			}
		}
	})
	sort.Slice(ds, func(i, j int) bool { return ds[i].Date > ds[j].Date })
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="actions"><h2>%s %s</h2><a class="btn" href="/buceador/%s/editar">Editar</a><a class="btn secondary" href="/informe/buceador/%s">Imprimir registro</a></div>`, esc(d.Rank), esc(d.Name), url.PathEscape(d.ID), url.PathEscape(d.ID))
	if a.currentIsAdmin(r) {
		fmt.Fprintf(&b, `<p class="no-print"><a class="btn danger" href="/buceador/%s/eliminar">Eliminar buceador</a></p>`, url.PathEscape(d.ID))
	}
	if d.Photo != "" {
		fmt.Fprintf(&b, `<img class="photo" src="/photos/%s">`, url.PathEscape(d.Photo))
	}
	fmt.Fprintf(&b, `<div class="card"><b>ID:</b> %s · <b>Autónomo:</b> %s · <b>SDS Aire:</b> %s · <b>SDS HELIOX:</b> %s</div>`, esc(d.ID), yn(d.QualBasic), yn(d.QualSDSAir), yn(d.QualSDSHeliox))
	b.WriteString(diveTable(ds, true))
	a.page(w, r, d.Name, b.String())
}
func (a *App) diverEdit(w http.ResponseWriter, r *http.Request, d Diver) {
	if r.Method == "GET" {
		a.diverForm(w, r, d, "Editar buceador", "/buceador/"+url.PathEscape(d.ID)+"/editar")
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	_ = r.ParseMultipartForm(8 << 20)
	if _, _, err := photoData(r); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	n := diverFromForm(r)
	n.ID = d.ID
	n.Photo = d.Photo
	if err := validateDiver(n); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err := a.Repo.Update(func(s *Store) error {
		p := s.DiverByID(d.ID)
		if p == nil {
			return fmt.Errorf("no existe")
		}
		*p = n
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := a.savePhoto(r, &n); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/buceador/"+url.PathEscape(d.ID), 302)
}
func (a *App) diverDelete(w http.ResponseWriter, r *http.Request, d Diver) {
	u, _ := a.currentUser(r)
	if !u.IsAdmin {
		http.Error(w, "Solo administrador", 403)
		return
	}
	if r.Method == "GET" {
		a.page(w, r, "Eliminar", fmt.Sprintf(`<div class="card"><h2>Eliminar %s</h2><p>Esta operación es definitiva y eliminará también sus inmersiones.</p><form method="post"><input type="hidden" name="csrf" value="%s"><button class="danger">Eliminar definitivamente</button></form></div>`, esc(d.Name), a.csrf(r)))
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	if err := a.autoBackup("antes de eliminar buceador " + d.ID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	err := a.Repo.Update(func(s *Store) error {
		nd := s.Divers[:0]
		for _, x := range s.Divers {
			if x.ID != d.ID {
				nd = append(nd, x)
			}
		}
		s.Divers = nd
		ni := s.Dives[:0]
		for _, x := range s.Dives {
			if x.DiverID != d.ID {
				ni = append(ni, x)
			}
		}
		s.Dives = ni
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/buceadores", 302)
}
func (a *App) diverForm(w http.ResponseWriter, r *http.Request, d Diver, title, action string) {
	chk := func(v bool) string {
		if v {
			return " checked"
		}
		return ""
	}
	body := fmt.Sprintf(`<div class="card"><h2>%s</h2><form method="post" enctype="multipart/form-data" action="%s"><input type="hidden" name="csrf" value="%s"><label>ID</label><input name="id" value="%s" %s required><label>Empleo</label><input name="rank" value="%s"><label>Nombre y apellidos</label><input name="name" value="%s" required><label>Fecha de entrada</label><input type="date" name="entry_date" value="%s"><label><input type="checkbox" name="qual_basic"%s> Autónomo CA aire/nitrox básico</label><label><input type="checkbox" name="qual_sds_air"%s> SDS aire</label><label><input type="checkbox" name="qual_sds_heliox"%s> SDS HELIOX</label><label><input type="checkbox" name="active"%s> Activo</label><label>Foto (opcional)</label><input type="file" name="photo" accept="image/png,image/jpeg"><label>Observaciones</label><textarea name="notes">%s</textarea><button>Guardar</button></form></div>`, esc(title), action, a.csrf(r), esc(d.ID), map[bool]string{true: "readonly", false: ""}[d.ID != ""], esc(d.Rank), esc(d.Name), esc(d.EntryDate), chk(d.QualBasic), chk(d.QualSDSAir), chk(d.QualSDSHeliox), chk(d.Active), esc(d.Notes))
	a.page(w, r, title, body)
}

func (a *App) dives(w http.ResponseWriter, r *http.Request) {
	var ds []Dive
	a.Repo.Read(func(s Store) { ds = append(ds, s.Dives...) })
	sort.Slice(ds, func(i, j int) bool { return ds[i].Date > ds[j].Date })
	body := `<div class="actions"><h2>Inmersiones</h2><a class="btn" href="/inmersion/nueva">Nueva inmersión</a></div>` + diveTable(ds, false)
	a.page(w, r, "Inmersiones", body)
}
func diveTable(ds []Dive, compact bool) string {
	var b strings.Builder
	b.WriteString(`<div class="card table-scroll"><table><tr><th>Fecha</th><th>Buceador</th><th>Cualificación</th><th>Tipo Dato IB</th><th>Código</th><th>Gas</th><th>Prof.</th><th>Fondo</th>`)
	if !compact {
		b.WriteString(`<th></th>`)
	}
	b.WriteString(`</tr>`)
	for _, d := range ds {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%.1f</td><td>%d</td>`, esc(d.Date), esc(d.DiverID), esc(d.Qualification), esc(d.DiveType), esc(d.IBCode), esc(d.Gas), d.MaxDepth, d.BottomTime)
		if !compact {
			fmt.Fprintf(&b, `<td><a href="/inmersion/%d/editar">Editar</a> · <a href="/inmersion/%d/eliminar">Eliminar</a></td>`, d.ID, d.ID)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</table></div>`)
	return b.String()
}

func (a *App) diveNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		a.diveForm(w, r, Dive{Valid: true}, "Nueva inmersión", "/inmersion/nueva")
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	d, err := a.diveFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err = a.Repo.Update(func(s *Store) error {
		if s.DiverByID(d.DiverID) == nil {
			return fmt.Errorf("buceador inexistente")
		}
		d.ID = s.NextDiveID
		s.NextDiveID++
		d.CreatedAt = nowUTC()
		s.Dives = append(s.Dives, d)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/inmersiones", 302)
}
func (a *App) diveRoutes(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/inmersion/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	id, _ := strconv.Atoi(parts[0])
	var d Dive
	found := false
	a.Repo.Read(func(s Store) {
		if x := s.DiveByID(id); x != nil {
			d = *x
			found = true
		}
	})
	if !found {
		http.NotFound(w, r)
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "editar":
		a.diveEdit(w, r, d)
	case "eliminar":
		a.diveDelete(w, r, d)
	default:
		http.NotFound(w, r)
	}
}
func (a *App) diveEdit(w http.ResponseWriter, r *http.Request, d Dive) {
	if r.Method == "GET" {
		a.diveForm(w, r, d, "Editar inmersión", fmt.Sprintf("/inmersion/%d/editar", d.ID))
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	n, err := a.diveFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	n.ID = d.ID
	n.CreatedAt = d.CreatedAt
	err = a.Repo.Update(func(s *Store) error {
		if s.DiverByID(n.DiverID) == nil {
			return fmt.Errorf("buceador inexistente")
		}
		p := s.DiveByID(d.ID)
		if p == nil {
			return fmt.Errorf("no existe")
		}
		*p = n
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/inmersiones", 302)
}
func (a *App) diveDelete(w http.ResponseWriter, r *http.Request, d Dive) {
	u, _ := a.currentUser(r)
	if !u.IsAdmin {
		http.Error(w, "Solo administrador", 403)
		return
	}
	if r.Method == "GET" {
		a.page(w, r, "Eliminar inmersión", fmt.Sprintf(`<div class="card"><h2>Eliminar inmersión %d</h2><form method="post"><input type="hidden" name="csrf" value="%s"><button class="danger">Eliminar definitivamente</button></form></div>`, d.ID, a.csrf(r)))
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	if err := a.autoBackup(fmt.Sprintf("antes de eliminar inmersión %d", d.ID)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	err := a.Repo.Update(func(s *Store) error {
		nd := s.Dives[:0]
		for _, x := range s.Dives {
			if x.ID != d.ID {
				nd = append(nd, x)
			}
		}
		s.Dives = nd
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/inmersiones", 302)
}

func (a *App) diveFromRequest(r *http.Request) (Dive, error) {
	_ = r.ParseForm()
	depth, depthErr := strconv.ParseFloat(strings.ReplaceAll(r.FormValue("max_depth"), ",", "."), 64)
	bt, timeErr := strconv.Atoi(r.FormValue("bottom_time"))
	if depthErr != nil || timeErr != nil || math.IsNaN(depth) || math.IsInf(depth, 0) || depth < 0 || bt < 0 {
		return Dive{}, fmt.Errorf("profundidad o tiempo no válido")
	}
	if strings.TrimSpace(r.FormValue("qualification")) == "" {
		return Dive{}, fmt.Errorf("cualificación requerida")
	}
	dt := r.FormValue("dive_type")
	code := IBCodeFor(dt)
	if code == "" {
		return Dive{}, fmt.Errorf("tipo de inmersión Dato IB no válido")
	}
	date := r.FormValue("date")
	if t, err := ParseDate(date); err != nil || t.Format("2006-01-02") > time.Now().Format("2006-01-02") {
		return Dive{}, fmt.Errorf("fecha no válida o futura")
	}
	return Dive{DiverID: r.FormValue("diver_id"), Date: date, Qualification: r.FormValue("qualification"), BreathingMode: r.FormValue("breathing_mode"), Gas: r.FormValue("gas"), SurfaceSupply: r.FormValue("surface_supply") == "on", MaxDepth: depth, BottomTime: bt, DiveType: dt, IBCode: code, Purpose: r.FormValue("purpose"), Notes: r.FormValue("notes"), Valid: r.FormValue("valid") == "on"}, nil
}
func (a *App) diveForm(w http.ResponseWriter, r *http.Request, d Dive, title, action string) {
	var st Store
	a.Repo.Read(func(s Store) { st = s })
	var opts strings.Builder
	for _, x := range st.Divers {
		if x.Active || x.ID == d.DiverID {
			fmt.Fprintf(&opts, `<option value="%s" %s>%s - %s</option>`, esc(x.ID), sel(x.ID, d.DiverID), esc(x.ID), esc(x.Name))
		}
	}
	var ib strings.Builder
	for _, o := range IBOptions {
		fmt.Fprintf(&ib, `<option value="%s" %s>%s → %s</option>`, esc(o.Label), sel(o.Label, d.DiveType), esc(o.Label), o.Code)
	}
	body := fmt.Sprintf(`<div class="card"><h2>%s</h2><form method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Buceador</label><select name="diver_id">%s</select><label>Fecha</label><input type="date" name="date" value="%s" required><label>Cualificación</label><select name="qualification"><option %s>Autónomo CA aire/nitrox básico</option><option %s>SDS aire</option><option %s>SDS HELIOX</option></select><label>Tipo de inmersión (MANUAL DATO IB)</label><select id="dive_type" name="dive_type" onchange="codeIB()">%s</select><label>Código Dato IB</label><input id="ib_code" value="%s" readonly><label>Medio respiratorio</label><input name="breathing_mode" value="%s"><label>Gas / mezcla</label><input name="gas" value="%s"><label><input type="checkbox" name="surface_supply" %s> Suministro desde superficie</label><label>Profundidad máxima (m)</label><input type="number" step="0.1" name="max_depth" value="%.1f"><label>Tiempo de fondo (min)</label><input type="number" name="bottom_time" value="%d"><label>Finalidad / clasificación</label><input name="purpose" value="%s"><label>Observaciones</label><textarea name="notes">%s</textarea><label><input type="checkbox" name="valid" %s> Registro válido para mantenimiento</label><button>Guardar</button></form><script>const map=%s;function codeIB(){document.getElementById('ib_code').value=map[document.getElementById('dive_type').value]||''}codeIB();</script></div>`, esc(title), action, a.csrf(r), opts.String(), esc(d.Date), sel("Autónomo CA aire/nitrox básico", d.Qualification), sel("SDS aire", d.Qualification), sel("SDS HELIOX", d.Qualification), ib.String(), esc(d.IBCode), esc(d.BreathingMode), esc(d.Gas), chkAttr(d.SurfaceSupply), d.MaxDepth, d.BottomTime, esc(d.Purpose), esc(d.Notes), chkAttr(d.Valid), jsIBMap())
	a.page(w, r, title, body)
}
func sel(a, b string) string {
	if a == b {
		return "selected"
	}
	return ""
}
func chkAttr(v bool) string {
	if v {
		return "checked"
	}
	return ""
}
func jsIBMap() string {
	m := map[string]string{}
	for _, o := range IBOptions {
		m[o.Label] = o.Code
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func (a *App) maintenance(w http.ResponseWriter, r *http.Request) {
	var st Store
	a.Repo.Read(func(s Store) { st = s })
	var b strings.Builder
	b.WriteString(`<div class="actions no-print"><h2>Mantenimiento de cualificaciones</h2><button onclick="window.print()">Imprimir control</button></div><div class="card table-scroll"><table class="maintenance-summary"><tr><th>Empleo</th><th>Nombre y apellidos</th><th>Autónomo</th><th>SDS Aire</th><th>SDS HELIOX</th><th class="no-print"></th></tr>`)
	sort.Slice(st.Divers, func(i, j int) bool { return st.Divers[i].Name < st.Divers[j].Name })
	for _, d := range st.Divers {
		if !d.Active {
			continue
		}
		qs := MaintenanceFor(d, st.Dives, time.Now())
		fmt.Fprintf(&b, `<tr><td>%s</td><td><a href="/mantenimiento/%s">%s</a></td>`, esc(d.Rank), url.PathEscape(d.ID), esc(d.Name))
		for _, q := range qs {
			fmt.Fprintf(&b, `<td class="status-cell"><span class="badge %s">%s</span></td>`, badge(q.Status), esc(q.Status))
		}
		fmt.Fprintf(&b, `<td class="no-print"><a href="/mantenimiento/%s">Detalle</a></td></tr>`, url.PathEscape(d.ID))
	}
	b.WriteString(`</table></div>`)
	a.page(w, r, "Mantenimiento", b.String())
}
func badge(s string) string {
	switch s {
	case "OK":
		return "ok"
	case "A RECUPERAR":
		return "warn"
	case "FUERA DE PLAZO":
		return "danger"
	default:
		return "neutral"
	}
}
func (a *App) maintenanceDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/mantenimiento/"))
	var st Store
	a.Repo.Read(func(s Store) { st = s })
	d := st.DiverByID(id)
	if d == nil {
		http.NotFound(w, r)
		return
	}
	qs := MaintenanceFor(*d, st.Dives, time.Now())
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="actions no-print"><h2>%s</h2><button onclick="window.print()">Imprimir ficha</button></div>`, esc(d.Name))
	for _, q := range qs {
		fmt.Fprintf(&b, `<div class="card qual-card"><div class="qual-title"><h3>%s</h3><span class="badge %s">%s</span></div><p>%s</p><p><b>Última:</b> %s · <b>12 meses:</b> %d/%d · <b>Límite orientativo:</b> %s</p><p><b>Recuperación:</b> %s</p>%s</div>`, esc(q.Label), badge(q.Status), esc(q.Status), esc(q.Detail), esc(q.LastDive), q.Count12m, q.Required, esc(q.Deadline), esc(q.Recovery), diveTable(q.Dives, true))
	}
	a.page(w, r, "Mantenimiento · "+d.Name, b.String())
}

func (a *App) reports(w http.ResponseWriter, r *http.Request) {
	body := `<div class="card"><h2>Informes</h2><p><a class="btn" href="/informe/mensual">Relación mensual</a> <a class="btn secondary" href="/informe/todas">Todas las inmersiones</a></p><p>La relación mensual permite editar cabeceras, firmas y formato sin editar las filas de inmersiones, y guardar el resultado como nueva plantilla.</p></div>`
	a.page(w, r, "Informes", body)
}
func (a *App) reportMonthly(w http.ResponseWriter, r *http.Request) {
	var st Store
	a.Repo.Read(func(s Store) { st = s })
	q := r.URL.Query()
	now := time.Now()
	y, _ := strconv.Atoi(q.Get("year"))
	m, _ := strconv.Atoi(q.Get("month"))
	if y == 0 {
		y = now.Year()
	}
	if m < 1 || m > 12 {
		m = int(now.Month())
	}
	tmpl := findTemplate(st, q.Get("format"))
	start := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	var ds []Dive
	for _, d := range st.Dives {
		t, e := ParseDate(d.Date)
		if e == nil && !t.Before(start) && !t.After(end) {
			ds = append(ds, d)
		}
	}
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].DiverID == ds[j].DiverID {
			return ds[i].Date < ds[j].Date
		}
		return diverName(st, ds[i].DiverID) < diverName(st, ds[j].DiverID)
	})
	ctx := map[string]string{"MES": strings.ToUpper(monthES(time.Month(m))), "AÑO": strconv.Itoa(y), "UNIDAD": a.Config.UnitName, "LOCALIDAD": a.Config.ReportLocation, "FECHA_FIN_MES": fmt.Sprintf("%d de %s de %d", end.Day(), monthES(end.Month()), end.Year())}
	var controls strings.Builder
	fmt.Fprintf(&controls, `<form class="no-print card" method="get"><label>Año</label><input type="number" name="year" min="1900" max="9999" value="%d"><label>Mes</label><select name="month">`, y)
	for month := 1; month <= 12; month++ {
		fmt.Fprintf(&controls, `<option value="%d" %s>%s</option>`, month, sel(strconv.Itoa(month), strconv.Itoa(m)), monthES(time.Month(month)))
	}
	controls.WriteString(`</select><label>Formato</label><select name="format">`)
	for _, f := range st.ReportTemplates {
		fmt.Fprintf(&controls, `<option value="%s" %s>%s</option>`, esc(f.ID), sel(f.ID, tmpl.ID), esc(f.Name))
	}
	controls.WriteString(`</select><button>Mostrar</button></form>`)
	body := controls.String() + renderMonthly(st, ds, tmpl, ctx, a.csrf(r), a.currentIsAdmin(r))
	a.page(w, r, "Relación mensual", body)
}
func findTemplate(st Store, id string) ReportTemplate {
	if id == "" {
		id = st.DefaultReportTemplateID
	}
	for _, t := range st.ReportTemplates {
		if t.ID == id {
			return t
		}
	}
	return DefaultStore().ReportTemplates[0]
}
func replaceVars(s string, c map[string]string) string {
	for k, v := range c {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}
func monthES(m time.Month) string {
	v := []string{"", "enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	return v[int(m)]
}
func diverName(st Store, id string) string {
	if d := st.DiverByID(id); d != nil {
		return d.Name
	}
	return id
}
func (a *App) currentIsAdmin(r *http.Request) bool { u, ok := a.currentUser(r); return ok && u.IsAdmin }
func renderMonthly(st Store, ds []Dive, t ReportTemplate, ctx map[string]string, csrf string, admin bool) string {
	if !allowedFont(t.FontFamily) {
		t.FontFamily = "Arial"
	}
	if t.BodyFontSize < 1 || t.BodyFontSize > 48 {
		t.BodyFontSize = 11
	}
	if t.TableFontSize < 1 || t.TableFontSize > 48 {
		t.TableFontSize = 9
	}
	if t.SignatureTopSpace < 0 || t.SignatureTopSpace > 300 {
		t.SignatureTopSpace = 52
	}
	var rows strings.Builder
	last := ""
	for _, d := range ds {
		name := diverName(st, d.DiverID)
		show := name
		if d.DiverID == last {
			show = ""
		}
		last = d.DiverID
		fmt.Fprintf(&rows, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, esc(show), esc(formatDateES(d.Date)), esc(d.DiveType), esc(d.Gas), esc(d.IBCode))
	}
	save := ""
	if admin {
		save = fmt.Sprintf(`<div class="no-print card"><h3>Guardar cambios como nuevo formato</h3><form method="post" action="/admin/formato/guardar"><input type="hidden" name="csrf" value="%s"><label>Nombre</label><input name="name" required><input type="hidden" name="source_format" value="%s"><label>Fuente</label><select name="font_family"><option>Arial</option><option>Times New Roman</option><option>Calibri</option><option>Helvetica</option></select><label>Tamaño del texto (pt)</label><input name="body_font_size" type="number" min="1" max="48" value="%d"><label>Tamaño de tabla (pt)</label><input name="table_font_size" type="number" min="1" max="48" value="%d"><label>Separación de firmas (px)</label><input name="signature_top_space" type="number" min="1" max="300" value="%d"><button>Guardar como formato nuevo</button></form></div>`, csrf, esc(t.ID), t.BodyFontSize, t.TableFontSize, t.SignatureTopSpace)
	}
	result := fmt.Sprintf(`<div class="no-print actions"><button onclick="window.print()">Imprimir</button></div><section id="monthly" contenteditable="false" style="font-family:%s;font-size:%dpt"><h2 class="editable" contenteditable="true">%s</h2><p class="editable" contenteditable="true">%s</p><table style="font-size:%dpt"><thead><tr><th class="editable" contenteditable="true">%s</th><th class="editable" contenteditable="true">%s</th><th class="editable" contenteditable="true">%s</th><th class="editable" contenteditable="true">%s</th><th class="editable" contenteditable="true">%s</th></tr></thead><tbody class="locked">%s</tbody></table><p class="editable" contenteditable="true">%s</p><div class="signatures" style="margin-top:%dpx"><div><p class="editable" contenteditable="true">%s</p><p class="editable" contenteditable="true">%s</p></div><div><p class="editable" contenteditable="true">%s</p><p class="editable" contenteditable="true">%s</p></div></div></section>%s`, esc(t.FontFamily), t.BodyFontSize, esc(t.Title), esc(replaceVars(t.IntroTemplate, ctx)), t.TableFontSize, esc(t.ColName), esc(t.ColDate), esc(t.ColEquipment), esc(t.ColGas), esc(t.ColCode), rows.String(), esc(replaceVars(t.LocationTemplate, ctx)), t.SignatureTopSpace, esc(t.Sign1Role), esc(t.Sign1Name), esc(t.Sign2Role), esc(t.Sign2Name), save)
	fields := []string{"title", "intro_template", "col_name", "col_date", "col_equipment", "col_gas", "col_code", "location_template", "sign1_role", "sign1_name", "sign2_role", "sign2_name"}
	for _, field := range fields {
		result = strings.Replace(result, `class="editable" contenteditable="true"`, `data-field="`+field+`" contenteditable="true" class="editable"`, 1)
	}
	if admin {
		raw := map[string]string{"intro_template": t.IntroTemplate, "location_template": t.LocationTemplate}
		rawJSON, _ := json.Marshal(raw)
		ctxJSON, _ := json.Marshal(ctx)
		result += fmt.Sprintf(`<script>
 const rawFields=%s, reportContext=%s;
 const form=document.querySelector('form[action="/admin/formato/guardar"]');
 form.elements.font_family.value=`+strconv.Quote(t.FontFamily)+`;const original={};document.querySelectorAll('[data-field]').forEach(e=>original[e.dataset.field]=e.innerText);
 form.addEventListener('submit',()=>{
 document.querySelectorAll('[data-field]').forEach(e=>{const k=e.dataset.field;let value=e.innerText;
 if(rawFields[k]!==undefined){if(value===original[k])value=rawFields[k];else {Object.entries(reportContext).sort((a,b)=>b[1].length-a[1].length).forEach(([key,text])=>{if(text)value=value.split(text).join('{{'+key+'}}')})}}
 const input=document.createElement('input');input.type='hidden';input.name=k;input.value=value;form.appendChild(input);
 });});</script>`, rawJSON, ctxJSON)
	}
	return result

}
func formatDateES(s string) string {
	t, e := ParseDate(s)
	if e != nil {
		return s
	}
	return t.Format("02/01/06")
}
func (a *App) reportAll(w http.ResponseWriter, r *http.Request) {
	var st Store
	a.Repo.Read(func(s Store) { st = s })
	sort.Slice(st.Dives, func(i, j int) bool { return st.Dives[i].Date > st.Dives[j].Date })
	a.page(w, r, "Todas las inmersiones", `<div class="actions no-print"><button onclick="window.print()">Imprimir</button></div><h2>Registro completo de inmersiones</h2>`+diveTable(st.Dives, true))
}
func (a *App) reportDiver(w http.ResponseWriter, r *http.Request) {
	id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/informe/buceador/"))
	var st Store
	a.Repo.Read(func(s Store) { st = s })
	d := st.DiverByID(id)
	if d == nil {
		http.NotFound(w, r)
		return
	}
	var ds []Dive
	for _, x := range st.Dives {
		if x.DiverID == id {
			ds = append(ds, x)
		}
	}
	photo := ""
	if r.URL.Query().Get("foto") != "0" && d.Photo != "" {
		photo = fmt.Sprintf(`<img class="photo" src="/photos/%s">`, url.PathEscape(d.Photo))
	}
	body := fmt.Sprintf(`<div class="no-print actions"><button onclick="window.print()">Imprimir</button> <a class="btn secondary" href="?foto=0">Sin foto</a> <a class="btn secondary" href="?foto=1">Con foto</a></div><h2>%s %s</h2>%s%s`, esc(d.Rank), esc(d.Name), photo, diveTable(ds, true))
	a.page(w, r, "Registro · "+d.Name, body)
}

func (a *App) adminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		if !a.csrfOK(r) {
			http.Error(w, "CSRF", 403)
			return
		}
		id := r.FormValue("id")
		status := r.FormValue("status")
		admin := r.FormValue("is_admin") == "on"
		err := a.Repo.Update(func(s *Store) error {
			if status != "pending" && status != "approved" && status != "rejected" {
				return fmt.Errorf("estado no válido")
			}
			u := s.UserByID(id)
			if u == nil {
				return fmt.Errorf("usuario no existe")
			}
			if strings.EqualFold(u.Username, "admin") {
				status = "approved"
				admin = true
			}
			u.Status = status
			u.IsAdmin = admin
			if status == "approved" && u.ApprovedAt == "" {
				u.ApprovedAt = nowUTC()
			}
			return nil
		})
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		http.Redirect(w, r, "/admin/usuarios", 302)
		return
	}
	var us []User
	a.Repo.Read(func(s Store) { us = append(us, s.Users...) })
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<h2>Administración</h2><div class="card"><form method="post" action="/admin/backup"><input type="hidden" name="csrf" value="%s"><button>Crear copia de seguridad</button></form><form method="post" enctype="multipart/form-data" action="/admin/restaurar"><input type="hidden" name="csrf" value="%s"><label>Restaurar backup ZIP</label><input type="file" name="backup" accept=".zip" required><button>Restaurar (cierra las sesiones)</button></form><form method="post" action="/admin/cerrar"><input type="hidden" name="csrf" value="%s"><button>Cerrar servidor</button></form></div><form method="post" enctype="multipart/form-data" action="/admin/importar"><input type="hidden" name="csrf" value="%s"><label>Importar CONTROL BUCEADORES.xlsx (sin sobrescribir personal existente)</label><input type="file" name="excel" accept=".xlsx" required><button>Importar Excel</button></form><h2>Usuarios</h2>`, a.csrf(r), a.csrf(r), a.csrf(r), a.csrf(r)))
	for _, u := range us {
		fmt.Fprintf(&b, `<div class="card"><form method="post"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="id" value="%s"><b>%s</b> (%s) · <select name="status"><option %s>pending</option><option %s>approved</option><option %s>rejected</option></select> <label class="inline"><input type="checkbox" name="is_admin" %s> Administrador</label> <button>Guardar</button></form></div>`, a.csrf(r), esc(u.ID), esc(u.DisplayName), esc(u.Username), sel("pending", u.Status), sel("approved", u.Status), sel("rejected", u.Status), chkAttr(u.IsAdmin))
	}
	a.page(w, r, "Usuarios", b.String())
}

func (a *App) saveReportTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Nombre requerido", 400)
		return
	}
	var id string
	err := a.Repo.Update(func(s *Store) error {
		base := findTemplate(*s, r.FormValue("source_format"))
		fields := map[string]*string{"title": &base.Title, "intro_template": &base.IntroTemplate, "col_name": &base.ColName, "col_date": &base.ColDate, "col_equipment": &base.ColEquipment, "col_gas": &base.ColGas, "col_code": &base.ColCode, "location_template": &base.LocationTemplate, "sign1_role": &base.Sign1Role, "sign1_name": &base.Sign1Name, "sign2_role": &base.Sign2Role, "sign2_name": &base.Sign2Name}
		for k, dst := range fields {
			if v, ok := r.PostForm[k]; ok && len(v) > 0 {
				if len(v[0]) > 10000 {
					return fmt.Errorf("texto demasiado largo")
				}
				*dst = v[0]
			}
		}
		if v := r.FormValue("font_family"); v != "" {
			if !allowedFont(v) {
				return fmt.Errorf("fuente no válida")
			}
			base.FontFamily = v
		}
		for k, dst := range map[string]*int{"body_font_size": &base.BodyFontSize, "table_font_size": &base.TableFontSize, "signature_top_space": &base.SignatureTopSpace} {
			if v := r.FormValue(k); v != "" {
				n, e := strconv.Atoi(v)
				max := 48
				if k == "signature_top_space" {
					max = 300
				}
				if e != nil || n < 1 || n > max {
					return fmt.Errorf("tamaño no válido")
				}
				*dst = n
			}
		}
		base.ID = "FMT" + randomID()
		id = base.ID
		base.Name = name
		base.CreatedAt = nowUTC()
		s.ReportTemplates = append(s.ReportTemplates, base)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/informe/mensual?format="+url.QueryEscape(id), 302)
}
func allowedFont(s string) bool {
	return s == "Arial" || s == "Times New Roman" || s == "Calibri" || s == "Helvetica"
}

func (a *App) autoBackup(reason string) error {
	_ = os.MkdirAll(a.BackupsDir, 0755)
	p := filepath.Join(a.BackupsDir, time.Now().UTC().Format("20060102_150405.000000000")+"_"+randomID()+"_auto.zip")
	return CreateBackup(p, a.Repo.Path, a.ConfigPath, a.PhotosDir, reason, a.Version)
}
func (a *App) backupNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	p := filepath.Join(a.BackupsDir, time.Now().UTC().Format("20060102_150405.000000000")+"_"+randomID()+"_auto.zip")
	if err := CreateBackup(p, a.Repo.Path, a.ConfigPath, a.PhotosDir, "manual", a.Version); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="control-buceadores-backup.zip"`)
	http.ServeFile(w, r, p)
}
func (a *App) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST", 405)
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	f, h, e := r.FormFile("backup")
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	defer f.Close()
	_ = h
	out, e := os.CreateTemp(a.DataDir, "restore-*.zip")
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	_, e = io.Copy(out, f)
	ce := out.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if _, e = ValidateBackup(tmp); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e = a.autoBackup("antes de restaurar"); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	if e = RestoreBackup(tmp, a.Repo.Path, a.ConfigPath, a.PhotosDir); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	nr, e := LoadOrCreate(a.Repo.Path)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	cfg, e := LoadConfig(a.ConfigPath)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	a.Repo = nr
	a.Config = cfg
	a.mu.Lock()
	a.Sessions = map[string]Session{}
	a.mu.Unlock()

	http.Redirect(w, r, "/inicio", 302)
}
func (a *App) shutdownWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	io.WriteString(w, "<html><body><h2>Servidor cerrado.</h2></body></html>")
	go func() {
		time.Sleep(200 * time.Millisecond)
		if a.Server != nil {
			_ = a.Server.Shutdown(context.Background())
		}
	}()
}
func (a *App) internalShutdown(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		http.Error(w, "Solo localhost", 403)
		return
	}
	if r.URL.Query().Get("token") != a.ShutdownToken {
		http.Error(w, "forbidden", 403)
		return
	}
	io.WriteString(w, "ok")
	go func() {
		time.Sleep(100 * time.Millisecond)
		if a.Server != nil {
			_ = a.Server.Shutdown(context.Background())
		}
	}()
}

func (a *App) page(w http.ResponseWriter, r *http.Request, title, body string) {
	u, _ := a.currentUser(r)
	admin := ""
	if u.IsAdmin {
		admin = `<a href="/admin/usuarios">Administración</a>`
	}
	nav := ""
	if u.ID != "" {
		nav = fmt.Sprintf(`<nav class="nav no-print"><a href="/inicio">Inicio</a><a href="/buceadores">Buceadores</a><a href="/inmersiones">Inmersiones</a><a href="/mantenimiento">Mantenimiento</a><a href="/informes">Informes</a><a href="/cuenta">Mi cuenta</a>%s<form method="post" action="/logout"><input type="hidden" name="csrf" value="%s"><button>Salir</button></form></nav>`, admin, a.csrf(r))
	}
	fmt.Fprintf(w, `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><style>%s</style></head><body><header><h1>%s</h1><div>%s</div></header>%s<main class="wrap">%s</main></body></html>`, esc(title), css, esc(a.Config.AppName), esc(a.Config.UnitName), nav, body)
}

const css = `:root{font-family:Arial,Helvetica,sans-serif;color:#17202a;background:#f4f6f8}*{box-sizing:border-box}body{margin:0}header{background:#0c3559;color:#fff;padding:16px 24px;display:flex;justify-content:space-between;gap:12px}header h1{margin:0;font-size:22px}.nav{background:#fff;border-bottom:1px solid #d7dde3;padding:0 20px;display:flex;gap:4px;flex-wrap:wrap}.nav a{color:#163b5c;text-decoration:none;padding:12px;font-weight:700}.wrap{max-width:1450px;margin:20px auto;padding:0 18px}.card{background:#fff;border:1px solid #dfe4e8;border-radius:8px;padding:16px;margin:14px 0}.narrow{max-width:520px;margin:40px auto}.actions{display:flex;gap:10px;align-items:center;justify-content:space-between;flex-wrap:wrap}.btn,button{display:inline-block;border:0;border-radius:5px;background:#0c5d92;color:#fff;padding:9px 13px;text-decoration:none;font-weight:700;cursor:pointer}.secondary{background:#e8eef3!important;color:#1e3a50!important}.danger{background:#9f2929!important}.grid{display:grid;gap:12px}.kpis{grid-template-columns:repeat(3,minmax(140px,1fr))}.kpi{background:#fff;border:1px solid #dfe4e8;border-radius:8px;padding:14px}.kpi span{display:block;color:#66717b}.kpi b{font-size:27px}.table-scroll{overflow:auto}table{border-collapse:collapse;width:100%;font-size:13px}th,td{border:1px solid #d8dde2;padding:8px;vertical-align:top}th{background:#edf2f5;text-align:left}label{display:block;font-weight:700;margin-top:10px}input,select,textarea{width:100%;padding:9px;border:1px solid #cbd3da;border-radius:4px;background:#fff}textarea{min-height:90px}.inline{display:inline}.inline input,label input[type=checkbox]{width:auto}.badge{display:inline-block;padding:4px 7px;border-radius:999px;font-size:11px;font-weight:800}.badge.ok{background:#dcefe3;color:#176535}.badge.warn{background:#fff0cf;color:#835800}.badge.neutral{background:#e8ecef;color:#59636b}.badge.danger{background:#f5dada;color:#8f2323}.status-cell{text-align:center}.photo{max-width:170px;max-height:210px;object-fit:cover;border-radius:6px}.signatures{display:grid;grid-template-columns:1fr 1fr;gap:60px;text-align:center}.locked{user-select:text}.editable{outline:1px dashed transparent}.editable:focus{outline-color:#0c5d92}@media print{.no-print,.nav{display:none!important}body{background:#fff}.wrap{max-width:none;margin:0;padding:0}.card{border:0}.maintenance-summary tr{page-break-inside:avoid}header{background:#fff;color:#000;padding:0 0 10px;border-bottom:1px solid #000}}`

func ListenAvailable(addr string, preferred int) (net.Listener, error) {
	if preferred <= 0 {
		preferred = 8765
	}
	if preferred > 65535 {
		return nil, fmt.Errorf("puerto fuera de rango")
	}
	for p := preferred; p < preferred+100 && p <= 65535; p++ {
		ln, e := net.Listen("tcp", net.JoinHostPort(addr, strconv.Itoa(p)))
		if e == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no hay puerto disponible")
}
func ChoosePort(addr string, preferred int) (int, error) {
	ln, e := ListenAvailable(addr, preferred)
	if e != nil {
		return 0, e
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (a *App) account(w http.ResponseWriter, r *http.Request) {
	u, _ := a.currentUser(r)
	if r.Method == "POST" {
		if !a.csrfOK(r) {
			http.Error(w, "CSRF", 403)
			return
		}
		if !VerifyPassword(u, r.FormValue("current_password")) {
			http.Error(w, "Contraseña actual incorrecta", 403)
			return
		}
		name := strings.TrimSpace(r.FormValue("display_name"))
		password := r.FormValue("password")
		if name == "" || password != "" && len(password) < 8 {
			http.Error(w, "Nombre o contraseña no válidos", 400)
			return
		}
		e := a.Repo.Update(func(s *Store) error {
			p := s.UserByID(u.ID)
			p.DisplayName = name
			if password != "" {
				p.PasswordSalt = NewSalt()
				p.PasswordHash = HashPassword(password, p.PasswordSalt, defaultIterations)
				p.HashIterations = defaultIterations
			}
			return nil
		})
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		http.Redirect(w, r, "/inicio", 302)
		return
	}
	a.page(w, r, "Mi cuenta", fmt.Sprintf(`<div class="card"><form method="post"><input type="hidden" name="csrf" value="%s"><label>Nombre visible</label><input name="display_name" value="%s" required><label>Contraseña actual</label><input type="password" name="current_password" required><label>Contraseña nueva (opcional)</label><input type="password" name="password" minlength="8"><button>Guardar</button></form></div>`, a.csrf(r), esc(u.DisplayName)))
}
