//go:build integration

package integration

import (
	"context"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL ALMACÉN DESCUENTA LO QUE DE VERDAD SE VENDIÓ (spec 028).

func onHand(t *testing.T, st *store.Store, product int64) decimal.Decimal {
	t.Helper()
	var q decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(on_hand), 0) from stock_levels where product_id = $1`, product).Scan(&q); err != nil {
		t.Fatal(err)
	}
	return q
}

// CANCELAR UN RENGLÓN Y DESPUÉS EL PEDIDO REPONE UNA SOLA VEZ.
//
// `RestockCancelledOrder` invertía TODOS los movimientos de venta del pedido: lo que ya se había
// repuesto al cancelar un renglón volvía a entrar, y lo que ya se había preparado también. Con los
// extras y los componentes descontando, el sobrante falso crece con cada cancelación.
func TestCancellingALineThenTheOrderRestocksOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_reponer", "cajero")
	soda := makeProduct(t, st, "Refresco reponer", decimal.RequireFromString("30"), true)
	water := makeProduct(t, st, "Agua reponer", decimal.RequireFromString("20"), true)
	// Una botella no se prepara: es lo que hace que vuelva al almacén al cancelar aunque ya haya
	// «salido a cocina». Lo que sí se prepara y ya salió no vuelve (spec 031, D11).
	sinPreparacion(t, st, water)
	abrirCajaPrincipal(t, st, cashier)
	svc := app.NewOrdersService(st, clock)

	order, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{
			{ProductID: soda, Qty: decimal.RequireFromString("2")},
			{ProductID: water, Qty: decimal.RequireFromString("1")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sodaLine int64
	for _, l := range order.Lines {
		if l.ProductName == "Refresco reponer" {
			sodaLine = l.ID
		}
	}
	// Confirmar manda todo a cocina; un renglón que todavía no sale (uno agregado y no enviado) es el
	// que se repone al cancelarlo. Se simula ese estado.
	if _, err := st.Pool.Exec(ctx, `update order_lines set enviado_a_cocina_at = null where id = $1`, sodaLine); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelarRenglon(ctx, order.ID, sodaLine, cashier, "el cliente cambió de opinión"); err != nil {
		t.Fatalf("cancelar el renglón: %v", err)
	}
	if got := onHand(t, st, soda); !got.IsZero() {
		t.Fatalf("cancelar el renglón antes de cocina repone el refresco: quería 0, hay %s", got)
	}
	if err := svc.CancelarConDevolucion(ctx, app.CancelacionCmd{OrderID: order.ID, Motivo: "se fue el cliente", ActorID: cashier}); err != nil {
		t.Fatalf("cancelar el pedido: %v", err)
	}
	if got := onHand(t, st, soda); !got.IsZero() {
		t.Fatalf("el refresco se repuso dos veces: quedaron %s de más en el almacén", got)
	}
	if got := onHand(t, st, water); !got.IsZero() {
		t.Fatalf("el agua se repone al cancelar el pedido, como hoy: quería 0, hay %s", got)
	}
}

func onHandIngredient(t *testing.T, st *store.Store, ingredient int64) decimal.Decimal {
	t.Helper()
	var q decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(on_hand), 0) from stock_levels where ingredient_id = $1`, ingredient).Scan(&q); err != nil {
		t.Fatal(err)
	}
	return q
}

