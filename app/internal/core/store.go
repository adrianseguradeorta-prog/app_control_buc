package core

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Repository struct {
	mu   sync.RWMutex
	Path string
	Data Store
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DefaultConfig() Config {
	return Config{AppName: "Control de Buceadores", UnitName: "Unidad local", PreferredPort: 8765, BindAddress: "0.0.0.0", ReportLocation: "Cartagena", Sign1Role: "El AN., Oficial de Buceo", Sign1Name: "", Sign2Role: "V.B. TN., Segundo Comandante.", Sign2Name: ""}
}

func DefaultStore() Store {
	s := Store{SchemaVersion: SchemaVersion, NextDiveID: 1}
	s.Users = []User{NewUser("admin", "Administrador", "Zocotroco$01", true, true)}
	s.ReportTemplates = []ReportTemplate{{ID: "FMT001", Name: "Unidad local · Relación mensual", Title: "RELACIÓN MENSUAL DE INMERSIONES Y PRESURIZACIONES EN CH", IntroTemplate: "Durante el mes de {{MES}} del año {{AÑO}} el siguiente personal del {{UNIDAD}} ha efectuado las inmersiones y presurizaciones en CH:", ColName: "NOMBRE", ColDate: "FECHA", ColEquipment: "EQUIPO", ColGas: "MEZCLA", ColCode: "CODIGO", LocationTemplate: "En {{LOCALIDAD}}, a {{FECHA_FIN_MES}}.", Sign1Role: "El AN., Oficial de Buceo", Sign1Name: "", Sign2Role: "V.B. TN., Segundo Comandante.", Sign2Name: "", FontFamily: "Arial", BodyFontSize: 11, TableFontSize: 9, SignatureTopSpace: 52, CreatedAt: nowUTC()}}
	s.DefaultReportTemplateID = "FMT001"
	return s
}

func LoadOrCreate(path string) (*Repository, error) {
	r := &Repository{Path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		r.Data = DefaultStore()
		if err := r.Save(); err != nil {
			return nil, err
		}
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &r.Data); err != nil {
		return nil, err
	}
	if r.Data.SchemaVersion == 0 {
		r.Data.SchemaVersion = 3
	}
	if r.Data.SchemaVersion < 3 || r.Data.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("versión de esquema no compatible: %d", r.Data.SchemaVersion)
	}
	// El esquema 4 añade iteraciones explícitas; los hashes legacy se conservan.
	r.Data.SchemaVersion = SchemaVersion
	for _, d := range r.Data.Dives {
		if d.ID >= r.Data.NextDiveID {
			r.Data.NextDiveID = d.ID + 1
		}
	}
	if r.Data.NextDiveID < 1 {
		r.Data.NextDiveID = 1
	}
	if err := ValidateStore(r.Data); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Repository) Save() error { r.mu.Lock(); defer r.mu.Unlock(); return r.saveLocked() }
func (r *Repository) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.Path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r.Data, "", "  ")
	if err != nil {
		return err
	}
	b, err = preserveJSON(r.Path, b)
	if err != nil {
		return err
	}
	tmp := r.Path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = f.Sync()
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return os.Rename(tmp, r.Path)
}
func (r *Repository) Read(fn func(Store)) { r.mu.RLock(); defer r.mu.RUnlock(); fn(cloneStore(r.Data)) }
func (r *Repository) Update(fn func(*Store) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := cloneStore(r.Data)
	if err := fn(&next); err != nil {
		return err
	}
	if err := ValidateStore(next); err != nil {
		return err
	}
	previous := r.Data
	r.Data = next
	if err := r.saveLocked(); err != nil {
		r.Data = previous
		return err
	}
	return nil
}

