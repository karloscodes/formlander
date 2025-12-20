package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// GeneratePasswordHash creates a bcrypt hash of the password.
func GeneratePasswordHash(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// VerifyPassword checks if a password matches its hash.
func VerifyPassword(hashedPassword string, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
