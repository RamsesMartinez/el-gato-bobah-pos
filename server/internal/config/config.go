package config

import "github.com/caarlos0/env/v11"

// Config holds all runtime knobs, env-only (12-factor / Compose-friendly).
type Config struct {
	Port        string `env:"PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL" envDefault:""`
	JWTSecret   string `env:"JWT_SECRET,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogDir      string `env:"LOG_DIR" envDefault:"logs"`
	CORSOrigin  string `env:"CORS_ORIGIN" envDefault:"*"`
	Env         string `env:"APP_ENV" envDefault:"development"`
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}
