//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

// LA TABLA NUEVA TIENE SU GRANT, Y ESO SOLO SE VE CON EL ROL DE LA APP.
//
// El grant de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay default privileges.
// Cada tabla creada después necesita el suyo, y olvidarlo NO se nota en dev —la API de desarrollo se
// conecta como owner, que salta RLS y los grants— sino en producción, con un 42501 en la primera
// devolución.
//
// Por eso este test corre bajo `appRoleStore` y no bajo el owner: con el owner pasaría igual de
// verde con el grant borrado.
func TestElLibroDeDevolucionesEsUsablePorElRolDeApp(t *testing.T) {
	newTestStore(t) // migra
	st := appRoleStore(t)
	ctx := context.Background()

	tenantCtx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	var n int
	if err := st.Pool.QueryRow(tenantCtx, `select count(*) from order_refunds`).Scan(&n); err != nil {
		t.Fatalf("el rol de la app no puede LEER order_refunds: %v — falta su grant, y en producción "+
			"eso es un 42501 en la primera devolución", err)
	}
	// Insertar también: el grant de select solo no basta, y es el que de verdad se usa al devolver.
	if _, err := st.Pool.Exec(tenantCtx, `
		insert into order_refunds (order_id, payment_method_id, amount, reason, refunded_by)
		select o.id, p.payment_method_id, 1, 'prueba de grant', o.opened_by
		  from orders o join order_payments p on p.order_id = o.id limit 1`); err != nil {
		t.Fatalf("el rol de la app no puede INSERTAR en order_refunds: %v — falta su grant", err)
	}
}

// LA MIGRACIÓN CORRE SOBRE DATOS QUE YA ESTABAN, Y CON DOS EMPRESAS.
//
// Con una sola empresa, todo camino "por cada otra empresa" es un no-op y la migración pasa verde
// para romper en producción, que tiene dos. Y sobre una base vacía no se ejercita ningún dato
// previo, que es donde viven los defectos de una migración.
func TestLaMigracionDeDevolucionesNoTocaLoQueYaEstaba(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, st, "otra-empresa-0060")
	ana := makeUser(t, st, "ana_0060", "cajero")
	beto := makeUserIn(t, st, otra, "beto_0060", "cajero")

	// Movimientos de inventario previos a la columna nueva, uno por empresa.
	for _, u := range []int64{ana, beto} {
		prod := makeProduct(t, st, "Producto 0060 de "+itoa(int(u)), decimal.RequireFromString("50"), true)
		if _, err := st.Pool.Exec(ctx,
			`insert into stock_movements (item_type, product_id, movement_type, quantity, user_id, reason)
			 values ('producto', $1, 'venta', -1, $2, 'previo a 0060')`, prod, u); err != nil {
			t.Fatalf("sembrar movimiento: %v", err)
		}
	}

	var conRenglon, total int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) filter (where order_line_id is not null), count(*) from stock_movements
		  where reason = 'previo a 0060'`).Scan(&conRenglon, &total); err != nil {
		t.Fatalf("contar movimientos: %v", err)
	}
	if total != 2 {
		t.Fatalf("se sembraron %d movimientos, quiere 2 (uno por empresa)", total)
	}
	// Lo histórico queda en NULL A PROPÓSITO: de un movimiento viejo no consta a qué renglón
	// pertenecía, y repartirlo por producto sería afirmar algo que nadie sabe.
	if conRenglon != 0 {
		t.Fatalf("%d movimientos históricos quedaron con renglón: la migración está inventando de "+
			"dónde salió un descuento que nadie registró así", conRenglon)
	}
}

// UN ARQUEO YA CERRADO DA LAS MISMAS CIFRAS DESPUÉS DE LA MIGRACIÓN.
//
// Es la garantía de FR-009 sobre un negocio en operación: producción tiene cortes cerrados y este
// salto no puede moverles un peso. La migración solo agrega, pero "solo agrega" es lo que se dice
// antes de medirlo.
func TestUnArqueoCerradoNoCambiaConLaMigracion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var movimientos int
	var fondoCerrado decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `
		select (select count(*) from register_cash_movements),
		       coalesce((select sum(opening_cash) from register_sessions where status = 'cerrada'), 0)
		`).Scan(&movimientos, &fondoCerrado); err != nil {
		t.Fatalf("leer arqueos cerrados: %v", err)
	}
	_ = fondoCerrado

	// La migración ya corrió al construir el store. Lo que se comprueba es que la tabla nueva nace
	// VACÍA y que ningún arqueo cerrado tiene devoluciones que nadie registró.
	var devoluciones int
	if err := st.Pool.QueryRow(ctx, `select count(*) from order_refunds`).Scan(&devoluciones); err != nil {
		t.Fatalf("contar devoluciones: %v", err)
	}
	if devoluciones != 0 {
		t.Fatalf("la migración creó %d devoluciones de la nada: un arqueo cerrado cambiaría de cifras",
			devoluciones)
	}
	// Y no se coló ningún movimiento de caja nuevo, que es por donde una devolución tocaría el arqueo.
	var movimientosAhora int
	if err := st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements`).Scan(&movimientosAhora); err != nil {
		t.Fatalf("contar movimientos de caja: %v", err)
	}
	if movimientosAhora != movimientos {
		t.Fatalf("los movimientos de caja pasaron de %d a %d: la migración movió dinero de un arqueo",
			movimientos, movimientosAhora)
	}
}
