//go:build integration

package integration

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA DETECCIÓN DE «STOCK BAJO» (spec 084): un trigger sobre los movimientos de existencia que abre y
// cierra avisos por estado, que nunca rompe una venta y que no abre subtransacciones cuando no hay nada
// que cambiar.

// alertOn enciende la alerta de un insumo con su mínimo (vacío = sin mínimo).
func alertOn(t *testing.T, st *store.Store, ing int64, min string, onMin, onOut bool) {
	t.Helper()
	var m any
	if min != "" {
		m = min
	}
	if _, err := st.Pool.Exec(context.Background(), `
		update ingredients set low_stock_enabled = true, low_stock_on_min = $2, low_stock_on_out = $3,
		       min_stock = $4::numeric where id = $1`, ing, onMin, onOut, m); err != nil {
		t.Fatal(err)
	}
}

// openKinds devuelve los tipos de aviso abiertos de un insumo, ordenados.
func openKinds(t *testing.T, st *store.Store, ing int64) []domain.NotificationKind {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(),
		`select kind from notifications where ingredient_id = $1 and status = 'open' order by kind`, ing)
	if err != nil {
		t.Fatal(err)
	}
	out := []domain.NotificationKind{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		out = append(out, domain.NotificationKind(k))
	}
	return out
}

func ingredientOnHand(t *testing.T, st *store.Store, ing int64) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(on_hand), 0) from stock_levels where ingredient_id = $1`, ing).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// La función de la base y domain.LowStockShould dicen lo mismo, caso por caso, recorriendo la
// existencia de arriba abajo y de regreso: abre por estado y cierra al volver.
func TestEvaluateLowStockMatchesTheDomain(t *testing.T) {
	st := newTestStore(t)
	configs := []struct {
		name         string
		min          string
		onMin, onOut bool
	}{
		{"mínimo 5, los dos eventos", "5", true, true},
		{"mínimo 0 = sin mínimo", "0", true, true},
		{"sin mínimo", "", true, true},
		{"solo agotado", "5", false, true},
		{"solo mínimo", "5", true, false},
	}
	for i, c := range configs {
		t.Run(c.name, func(t *testing.T) {
			ing := pieceIngredient(t, st, "Insumo 084 verdad "+itoa(i))
			adjust(t, st, "ingrediente", ing, "10")
			alertOn(t, st, ing, c.min, c.onMin, c.onOut)
			cfg := domain.LowStockConfig{Enabled: true, OnMin: c.onMin, OnOut: c.onOut}
			if c.min != "" {
				cfg.Min = dp(c.min)
			}
			for _, delta := range []string{"-4", "-1", "-5", "-1", "-2", "3", "4", "9"} {
				adjust(t, st, "ingrediente", ing, delta)
				onHand := ingredientOnHand(t, st, ing)
				low, out := domain.LowStockShould(cfg, onHand)
				want := []domain.NotificationKind{}
				if low {
					want = append(want, domain.KindStockLow)
				}
				if out {
					want = append(want, domain.KindStockOut)
				}
				sort.Slice(want, func(a, b int) bool { return want[a] < want[b] })
				got := openKinds(t, st, ing)
				if len(got) != len(want) || (len(got) > 0 && got[0] != want[0]) || (len(got) > 1 && got[1] != want[1]) {
					t.Fatalf("con %s abiertos %v; el dominio dice %v", onHand, got, want)
				}
			}
		})
	}
}

// Sin la casilla no hay aviso por mínimo, por baja que esté la existencia.
func TestWithoutTheCheckboxNothingOpens(t *testing.T) {
	st := newTestStore(t)
	ing := pieceIngredient(t, st, "Insumo 084 sin casilla")
	if _, err := st.Pool.Exec(context.Background(), `update ingredients set min_stock = 5 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	adjust(t, st, "ingrediente", ing, "-3")
	if k := openKinds(t, st, ing); len(k) != 0 {
		t.Fatalf("un insumo sin la casilla abrió %v", k)
	}
}

// pedir conteo: con la existencia en negativo, o sin conteo en 30 días, el aviso no dice «agotado».
func TestNegativeOrUncountedNeedsCount(t *testing.T) {
	st := newTestStore(t)
	ing := pieceIngredient(t, st, "Insumo 084 sin conteo")
	alertOn(t, st, ing, "5", true, true)
	adjust(t, st, "ingrediente", ing, "-2")
	var needs bool
	if err := st.Pool.QueryRow(context.Background(),
		`select bool_and(needs_count) from notifications where ingredient_id = $1 and status = 'open'`, ing).Scan(&needs); err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatal("una existencia negativa y nunca contada abrió el aviso sin «pedir conteo»")
	}
}

