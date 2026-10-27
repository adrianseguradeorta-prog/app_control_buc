package core

import (
	"fmt"
	"path/filepath"
)

// ResetPassword es una recuperación administrativa explícita; no interpreta ni
// sustituye silenciosamente hashes legacy. El llamante debe detener el servidor.
func (a *App) ResetPassword(username, password string) error {
	if len(password) < 8 {
		return fmt.Errorf("la contraseña debe tener al menos 8 caracteres")
	}
	var found bool
	a.Repo.Read(func(s Store) { found = s.UserByUsername(username) != nil })
	if !found {
		return fmt.Errorf("usuario no existe")
	}
	if e := a.autoBackup("recuperación administrativa de contraseña"); e != nil {
		return e
	}
	return a.Repo.Update(func(s *Store) error {
		u := s.UserByUsername(username)
		salt := NewSalt()
		u.PasswordSalt = salt
		u.PasswordHash = HashPassword(password, salt, defaultIterations)
		u.HashIterations = defaultIterations
		return nil
	})
}
func (a *App) BackupFiles() ([]string, error) {
	return filepath.Glob(filepath.Join(a.BackupsDir, "*.zip"))
}
