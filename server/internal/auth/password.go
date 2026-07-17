package auth

import "golang.org/x/crypto/bcrypt"

// HashSecret hashes a password or PIN with bcrypt. PINs get the same treatment;
// the small keyspace is covered by app-level rate limiting on the login path.
func HashSecret(secret string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckSecret is constant-time via bcrypt; returns true on match.
func CheckSecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}
