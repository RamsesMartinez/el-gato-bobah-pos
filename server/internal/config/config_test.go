package config

import "testing"

func base() Config {
	return Config{
		DatabaseURL: "postgres://x",
		JWTSecret:   "0123456789abcdef0123456789abcdef", // 32 chars
		// El de la consola es OTRO, y el default de este helper lo refleja: un base() con los dos
		// iguales haría pasar en verde al test que existe para prohibirlo.
		PlatformJWTSecret: "fedcba9876543210fedcba9876543210",
		Env:               "development",
	}
}

func TestValidate_RejectsWeakJWTSecret(t *testing.T) {
	for _, s := range []string{"", "cambia-esto-por-un-secreto", "short", "0123456789abcdef"} { // last is 16 chars
		c := base()
		c.JWTSecret = s
		if err := Validate(c); err == nil {
			t.Errorf("JWT_SECRET %q should be rejected", s)
		}
	}
}

func TestValidate_AcceptsStrongSecret(t *testing.T) {
	if err := Validate(base()); err != nil {
		t.Fatalf("strong config should pass, got %v", err)
	}
}

func TestValidate_RejectsWildcardCORSInProd(t *testing.T) {
	c := base()
	c.Env = "production"
	c.AppDatabaseURL = "postgres://gatobobah_app:pw@x"           // requerido en prod (ver test de abajo)
	c.PlatformDatabaseURL = "postgres://gatobobah_platform:pw@x" // idem, por sus grants
	c.PlatformDBPassword = "pw"                                  // idem: el compose la mete en esa URL
	c.CORSOrigin = "*"
	if err := Validate(c); err == nil {
		t.Fatal("CORS_ORIGIN=* must be rejected in production")
	}
	c.CORSOrigin = "https://app.elgatobobah.com"
	if err := Validate(c); err != nil {
		t.Fatalf("exact origin should pass in prod, got %v", err)
	}
}

// En producción el API debe servir como el rol no-superusuario (APP_DATABASE_URL) para que RLS
// aísle los tenants; sin él caería al owner y anularía el aislamiento. Fail-fast al arranque.
func TestValidate_RequiresAppDatabaseURLInProd(t *testing.T) {
	c := base()
	c.Env = "production"
	c.CORSOrigin = "https://app.elgatobobah.com"
	c.PlatformDatabaseURL = "postgres://gatobobah_platform:pw@x"
	c.PlatformDBPassword = "pw"
	c.AppDatabaseURL = ""
	if err := Validate(c); err == nil {
		t.Fatal("APP_DATABASE_URL vacío debe rechazarse en producción")
	}
	c.AppDatabaseURL = "postgres://gatobobah_app:pw@db/gatobobah"
	if err := Validate(c); err != nil {
		t.Fatalf("con APP_DATABASE_URL debe pasar en prod, got %v", err)
	}
	// En desarrollo es opcional (se sirve como owner single-tenant).
	dev := base()
	dev.AppDatabaseURL = ""
	if err := Validate(dev); err != nil {
		t.Fatalf("APP_DATABASE_URL opcional en dev, got %v", err)
	}
}

func TestIsPlaceholder(t *testing.T) {
	for _, s := range []string{"", "cambia-esto", "cambia-esto-por-un-secreto", "your_secret_here"} {
		if !IsPlaceholder(s) {
			t.Errorf("%q should be a placeholder", s)
		}
	}
	if IsPlaceholder("un-secreto-real-aleatorio") {
		t.Error("real secret must not be a placeholder")
	}
}

// El secreto de la consola es OTRO secreto, y el arranque lo exige.
//
// Tres rechazos, y el tercero es el que de verdad importa: con `PLATFORM_JWT_SECRET` igual a
// `JWT_SECRET`, un token del negocio valida en la consola y uno de la consola en el negocio. Toda
// la separación de esta feature se apoya en que las dos firmas no coincidan, y ese fallo es
// silencioso: nada se rompe, solo deja de separar.
func TestValidate_ExigeUnSecretoPropioParaLaConsola(t *testing.T) {
	casos := []struct {
		nombre  string
		secreto string
	}{
		{"ausente", ""},
		{"de ejemplo", "cambia-esto-por-un-secreto-de-plataforma"},
		{"corto", "0123456789abcdef"},
		{"igual al del negocio", base().JWTSecret},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cfg := base()
			cfg.PlatformJWTSecret = c.secreto
			if err := Validate(cfg); err == nil {
				t.Fatalf("PLATFORM_JWT_SECRET %q debe rechazarse: %s", c.secreto, c.nombre)
			}
		})
	}

	ok := base()
	ok.PlatformJWTSecret = "fedcba9876543210fedcba9876543210" // 32, distinto del de negocio
	if err := Validate(ok); err != nil {
		t.Fatalf("un secreto propio y fuerte debe pasar, y falló: %v", err)
	}
}