func (s *Store) DiverByID(id string) *Diver {
	for i := range s.Divers {
		if s.Divers[i].ID == id {
			return &s.Divers[i]
		}
	}
	return nil
}
func (s *Store) UserByUsername(u string) *User {
	for i := range s.Users {
		if strings.EqualFold(s.Users[i].Username, u) {
			return &s.Users[i]
		}
	}
	return nil
}
func (s *Store) UserByID(id string) *User {
	for i := range s.Users {
		if s.Users[i].ID == id {
			return &s.Users[i]
		}
	}
	return nil
}
func (s *Store) DiveByID(id int) *Dive {
	for i := range s.Dives {
		if s.Dives[i].ID == id {
			return &s.Dives[i]
		}
	}
	return nil
}

func LoadConfig(path string) (Config, error) {
	c := DefaultConfig()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &c)
	return c, err
}
func SaveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	b, e := preserveJSON(path, b)
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}

func cloneStore(s Store) Store {
	s.Users = append([]User(nil), s.Users...)
	s.Divers = append([]Diver(nil), s.Divers...)
	s.Dives = append([]Dive(nil), s.Dives...)
	s.ReportTemplates = append([]ReportTemplate(nil), s.ReportTemplates...)
	return s
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func validateDiver(d Diver) error {
	if !safeID.MatchString(d.ID) || strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("ID o nombre de buceador no válido")
	}
	if d.EntryDate != "" {
		if _, err := ParseDate(d.EntryDate); err != nil {
			return fmt.Errorf("fecha de entrada no válida")
		}
	}
	if d.Photo != "" && (filepath.Base(d.Photo) != d.Photo || strings.ContainsAny(d.Photo, `/\\`)) {
		return fmt.Errorf("foto no válida")
	}
	return nil
}
func ValidateStore(s Store) error {
	if s.SchemaVersion < 3 || s.SchemaVersion > SchemaVersion {
		return fmt.Errorf("esquema no compatible: %d", s.SchemaVersion)
	}
	seen := map[string]bool{}
	for _, d := range s.Divers {
		if err := validateDiver(d); err != nil {
			return err
		}
		if seen[d.ID] {
			return fmt.Errorf("ID de buceador duplicado: %q", d.ID)
		}
		seen[d.ID] = true
	}
	ids := map[int]bool{}
	for _, d := range s.Dives {
		if d.ID <= 0 || ids[d.ID] {
			return fmt.Errorf("ID de inmersión inválido o duplicado: %d", d.ID)
		}
		ids[d.ID] = true
		if !seen[d.DiverID] {
			return fmt.Errorf("inmersión %d referencia buceador inexistente", d.ID)
		}
		if _, err := ParseDate(d.Date); err != nil {
			return fmt.Errorf("fecha de inmersión no válida")
		}
		if math.IsNaN(d.MaxDepth) || math.IsInf(d.MaxDepth, 0) || d.MaxDepth < 0 || d.BottomTime < 0 {
			return fmt.Errorf("profundidad o tiempo no válido")
		}
		if d.IBCode != "" && IBLabelFor(d.IBCode) == "" {
			return fmt.Errorf("código IB no admitido: %s", d.IBCode)
		}
		if code := IBCodeFor(d.DiveType); code != "" && code != d.IBCode {
			return fmt.Errorf("código IB incoherente")
		}
	}
	users := map[string]bool{}
	userIDs := map[string]bool{}
	for _, u := range s.Users {
		key := strings.ToLower(strings.TrimSpace(u.Username))
		if key == "" || u.ID == "" || users[key] || userIDs[u.ID] {
			return fmt.Errorf("usuario inválido o duplicado")
		}
		users[key] = true
		userIDs[u.ID] = true
		if u.Status != "pending" && u.Status != "approved" && u.Status != "rejected" {
			return fmt.Errorf("estado de usuario no válido")
		}
		if key == "admin" && (!u.IsAdmin || u.Status != "approved") {
			return fmt.Errorf("admin debe permanecer aprobado y administrador")
		}
	}
	return nil
}

// LoadOrCreateReadOnly nunca crea ni migra datos sobre disco.
func LoadOrCreateReadOnly(path string) (Store, error) {
	var s Store
	b, e := os.ReadFile(path)
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
