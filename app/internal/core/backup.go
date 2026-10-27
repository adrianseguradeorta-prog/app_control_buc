package core

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackupManifest struct {
	AppID         string            `json:"app_id"`
	AppVersion    string            `json:"app_version"`
	SchemaVersion int               `json:"schema_version"`
	CreatedAt     string            `json:"created_at"`
	Reason        string            `json:"reason"`
	Files         map[string]string `json:"files_sha256"`
}

const maxBackupBytes = 128 << 20

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func CreateBackup(dst, storePath, configPath, photosDir, reason, version string) error {
	files := map[string][]byte{}
	for name, p := range map[string]string{"data/store.json": storePath, "config.json": configPath} {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[name] = b
	}
	err := filepath.Walk(photosDir, func(p string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) && p == photosDir {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("no se admiten enlaces en fotos")
		}
		if info.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(photosDir, p)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		files["data/photos/"+filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		return err
	}
	var s Store
	if err = json.Unmarshal(files["data/store.json"], &s); err != nil {
		return err
	}
	if err = ValidateStore(s); err != nil {
		return err
	}
	mf := BackupManifest{AppID: "control-buceadores-bamis", AppVersion: version, SchemaVersion: s.SchemaVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339), Reason: reason, Files: map[string]string{}}
	keys := []string{}
	for n, b := range files {
		mf.Files[n] = digest(b)
		keys = append(keys, n)
	}
	sort.Strings(keys)
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(dst), ".backup-*.tmp")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	zw := zip.NewWriter(out)
	mb, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	files["manifest.json"] = mb
	keys = append([]string{"manifest.json"}, keys...)
	for _, n := range keys {
		w, e := zw.Create(n)
		if e != nil {
			out.Close()
			return e
		}
		if _, e = w.Write(files[n]); e != nil {
			out.Close()
			return e
		}
	}
	if err = zw.Close(); err != nil {
		out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
func readBackup(filename string) (BackupManifest, map[string][]byte, error) {
	var mf BackupManifest
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return mf, nil, err
	}
	defer zr.Close()
	files := map[string][]byte{}
	var total uint64
	if len(zr.File) > 10000 {
		return mf, nil, fmt.Errorf("demasiados archivos")
	}
	for _, f := range zr.File {
		n := f.Name
		if f.FileInfo().IsDir() {
			continue
		}
		if path.Clean(n) != n || strings.Contains(n, "\\") || strings.Contains(n, ":") || strings.HasPrefix(n, "/") || strings.HasPrefix(n, "../") || f.Mode()&os.ModeSymlink != 0 {
			return mf, nil, fmt.Errorf("ruta no segura: %q", n)
		}
		if n != "manifest.json" && n != "config.json" && n != "data/store.json" && !strings.HasPrefix(n, "data/photos/") {
			return mf, nil, fmt.Errorf("archivo no admitido: %q", n)
		}
		if _, ok := files[n]; ok {
			return mf, nil, fmt.Errorf("archivo duplicado: %q", n)
		}
		total += f.UncompressedSize64
		if total > maxBackupBytes {
			return mf, nil, fmt.Errorf("backup demasiado grande")
		}
		rc, e := f.Open()
		if e != nil {
			return mf, nil, e
		}
		b, e := io.ReadAll(io.LimitReader(rc, maxBackupBytes+1))
		rc.Close()
		if e != nil {
			return mf, nil, e
		}
		if len(b) > maxBackupBytes {
			return mf, nil, fmt.Errorf("archivo demasiado grande")
		}
		files[n] = b
	}
	if err = json.Unmarshal(files["manifest.json"], &mf); err != nil {
		return mf, nil, err
	}
	if mf.AppID != "control-buceadores-bamis" || mf.SchemaVersion < 3 || mf.SchemaVersion > SchemaVersion {
		return mf, nil, fmt.Errorf("aplicación o esquema no compatible")
	}
	for _, n := range []string{"data/store.json", "config.json"} {
		if _, ok := mf.Files[n]; !ok {
			return mf, nil, fmt.Errorf("falta %s en manifiesto", n)
		}
	}
	if len(files) != len(mf.Files)+1 {
		return mf, nil, fmt.Errorf("archivos sin verificar")
	}
	for n, want := range mf.Files {
		b, ok := files[n]
		if !ok || !strings.EqualFold(digest(b), want) {
			return mf, nil, fmt.Errorf("falta archivo o SHA-256 incorrecto: %s", n)
		}
	}
	var s Store
	if err = json.Unmarshal(files["data/store.json"], &s); err != nil {
		return mf, nil, err
	}
	if s.SchemaVersion != mf.SchemaVersion {
		return mf, nil, fmt.Errorf("esquema incoherente")
	}
	if err = ValidateStore(s); err != nil {
		return mf, nil, err
	}
	var cfg Config
	if err = json.Unmarshal(bytes.TrimPrefix(files["config.json"], []byte{0xef, 0xbb, 0xbf}), &cfg); err != nil {
		return mf, nil, err
	}
	return mf, files, nil
}
func ValidateBackup(filename string) (BackupManifest, error) {
	mf, _, err := readBackup(filename)
	return mf, err
}
func RestoreBackup(filename, storePath, configPath, photosDir string) error {
	_, files, err := readBackup(filename)
	if err != nil {
		return err
	}
	// Preparar todo en disco antes de sustituir el estado; rollback ante fallo.
	parent := filepath.Dir(storePath)
	if err = os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".restore-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(stage)
		}
	}()
	stagedPhotos := filepath.Join(stage, "photos")
	if err = os.MkdirAll(stagedPhotos, 0755); err != nil {
		return err
	}
	for n, b := range files {
		if strings.HasPrefix(n, "data/photos/") {
			p := filepath.Join(stagedPhotos, filepath.FromSlash(strings.TrimPrefix(n, "data/photos/")))
			if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				return err
			}
			if err = os.WriteFile(p, b, 0600); err != nil {
				return err
			}
		}
	}
	type replacement struct {
		dst, src, old      string
		existed, installed bool
	}
	items := []replacement{{dst: storePath, src: filepath.Join(stage, "store.json")}, {dst: configPath, src: filepath.Join(stage, "config.json")}, {dst: photosDir, src: stagedPhotos}}
	if err = os.WriteFile(items[0].src, files["data/store.json"], 0600); err != nil {
		return err
	}
	if err = os.WriteFile(items[1].src, files["config.json"], 0600); err != nil {
		return err
	}
	for i := range items {
		x := &items[i]
		x.old = filepath.Join(stage, fmt.Sprintf("old-%d", i))
		if _, e := os.Lstat(x.dst); e == nil {
			if err = os.Rename(x.dst, x.old); err != nil {
				break
			}
			x.existed = true
		} else if !os.IsNotExist(e) {
			err = e
			break
		}
		if err = os.MkdirAll(filepath.Dir(x.dst), 0755); err != nil {
			break
		}
		if err = os.Rename(x.src, x.dst); err != nil {
			break
		}
		x.installed = true
	}
	if err != nil {
		for i := len(items) - 1; i >= 0; i-- {
			x := items[i]
			if x.installed {
				os.RemoveAll(x.dst)
			}
			if x.existed {
				if e := os.Rename(x.old, x.dst); e != nil {
					cleanup = false
					return fmt.Errorf("restauración: %v; rollback: %v; originales conservados en %s", err, e, stage)
				}
			}
		}
		return err
	}
	return nil
}
