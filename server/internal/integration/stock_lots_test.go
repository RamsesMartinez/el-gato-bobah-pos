//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CADUCIDAD POR LOTE (spec 098): la regla de consumo vive en el trigger de stock_movements, así que
// estas pruebas insertan movimientos como lo haría cualquier camino —también la versión anterior de
// la API— y leen lo que quedó en los lotes.

type lotKit struct {
	st            *store.Store
	local, bodega int64
	leche         int64
}

func newLotKit(t *testing.T) *lotKit {
	t.Helper()
	st := newTestStore(t)
	k := &lotKit{st: st}
	k.local, k.bodega = comoEnLaSegundaSubida(t, st, defaultCompanyID)
	k.leche = ingredientIn(t, st, defaultCompanyID, "Leche lotes", "ml")
	return k
}

// today es el día del negocio según la base (la zona del negocio), el mismo que usa la regla.
func (k *lotKit) today(t *testing.T) time.Time {
	t.Helper()
	var d time.Time
	if err := k.st.Pool.QueryRow(context.Background(), `select business_today($1)`, defaultCompanyID).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d
}

// move inserta un movimiento de artículo; expires vacío = sin fecha.
func (k *lotKit) move(t *testing.T, warehouse int64, kind, qty, expires string) {
	t.Helper()
	var exp any
	if expires != "" {
		exp = expires
	}
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements
		(warehouse_id, item_type, ingredient_id, movement_type, quantity, expires_on)
		values ($1, 'ingrediente', $2, $3::stock_movement_type, $4::numeric, $5::date)`,
		warehouse, k.leche, kind, qty, exp); err != nil {
		t.Fatalf("insertar %s %s (%s): %v", kind, qty, expires, err)
	}
}

// lots devuelve fecha → cantidad de los lotes con algo, del almacén.
func (k *lotKit) lots(t *testing.T, warehouse int64) map[string]string {
	t.Helper()
	rows, err := k.st.Pool.Query(context.Background(), `select expires_on::text, qty::text from stock_lots
		where warehouse_id = $1 and ingredient_id = $2 and qty > 0 order by expires_on`, warehouse, k.leche)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, q string
		if err := rows.Scan(&d, &q); err != nil {
			t.Fatal(err)
		}
		out[d] = decimal.RequireFromString(q).String()
	}
	return out
}

// undated es la existencia menos los lotes: lo sin fecha.
func (k *lotKit) undated(t *testing.T, warehouse int64) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select
		coalesce((select on_hand from stock_levels where warehouse_id = $1 and ingredient_id = $2), 0)
		- coalesce((select sum(qty) from stock_lots where warehouse_id = $1 and ingredient_id = $2), 0)`,
		warehouse, k.leche).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func day(t time.Time, n int) string { return t.AddDate(0, 0, n).Format(time.DateOnly) }

func wantLots(t *testing.T, got map[string]string, want map[string]string, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: lotes = %v, quería %v", what, got, want)
	}
	for d, q := range want {
		if got[d] != q {
			t.Fatalf("%s: lotes = %v, quería %v", what, got, want)
		}
	}
}

// Un artículo sin fechas se comporta como hoy: ningún lote, todo sin fecha.
func TestAMovementWithoutDateCreatesNoLot(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	k.move(t, k.local, "ajuste", "500", "")
	k.move(t, k.local, "venta", "-200", "")
	wantLots(t, k.lots(t, k.local), map[string]string{}, "sin fechas")
	if u := k.undated(t, k.local); !u.Equal(decimal.NewFromInt(300)) {
		t.Fatalf("sin fecha = %s, quería 300", u)
	}
}

