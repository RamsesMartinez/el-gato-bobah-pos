package config

import (
	"errors"
	"strings"

	"github.com/caarlos0/env/v11"
)

// minSecretLen is the floor for JWT_SECRET. With HS256 and a PUBLIC source tree, a
// short/guessable key means an attacker can forge admin tokens offline.
const minSecretLen = 32

// Config holds all runtime knobs, env-only (12-factor / Compose-friendly).
type Config struct {
	Port        string `env:"PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL" envDefault:""`
	JWTSecret   string `env:"JWT_SECRET,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogDir      string `env:"LOG_DIR" envDefault:"logs"`
	// CORSOrigin: exact allowed origin (scheme+host), e.g. https://pos.elgatobobah.mx.
	// Empty = same-origin only (no CORS headers). "*" is only honored in development.
	CORSOrigin string `env:"CORS_ORIGIN" envDefault:""`
	Env        string `env:"APP_ENV" envDefault:"development"`
}

func Load() (Config, error) {
	c, err := env.ParseAs[Config]()
	if err != nil {
		return c, err
	}
	if err := Validate(c); err != nil {
		return c, err
	}
	return c, nil
}

// Validate rejects configurations that are unsafe to run, so misconfiguration
// fails fast at startup instead of silently shipping a weak secret to prod.
func Validate(c Config) error {
	if IsPlaceholder(c.JWTSecret) || len(c.JWTSecret) < minSecretLen {
		return errors.New("JWT_SECRET débil o de ejemplo: usa 32+ caracteres aleatorios (openssl rand -base64 48)")
	}
	if c.Env == "production" && c.CORSOrigin == "*" {
		return errors.New("CORS_ORIGIN=* no está permitido en producción: define el origen exacto (https://tu-dominio)")
	}
	return nil
}

// IsPlaceholder reports whether s is empty or one of the shipped example values,
// so we can refuse to start when secrets were copy-pasted from .env.example.
func IsPlaceholder(s string) bool {
	if s == "" {
		return true
	}
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "cambia-esto") || strings.HasPrefix(l, "your_")
}
