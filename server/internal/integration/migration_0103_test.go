//go:build integration

package integration

import (
	"context"
	"strconv"
	"testing"
)

// effectiveOnVersion es la migración de la fecha de entrega de los movimientos (spec 053).
const effectiveOnVersion = 103

// LA 0103 CON DOS EMPRESAS EN ZONAS DISTINTAS (constitución IV). La fecha de entrega se llena con el
// día del negocio de CADA empresa: un movimiento de las 20:00 en México es del día siguiente en
// Tokio, y un relleno con una sola zona (o con la de la sesión) fecharía mal el de la otra.
func TestMigrationEffectiveOn(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "mig-entregas-b")
	if _, err := st.Pool.Exec(ctx, `insert into business_settings (company_id, business_name, timezone) values ($1, 'Mig 53 B', 'Asia/Tokyo')
		on conflict (company_id) do update set timezone = excluded.timezone`, coB); err != nil {
		t.Fatal(err)
	}
	migrarAbajoHasta(t, st.Pool, effectiveOnVersion-1)

	ingA := ingredientIn(t, st, defaultCompanyID, "Tapas mig 53", "pieza")
	ingB := ingredientIn(t, st, coB, "Tapas mig 53 b", "pieza")
	// 2026-10-10 20:00 en México = 2026-10-11 02:00 UTC = 2026-10-11 11:00 en Tokio.
	insert := func(company, ing int64, kind string) int64 {
		t.Helper()
		var id int64
		if err := st.Pool.QueryRow(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, created_at)
			values ($1, 'ingrediente', $2, $3::stock_movement_type, 5, '2026-10-11 02:00:00+00') returning id`, company, ing, kind).Scan(&id); err != nil {
			t.Fatalf("movimiento %s: %v", kind, err)
		}
		return id
	}
	compraA, compraB := insert(defaultCompanyID, ingA, "compra"), insert(coB, ingB, "compra")
	ajusteA := insert(defaultCompanyID, ingA, "ajuste")

	migrarArriba(t, st.Pool)

	day := func(id int64) *string {
		t.Helper()
		var d *string
		if err := st.Pool.QueryRow(ctx, `select effective_on::text from stock_movements where id = $1`, id).Scan(&d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := day(compraA); d == nil || *d != "2026-10-10" {
		t.Fatalf("la compra de México quedó con %v: el relleno no usó la zona de su empresa", d)
	}
	if d := day(compraB); d == nil || *d != "2026-10-11" {
		t.Fatalf("la compra de Tokio quedó con %v: el relleno no usó la zona de su empresa", d)
	}
	if d := day(ajusteA); d != nil {
		t.Fatalf("un ajuste sin gasto quedó con fecha de entrega %v: solo las compras llevan", *d)
	}

	// La API anterior durante la subida inserta sin la columna: el trigger la llena con el día de su
	// captura en la zona de la empresa.
	var fresh int64
	var got, want string
	if err := st.Pool.QueryRow(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'compra', 1) returning id, effective_on::text, (now() at time zone 'Asia/Tokyo')::date::text`,
		coB, ingB).Scan(&fresh, &got, &want); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("una compra sin fecha de la API anterior quedó del %s; el día del negocio es %s", got, want)
	}
	// Una fecha explícita se respeta.
	if _, err := st.Pool.Exec(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, effective_on)
		values ($1, 'ingrediente', $2, 'compra', 1, '2026-10-01')`, defaultCompanyID, ingA); err != nil {
		t.Fatal(err)
	}

	// Con una fecha capturada que no sale de created_at, el Down se niega: la perdería.
	if err := migrarAbajoHastaErr(t, st.Pool, effectiveOnVersion-1); err == nil {
		t.Fatal("el Down corrió con una fecha de entrega capturada: se perdería")
	}
	if _, err := st.Pool.Exec(ctx, `delete from stock_movements where effective_on = '2026-10-01'`); err != nil {
		t.Fatal(err)
	}
	if err := migrarAbajoHastaErr(t, st.Pool, effectiveOnVersion-1); err != nil {
		t.Fatalf("sin fechas capturadas el Down debió bajar: %v", err)
	}
	var exists bool
	if err := st.Pool.QueryRow(ctx, `select exists (select 1 from information_schema.columns
		where table_name = 'stock_movements' and column_name = 'effective_on')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("el Down dejó la columna")
	}
	migrarArriba(t, st.Pool)

	// Revisión de seguridad (4): la API anterior inserta como gatobobah_app, bajo RLS. El trigger lee
	// la zona de la empresa con esos mismos permisos y la fecha sale del día de Tokio.
	as := appRoleStore(t)
	tx, err := as.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, strconv.FormatInt(coB, 10)); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
		values ('ingrediente', $1, 'compra', 1) returning effective_on::text, (now() at time zone 'Asia/Tokyo')::date::text`,
		ingB).Scan(&got, &want); err != nil {
		t.Fatalf("una compra sin fecha como gatobobah_app: %v", err)
	}
	if got != want {
		t.Fatalf("como gatobobah_app, la compra quedó del %s; el día del negocio en Tokio es %s", got, want)
	}
}
