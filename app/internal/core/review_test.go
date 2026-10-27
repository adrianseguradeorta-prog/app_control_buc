package core

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testApp(t *testing.T) (*App, http.Handler, *http.Cookie, string) {
	t.Helper()
	a, e := NewApp(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	h := a.Handler()
	w := request(h, nil, "POST", "/login", url.Values{"username": {"admin"}, "password": {"Zocotroco$01"}})
	r := httptest.NewRequest("GET", "http://localhost/", nil)
	if w.Code != 302 {
		t.Fatal(w.Code, w.Body.String())
	}
	c := w.Result().Cookies()[0]
	r.AddCookie(c)
	return a, h, c, a.csrf(r)
}
func request(h http.Handler, c *http.Cookie, method, target string, v url.Values) *httptest.ResponseRecorder {
	var pc *http.Cookie
	if method == "POST" && (target == "/login" || target == "/registro") {
		w := request(h, nil, "GET", target, nil)
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == "cb_public_csrf" {
				pc = cookie
				v.Set("csrf", cookie.Value)
			}
		}
	}
	var body io.Reader
	if v != nil {
		body = strings.NewReader(v.Encode())
	}
	r := httptest.NewRequest(method, "http://localhost"+target, body)
	if v != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c != nil {
		r.AddCookie(c)
	}
	if pc != nil {
		r.AddCookie(pc)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func upload(h http.Handler, c *http.Cookie, target, csrf, field, name string, b []byte, fields url.Values) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("csrf", csrf)
	for k, vs := range fields {
		for _, v := range vs {
			mw.WriteField(k, v)
		}
	}
	f, _ := mw.CreateFormFile(field, name)
	f.Write(b)
	mw.Close()
	r := httptest.NewRequest("POST", "http://localhost"+target, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func status(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d != %d: %s", w.Code, want, w.Body.String())
	}
}
func diverValues(csrf string) url.Values {
	return url.Values{"csrf": {csrf}, "id": {"BTEST"}, "name": {"Prueba <script>alert(1)</script>"}, "active": {"on"}, "qual_basic": {"on"}, "qual_sds_air": {"on"}, "qual_sds_heliox": {"on"}}
}
func diveValues(csrf string) url.Values {
	return url.Values{"csrf": {csrf}, "diver_id": {"BTEST"}, "date": {"2026-01-15"}, "qualification": {"SDS aire"}, "dive_type": {"SDS Aire"}, "ib_code": {"65001"}, "gas": {"AIRE"}, "surface_supply": {"on"}, "max_depth": {"25,5"}, "bottom_time": {"20"}, "valid": {"on"}}
}
func TestReviewHTTPWorkflow(t *testing.T) {
	a, h, c, csrf := testApp(t)
	for _, p := range []string{"/inicio", "/buceadores", "/inmersiones", "/mantenimiento", "/informes", "/informe/mensual", "/informe/todas", "/admin/usuarios", "/buceador/nuevo", "/inmersion/nueva"} {
		t.Run(p, func(t *testing.T) {
			status(t, request(h, c, "GET", p, nil), 200)
			status(t, request(h, nil, "GET", p, nil), 302)
		})
	}
	status(t, request(h, c, "GET", "/no-existe", nil), 404)
	v := diverValues(csrf)
	v.Set("csrf", "incorrecto")
	status(t, request(h, c, "POST", "/buceador/nuevo", v), 403)
	v.Set("csrf", csrf)
	status(t, request(h, c, "POST", "/buceador/nuevo", v), 302)
	status(t, request(h, c, "POST", "/buceador/nuevo", v), 400)
	for _, p := range []string{"/buceador/BTEST", "/buceador/BTEST/editar", "/mantenimiento/BTEST", "/informe/buceador/BTEST", "/informe/buceador/BTEST?foto=0", "/informe/buceador/BTEST?foto=1"} {
		w := request(h, c, "GET", p, nil)
		status(t, w, 200)
		if strings.Contains(w.Body.String(), "Prueba <script>") {
			t.Fatal("HTML sin escapar")
		}
	}
	v.Set("name", "Nombre editado")
	status(t, upload(h, c, "/buceador/BTEST/editar", csrf, "photo", "image.png", mustPNG(), v), 302)
	a.Repo.Read(func(s Store) {
		if s.DiverByID("BTEST").Name != "Nombre editado" {
			t.Fatal("edición perdida")
		}
	})
	d := diveValues(csrf)
	for _, o := range IBOptions {
		d.Set("dive_type", o.Label)
		status(t, request(h, c, "POST", "/inmersion/nueva", d), 302)
		a.Repo.Read(func(s Store) {
			if s.Dives[len(s.Dives)-1].IBCode != o.Code {
				t.Fatal("código IB del cliente aceptado")
			}
		})
	}
	d.Set("dive_type", "SDS Aire")
	d.Set("gas", "HELIOX")
	status(t, request(h, c, "POST", "/inmersion/1/editar", d), 302)
	d.Set("diver_id", "inexistente")
	status(t, request(h, c, "POST", "/inmersion/1/editar", d), 500)
	d.Set("diver_id", "BTEST")
	for _, pair := range [][2]string{{"max_depth", "NaN"}, {"max_depth", "-1"}, {"max_depth", "no"}, {"bottom_time", "-1"}, {"bottom_time", "abc"}, {"date", "2099-01-01"}, {"date", "2026-02-30"}, {"dive_type", "65001"}, {"qualification", ""}} {
		bad := diveValues(csrf)
		bad.Set(pair[0], pair[1])
		status(t, request(h, c, "POST", "/inmersion/nueva", bad), 400)
	}
	status(t, request(h, c, "GET", "/informe/mensual?year=2026&month=1", nil), 200)
	w := request(h, c, "GET", "/informe/mensual?year=2026&month=1", nil)
	if !strings.Contains(w.Body.String(), `<tbody class="locked">`) {
		t.Fatal("tabla no bloqueada")
	}
	status(t, request(h, c, "POST", "/admin/formato/guardar", url.Values{"csrf": {csrf}, "name": {"Formato prueba"}, "title": {"Título editado"}, "intro_template": {"Durante {{MES}} de {{AÑO}}"}, "font_family": {"Calibri"}, "body_font_size": {"14"}}), 302)
	a.Repo.Read(func(s Store) {
		f := s.ReportTemplates[len(s.ReportTemplates)-1]
		if f.Title != "Título editado" || f.IntroTemplate != "Durante {{MES}} de {{AÑO}}" || f.BodyFontSize != 14 {
			t.Fatal("formato no persistido")
		}
	})
	status(t, request(h, c, "POST", "/admin/backup", url.Values{"csrf": {csrf}}), 200)
	backups, _ := filepath.Glob(filepath.Join(a.BackupsDir, "*.zip"))
	if len(backups) != 1 {
		t.Fatal("backup manual no creado")
	}
	if _, e := ValidateBackup(backups[0]); e != nil {
		t.Fatal(e)
	}
	status(t, request(h, c, "POST", "/inmersion/1/eliminar", url.Values{"csrf": {csrf}}), 302)
	status(t, request(h, c, "POST", "/buceador/BTEST/eliminar", url.Values{"csrf": {csrf}}), 302)
	backups, _ = filepath.Glob(filepath.Join(a.BackupsDir, "*.zip"))
	if len(backups) != 3 {
		t.Fatal("backups sobrescritos o no creados")
	}
	a.Repo.Read(func(s Store) {
		if len(s.Dives) != 0 || len(s.Divers) != 0 {
			t.Fatal("eliminación inconsistente")
		}
	})
	var restore []byte
	for _, p := range backups {
		b, _ := os.ReadFile(p)
		zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		for _, f := range zr.File {
			if f.Name == "data/store.json" {
				r, _ := f.Open()
				var st Store
				json.NewDecoder(r).Decode(&st)
				r.Close()
				if len(st.Dives) == 11 {
					restore = b
				}
			}
		}
	}
	if restore == nil {
		t.Fatal("no se conserva copia previa")
	}
	status(t, upload(h, c, "/admin/restaurar", csrf, "backup", "backup.zip", restore, nil), 302)
	status(t, request(h, c, "GET", "/inicio", nil), 302)
	a.Repo.Read(func(s Store) {
		if len(s.Dives) != 11 {
			t.Fatal("restore no cargado")
		}
	})
}
func TestReviewUserApproval(t *testing.T) {
	a, h, c, csrf := testApp(t)
	v := url.Values{"username": {"usuario"}, "password": {"contraseña123"}, "display_name": {"Usuario"}}
	status(t, request(h, nil, "POST", "/registro", v), 200)
	status(t, request(h, nil, "POST", "/registro", v), 400)
	status(t, request(h, nil, "POST", "/login", v), 403)
	var id, adminID string
	a.Repo.Read(func(s Store) { id = s.UserByUsername("usuario").ID; adminID = s.UserByUsername("admin").ID })
	status(t, request(h, c, "POST", "/admin/usuarios", url.Values{"csrf": {csrf}, "id": {id}, "status": {"approved"}}), 302)
	w := request(h, nil, "POST", "/login", v)
	status(t, w, 302)
	uc := w.Result().Cookies()[0]
	status(t, request(h, uc, "GET", "/admin/usuarios", nil), 403)
	status(t, request(h, uc, "POST", "/admin/backup", url.Values{"csrf": {csrf}}), 403)
	status(t, request(h, c, "POST", "/admin/usuarios", url.Values{"csrf": {csrf}, "id": {adminID}, "status": {"rejected"}}), 302)
	a.Repo.Read(func(s Store) {
		u := s.UserByUsername("admin")
		if !u.IsAdmin || u.Status != "approved" {
			t.Fatal("admin deshabilitado")
		}
	})
	status(t, request(h, c, "POST", "/admin/usuarios", url.Values{"csrf": {csrf}, "id": {id}, "status": {"invalid"}}), 400)
	status(t, request(h, c, "POST", "/admin/usuarios", url.Values{"csrf": {csrf}, "id": {id}, "status": {"rejected"}}), 302)
	status(t, request(h, uc, "GET", "/inicio", nil), 403)
}
func TestReviewBackupRejectsMaliciousArchives(t *testing.T) {
	for _, tc := range []string{"traversal", "unhashed", "missing", "future", "duplicate", "badjson", "checksum", "orphan"} {
		t.Run(tc, func(t *testing.T) {
			a, _, _, _ := testApp(t)
			original, _ := os.ReadFile(a.Repo.Path)
			mf := BackupManifest{AppID: "control-buceadores-bamis", SchemaVersion: SchemaVersion, Files: map[string]string{}}
			cfg, _ := os.ReadFile(a.ConfigPath)
			files := map[string][]byte{"data/store.json": original, "config.json": cfg}
			switch tc {
			case "traversal":
				files["data/photos/../../outside.txt"] = []byte("attack")
			case "unhashed":
				files["data/photos/unhashed.png"] = []byte("attack")
			case "missing":
				delete(files, "data/store.json")
			case "future":
				mf.SchemaVersion = 999
			case "badjson":
				files["data/store.json"] = []byte("{")
			case "orphan":
				var st Store
				json.Unmarshal(original, &st)
				st.Dives = []Dive{{ID: 1, DiverID: "missing", Date: "2026-01-01"}}
				files["data/store.json"], _ = json.Marshal(st)
			}
			for n, b := range files {
				mf.Files[n] = digest(b)
			}
			if tc == "unhashed" {
				delete(mf.Files, "data/photos/unhashed.png")
			}
			if tc == "checksum" {
				mf.Files["config.json"] = "bad"
			}
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			mb, _ := json.Marshal(mf)
			w, _ := zw.Create("manifest.json")
			w.Write(mb)
			for n, b := range files {
				w, _ = zw.Create(n)
				w.Write(b)
			}
			if tc == "duplicate" {
				w, _ = zw.Create("config.json")
				w.Write(cfg)
			}
			zw.Close()
			p := filepath.Join(t.TempDir(), "bad.zip")
			os.WriteFile(p, buf.Bytes(), 0600)
			if _, err := ValidateBackup(p); err == nil {
				t.Fatal("backup peligroso aceptado")
			}
			if err := RestoreBackup(p, a.Repo.Path, a.ConfigPath, a.PhotosDir); err == nil {
				t.Fatal("restaurado")
			}
			after, _ := os.ReadFile(a.Repo.Path)
			if !bytes.Equal(original, after) {
				t.Fatal("datos originales alterados")
			}
		})
	}
}
func TestReviewMaintenanceBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 27, 16, 0, 0, 0, time.UTC)
	diver := Diver{ID: "B", QualBasic: true, QualSDSAir: true, QualSDSHeliox: true}
	base := Dive{ID: 1, DiverID: "B", Date: "2025-10-27", Qualification: "Autónomo", Gas: "AIRE", MaxDepth: 26, BottomTime: 20, Valid: true}
	for _, tc := range []struct {
		name   string
		change func(*Dive)
		count  int
	}{{"inclusive", func(*Dive) {}, 1}, {"expired", func(d *Dive) { d.Date = "2025-10-26" }, 0}, {"tomorrow", func(d *Dive) { d.Date = "2026-10-28" }, 0}, {"invalid", func(d *Dive) { d.Date = "not-date" }, 0}, {"notvalid", func(d *Dive) { d.Valid = false }, 0}, {"noqualification", func(d *Dive) { d.Qualification = ""; d.IBCode = "61001" }, 0}, {"nodepth", func(d *Dive) { d.MaxDepth = 0 }, 0}, {"notime", func(d *Dive) { d.BottomTime = 0 }, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.change(&d)
			q := MaintenanceFor(diver, []Dive{d}, now)[0]
			if q.Count12m != tc.count {
				t.Fatalf("count=%d", q.Count12m)
			}
		})
	}
	air := base
	air.Date = "2026-10-01"
	air.Qualification = "SDS aire"
	air.SurfaceSupply = true
	bad := air
	bad.SurfaceSupply = false
	if q := MaintenanceFor(diver, []Dive{air, bad}, now)[1]; q.Status == "OK" || q.Count12m != 1 {
		t.Fatalf("SDS incorrecto: %+v", q)
	}
	if q := MaintenanceFor(Diver{ID: "B"}, []Dive{base}, now)[0]; q.Status != "NO ASIGNADA" {
		t.Fatal(q)
	}
	if q := MaintenanceFor(diver, nil, now)[0]; q.Status != "SIN DATOS" {
		t.Fatal(q)
	}
	old := base
	old.Date = "2020-01-01"
	if q := MaintenanceFor(diver, []Dive{old}, now)[0]; q.Status != "FUERA DE PLAZO" {
		t.Fatal(q)
	}
}
func TestReviewRepositoryRollbackAndReadIsolation(t *testing.T) {
	a, _, _, _ := testApp(t)
	a.Repo.Read(func(s Store) { s.Users[0].Username = "tampered" })
	a.Repo.Read(func(s Store) {
		if s.Users[0].Username != "admin" {
			t.Fatal("Read expone estado mutable")
		}
	})
	e := a.Repo.Update(func(s *Store) error { s.Users[0].DisplayName = "changed"; return fmt.Errorf("abort") })
	if e == nil {
		t.Fatal("error perdido")
	}
	a.Repo.Read(func(s Store) {
		if s.Users[0].DisplayName == "changed" {
			t.Fatal("sin rollback")
		}
	})
	a.Repo.Path = filepath.Join(a.BaseDir, "block", "store.json")
	os.WriteFile(filepath.Join(a.BaseDir, "block"), []byte("block"), 0600)
	if e = a.Repo.Update(func(s *Store) error { s.Users[0].DisplayName = "changed"; return nil }); e == nil {
		t.Fatal("escritura debía fallar")
	}
	a.Repo.Read(func(s Store) {
		if s.Users[0].DisplayName == "changed" {
			t.Fatal("cambio pese a fallo de disco")
		}
	})
}
func TestReviewExcel(t *testing.T) {
	b, e := os.ReadFile("../../..//reference/source_materials/CONTROL BUCEADORES.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	ds, dives, e := ParseExcel(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(ds) != 11 || len(dives) != 15 {
		t.Fatal(len(ds), len(dives))
	}
	a, h, c, csrf := testApp(t)
	status(t, upload(h, c, "/admin/importar", csrf, "excel", "control.xlsx", b, nil), 302)
	status(t, upload(h, c, "/admin/importar", csrf, "excel", "control.xlsx", b, nil), 302)
	a.Repo.Read(func(s Store) {
		if len(s.Divers) != 11 || len(s.Dives) != 15 {
			t.Fatal("duplicados", len(s.Divers), len(s.Dives))
		}
	})
	status(t, upload(h, c, "/admin/importar", csrf, "excel", "broken.xlsx", []byte("not a zip"), nil), 400)
}
func TestReviewConcurrentReadsAndWrites(t *testing.T) {
	_, h, c, csrf := testApp(t)
	status(t, request(h, c, "POST", "/buceador/nuevo", diverValues(csrf)), 302)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := "/mantenimiento"
			if i%2 == 0 {
				path = "/informe/todas"
			}
			w := request(h, c, "GET", path, nil)
			if w.Code != 200 {
				t.Errorf("%d", w.Code)
			}
		}(i)
	}
	wg.Wait()
}
func TestReviewPortOccupiedAndShutdown(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	p := ln.Addr().(*net.TCPAddr).Port
	got, e := ChoosePort("127.0.0.1", p)
	if e != nil || got == p {
		t.Fatal(got, e)
	}
	a, h, _, _ := testApp(t)
	r := httptest.NewRequest("GET", "http://localhost/internal/shutdown?token="+a.ShutdownToken, nil)
	r.RemoteAddr = "203.0.113.10:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	status(t, w, 403)
	r = httptest.NewRequest("GET", "http://localhost/internal/shutdown?token=wrong", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	status(t, w, 403)
}

func mustPNG() []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	return b.Bytes()
}

