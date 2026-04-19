package util

import "golang.org/x/crypto/bcrypt"

// HashPassword hashes local SMTP password with bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword compares bcrypt hash with provided plaintext password.
func VerifyPassword(hash string, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