// Un insumo archivado no avisa aunque una receta viva lo siga descontando.
func TestInactiveIngredientNeverNotifies(t *testing.T) {
	st := newTestStore(t)
	ing := pieceIngredient(t, st, "Insumo 084 archivado")
	adjust(t, st, "ingrediente", ing, "10")
	alertOn(t, st, ing, "5", true, true)
	if _, err := st.Pool.Exec(context.Background(), `update ingredients set is_active = false where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	adjust(t, st, "ingrediente", ing, "-8")
	if k := openKinds(t, st, ing); len(k) != 0 {
		t.Fatalf("un insumo archivado abrió %v", k)
	}
}

// DOS CONEXIONES CRUZAN EL MISMO MÍNIMO A LA VEZ: un solo aviso. La segunda venta espera la fila de
// existencia de la primera y, al seguir, ve el aviso ya abierto (o choca en el índice único y lo deja).
func TestTwoConnectionsCrossingTheSameMinimumOpenOne(t *testing.T) {
	st := newTestStore(t)
	ing := pieceIngredient(t, st, "Insumo 084 carrera")
	adjust(t, st, "ingrediente", ing, "7")
	alertOn(t, st, ing, "5", true, false)

	app := appRoleStoreWithOneConnN(t, 4)
	ctx := context.Background()
	begin := func() pgx.Tx {
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, itoa(defaultCompanyID)); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	move := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
			values ('ingrediente', $1, 'venta', -1)`, ing)
		return err
	}
	tx1, tx2 := begin(), begin()
	if err := move(tx1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var err2 error
	wg.Add(1)
	go func() {
		defer wg.Done()
		err2 = move(tx2)
		if err2 == nil {
			err2 = tx2.Commit(ctx)
		}
	}()
	time.Sleep(200 * time.Millisecond) // la segunda queda esperando la fila de existencia
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err2 != nil {
		t.Fatalf("la segunda venta falló: %v", err2)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from notifications where ingredient_id = $1 and kind = 'stock_low'`, ing).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("dos ventas que cruzaron el mismo mínimo dejaron %d avisos", n)
	}
}

// UNA FALLA AL CREAR EL AVISO NUNCA ROMPE LA VENTA: se fuerza con una restricción que rechaza toda
// fila de avisos y la venta se guarda igual.
func TestANotificationFailureNeverBreaksASale(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ing := pieceIngredient(t, st, "Insumo 084 falla")
	adjust(t, st, "ingrediente", ing, "6")
	alertOn(t, st, ing, "5", true, true)
	if _, err := st.Pool.Exec(ctx, `alter table notifications add constraint prueba_084_falla check (kind = 'nunca') not valid`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.Pool.Exec(context.Background(), `alter table notifications drop constraint if exists prueba_084_falla`)
	})
	if _, err := st.Pool.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
		values ('ingrediente', $1, 'venta', -3)`, ing); err != nil {
		t.Fatalf("la venta falló por el aviso: %v", err)
	}
	if got := ingredientOnHand(t, st, ing); !got.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("la existencia quedó en %s; la venta no se aplicó", got)
	}
	// Y como la regla es por estado, el siguiente movimiento sin la falla abre el aviso perdido.
	if _, err := st.Pool.Exec(ctx, `alter table notifications drop constraint prueba_084_falla`); err != nil {
		t.Fatal(err)
	}
	adjust(t, st, "ingrediente", ing, "-1")
	if k := openKinds(t, st, ing); len(k) != 1 || k[0] != domain.KindStockLow {
		t.Fatalf("el aviso perdido no se recuperó con el siguiente movimiento: %v", k)
	}
}

// UNA VENTA SIN CAMBIO DE ESTADO NO LLAMA A LA FUNCIÓN (ni abre subtransacción ni avisa): un insumo ya
// bajo el mínimo con su aviso abierto es el caso de cada venta en una mala racha.
func TestAQuietSaleOpensNoSubtransaction(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ing := pieceIngredient(t, st, "Insumo 084 racha")
	adjust(t, st, "ingrediente", ing, "4")
	alertOn(t, st, ing, "5", true, false)
	adjust(t, st, "ingrediente", ing, "-1") // abre el aviso
	if k := openKinds(t, st, ing); len(k) != 1 {
		t.Fatalf("preparación: %v", k)
	}
	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `set local track_functions = 'all'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := tx.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
			values ('ingrediente', $1, 'venta', -0.5)`, ing); err != nil {
			t.Fatal(err)
		}
	}
	var calls int64
	if err := tx.QueryRow(ctx, `select coalesce(sum(calls), 0) from pg_stat_xact_user_functions where funcname = 'evaluate_low_stock'`).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("una venta sin cambio de estado llamó a evaluate_low_stock %d veces (cada llamada es una subtransacción)", calls)
	}
}

// El trigger de avisos corre DESPUÉS del que suma la existencia: si un renombre cambiara el orden,
// evaluaría con la existencia de antes de la venta.
func TestTheNotificationTriggerRunsAfterTheStockTrigger(t *testing.T) {
	st := newTestStore(t)
	rows, err := st.Pool.Query(context.Background(), `
		select tgname from pg_trigger
		where tgrelid = 'stock_movements'::regclass and not tgisinternal
		  and tgname in ('trg_stock_movement', 'trg_stock_movement_notify') order by tgname`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "trg_stock_movement" || got[1] != "trg_stock_movement_notify" {
		t.Fatalf("triggers en orden %v", got)
	}
}
