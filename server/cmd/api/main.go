package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/hibp"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/mailer"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"

	"github.com/jackc/pgx/v5"
	"golang.org/x/term"
)

// Inyectadas por ldflags en el build de producción (server/Dockerfile: -X main.version / main.builtAt).
// En dev quedan con estos defaults.
var (
	version = "dev"
	builtAt = ""
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "ping local /healthz and exit (for Docker HEALTHCHECK)")
	resetAdmin := flag.Bool("reset-admin", false, "actualiza/crea el admin con ADMIN_* y sale (sin borrar datos)")
	createCompany := flag.Bool("create-company", false, "provisiona una empresa nueva (COMPANY_SLUG/NAME) + su admin (ADMIN_*) y sale")
	resetPassword := flag.String("reset-password", "", "resetea la contraseña de username@slug (prompt interactivo, oculto) y sale")
	flag.Parse()

	config.LoadEnvFile() // carga deploy/.env de forma literal (soporta # $ espacios, sin expansión de shell)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	if *healthcheck {
		os.Exit(runHealthcheck(cfg.Port))
	}

	logger, err := logging.Setup(cfg.LogDir, cfg.LogLevel)
	if err != nil {
		slog.Error("logging", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	ctx := context.Background()
	// Dual-pool multi-tenant: el store ADMIN (owner/superuser, DATABASE_URL) migra y hace
	// bootstrap — salta RLS, necesario para DDL/backfills/provisioning. El store de SERVICIO
	// (rol gatobobah_app, APP_DATABASE_URL) atiende requests SUJETO a RLS (un superuser la
	// saltaría). Si no hay APP_DATABASE_URL, cae a DATABASE_URL (dev single-tenant sin aislamiento).
	admin, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("store", "error", err)
		os.Exit(1)
	}

	if err := store.Migrate(ctx, admin.Pool); err != nil {
		slog.Error("migrate", "error", err)
		admin.Close()
		os.Exit(1)
	}
	if err := ensureAppRolePassword(ctx, admin, cfg.AppDBPassword); err != nil {
		slog.Error("app role password", "error", err)
		admin.Close()
		os.Exit(1)
	}

	if *resetAdmin {
		if err := resetAdminUser(ctx, admin); err != nil {
			slog.Error("reset admin", "error", err)
			admin.Close()
			os.Exit(1)
		}
		slog.Info("admin actualizado")
		admin.Close()
		return
	}

	if *createCompany {
		if err := provisionCompany(ctx, admin); err != nil {
			slog.Error("create company", "error", err)
			admin.Close()
			os.Exit(1)
		}
		admin.Close()
		return
	}

	if err := bootstrapAdmin(ctx, admin); err != nil {
		slog.Error("bootstrap admin", "error", err)
		admin.Close()
		os.Exit(1)
	}
	admin.Close() // ya no se usa: servir va por el rol de app bajo RLS

	st, err := store.New(ctx, cfg.AppDatabaseURLOrDefault())
	if err != nil {
		slog.Error("serving store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Verificación en runtime de que el aislamiento multi-tenant ES real (no solo config). Solo
	// cuando servimos con el rol de app dedicado (APP_DATABASE_URL); en dev sin él se sirve como
	// owner single-tenant a propósito. Aborta si el rol saltaría RLS → jamás servir sin aislar.
	if cfg.AppDatabaseURL != "" {
		if err := assertRLSEnforced(ctx, st); err != nil {
			slog.Error("aislamiento multi-tenant no garantizado", "error", err)
			os.Exit(1)
		}
	}

	// Corre sobre `st` (rol de servicio, RLS-enforced en prod): el GetUserByUsername de abajo
	// depende de RLS para acotar a la empresa correcta — hacerlo como owner (admin) vería
	// usernames repetidos entre empresas y podría resetear el usuario equivocado.
	if *resetPassword != "" {
		if err := runResetPassword(ctx, cfg, st, *resetPassword); err != nil {
			slog.Error("reset password", "error", err)
			os.Exit(1)
		}
		slog.Info("password actualizado", "identifier", *resetPassword)
		return
	}

	jm := auth.NewManager(cfg.JWTSecret, nil)
	hibpClient := hibp.New(&http.Client{Timeout: 5 * time.Second})
	mail := mailer.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.MailFrom)
	handlers := httpapi.NewHandlers(httpapi.Deps{
		Cfg:        cfg,
		Version:    version,
		BuiltAt:    builtAt,
		JWT:        jm,
		Auth:       app.NewAuthService(st, jm, nil),
		Users:      app.NewUsersService(st, hibpClient, cfg.HIBPEnabled),
		Menu:       app.NewMenuService(st, nil),
		MenuCache:  cache.NewMenuCache(cfg.RedisURL),
		Suggest:    app.NewSuggestService(st, nil),
		Costing:    app.NewCostingService(st),
		Orders:     app.NewOrdersService(st, nil),
		Backoffice: app.NewBackofficeService(st, nil),
		Admin:      app.NewAdminService(st),
		Settings:   app.NewSettingsService(st),
		Company:    app.NewCompanyService(st),
		Reset:      app.NewResetService(st, mail, hibpClient, cfg.HIBPEnabled, cfg.AppBaseURL, nil),
		Broker:     realtime.NewBroker(),
		// nil cuando no hay ANTHROPIC_API_KEY: la extracción de tickets es opcional y el POS
		// funciona capturando las líneas del gasto a mano (el handler responde 501).
		PurchaseDoc: app.NewPurchaseDocService(cfg.AnthropicAPIKey, cfg.AnthropicModel),
	})
	router := httpapi.Router(cfg, jm, handlers, st)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second, // cuerpo completo: corta slowloris/slow-body
		IdleTimeout:       120 * time.Second,
		// Sin WriteTimeout global: SSE (/events) mantiene la respuesta abierta.
	}

	go func() {
		slog.Info("listening", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serve", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "error", err)
	}
}

// assertRLSEnforced verifica en runtime que el rol de servicio NO puede saltar RLS: (1) no es
// superuser ni bypassrls; (2) prueba funcional — sin contexto de tenant, `users` (que tras el
// bootstrap tiene ≥1 fila) debe verse VACÍA por RLS. Si se ven filas, el rol está saltando RLS
// (es owner/superuser) y servir así filtraría datos entre empresas → abortar.
func assertRLSEnforced(ctx context.Context, st *store.Store) error {
	var bypass bool
	if err := st.Pool.QueryRow(ctx,
		"select coalesce(bool_or(rolsuper or rolbypassrls), false) from pg_roles where rolname = current_user").Scan(&bypass); err != nil {
		return err
	}
	if bypass {
		return fmt.Errorf("el rol de servicio (%s) es superuser/bypassrls: RLS no aislaría los tenants", "current_user")
	}
	var n int
	if err := st.Pool.QueryRow(ctx, "select count(*) from users").Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("RLS no aísla: sin contexto de tenant se ven %d usuarios (el rol de servicio no debe ser el owner de las tablas)", n)
	}
	return nil
}