func TestReviewDestructiveOperationsStopOnBackupFailure(t *testing.T) {
	a, h, c, csrf := testApp(t)
	status(t, request(h, c, "POST", "/buceador/nuevo", diverValues(csrf)), 302)
	status(t, request(h, c, "POST", "/inmersion/nueva", diveValues(csrf)), 302)
	block := filepath.Join(a.BaseDir, "blocked")
	os.WriteFile(block, []byte("not directory"), 0600)
	a.BackupsDir = block
	status(t, request(h, c, "POST", "/inmersion/1/eliminar", url.Values{"csrf": {csrf}}), 500)
	status(t, request(h, c, "POST", "/buceador/BTEST/eliminar", url.Values{"csrf": {csrf}}), 500)
	a.Repo.Read(func(s Store) {
		if len(s.Divers) != 1 || len(s.Dives) != 1 {
			t.Fatal("se eliminó sin backup")
		}
	})
}
func TestReviewRejectsBadPhotosAndCrossOrigin(t *testing.T) {
	_, h, c, csrf := testApp(t)
	status(t, upload(h, c, "/buceador/nuevo", csrf, "photo", "bad.png", []byte("not image"), diverValues(csrf)), 400)
	status(t, upload(h, c, "/buceador/nuevo", csrf, "photo", "bad.html", []byte("<script>"), diverValues(csrf)), 400)
	status(t, request(h, c, "GET", "/photos/unknown.png", nil), 404)
	status(t, request(h, nil, "GET", "/photos/unknown.png", nil), 302)
	r := httptest.NewRequest("POST", "http://localhost/buceador/nuevo", strings.NewReader(diverValues(csrf).Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	status(t, w, 403)
	r = httptest.NewRequest("GET", "http://evil.example/inicio", nil)
	r.AddCookie(c)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	status(t, w, 403)
	for _, p := range []string{"/buceador/nuevo", "/buceador/X/editar", "/inmersion/nueva", "/admin/usuarios", "/admin/importar", "/admin/backup", "/admin/restaurar", "/admin/cerrar", "/admin/formato/guardar", "/logout"} {
		status(t, request(h, c, "DELETE", p, nil), 405)
	}
}
func TestReviewLegacyPasswordRecovery(t *testing.T) {
	a, _, _, _ := testApp(t)
	u := a.Repo.Data.Users[0]
	u.HashIterations = 0
	u.PasswordHash = "legacy-unknown"
	a.Repo.Update(func(s *Store) error { s.SchemaVersion = 3; s.Users[0] = u; return nil })
	loaded, err := LoadOrCreate(a.Repo.Path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Data.SchemaVersion != SchemaVersion {
		t.Fatal("sin migración de esquema")
	}
	if VerifyPassword(loaded.Data.Users[0], "Zocotroco$01") {
		t.Fatal("legacy aceptado sin migración")
	}
	if err = a.ResetPassword("admin", "new-password-test"); err != nil {
		t.Fatal(err)
	}
	a.Repo.Read(func(s Store) {
		if !VerifyPassword(s.Users[0], "new-password-test") {
			t.Fatal("recuperación fallida")
		}
	})
	files, _ := a.BackupFiles()
	if len(files) != 1 {
		t.Fatal("sin copia antes de cambiar contraseña")
	}
	if err = a.ResetPassword("missing", "new-password-test"); err == nil {
		t.Fatal("usuario inexistente")
	}
	if err = a.ResetPassword("admin", "short"); err == nil {
		t.Fatal("contraseña corta")
	}
}
func TestReviewBackupPhotosRoundtrip(t *testing.T) {
	a, _, _, _ := testApp(t)
	os.MkdirAll(a.PhotosDir, 0755)
	os.WriteFile(filepath.Join(a.PhotosDir, "saved.png"), mustPNG(), 0600)
	backup := filepath.Join(t.TempDir(), "backup.zip")
	if e := CreateBackup(backup, a.Repo.Path, a.ConfigPath, a.PhotosDir, "test", a.Version); e != nil {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(a.PhotosDir, "saved.png"))
	os.WriteFile(filepath.Join(a.PhotosDir, "extra.png"), []byte("obsolete"), 0600)
	if e := RestoreBackup(backup, a.Repo.Path, a.ConfigPath, a.PhotosDir); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(filepath.Join(a.PhotosDir, "saved.png")); e != nil || !bytes.Equal(b, mustPNG()) {
		t.Fatal("foto no restaurada")
	}
	if _, e := os.Stat(filepath.Join(a.PhotosDir, "extra.png")); !os.IsNotExist(e) {
		t.Fatal("foto ajena conservada")
	}
}
func TestReviewBOMConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	b, _ := json.Marshal(DefaultConfig())
	os.WriteFile(p, append([]byte{0xef, 0xbb, 0xbf}, b...), 0600)
	if _, e := LoadConfig(p); e != nil {
		t.Fatal(e)
	}
}

func TestReviewExcelMatchesReferenceFixture(t *testing.T) {
	b, e := os.ReadFile("../../../reference/source_materials/CONTROL BUCEADORES.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	ds, dives, e := ParseExcel(b)
	if e != nil {
		t.Fatal(e)
	}
	fixture, e := os.ReadFile("../../testdata/store_from_excel.json")
	if e != nil {
		t.Fatal(e)
	}
	var st Store
	if e = json.Unmarshal(fixture, &st); e != nil {
		t.Fatal(e)
	}
	if len(ds) != len(st.Divers) || len(dives) != len(st.Dives) {
		t.Fatal("conteos distintos")
	}
	for _, d := range ds {
		ref := st.DiverByID(d.ID)
		if ref == nil || *ref != d {
			t.Errorf("datos de buceador difieren: %s", d.ID)
		}
	}
	for i, d := range dives {
		found := false
		for _, r := range st.Dives {
			if sameDive(d, r) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("fila Excel %d difiere de fixture", i+5)
		}
	}
}
func TestReviewActualLegacyStoreLoadsWithoutResettingHashes(t *testing.T) {
	b, e := os.ReadFile("../../../reference/v7_binary/store_v7.json")
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "store.json")
	os.WriteFile(p, b, 0600)
	repo, e := LoadOrCreate(p)
	if e != nil {
		t.Fatal(e)
	}
	var original Store
	json.Unmarshal(b, &original)
	if repo.Data.Users[0].PasswordHash != original.Users[0].PasswordHash {
		t.Fatal("hash legacy alterado")
	}
	if len(repo.Data.Divers) != len(original.Divers) {
		t.Fatal("datos perdidos")
	}
}
func TestReviewAllEditableReportFieldsAreDistinct(t *testing.T) {
	st := DefaultStore()
	out := renderMonthly(st, nil, st.ReportTemplates[0], map[string]string{"MES": "ENERO", "AÑO": "2026"}, "csrf", true)
	for _, f := range []string{"title", "intro_template", "col_name", "col_date", "col_equipment", "col_gas", "col_code", "location_template", "sign1_role", "sign1_name", "sign2_role", "sign2_name"} {
		if n := strings.Count(out, `data-field="`+f+`"`); n != 1 {
			t.Fatalf("campo %s aparece %d veces", f, n)
		}
	}
}

func TestAccountNameAndPasswordChange(t *testing.T) {
	a, h, c, csrf := testApp(t)
	status(t, request(h, c, "GET", "/cuenta", nil), 200)
	v := url.Values{"csrf": {csrf}, "current_password": {"wrong"}, "display_name": {"New display"}, "password": {"new-test-password"}}
	status(t, request(h, c, "POST", "/cuenta", v), 403)
	v.Set("current_password", "Zocotroco$01")
	status(t, request(h, c, "POST", "/cuenta", v), 302)
	a.Repo.Read(func(s Store) {
		u := s.UserByUsername("admin")
		if u.DisplayName != "New display" || !VerifyPassword(*u, "new-test-password") {
			t.Fatal("account change failed")
		}
	})
}

func TestPublishedFixturesContainOnlySyntheticPeople(t *testing.T) {
	b, e := os.ReadFile("../../testdata/store_from_excel.json")
	if e != nil {
		t.Fatal(e)
	}
	var s Store
	if e = json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	for _, d := range s.Divers {
		if !strings.HasPrefix(d.Name, "Persona ficticia ") || d.Rank != "PRUEBA" {
			t.Fatal("non-synthetic fixture")
		}
	}
	for _, d := range s.Dives {
		if d.Notes != "Ejercicio ficticio" || d.Purpose != "PRUEBA" {
			t.Fatal("non-synthetic operation")
		}
	}
}

func TestWildcardUsesIPv4AndAvoidsOccupiedIPv4Port(t *testing.T) {
	occupied, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	listener, e := ListenAvailable("0.0.0.0", port)
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	if addr.IP.To4() == nil || addr.Port == port {
		t.Fatalf("IPv4 listener was not reserved correctly: %s", addr)
	}
}
