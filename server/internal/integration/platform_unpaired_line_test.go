//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

const detalleConExtra = `{
  "id":"ped-extra","display_id":"X1","placed_at":"2026-10-03T18:04:00Z","type":"DELIVERY",
  "eater":{"first_name":"Ana"},
  "payment":{"charges":{"total":{"amount":15900}}},
  "cart":{"items":[{"id":"taro","title":"Frappé Taro","quantity":1,
                    "price":{"unit_price":{"amount":14900}},
                    "selected_modifier_groups_items":[
                      {"id":"perla","title":"Extra perla","quantity":1,"price":{"unit_price":{"amount":1000}}}]}]}}`

// UN PLATILLO DE UBER SIN PAREJA ENTRA LIGADO AL PRODUCTO GENÉRICO, CON SUS OPCIONES EN LA NOTA.
//
// Sin esto el renglón entra sin producto: no sigue el camino de los demás en reportes y comanda, y
// lo que el cliente eligió solo queda en el nombre. Aceptar sigue siendo un toque: nadie decide nada.
func TestAnUnpairedPlatformLineGoesToTheGenericProduct(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-del-platillo-sin-pareja"
	empresa := makeCompany(t, st, "empresa-sin-pareja")
	usuario := makeUserIn(t, st, empresa, "cajera-sin-pareja", "cajero")
	tiendaConLlave(t, st, empresa, "tienda-sin-pareja", llave)

	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	svc := app.NewPedidosDePlataformaService(st,
		fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalleConExtra)}}},
		signingKeyCipher, "sandbox", clock)
	cuerpo := avisoDePedido("evt-sin-pareja", "tienda-sin-pareja")
	if _, err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	}); err != nil {
		t.Fatalf("recibir: %v", err)
	}
	pend, err := svc.Pendientes(ctxT)
	if err != nil || len(pend) != 1 {
		t.Fatalf("pendientes: %v (%d)", err, len(pend))
	}
	vista, err := svc.Aceptar(ctxT, pend[0].ID, usuario)
	if err != nil {
		t.Fatalf("aceptar: %v", err)
	}

	var generico int64
	if err := st.Pool.QueryRow(ctx,
		`select id from products where company_id = $1 and system_kind = 'platform_unpaired'`, empresa).Scan(&generico); err != nil {
		t.Fatalf("la empresa debe tener su producto genérico: %v", err)
	}
	var producto *int64
	var nombre string
	var nota *string
	var renglones int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) over (), product_id, product_name, notes from order_lines where order_id = $1`, vista.ID).Scan(&renglones, &producto, &nombre, &nota); err != nil {
		t.Fatal(err)
	}
	if renglones != 1 {
		t.Fatalf("la opción viaja en la nota, no como renglón aparte: %d renglones", renglones)
	}
	if producto == nil || *producto != generico {
		t.Fatalf("el renglón sin pareja va al producto genérico %d, quedó en %v", generico, producto)
	}
	if nombre != "Frappé Taro" {
		t.Fatalf("el renglón conserva el nombre de Uber: %q", nombre)
	}
	if nota == nil || *nota != "Extra perla (+$10.00)" {
		t.Fatalf("la nota dice a cocina lo que eligió el cliente: %v", nota)
	}
	var movimientos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements where order_id = $1`, vista.ID).Scan(&movimientos); err != nil {
		t.Fatal(err)
	}
	if movimientos != 0 {
		t.Fatalf("el genérico no descuenta almacén: no se sabe qué lleva")
	}
}