// ensureAppRolePassword le fija el password al rol gatobobah_app (creado sin password en la
// migración 0024 para no versionar secretos). Corre como owner (admin store). No-op si no se
// definió APP_DB_PASSWORD (dev que sirve como owner). El password de un rol NO admite parámetro
// en DDL → se escapa como literal SQL; viene de env de confianza (deploy/.env).
func ensureAppRolePassword(ctx context.Context, st *store.Store, pw string) error {
	if pw == "" {
		return nil
	}
	lit := "'" + strings.ReplaceAll(pw, "'", "''") + "'"
	_, err := st.Pool.Exec(ctx, "alter role gatobobah_app with login password "+lit)
	return err
}

// defaultCompany resuelve (o crea) la empresa de arranque por COMPANY_SLUG (default 'gatobobah',
// sembrada en la migración 0022) y devuelve su id. Provisioning de plataforma → corre como owner.
func defaultCompany(ctx context.Context, st *store.Store) (int64, error) {
	slug := envOr("COMPANY_SLUG", "gatobobah")
	id, err := st.Q.ResolveCompanyBySlug(ctx, slug)
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}
	co, err := st.Q.CreateCompany(ctx, db.CreateCompanyParams{Slug: slug, Name: envOr("COMPANY_NAME", slug)})
	if err != nil {
		return 0, err
	}
	return co.ID, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// provisionCompany crea una empresa NUEVA (COMPANY_SLUG/COMPANY_NAME) y su admin inicial
