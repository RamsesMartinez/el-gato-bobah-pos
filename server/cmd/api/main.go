package main

import (
	"bufio"
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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "ping local /healthz and exit (for Docker HEALTHCHECK)")
	resetAdmin := flag.Bool("reset-admin", false, "actualiza/crea el admin con ADMIN_* y sale (sin borrar datos)")
	flag.Parse()

	loadEnvFile() // carga deploy/.env de forma literal (soporta # $ espacios, sin expansión de shell)

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
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := store.Migrate(ctx, st.Pool); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}

	if *resetAdmin {
		if err := resetAdminUser(ctx, st); err != nil {
			slog.Error("reset admin", "error", err)
			os.Exit(1)
		}
		slog.Info("admin actualizado")
		return
	}

	if err := bootstrapAdmin(ctx, st); err != nil {
		slog.Error("bootstrap admin", "error", err)
		os.Exit(1)
	}

	jm := auth.NewManager(cfg.JWTSecret, nil)
	handlers := httpapi.NewHandlers(httpapi.Deps{
		Cfg:        cfg,
		JWT:        jm,
		Auth:       app.NewAuthService(st, jm, nil),
		Users:      app.NewUsersService(st),
		Menu:       app.NewMenuService(st, nil),
		MenuCache:  cache.NewMenuCache(cfg.RedisURL),
		Suggest:    app.NewSuggestService(st, nil),
		Costing:    app.NewCostingService(st),
		Orders:     app.NewOrdersService(st, nil),
		Backoffice: app.NewBackofficeService(st, nil),
		Admin:      app.NewAdminService(st),
		Settings:   app.NewSettingsService(st),
		Broker:     realtime.NewBroker(),
	})
	router := httpapi.Router(cfg, jm, handlers, st.Pool.Ping)

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

// bootstrapAdmin creates the first admin if the users table is empty, using
// ADMIN_USERNAME / ADMIN_PASSWORD (required) and ADMIN_PIN (optional) from the
// environment. No hardcoded credentials: if they're missing on an empty DB the
// API refuses to start with a clear message.
func bootstrapAdmin(ctx context.Context, st *store.Store) error {
	n, err := st.Q.CountUsers(ctx)
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
	if _, err := st.Q.CreateUser(ctx, params); err != nil {
		return err
	}
	slog.Info("admin inicial creado", "username", username)
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

// loadEnvFile carga variables desde un archivo .env con parseo 100% LITERAL: el valor
// es todo lo que sigue al '=' hasta el fin de línea, SIN expansión de $ ni comentarios
// inline (soporta cualquier contraseña: #, $, !, espacios, etc., como python-decouple).
// Solo se quitan comillas envolventes opcionales. No sobrescribe variables ya definidas
// (dev inyecta la conexión; prod usa compose).
func loadEnvFile() {
	var path string
	for _, f := range []string{os.Getenv("ENV_FILE"), "deploy/.env", "../deploy/.env"} {
		if f == "" {
			continue
		}
		if _, err := os.Stat(f); err == nil {
			path = f
			break
		}
	}
	if path == "" {
		return
	}
	f, err := os.Open(path) //nolint:gosec // ruta de configuración conocida
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // línea vacía o comentario (solo a inicio de línea)
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// quitar comillas envolventes opcionales, sin tocar el contenido
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
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
	n, err := st.Q.SetUserSecretsByUsername(ctx, db.SetUserSecretsByUsernameParams{
		Username: &username, PasswordHash: &pwHash, PinHash: pinHash,
	})
	if err != nil {
		return err
	}
	if n == 0 { // no existía → crear
		_, err = st.Q.CreateUser(ctx, db.CreateUserParams{
			Name: "Administrador", Username: &username, Role: string(domain.RoleAdmin),
			PasswordHash: &pwHash, PinHash: pinHash,
		})
	}
	return err
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
