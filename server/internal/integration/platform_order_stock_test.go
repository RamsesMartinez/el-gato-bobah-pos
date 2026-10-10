//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

const detalleConExtraDoble = `{
  "id":"ped-almacen","display_id":"A1","placed_at":"2026-10-07T18:04:00Z","type":"DELIVERY",
  "eater":{"first_name":"Ana"},
  "payment":{"charges":{"total":{"amount":31800}}},
  "cart":{"items":[
    {"id":"taro","title":"Frappé Taro","quantity":2,"price":{"unit_price":{"amount":14900}},
     "selected_modifier_groups_items":[
       {"id":"perla","title":"Extra perla","quantity":1,"price":{"unit_price":{"amount":1000}}}]},
    {"id":"misterio","title":"Platillo sin pareja","quantity":1,"price":{"unit_price":{"amount":0}}}]}}`

// ACEPTAR UN PEDIDO DE PLATAFORMA DESCUENTA EL ALMACÉN.
//
// Antes no descontaba nada: todo lo que salía por Uber se quedaba en el inventario, y el almacén se
// veía más lleno cuanto más se vendía en la plataforma. Lo emparejado descuenta como en mostrador —el
// platillo y su opción—; el producto genérico no, porque no se sabe qué lleva.
func TestAcceptingAPlatformOrderDepletesStock(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-del-almacen-de-plataforma"
	empresa := makeCompany(t, st, "empresa-almacen")
	usuario := makeUserIn(t, st, empresa, "cajera-almacen", "cajero")
	conexion := tiendaConLlave(t, st, empresa, "tienda-almacen", llave)

	insumo := func(name string) int64 {
		var id int64
		if err := st.Pool.QueryRow(ctx, `
			insert into ingredients (company_id, name, base_unit_id) select $1, $2, id from units where code = 'g' returning id`,
			empresa, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	receta := func(ing int64, qty string) int64 {
		var id int64
		if err := st.Pool.QueryRow(ctx, `insert into recipes (company_id) values ($1) returning id`, empresa).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `
			insert into recipe_items (company_id, recipe_id, ingredient_id, quantity, unit_id)
			select $1, $2, $3, $4::numeric, id from units where code = 'g'`, empresa, id, ing, qty); err != nil {
			t.Fatal(err)
		}
		return id
	}
	leche, perla := insumo("Leche plataforma"), insumo("Perla plataforma")

	var categoria, frappe, grupo, extra int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'Bebidas plataforma') returning id`, empresa).Scan(&categoria); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `
		insert into products (company_id, name, category_id, price, recipe_id) values ($1, 'Frappé', $2, 70, $3) returning id`,
		empresa, categoria, receta(leche, "200")).Scan(&frappe); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into modifier_groups (company_id, name) values ($1, 'Extras') returning id`, empresa).Scan(&grupo); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `
		insert into modifier_options (company_id, group_id, name, price_delta, recipe_id) values ($1, $2, 'Perla', 10, $3) returning id`,
		empresa, grupo, receta(perla, "50")).Scan(&extra); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `
		insert into platform_item_links (company_id, connection_id, external_id, kind, local_kind, product_id, modifier_option_id)
		values ($1, $2, 'taro', 'platillo', 'producto', $3, null),
		       ($1, $2, 'perla', 'opcion', 'opcion_de_modificador', null, $4)`,
		empresa, conexion, frappe, extra); err != nil {
		t.Fatalf("emparejar: %v", err)
	}

	svc := app.NewPedidosDePlataformaService(st,
		fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalleConExtraDoble)}}},
		signingKeyCipher, "sandbox", clock)
	cuerpo := avisoDePedido("evt-almacen", "tienda-almacen")
	if _, err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	}); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	var opcionGuardada *int64
	if err := st.Pool.QueryRow(ctx, `
		select modifier_option_id from platform_incoming_order_lines
		 where company_id = $1 and external_item_id = 'perla'`, empresa).Scan(&opcionGuardada); err != nil {
		t.Fatal(err)
	}
	if opcionGuardada == nil || *opcionGuardada != extra {
		t.Fatalf("al registrar, la opción del renglón hijo guarda su pareja del POS: quedó %v", opcionGuardada)
	}

	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	pend, err := svc.Pendientes(ctxT)
	if err != nil || len(pend) != 1 {
		t.Fatalf("pendientes: %v (%d)", err, len(pend))
	}
	vista, err := svc.Aceptar(ctxT, pend[0].ID, usuario)
	if err != nil {
		t.Fatalf("aceptar: %v", err)
	}

	type salida struct {
		ingrediente, opcion int64
	}
	rows, err := st.Pool.Query(ctx, `
		select coalesce(ingredient_id, 0), coalesce(modifier_option_id, 0), sum(quantity)
		  from stock_movements where order_id = $1 group by 1, 2`, vista.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[salida]decimal.Decimal{}
	for rows.Next() {
		var s salida
		var q decimal.Decimal
		if err := rows.Scan(&s.ingrediente, &s.opcion, &q); err != nil {
			t.Fatal(err)
		}
		got[s] = q
	}
	rows.Close()

	// Dos frappés: 400 de leche. La perla es UNA por frappé, así que 100.
	if !got[salida{leche, 0}].Equal(decimal.RequireFromString("-400")) {
		t.Fatalf("el platillo emparejado descuenta su receta por la cantidad: %v", got)
	}
	if !got[salida{perla, extra}].Equal(decimal.RequireFromString("-100")) {
		t.Fatalf("la opción emparejada descuenta la suya, por frappé, con su origen: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("el producto genérico no descuenta nada: %v", got)
	}
}