// (ADMIN_*), para onboarding de un tenant adicional. Operación de plataforma → corre como owner.
// Falla si el slug ya existe.
func provisionCompany(ctx context.Context, st *store.Store) error {
	slug := os.Getenv("COMPANY_SLUG")
	name := envOr("COMPANY_NAME", slug)
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	pin := os.Getenv("ADMIN_PIN")
	if !domain.ValidSlug(slug) {
		return fmt.Errorf("COMPANY_SLUG inválido (2-40, minúsculas/dígitos/guiones): %q", slug)
	}
	if username == "" || password == "" {
		return fmt.Errorf("define ADMIN_USERNAME/ADMIN_PASSWORD para el admin de la empresa")
	}
	if err := checkAdminSecrets(password, pin); err != nil {
		return err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return err
	}
	if existing, err := st.Q.ResolveCompanyBySlug(ctx, slug); err != nil {
		return err
	} else if existing != 0 {
		return fmt.Errorf("el slug %q ya está en uso", slug)
	}
	co, err := st.Q.CreateCompany(ctx, db.CreateCompanyParams{Slug: slug, Name: name})
	if err != nil {
		return err
	}
	pwHash, err := auth.HashSecret(password)
	if err != nil {
		return err
	}
	params := db.CreateUserParams{
		Name: "Administrador", Username: &username, Role: string(domain.RoleAdmin), PasswordHash: &pwHash,
	}
	if pin != "" {
		pinHash, err := auth.HashSecret(pin)
		if err != nil {
			return err
		}
		params.PinHash = &pinHash
	}
	if err := st.WithTenant(ctx, co.ID, func(q *db.Queries) error {
		if _, err := q.CreateUser(ctx, params); err != nil {
			return err
		}
		// Sin esto la empresa nace sin poder cobrar: desde 0037 payment_methods es per-tenant y ya
		// no se heredan por ser una tabla global. Los de plataforma NO se siembran — ese negocio
		// tiene que hacer su propia vinculación con Uber/DiDi/Rappi antes de cobrar por ahí.
		if err := q.SeedBasePaymentMethods(ctx, co.ID); err != nil {
			return err
		}
		// Y sin fila de ajustes nace sin zona horaria, así que sus fechas se calcularían en UTC y
		// la cena caería en el día siguiente (el bug que arregló 0038).
		return q.SeedBusinessSettings(ctx, name)
	}); err != nil {
		return err
	}
	slog.Info("empresa provisionada", "slug", slug, "company_id", co.ID, "admin", username)
	return nil
}

// bootstrapAdmin creates the first admin (en la empresa por defecto) if the users table is
// empty, using ADMIN_USERNAME / ADMIN_PASSWORD (required) and ADMIN_PIN (optional).
func bootstrapAdmin(ctx context.Context, st *store.Store) error {
	n, err := st.Q.CountUsers(ctx) // owner: RLS bypassed → cuenta global (arranque = BD vacía)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	pin := os.Getenv("ADMIN_PIN")
	if username == "" || password == "" {
		return fmt.Errorf("no hay usuarios y faltan ADMIN_USERNAME/ADMIN_PASSWORD para crear el admin inicial (defínelos en deploy/.env)")
	}
	if err := checkAdminSecrets(password, pin); err != nil {
		return err
	}
	companyID, err := defaultCompany(ctx, st)
	if err != nil {
		return err
	}
	pwHash, err := auth.HashSecret(password)
	if err != nil {
		return err
	}
	params := db.CreateUserParams{
		Name:         "Administrador",
		Username:     &username,
		Role:         string(domain.RoleAdmin),
		PasswordHash: &pwHash,
	}
	if pin != "" {
		pinHash, err := auth.HashSecret(pin)
		if err != nil {
			return err
		}
		params.PinHash = &pinHash
	}
	// WithTenant fija app.company_id → el DEFAULT de users.company_id lo auto-sella al insertar.
	if err := st.WithTenant(ctx, companyID, func(q *db.Queries) error {
		_, err := q.CreateUser(ctx, params)
		return err
	}); err != nil {
		return err
	}
	slog.Info("admin inicial creado", "username", username, "company_id", companyID)
	return nil
}

