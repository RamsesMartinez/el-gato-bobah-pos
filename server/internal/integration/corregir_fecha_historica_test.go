//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MIGRACIÓN 0062, PROBADA SOBRE VENTAS QUE YA TRAÍAN LA FECHA DEL TURNO.
//
// Corrige el día de las ventas archivadas con la fecha del turno que las cobró, para que la columna
// signifique lo mismo en todo el histórico. Lo que NO puede hacer es mover dinero: el arqueo agrupa
// por turno y no lee esa columna, y este test lo comprueba en vez de confiar en ello.
//
// Con DOS empresas a propósito: con una sola, un defecto de alcance —una migración que solo toca la
// empresa "actual"— es un no-op y pasa verde para romper en producción.
func TestLaMigracionCorrigeElDiaSinMoverDineroDeArqueo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// La tabla de respaldo nace vacía al migrar el esquema limpio. Se quita para dejar la base como
	// la encontrará la migración en un servidor con historia.
	if _, err := st.Pool.Exec(ctx, `drop table if exists orders_business_date_fix`); err != nil {
		t.Fatalf("preparar el estado previo: %v", err)
	}

	otra := makeCompany(t, st, "otra-empresa-0062")
	cajero := makeUser(t, st, "cajero_0062", "cajero")
	makeUserIn(t, st, otra, "beto_0062", "cajero")
	prod := makeProduct(t, st, "Café histórico", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	sess := abrirCajaPrincipal(t, st, cajero)

	svc := app.NewOrdersService(st, clock)
	var ids []int64
	for range 3 {
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
		})
		if err != nil {
			t.Fatalf("sembrar: %v", err)
		}
		ids = append(ids, o.ID)
	}

	// El estado previo al arreglo: la venta con la fecha del TURNO, que abrió cuatro días antes.
	// Es exactamente lo que dejó el turno olvidado de producción.
	delTurno := fixedNow.AddDate(0, 0, -4)
	if _, err := st.Pool.Exec(ctx,
		`update orders set business_date = $1::date where id = any($2)`, delTurno, ids); err != nil {
		t.Fatalf("simular la herencia vieja: %v", err)
	}
	// Y una venta de la otra empresa, para que un alcance mal escrito se note.
	var ajena int64
	if err := st.Pool.QueryRow(ctx, `
		insert into orders (company_id, client_uuid, business_date, daily_number, service_type,
		                    subtotal, total, opened_by, opened_at, status)
		values ($1, gen_random_uuid(), $2::date, 7001, 'mostrador', 50, 50,
		        (select id from users where company_id = $1 limit 1), $3::timestamptz, 'entregada')
		returning id`, otra, delTurno, fixedNow).Scan(&ajena); err != nil {
		t.Fatalf("sembrar la venta ajena: %v", err)
	}

	antesArqueo := totalesDelTurno(t, st, sess)
	antesFolios := foliosDe(t, st, ids)

	if _, err := st.Pool.Exec(ctx, sqlDeLaMigracion(t, "0062_fecha_de_venta_del_reloj.sql")); err != nil {
		t.Fatalf("correr la migración: %v", err)
	}

	// La verdad de cuándo ocurrió una venta es su `opened_at`, y de ahí es de donde la migración
	// tiene que derivar el día. No se compara contra el reloj de los tests a propósito: `opened_at`
	// lo pone la BASE y `business_date` lo ponía el servicio, así que en pruebas los dos relojes no
	// coinciden — y es justo la diferencia que la corrección tiene que resolver.
	zona := domain.LoadBusinessLocation(domain.DefaultTimezone)
	for _, id := range append(append([]int64{}, ids...), ajena) {
		var d, ocurrio time.Time
		if err := st.Pool.QueryRow(ctx,
			`select business_date, opened_at from orders where id = $1`, id).Scan(&d, &ocurrio); err != nil {
			t.Fatalf("leer %d: %v", id, err)
		}
		quiere := domain.BusinessDate(ocurrio, zona)
		if !d.Equal(quiere) {
			t.Errorf("el pedido %d quedó archivado el %s y ocurrió el %s: la corrección no lo alcanzó "+
				"(¿alcance limitado a una empresa?)", id, d.Format("2006-01-02"), quiere.Format("2006-01-02"))
		}
	}

	// LO QUE NO PUEDE CAMBIAR.
	if despues := totalesDelTurno(t, st, sess); despues != antesArqueo {
		t.Errorf("el arqueo del turno pasó de %s a %s: la corrección movió dinero de un corte a otro",
			antesArqueo, despues)
	}
	for id, antes := range antesFolios {
		if despues := foliosDe(t, st, []int64{id})[id]; despues != antes {
			t.Errorf("el pedido %d cambió de folio/turno (%s → %s): la corrección solo debía tocar la fecha",
				id, antes, despues)
		}
	}

	// Y SE PUEDE REVERTIR: el respaldo devuelve las fechas de antes.
	if _, err := st.Pool.Exec(ctx, downDeLaMigracion(t, "0062_fecha_de_venta_del_reloj.sql")); err != nil {
		t.Fatalf("revertir: %v", err)
	}
	var vuelta time.Time
	if err := st.Pool.QueryRow(ctx, `select business_date from orders where id = $1`, ids[0]).Scan(&vuelta); err != nil {
		t.Fatalf("leer tras revertir: %v", err)
	}
	if !vuelta.Equal(domain.BusinessDate(delTurno, time.UTC)) {
		t.Errorf("tras revertir, el pedido quedó en %s y debía volver a %s: el respaldo no restaura",
			vuelta.Format("2006-01-02"), delTurno.Format("2006-01-02"))
	}
}

