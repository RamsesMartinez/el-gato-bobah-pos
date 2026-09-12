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
	// PlatformJWTSecret: firma de la consola de plataforma, y tiene que ser OTRA. Con un solo
	// secreto, «usar el token de allá acá» se rechaza con un `if`; con dos, no se puede construir.
	// Es obligatorio a propósito: una consola que no arranca se nota, y una que comparte firma con
	// el negocio no se nota nunca.
	PlatformJWTSecret string `env:"PLATFORM_JWT_SECRET,required"`
	// PlatformDatabaseURL: conexión de la consola, como el rol gatobobah_platform, que solo tiene
	// `select` sobre `companies` y las tablas de plataforma. Vacío = cae a la conexión con la que
	// ya se sirve (dev). Validate lo exige en producción, donde caer al owner anularía los grants.
	PlatformDatabaseURL string `env:"PLATFORM_DATABASE_URL" envDefault:""`
	// PlatformDBPassword: password que el bootstrap le fija al rol gatobobah_platform, igual que
	// AppDBPassword con el de la aplicación — la migración crea el rol sin password para no
	// versionar secretos.
	PlatformDBPassword string `env:"PLATFORM_DB_PASSWORD" envDefault:""`
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

// PlatformDatabaseURLOrDefault devuelve la conexión de la consola o, si no se configuró, la misma
// con la que se sirve el negocio. En producción Validate exige la propia: servir la consola como
// owner convertiría los grants —que son su única barrera— en decoración.
func (c Config) PlatformDatabaseURLOrDefault() string {
	if c.PlatformDatabaseURL != "" {
		return c.PlatformDatabaseURL
	}
	return c.AppDatabaseURLOrDefault()
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
	// El ambiente se compara contra una lista cerrada porque decide cosas en DOS direcciones
	// opuestas: aquí abajo solo se prohíbe `CORS_ORIGIN=*` cuando el valor es exactamente
	// "production", y el router refleja cualquier Origin cuando es cualquier cosa DISTINTA de
	// "production". Un `APP_ENV=prod` cae en el peor cuadrante de los dos —arranca con `*` y
	// refleja— sin que nada lo diga.
	if c.Env != "development" && c.Env != "production" {
		return errors.New("APP_ENV inválido: solo 'development' o 'production' (un valor desconocido apaga las dos mitades del control de CORS)")
	}
	if IsPlaceholder(c.JWTSecret) || len(c.JWTSecret) < minSecretLen {
		return errors.New("JWT_SECRET débil o de ejemplo: usa 32+ caracteres aleatorios (openssl rand -base64 48)")
	}
	// La consola firma con OTRO secreto, y las dos mitades de esta comprobación importan: uno débil
	// se rompe offline (el código es público), y uno IGUAL al del negocio hace que un token de
	// cualquiera de las dos superficies valide en la otra — sin romper nada, solo dejando de
	// separar. Por eso es al arranque y no un chequeo que alguien recuerde hacer.
	if IsPlaceholder(c.PlatformJWTSecret) || len(c.PlatformJWTSecret) < minSecretLen {
		return errors.New("PLATFORM_JWT_SECRET débil o de ejemplo: usa 32+ caracteres aleatorios (openssl rand -base64 48)")
	}
	if c.PlatformJWTSecret == c.JWTSecret {
		return errors.New("PLATFORM_JWT_SECRET igual a JWT_SECRET: dos secretos iguales son uno solo, y un token de la consola valdría en el negocio (y al revés)")
	}
	// PIN_PEPPER es OPCIONAL: sin él el sistema arranca igual y solo el modo de solo-PIN queda
	// bloqueado. Lo que no se acepta es que esté PUESTO y sea basura: la huella del PIN cubre un
	// espacio de un millón, así que con una llave adivinable quien tenga la base lo recorre en
	// segundos — y peor, con la falsa sensación de que hay protección.
	if c.PinPepper != "" && (IsPlaceholder(c.PinPepper) || len(c.PinPepper) < minSecretLen ||
		strings.Contains(strings.ToLower(c.PinPepper), "prueba")) {
		return errors.New("PIN_PEPPER débil o de ejemplo: usa 32+ caracteres aleatorios (openssl rand -base64 48), o déjalo vacío")
	}
	// CORS_ORIGIN admite varios orígenes separados por coma (el POS y la consola viven en dominios
	// distintos). Se revisa CADA uno: "https://app.ejemplo.com,*" dejaría entrar a cualquiera y a
	// simple vista parece configurado.
	if c.Env == "production" {
		for _, o := range strings.Split(c.CORSOrigin, ",") {
			if strings.TrimSpace(o) == "*" {
				return errors.New("CORS_ORIGIN con * no está permitido en producción: define los orígenes exactos separados por coma (https://app…,https://staff…)")
			}
		}
	}
	// Multi-tenant fail-fast: en producción el API DEBE servir como el rol no-superusuario
	// (APP_DATABASE_URL) para que RLS aísle las empresas. Sin él caería al owner (que salta RLS)
	// y anularía el aislamiento en silencio. main.go además lo verifica en runtime (assertRLSEnforced).
	if c.Env == "production" && c.AppDatabaseURL == "" {
		return errors.New("APP_DATABASE_URL requerido en producción: el API debe conectarse como el rol de app (no-superusuario) para que RLS aísle los tenants")
	}
	// La consola se sirve con su propio rol de base por la misma razón: sus grants son la barrera,
	// y el owner los salta. Sin esta variable en producción caería al rol del negocio o al owner y
	// leería tablas de operación sin que nada fallara.
	if c.Env == "production" && c.PlatformDatabaseURL == "" {
		return errors.New("PLATFORM_DATABASE_URL requerido en producción: la consola debe conectarse como gatobobah_platform, cuyos grants son su única barrera")
	}
	// Y su contraseña, que sin este check falla tarde y mal: el compose interpola
	// ${PLATFORM_DB_PASSWORD} dentro de la URL, así que sin ella la URL no queda vacía —pasa el
	// check de arriba—, el bootstrap no le fija contraseña al rol porque no hay ninguna, y el
	// arranque muere al conectar con un error de autenticación que no nombra la variable que falta.
	if c.Env == "production" && c.PlatformDBPassword == "" {
		return errors.New("PLATFORM_DB_PASSWORD requerido en producción: sin él el rol de la consola se queda sin contraseña y la API no puede conectarse")
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