// checkAdminSecrets refuses to create/reset the admin with shipped example values or
// a trivially guessable PIN. This runs even when the operator does `docker compose up`
// directly (bypassing scripts/check-env.sh), so the guard must live in code.
func checkAdminSecrets(password, pin string) error {
	if config.IsPlaceholder(password) {
		return fmt.Errorf("ADMIN_PASSWORD es un valor de ejemplo; define una contraseña real en deploy/.env")
	}
	if pin != "" && auth.IsWeakPin(pin) {
		return fmt.Errorf("ADMIN_PIN es demasiado débil (evita 1234/0000/secuencias); usa uno menos obvio en deploy/.env")
	}
	return nil
}

// resetAdminUser actualiza la contraseña/PIN del admin (por ADMIN_USERNAME) desde el
// entorno, o lo crea si no existe. No borra datos. Para cuando cambias credenciales
// después del primer arranque (el bootstrap solo corre con la base vacía).
func resetAdminUser(ctx context.Context, st *store.Store) error {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	pin := os.Getenv("ADMIN_PIN")
	if username == "" || password == "" {
		return fmt.Errorf("define ADMIN_USERNAME/ADMIN_PASSWORD")
	}
	if err := checkAdminSecrets(password, pin); err != nil {
		return err
	}
	pwHash, err := auth.HashSecret(password)
	if err != nil {
		return err
	}
	var pinHash *string
	if pin != "" {
		h, err := auth.HashSecret(pin)
		if err != nil {
			return err
		}
		pinHash = &h
	}
	companyID, err := defaultCompany(ctx, st)
	if err != nil {
		return err
	}
	// Scopeado a la empresa por defecto. ponytail: SetUserSecretsByUsername filtra solo por
	// username; como owner (admin store) RLS no acota, pero en el arranque hay una sola empresa.
	// Si algún día se resetea admin con multi-empresa poblada, añadir company_id al WHERE.
	return st.WithTenant(ctx, companyID, func(q *db.Queries) error {
		n, err := q.SetUserSecretsByUsername(ctx, db.SetUserSecretsByUsernameParams{
			Username: &username, PasswordHash: &pwHash, PinHash: pinHash,
		})
		if err != nil {
			return err
		}
		if n == 0 { // no existía → crear
			_, err = q.CreateUser(ctx, db.CreateUserParams{
				Name: "Administrador", Username: &username, Role: string(domain.RoleAdmin),
				PasswordHash: &pwHash, PinHash: pinHash,
			})
		}
		return err
	})
}

// runResetPassword resetea la contraseña de username@slug pidiéndola por terminal (oculta, con
// confirmación) — misma política que el reset por admin vía HTTP (fuerza + HIBP,
// must_change_password=true tras el reset): no es un atajo que bypasee esas reglas.
func runResetPassword(ctx context.Context, cfg config.Config, st *store.Store, identifier string) error {
	username, slug, ok := strings.Cut(identifier, "@")
	if !ok || username == "" || slug == "" {
		return fmt.Errorf("formato inválido: usa username@slug")
	}
	companyID, err := st.Q.ResolveCompanyBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if companyID == 0 {
		return fmt.Errorf("no existe la empresa %q", slug)
	}

	tenantCtx, release, err := st.AcquireTenant(ctx, companyID)
	if err != nil {
		return err
	}
	defer release()

	u, err := st.QC(tenantCtx).GetUserByUsername(tenantCtx, &username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no existe el usuario %q en la empresa %q", username, slug)
		}
		return err
	}

	pw, err := promptPassword("Nuevo password: ")
	if err != nil {
		return err
	}
	confirm, err := promptPassword("Confirma: ")
	if err != nil {
		return err
	}
	if pw != confirm {
		return fmt.Errorf("los passwords no coinciden")
	}

	users := app.NewUsersService(st, hibp.New(nil), cfg.HIBPEnabled)
	return users.AdminSetPassword(tenantCtx, u.ID, pw)
}

// promptPassword pide un valor por terminal sin hacer eco (no queda en el scrollback ni en
// grabaciones de pantalla, y nunca pasa por argv/shell history).
func promptPassword(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func runHealthcheck(port string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:"+port+"/healthz", nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