// totalesDelTurno resume lo que el arqueo dice del turno, en una sola cadena para poder compararla
// entera: si algo del corte cambia, el mensaje muestra qué era y en qué quedó.
func totalesDelTurno(t *testing.T, st *store.Store, sess int64) string {
	t.Helper()
	var cobros int
	var monto decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(), `
		select count(*), coalesce(sum(op.amount), 0)
		from order_payments op join orders o on o.id = op.order_id
		where op.register_session_id = $1 and o.status not in ('cancelada','reembolsada')`,
		sess).Scan(&cobros, &monto); err != nil {
		t.Fatalf("totales del turno: %v", err)
	}
	return itoa(cobros) + " cobros por " + monto.String()
}

// foliosDe devuelve folio y turno de cada pedido, que es lo que la corrección NO debe tocar.
func foliosDe(t *testing.T, st *store.Store, ids []int64) map[int64]string {
	t.Helper()
	out := map[int64]string{}
	for _, id := range ids {
		var num int
		var nombre *string
		var turno *int64
		if err := st.Pool.QueryRow(context.Background(),
			`select daily_number, folio_name, register_session_id from orders where id = $1`,
			id).Scan(&num, &nombre, &turno); err != nil {
			t.Fatalf("folio de %d: %v", id, err)
		}
		n := ""
		if nombre != nil {
			n = *nombre
		}
		s := int64(0)
		if turno != nil {
			s = *turno
		}
		out[id] = "#" + itoa(num) + " " + n + " turno " + itoa(int(s))
	}
	return out
}

// downDeLaMigracion devuelve el bloque Down del archivo, para probar que revertir es de verdad
// posible y no una promesa escrita en un comentario.
func downDeLaMigracion(t *testing.T, nombre string) string {
	t.Helper()
	crudo, err := os.ReadFile(filepath.Join("..", "..", "migrations", nombre))
	if err != nil {
		t.Fatalf("leer la migración: %v", err)
	}
	_, down, ok := strings.Cut(string(crudo), "-- +goose Down")
	if !ok || strings.TrimSpace(down) == "" {
		t.Fatalf("%s no tiene bloque Down", nombre)
	}
	return down
}