// Dos entradas con la misma fecha en el mismo almacén son un solo lote; en otro almacén, otro.
func TestTwoEntriesWithTheSameDateAreOneLot(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	d := day(k.today(t), 20)
	k.move(t, k.local, "compra", "300", d)
	k.move(t, k.local, "compra", "200", d)
	k.move(t, k.bodega, "compra", "100", d)
	wantLots(t, k.lots(t, k.local), map[string]string{d: "500"}, "Local")
	wantLots(t, k.lots(t, k.bodega), map[string]string{d: "100"}, "Bodega")
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_lots where ingredient_id = $1`, k.leche).Scan(&n); err != nil || n != 2 {
		t.Fatalf("filas de lote = %d (%v), quería 2", n, err)
	}
}

// Una venta toma primero lo sin fecha y después el lote que caduca antes.
func TestASaleTakesUndatedFirstThenTheEarliestLot(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	d10, d20 := day(today, 10), day(today, 20)
	k.move(t, k.local, "ajuste", "100", "")
	k.move(t, k.local, "compra", "200", d20)
	k.move(t, k.local, "compra", "200", d10)
	k.move(t, k.local, "venta", "-200", "")
	wantLots(t, k.lots(t, k.local), map[string]string{d10: "100", d20: "200"}, "tras vender 200")
	if u := k.undated(t, k.local); !u.IsZero() {
		t.Fatalf("sin fecha = %s, quería 0", u)
	}
	k.move(t, k.local, "merma", "-150", "")
	wantLots(t, k.lots(t, k.local), map[string]string{d20: "150"}, "una merma descuenta igual que una venta")
}

// Nunca de un caducado: si lo bueno no alcanza, lo sin fecha queda en negativo, el caducado intacto, y
// la campana avisa una sola vez.
func TestASaleNeverTakesFromAnExpiredLot(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	old, good := day(today, -2), day(today, 5)
	k.move(t, k.local, "compra", "300", old)
	k.move(t, k.local, "compra", "100", good)
	k.move(t, k.local, "venta", "-150", "")
	k.move(t, k.local, "venta", "-30", "")
	wantLots(t, k.lots(t, k.local), map[string]string{old: "300"}, "el caducado intacto")
	if u := k.undated(t, k.local); !u.Equal(decimal.NewFromInt(-80)) {
		t.Fatalf("sin fecha = %s, quería -80 (lo bueno en negativo)", u)
	}
	var open int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from notifications
		where kind = 'expired_left' and status = 'open' and ingredient_id = $1 and warehouse_id = $2`, k.leche, k.local).Scan(&open); err != nil || open != 1 {
		t.Fatalf("avisos «lo que queda está caducado» abiertos = %d (%v), quería 1 aunque fueron dos ventas", open, err)
	}

	// Mandar el caducado a merma desde su lote cierra el aviso.
	k.move(t, k.local, "merma", "-300", old)
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from notifications
		where kind = 'expired_left' and status = 'open' and ingredient_id = $1`, k.leche).Scan(&open); err != nil || open != 0 {
		t.Fatalf("el aviso siguió abierto tras mandar el caducado a merma (%d, %v)", open, err)
	}
}

// Llegar existencia buena que cubre el negativo también cierra el aviso.
func TestTheExpiredNoticeClosesWhenGoodStockCoversIt(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	k.move(t, k.local, "compra", "100", day(today, -1))
	k.move(t, k.local, "venta", "-50", "")
	k.move(t, k.local, "compra", "80", "")
	var open int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from notifications
		where kind = 'expired_left' and status = 'open' and ingredient_id = $1`, k.leche).Scan(&open); err != nil || open != 0 {
		t.Fatalf("aviso abierto con lo bueno de vuelta en positivo (%d, %v)", open, err)
	}
	var reason string
	if err := k.st.Pool.QueryRow(context.Background(), `select resolved_reason from notifications
		where kind = 'expired_left' and ingredient_id = $1`, k.leche).Scan(&reason); err != nil || reason != "restocked" {
		t.Fatalf("razón de cierre = %q (%v), quería restocked", reason, err)
	}
}

// Un lote que caduca hoy todavía es bueno (caduca al terminar el día del negocio).
func TestALotExpiringTodayIsStillUsed(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	d := day(k.today(t), 0)
	k.move(t, k.local, "compra", "100", d)
	k.move(t, k.local, "venta", "-40", "")
	wantLots(t, k.lots(t, k.local), map[string]string{d: "60"}, "caduca hoy")
}

// Un insert de varias filas del mismo artículo descuenta en orden, sin contar dos veces.
func TestAMultiRowInsertTakesInOrder(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	today := k.today(t)
	d5, d9 := day(today, 5), day(today, 9)
	k.move(t, k.local, "compra", "100", d5)
	k.move(t, k.local, "compra", "100", d9)
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements
		(warehouse_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'venta', -60), ($1, 'ingrediente', $2, 'venta', -60)`, k.local, k.leche); err != nil {
		t.Fatal(err)
	}
	wantLots(t, k.lots(t, k.local), map[string]string{d9: "80"}, "dos filas en un insert")
}

// Una corrección negativa más grande que el lote lo deja en 0, nunca negativo; el resto sale de lo
// sin fecha.
func TestNoLotEverGoesNegative(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	d := day(k.today(t), 30)
	k.move(t, k.local, "ajuste", "50", "")
	k.move(t, k.local, "compra", "100", d)
	k.move(t, k.local, "venta", "-120", "") // 50 sin fecha + 70 del lote
	k.move(t, k.local, "compra", "-100", d) // corregir la compra: solo quedan 30 en el lote
	var q decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select qty from stock_lots where ingredient_id = $1`, k.leche).Scan(&q); err != nil || !q.IsZero() {
		t.Fatalf("lote = %s (%v), quería 0", q, err)
	}
	if u := k.undated(t, k.local); !u.Equal(decimal.NewFromInt(-70)) {
		t.Fatalf("sin fecha = %s, quería -70", u)
	}
}

// La suma de lotes más lo sin fecha es la existencia, siempre (SC-002), y el resumen de la mañana no
// cuenta el aviso de caducados (R12 M3).
func TestTheMorningSummaryIgnoresTheExpiredNotice(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	k.move(t, k.local, "compra", "100", day(k.today(t), -1))
	k.move(t, k.local, "venta", "-50", "")
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from notifications where kind = 'expired_left' and status = 'open'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("aviso abierto = %d (%v)", n, err)
	}
	items, err := db.New(k.st.Pool).OpenNotificationsForMorning(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == "expired_left" {
			t.Fatal("el resumen de la mañana trae el aviso de caducados: su asunto contaría un aviso que el cuerpo no muestra")
		}
	}
}
