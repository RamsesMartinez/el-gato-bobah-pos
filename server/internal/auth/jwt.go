package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

const AccessTokenTTL = 15 * time.Minute

var ErrInvalidToken = errors.New("token inválido")

type Claims struct {
	Name string      `json:"name"`
	Role domain.Role `json:"role"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	now    func() time.Time
}

func NewManager(secret string, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{secret: []byte(secret), now: now}
}

// Issue signs a short-lived access token for a user.
func (m *Manager) Issue(u domain.User) (string, error) {
	now := m.now()
	claims := Claims{
		Name: u.Name,
		Role: u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse verifies and returns claims.
func (m *Manager) Parse(token string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	return c, nil
}

// NewRefreshToken returns a random opaque token and its sha256 hash (stored in DB).
func NewRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	hash = HashToken(token)
	return token, hash, nil
}

// HashToken returns the sha256 hex of a refresh token (opaque tokens are looked up by hash).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
