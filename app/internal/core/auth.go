package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

const defaultIterations = 120000

func NewSalt() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := 32
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	for i := 1; i <= blocks; i++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for j := 1; j < iter; j++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func HashPassword(password, salt string, iterations int) string {
	if iterations <= 0 {
		iterations = defaultIterations
	}
	return hex.EncodeToString(pbkdf2SHA256([]byte(password), []byte(salt), iterations, 32))
}

func VerifyPassword(u User, password string) bool {
	if u.HashIterations <= 0 {
		return false
	} // los hashes legacy quedan para migración explícita
	got := HashPassword(password, u.PasswordSalt, u.HashIterations)
	return subtle.ConstantTimeCompare([]byte(got), []byte(u.PasswordHash)) == 1
}

func NewUser(username, displayName, password string, admin bool, approved bool) User {
	salt := NewSalt()
	status := "pending"
	approvedAt := ""
	if approved {
		status = "approved"
		approvedAt = nowUTC()
	}
	return User{ID: randomID(), Username: username, DisplayName: displayName, PasswordSalt: salt, PasswordHash: HashPassword(password, salt, defaultIterations), HashIterations: defaultIterations, Status: status, IsAdmin: admin, CreatedAt: nowUTC(), ApprovedAt: approvedAt}
}