// En producción la consola tiene que servir con SU rol, por la misma razón que el negocio sirve
// con el suyo: el owner salta grants y RLS, así que un PLATFORM_DATABASE_URL ausente convertiría
// las tres barreras en cero sin decir nada.
func TestValidate_ExigePlatformDatabaseURLEnProduccion(t *testing.T) {
	c := base()
	c.Env = "production"
	c.CORSOrigin = "https://app.elgatobobah.com"
	c.AppDatabaseURL = "postgres://gatobobah_app:pw@db/gatobobah"
	c.PlatformJWTSecret = "fedcba9876543210fedcba9876543210"
	c.PlatformDBPassword = "pw"
	c.PlatformDatabaseURL = ""
	if err := Validate(c); err == nil {
		t.Fatal("PLATFORM_DATABASE_URL vacío debe rechazarse en producción")
	}
	c.PlatformDatabaseURL = "postgres://gatobobah_platform:pw@db/gatobobah"
	if err := Validate(c); err != nil {
		t.Fatalf("con PLATFORM_DATABASE_URL debe pasar en prod, y falló: %v", err)
	}

	// En desarrollo es opcional: se cae al mismo rol con el que ya se sirve, y por eso el
	// aislamiento se prueba en integración y no aquí.
	dev := base()
	dev.PlatformJWTSecret = "fedcba9876543210fedcba9876543210"
	dev.PlatformDatabaseURL = ""
	if err := Validate(dev); err != nil {
		t.Fatalf("en desarrollo debe pasar sin PLATFORM_DATABASE_URL, y falló: %v", err)
	}
	if got := dev.PlatformDatabaseURLOrDefault(); got != dev.DatabaseURL {
		t.Fatalf("sin PLATFORM_DATABASE_URL la consola debe caer a la conexión con la que ya se sirve; usó %q", got)
	}
}

// CORS_ORIGIN admite una lista, y el "*" se busca EN CADA entrada.
//
// El caso que cierra: "https://app.elgatobobah.com,*" se lee como configurado —tiene el dominio
// bueno a la vista— y deja entrar a cualquier origen. Nació al agregar el segundo frente (la
// consola de plataforma, en otro subdominio).
func TestValidate_RechazaElAsteriscoEnCualquierEntradaDeLaLista(t *testing.T) {
	prod := func(origen string) Config {
		c := base()
		c.Env = "production"
		c.AppDatabaseURL = "postgres://gatobobah_app:pw@x"
		c.PlatformDatabaseURL = "postgres://gatobobah_platform:pw@x"
		c.PlatformDBPassword = "pw"
		c.CORSOrigin = origen
		return c
	}
	for _, malo := range []string{"*", "https://app.elgatobobah.com,*", "*,https://app.elgatobobah.com", "https://app.elgatobobah.com, *"} {
		if err := Validate(prod(malo)); err == nil {
			t.Errorf("CORS_ORIGIN %q debe rechazarse en producción", malo)
		}
	}
	bueno := "https://app.elgatobobah.com,https://staff.elgatobobah.com"
	if err := Validate(prod(bueno)); err != nil {
		t.Fatalf("dos orígenes exactos deben pasar en producción, y falló: %v", err)
	}
}

// UN `APP_ENV` DESCONOCIDO NO ARRANCA, porque apaga dos protecciones a la vez y en silencio.
//
// El valor decide dos cosas en direcciones opuestas: `Validate` solo prohíbe `CORS_ORIGIN=*` cuando
// dice exactamente "production", y el router activa el reflejo de cualquier Origin cuando dice
// cualquier cosa DISTINTA de "production". Un `APP_ENV=prod` —o `Production`, o un espacio de más—
// cae en el peor cuadrante de los dos: arranca con `*` y refleja el origen que le manden. Un typo
// que abre CORS de par en par no puede pasar por configuración válida.
func TestValidate_RechazaUnAmbienteDesconocido(t *testing.T) {
	for _, malo := range []string{"prod", "Production", "PRODUCTION", "staging", "dev", ""} {
		c := base()
		c.Env = malo
		if err := Validate(c); err == nil {
			t.Errorf("APP_ENV %q debe rechazarse: decide CORS en dos lugares y un typo los apaga los dos", malo)
		}
	}
	for _, bueno := range []string{"development", "production"} {
		c := base()
		c.Env = bueno
		if bueno == "production" {
			c.CORSOrigin = "https://app.elgatobobah.com"
			c.AppDatabaseURL = "postgres://gatobobah_app:pw@x"
			c.PlatformDatabaseURL = "postgres://gatobobah_platform:pw@x"
			c.PlatformDBPassword = "pw"
		}
		if err := Validate(c); err != nil {
			t.Errorf("APP_ENV %q debe pasar, y falló: %v", bueno, err)
		}
	}
}

// En producción, la contraseña del rol de la consola es obligatoria — y su ausencia era invisible.
//
// La cadena completa: el compose arma `PLATFORM_DATABASE_URL` interpolando `${PLATFORM_DB_PASSWORD}`,
// así que sin ella la URL queda NO vacía (`postgres://gatobobah_platform:@postgres:…`) y pasa la
// comprobación de arriba; el bootstrap no le fija contraseña al rol porque no hay ninguna que
// fijar; y el arranque muere al conectar, con un error de autenticación que no menciona la variable
// que falta.
func TestValidate_ExigeLaContrasenaDelRolDeLaConsolaEnProduccion(t *testing.T) {
	c := base()
	c.Env = "production"
	c.CORSOrigin = "https://app.elgatobobah.com"
	c.AppDatabaseURL = "postgres://gatobobah_app:pw@x"
	c.PlatformDatabaseURL = "postgres://gatobobah_platform:@db/gatobobah"
	c.PlatformDBPassword = ""
	if err := Validate(c); err == nil {
		t.Fatal("sin PLATFORM_DB_PASSWORD el arranque debe rechazarse en producción, no morir después al conectar")
	}
	c.PlatformDBPassword = "una-contrasena-de-rol"
	if err := Validate(c); err != nil {
		t.Fatalf("con la contraseña debe pasar, y falló: %v", err)
	}
}
