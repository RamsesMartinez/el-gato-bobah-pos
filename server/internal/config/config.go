package config

import (
	"errors"
	"strings"

	"github.com/caarlos0/env/v11"
)

// minSecretLen is the floor for JWT_SECRET. With HS256 and a PUBLIC source tree, a
// short/guessable key means an attacker can forge admin tokens offline.
const minSecretLen = 32

// DefaultAnthropicModel es el modelo de extracción cuando ANTHROPIC_MODEL no está definido.
// Debe coincidir con el envDefault del campo AnthropicModel (un tag de struct no acepta
// constantes); las herramientas que no cargan Config completa lo usan desde aquí.
const DefaultAnthropicModel = "claude-opus-5"

// Config holds all runtime knobs, env-only (12-factor / Compose-friendly).
type Config struct {
	Port        string `env:"PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	// AppDatabaseURL: conexión de SERVICIO del API, como el rol no-superusuario gatobobah_app,
	// para que RLS aplique (un superusuario la saltaría). DATABASE_URL queda para migrar/bootstrap
	// (owner, salta RLS). Vacío = usa DATABASE_URL (modo sin aislamiento; Validate lo prohíbe en prod).
	AppDatabaseURL string `env:"APP_DATABASE_URL" envDefault:""`
	// AppDBPassword: password que el bootstrap le fija al rol gatobobah_app (creado sin password
	// en la migración para no versionar secretos).
	AppDBPassword string `env:"APP_DB_PASSWORD" envDefault:""`
	RedisURL      string `env:"REDIS_URL" envDefault:""`
	JWTSecret     string `env:"JWT_SECRET,required"`
	// PinPepper: secreto que vuelve inútil la huella determinista del PIN para quien se lleve la
	// base. OPCIONAL a propósito — sin él el sistema funciona igual, solo que el modo de solo-PIN no
	// se puede encender (fail-closed). Hacerlo obligatorio rompería todos los despliegues actuales
	// por una funcionalidad que nadie ha pedido todavía.
	PinPepper string `env:"PIN_PEPPER"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogDir    string `env:"LOG_DIR" envDefault:"logs"`
	// CORSOrigin: exact allowed origin (scheme+host), e.g. https://app.elgatobobah.com.
	// Empty = same-origin only (no CORS headers). "*" is only honored in development.
	CORSOrigin string `env:"CORS_ORIGIN" envDefault:""`
	Env        string `env:"APP_ENV" envDefault:"development"`

	// --- Email (recuperación de contraseña). Local: Mailpit. Prod: Zoho Mail SMTP. ---
	SMTPHost string `env:"SMTP_HOST" envDefault:""` // vacío = email deshabilitado (recuperación no disponible)
	SMTPPort int    `env:"SMTP_PORT" envDefault:"1025"`
	SMTPUser string `env:"SMTP_USER" envDefault:""`
	SMTPPass string `env:"SMTP_PASS" envDefault:""`
	MailFrom string `env:"MAIL_FROM" envDefault:"no-reply@elgatobobah.com"`
	// AppBaseURL: origen público del frontend, para armar el link de reset en el email.
	AppBaseURL string `env:"APP_BASE_URL" envDefault:"http://localhost:3000"`

	// HIBPEnabled: verifica la contraseña contra Have I Been Pwned (k-anonymity) al fijarla.
	// Fail-open: si HIBP no responde, se permite (con evento de seguridad) para no bloquear el
	// alta de usuarios cuando el POS está sin internet.
	HIBPEnabled bool `env:"HIBP_ENABLED" envDefault:"true"`

	// --- Extracción de tickets/facturas de compra (Anthropic API) ---
	// Vacío = feature apagada: las líneas del gasto se capturan a mano. Es opcional a propósito
	// para que el POS no dependa de un servicio externo (ni de internet) para operar.
	AnthropicAPIKey string `env:"ANTHROPIC_API_KEY" envDefault:""`
	// AnthropicModel es configurable porque el costo/calidad de la extracción se ajusta sin
	// recompilar. No se manda thinking ni effort en la llamada justamente para que cualquier
	// modelo del catálogo sea válido aquí (Haiku rechaza esos parámetros).
	AnthropicModel string `env:"ANTHROPIC_MODEL" envDefault:"claude-opus-5"`
}

// DocExtractEnabled reports whether purchase-document extraction is configured.
func (c Config) DocExtractEnabled() bool { return c.AnthropicAPIKey != "" }

// AppDatabaseURLOrDefault devuelve la conexión de servicio (rol app) o, si no se configuró,
// DATABASE_URL. En producción Validate exige APP_DATABASE_URL para que RLS no quede desactivado.
func (c Config) AppDatabaseURLOrDefault() string {
	if c.AppDatabaseURL != "" {
		return c.AppDatabaseURL
	}
	return c.DatabaseURL
}

// EmailEnabled reports whether SMTP is configured (host set).
func (c Config) EmailEnabled() bool { return c.SMTPHost != "" }

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
	// Multi-tenant fail-fast: en producción el API DEBE servir como el rol no-superusuario
	// (APP_DATABASE_URL) para que RLS aísle las empresas. Sin él caería al owner (que salta RLS)
	// y anularía el aislamiento en silencio. main.go además lo verifica en runtime (assertRLSEnforced).
	if c.Env == "production" && c.AppDatabaseURL == "" {
		return errors.New("APP_DATABASE_URL requerido en producción: el API debe conectarse como el rol de app (no-superusuario) para que RLS aísle los tenants")
	}
	// Una llave de Anthropic copiada del ejemplo, o con el prefijo equivocado, falla en la
	// primera extracción y con un 401 opaco. Mejor no arrancar: es config, no un error de uso.
	if c.AnthropicAPIKey != "" {
		if IsPlaceholder(c.AnthropicAPIKey) || !strings.HasPrefix(c.AnthropicAPIKey, "sk-ant-") {
			return errors.New("ANTHROPIC_API_KEY inválida: debe empezar con sk-ant- (o déjala vacía para desactivar la extracción de tickets)")
		}
		if c.AnthropicModel == "" {
			return errors.New("ANTHROPIC_MODEL vacío: define el modelo (p. ej. claude-opus-5) o quita ANTHROPIC_API_KEY")
		}
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
