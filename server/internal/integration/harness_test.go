//go:build integration

// Package integration runs end-to-end tests against a REAL Postgres — the flows that unit
// tests can't reach because the store is a concrete *db.Queries (no mockeable interface):
// rotación/reuso de refresh, la tx de creación de orden + depleción, y el reembolso.
//
// Correr: TEST_DATABASE_URL="postgres://…/gatobobah_test?sslmode=disable" go test -tags=integration ./internal/integration/...
// Sin la env se omiten (Skip). Cada test parte de un esquema limpio (drop+migrate).
package integration

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Reloj fijo: fechas de negocio deterministas para asertar sobre reportes por día.
var fixedNow = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// defaultCompanyID: la empresa 'gatobobah' que siembra la migración 0022 (id=1 en BD limpia).
const defaultCompanyID = 1

// appRolePassword: password de prueba para gatobobah_app, fijado tras migrar (en prod lo hace
// el bootstrap desde APP_DB_PASSWORD).
const appRolePassword = "test_app_pw"

func testURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		t.Skip("TEST_DATABASE_URL no definido; omitiendo tests de integración")
	}
	return u
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbURL := testURL(t)
	ctx := context.Background()

	// Fase de setup en un store temporal (owner): schema limpio + migrar + preparar el rol de app.
	setup, err := store.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if _, err := setup.Pool.Exec(ctx, "drop schema public cascade; create schema public;"); err != nil {
		setup.Close()
		t.Fatalf("reset schema: %v", err)
	}
	if err := store.Migrate(ctx, setup.Pool); err != nil {
		setup.Close()
		t.Fatalf("migrate: %v", err)
	}
	// Rol de app usable en los tests de aislamiento (RLS aplica a él, no al owner).
	u, _ := url.Parse(dbURL)
	dbName := u.Path[1:]
	for _, stmt := range []string{
		"alter role gatobobah_app with login password '" + appRolePassword + "'",
		"grant connect on database " + dbName + " to gatobobah_app",
		// GUC de tenant por defecto a nivel BD: las conexiones del OWNER (que salta RLS)
		// auto-sellan company_id=1 en sus inserts sin fijar el GUC en cada test. Aplica a
		// conexiones NUEVAS → reabrimos el pool abajo.
		"alter database " + dbName + " set app.company_id = '" + itoa(defaultCompanyID) + "'",
	} {
		if _, err := setup.Pool.Exec(ctx, stmt); err != nil {
			setup.Close()
			t.Fatalf("setup rol/guc (%q): %v", stmt, err)
		}
	}
	setup.Close()

	// Pool definitivo (owner): sus conexiones heredan app.company_id=1 por el ALTER DATABASE.
	st, err := store.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("store.New (final): %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// appRoleStore devuelve un store conectado como gatobobah_app (no-superusuario) → RLS SÍ aplica.
// Úsalo para verificar el aislamiento real de tenant a través del store/servicios.
func appRoleStore(t *testing.T) *store.Store {
	t.Helper()
	u, _ := url.Parse(testURL(t))
	u.User = url.UserPassword("gatobobah_app", appRolePassword)
	st, err := store.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("app-role store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// --- fixtures mínimos vía SQL crudo (owner: salta RLS). company_id explícito para poder sembrar
// en cualquier empresa (los inserts del owner sin company_id caen en la empresa 1 por el GUC). ---

func makeCompany(t *testing.T, st *store.Store, slug string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`insert into companies (slug, name) values ($1, $2) returning id`, slug, "Test "+slug).Scan(&id); err != nil {
		t.Fatalf("makeCompany(%s): %v", slug, err)
	}
	// Espeja a provisionCompany: una empresa sin métodos de pago no puede cobrar, así que un test
	// que la creara pelada estaría probando un mundo que el sistema no produce.
	if err := st.Q.SeedBasePaymentMethods(context.Background(), id); err != nil {
		t.Fatalf("sembrar métodos de %s: %v", slug, err)
	}
	// Por WithTenant: el seed toma company_id del GUC, no de un parámetro.
	if err := st.WithTenant(context.Background(), id, func(q *db.Queries) error {
		return q.SeedDeliveryPlatforms(context.Background())
	}); err != nil {
		t.Fatalf("sembrar plataformas de %s: %v", slug, err)
	}
	return id
}

func makeUser(t *testing.T, st *store.Store, username, role string) int64 {
	return makeUserIn(t, st, defaultCompanyID, username, role)
}

func makeUserIn(t *testing.T, st *store.Store, companyID int64, username, role string) int64 {
	t.Helper()
	var id int64
	err := st.Pool.QueryRow(context.Background(),
		`insert into users (company_id, name, username, role) values ($1, $2, $3, $4::user_role) returning id`,
		companyID, "Test "+username, username, role).Scan(&id)
	if err != nil {
		t.Fatalf("makeUser(%s): %v", username, err)
	}
	return id
}

func makeProduct(t *testing.T, st *store.Store, name string, price decimal.Decimal, trackStock bool) int64 {
	t.Helper()
	ctx := context.Background()
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into categories (name) values ($1) returning id`, "cat-"+name).Scan(&catID); err != nil {
		t.Fatalf("makeCategory(%s): %v", name, err)
	}
	var id int64
	if err := st.Pool.QueryRow(ctx,
		`insert into products (name, category_id, price, track_stock) values ($1, $2, $3, $4) returning id`,
		name, catID, price, trackStock).Scan(&id); err != nil {
		t.Fatalf("makeProduct(%s): %v", name, err)
	}
	return id
}

func countOrderMovements(t *testing.T, st *store.Store, orderID int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from stock_movements where order_id = $1`, orderID).Scan(&n); err != nil {
		t.Fatalf("countOrderMovements: %v", err)
	}
	return n
}

// abrirCajaPrincipal deja la caja principal con turno abierto. Desde que cobrar exige caja abierta
// (domain.ErrNoOpenRegister) es precondición de cualquier test que cree una venta, así que vive
// aquí y no copiada en cada archivo.
func abrirCajaPrincipal(t *testing.T, st *store.Store, por int64) int64 {
	t.Helper()
	ctx := context.Background()
	var regID int64
	if err := st.Pool.QueryRow(ctx,
		`select id from cash_registers where is_primary and is_active limit 1`).Scan(&regID); err != nil {
		t.Fatalf("caja principal: %v", err)
	}
	var sessID int64
	// La fecha del turno sale del RELOJ DE LOS TESTS, no de current_date. Desde que la venta hereda
	// su fecha de negocio del turno (para que uno que cruce la medianoche no reinicie el folio), un
	// turno abierto en la fecha real dejaría las ventas fuera del rango que consultan los reportes.
	if err := st.Pool.QueryRow(ctx,
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id)
		 values ($3, 0, $1, $2) returning id`, por, regID, fixedNow).Scan(&sessID); err != nil {
		t.Fatalf("abrir caja principal: %v", err)
	}
	return sessID
}

// platformID resuelve una plataforma de reparto DENTRO de una empresa. El filtro por empresa no es
// decorativo: delivery_platforms es per-tenant y cada empresa tiene su propia "Uber Eats", así que
// buscar solo por nombre devolvería la del tenant equivocado sin dar error.
func platformID(t *testing.T, st *store.Store, companyID int64, name string) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(),
		`select id from delivery_platforms where company_id = $1 and name = $2`,
		companyID, name).Scan(&id); err != nil {
		t.Fatalf("platformID(%d, %s): %v", companyID, name, err)
	}
	return id
}

// optionID devuelve una opción de modificador cualquiera de la empresa, creando el grupo si hace
// falta. Sirve para probar los precios de plataforma de los extras sin montar un menú completo.
func optionID(t *testing.T, st *store.Store, companyID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var groupID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into modifier_groups (company_id, name) values ($1, 'Extras de prueba') returning id`,
		companyID).Scan(&groupID); err != nil {
		t.Fatalf("grupo de modificadores: %v", err)
	}
	var id int64
	if err := st.Pool.QueryRow(ctx,
		`insert into modifier_options (company_id, group_id, name, price_delta) values ($1, $2, 'Extra', 20) returning id`,
		companyID, groupID).Scan(&id); err != nil {
		t.Fatalf("opción de modificador: %v", err)
	}
	return id
}
