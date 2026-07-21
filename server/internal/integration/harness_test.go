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
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Reloj fijo: fechas de negocio deterministas para asertar sobre reportes por día.
var fixedNow = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definido; omitiendo tests de integración")
	}
	ctx := context.Background()
	st, err := store.New(ctx, url)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	// Slate limpio en cada corrida (reproducible local y en CI). drop schema borra también
	// la tabla de versiones de goose, así que Migrate re-aplica todo desde cero.
	if _, err := st.Pool.Exec(ctx, "drop schema public cascade; create schema public;"); err != nil {
		st.Close()
		t.Fatalf("reset schema: %v", err)
	}
	if err := store.Migrate(ctx, st.Pool); err != nil {
		st.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// --- fixtures mínimos vía SQL crudo (válidos para los flujos bajo prueba) ---

func makeUser(t *testing.T, st *store.Store, username, role string) int64 {
	t.Helper()
	var id int64
	err := st.Pool.QueryRow(context.Background(),
		`insert into users (name, username, role) values ($1, $2, $3::user_role) returning id`,
		"Test "+username, username, role).Scan(&id)
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