func makeIngredient(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into ingredients (name, base_unit_id) select $1, id from units where code = 'g' returning id`,
		name).Scan(&id); err != nil {
		t.Fatalf("insumo %s: %v", name, err)
	}
	return id
}

// makeRecipe crea una receta en gramos: insumo → cantidad.
func makeRecipe(t *testing.T, st *store.Store, items map[int64]string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := st.Pool.QueryRow(ctx, `insert into recipes default values returning id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for ing, qty := range items {
		if _, err := st.Pool.Exec(ctx, `
			insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id)
			select $1, $2, $3::numeric, id from units where code = 'g'`, id, ing, qty); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

type movimiento struct {
	itemType            string
	item                int64
	option, componentOf int64
	branchOK            bool
}

// movimientosDelRenglon suma por insumo/producto y origen lo que salió por un renglón.
func movimientosDelRenglon(t *testing.T, st *store.Store, line int64) map[movimiento]decimal.Decimal {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(), `
		select sm.item_type::text, coalesce(sm.ingredient_id, sm.product_id),
		       coalesce(sm.modifier_option_id, 0), coalesce(sm.component_of_product_id, 0),
		       sm.branch_id = o.branch_id, sum(sm.quantity)
		  from stock_movements sm join orders o on o.id = sm.order_id
		 where sm.order_line_id = $1
		 group by 1, 2, 3, 4, 5`, line)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[movimiento]decimal.Decimal{}
	for rows.Next() {
		var m movimiento
		var q decimal.Decimal
		if err := rows.Scan(&m.itemType, &m.item, &m.option, &m.componentOf, &m.branchOK, &q); err != nil {
			t.Fatal(err)
		}
		out[m] = q
	}
	return out
}

// LA VENTA DE MOSTRADOR DESCUENTA LOS EXTRAS Y LOS COMPONENTES DEL PAQUETE.
//
// Antes solo bajaba el producto principal: la perla extra, el refresco del combo y lo que lleva un
// paquete salían del local sin tocar el almacén, y el inventario se veía más lleno cada día.
func TestCounterSaleDepletesExtrasAndPackages(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_extras", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	svc := app.NewOrdersService(st, clock)

	milk := makeIngredient(t, st, "Leche extras")
	pearl := makeIngredient(t, st, "Perla extras")
	frappe := makeProduct(t, st, "Frappé extras", decimal.RequireFromString("70"), false)
	soda := makeProduct(t, st, "Refresco extras", decimal.RequireFromString("30"), true)
	if _, err := st.Pool.Exec(ctx, `update products set recipe_id = $1 where id = $2`,
		makeRecipe(t, st, map[int64]string{milk: "200"}), frappe); err != nil {
		t.Fatal(err)
	}
	pearlExtra := opcionConTope(t, st, "Extras almacén", "Perla extra", decimal.RequireFromString("10"), 3)
	sodaExtra := opcionConTope(t, st, "Extras almacén", "Refresco del combo", decimal.RequireFromString("20"), 1)
	noIce := opcionConTope(t, st, "Extras almacén", "Sin hielo", decimal.Zero, 1)
	if _, err := st.Pool.Exec(ctx, `update modifier_options set recipe_id = $1 where id = $2`,
		makeRecipe(t, st, map[int64]string{pearl: "50"}), pearlExtra); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update modifier_options set linked_product_id = $1 where id = $2`, soda, sodaExtra); err != nil {
		t.Fatal(err)
	}

	pkg := makeProduct(t, st, "Paquete extras", decimal.RequireFromString("120"), false)
	if _, err := st.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, pkg); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []struct {
		product int64
		min     int
	}{{frappe, 1}, {soda, 2}} {
		if _, err := st.Pool.Exec(ctx, `
			with s as (insert into combo_slots (combo_id, name, min_select, max_select) values ($1, 'hueco', $3, $3) returning id)
			insert into combo_slot_products (slot_id, product_id, is_default) select id, $2, true from s`,
			pkg, slot.product, slot.min); err != nil {
			t.Fatal(err)
		}
	}

	order, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{
			{ProductID: frappe, Qty: decimal.RequireFromString("2"), Modifiers: []domain.OrderModInput{
				{OptionID: pearlExtra, Qty: 2}, {OptionID: sodaExtra, Qty: 1}, {OptionID: noIce, Qty: 1},
			}},
			{ProductID: pkg, Qty: decimal.RequireFromString("1")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var frappeLine, pkgLine int64
	for _, l := range order.Lines {
		switch l.ProductName {
		case "Frappé extras":
			frappeLine = l.ID
		case "Paquete extras":
			pkgLine = l.ID
		}
	}

	got := movimientosDelRenglon(t, st, frappeLine)
	want := map[movimiento]string{
		{itemType: "ingrediente", item: milk, branchOK: true}:                      "-400",
		{itemType: "ingrediente", item: pearl, option: pearlExtra, branchOK: true}: "-200",
		{itemType: "producto", item: soda, option: sodaExtra, branchOK: true}:      "-2",
	}
	if len(got) != len(want) {
		t.Fatalf("el frappé con extras debe dejar %d movimientos con su origen y en la sucursal del pedido; dejó %v", len(want), got)
	}
	for k, v := range want {
		if !got[k].Equal(decimal.RequireFromString(v)) {
			t.Fatalf("%+v: quería %s, salió %s (todo: %v)", k, v, got[k], got)
		}
	}

	got = movimientosDelRenglon(t, st, pkgLine)
	if !got[movimiento{itemType: "ingrediente", item: milk, componentOf: pkg, branchOK: true}].Equal(decimal.RequireFromString("-200")) ||
		!got[movimiento{itemType: "producto", item: soda, componentOf: pkg, branchOK: true}].Equal(decimal.RequireFromString("-2")) {
		t.Fatalf("el paquete descuenta lo que lleva, con el paquete como origen: %v", got)
	}
	var comps int
	if err := st.Pool.QueryRow(ctx, `
		select count(*) from order_line_components
		 where order_line_id = $1 and ((product_id = $2 and quantity = 1) or (product_id = $3 and quantity = 2))`,
		pkgLine, frappe, soda).Scan(&comps); err != nil {
		t.Fatal(err)
	}
	if comps != 2 {
		t.Fatalf("los componentes vendidos se copian al renglón: hay %d de 2", comps)
	}

	// Cancelar el renglón antes de cocina repone TODO lo que salió por él, extras incluidos.
	if _, err := st.Pool.Exec(ctx, `update order_lines set enviado_a_cocina_at = null where id = $1`, frappeLine); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelarRenglon(ctx, order.ID, frappeLine, cashier, "se equivocó de sabor"); err != nil {
		t.Fatal(err)
	}
	for k, v := range movimientosDelRenglon(t, st, frappeLine) {
		if !v.IsZero() {
			t.Fatalf("cancelar el renglón dejó %s de %+v sin reponer", v, k)
		}
	}
	if got := onHandIngredient(t, st, pearl); !got.IsZero() {
		t.Fatalf("la perla extra se repone con el renglón: quedó %s", got)
	}
}

// EL MOVIMIENTO DE UNA VENTA QUEDA EN LA SUCURSAL DEL PEDIDO.
//
// La 0076 llenaba la sucursal de un movimiento con «la de la empresa», como cualquier fila sin
// sucursal. La venta no la pasa, así que con dos sucursales cada venta tronaba con BRANCH_AMBIGUOUS:
// el segundo local no podía vender nada. La prueba de la 0076 insertaba el movimiento con la
// sucursal ya puesta, y por eso no lo vio.
func TestASaleMovementTakesTheBranchOfItsOrder(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, owner, "cajero_sur", "cajero")
	product := makeProduct(t, owner, "Galleta sur", decimal.RequireFromString("20.00"), true)
	southBranch, southRegister := addBranch(t, owner, defaultCompanyID, "SUR")

	var session, order int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id) values ($1, 0, $2, $3) returning id`,
		fixedNow, cashier, southRegister).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if err := owner.Pool.QueryRow(ctx,
		`insert into orders (client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id)
		 values (gen_random_uuid(), current_date, 1, 'mostrador', 20, 20, $1, $2) returning id`,
		cashier, session).Scan(&order); err != nil {
		t.Fatal(err)
	}
	var branch int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into stock_movements (item_type, product_id, movement_type, quantity, order_id, user_id)
		 values ('producto', $1, 'venta', -1, $2, $3) returning branch_id`, product, order, cashier).Scan(&branch); err != nil {
		t.Fatalf("con dos sucursales, la venta del segundo local no puede descontar: %v", err)
	}
	if branch != southBranch {
		t.Fatalf("el movimiento debe quedar en la sucursal del pedido (%d), quedó en %d", southBranch, branch)
	}
}
