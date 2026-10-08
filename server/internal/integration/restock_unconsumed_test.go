//go:build integration

package integration

import (
	"context"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Un producto sin preparación —el refresco embotellado— vuelve al almacén al quitarlo, aunque su
// renglón haya nacido «enviado a cocina» como todos. Antes solo reponía lo que no había salido a
// cocina, y como todo renglón nace enviado, en la práctica nunca reponía: cada refresco quitado
// era una merma inventada. Sin desmarcar la cocina a mano: así está en producción.
func TestRemovingAProductWithoutPrepRestocksIt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_refresco_quitado", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	refresco := makeProduct(t, st, "Refresco embotellado", pesos("25"), true)
	sinPreparacion(t, st, refresco)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)

	antes := existencias(t, st, refresco)
	ord := crearPedidoSimple(t, tctx, svc, refresco, cajero)
	linea, enviado := primerRenglon(t, st, ord)
	if !enviado {
		t.Fatal("el renglón nació sin enviar a cocina: la prueba ya no reproduce producción")
	}
	repuso, err := svc.CancelarRenglon(tctx, ord, linea, cajero, "Ya no lo quiere")
	if err != nil {
		t.Fatalf("CancelarRenglon: %v", err)
	}
	if !repuso {
		t.Fatal("quitar un refresco sin preparación no repuso: el almacén pierde uno que sigue en el refri")
	}
	if e := existencias(t, st, refresco); !e.Equal(antes) {
		t.Fatalf("existencias %s → %s: el refresco no volvió", antes, e)
	}
}

// Cancelar el pedido después de quitarle un renglón repone cada pieza UNA vez, en las dos formas:
//   - el renglón ya repuesto al quitarlo (sin preparación) no vuelve a reponerse;
//   - el renglón quitado ya consumido (salió a cocina) no se repone al cancelar el pedido: esa
//     comida se hizo, y reponerla inventaría existencias.
func TestCancellingAfterRemovingALineRestocksOnce(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_repone_una_vez", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	refresco := makeProduct(t, st, "Refresco repuesto", pesos("25"), true)
	sinPreparacion(t, st, refresco)
	frappe := makeProduct(t, st, "Frappé consumido", pesos("60"), true)
	queda := makeProduct(t, st, "Café que queda", pesos("40"), true)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)

	antes := map[int64]string{}
	for _, p := range []int64{refresco, frappe, queda} {
		antes[p] = existencias(t, st, p).String()
	}
	ord, err := svc.Create(tctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{
			{ProductID: refresco, Qty: pesos("1")},
			{ProductID: frappe, Qty: pesos("1")},
			{ProductID: queda, Qty: pesos("1")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, l := range ord.Lines[:2] {
		if _, err := svc.CancelarRenglon(tctx, ord.ID, l.ID, cajero, "Ya no lo quiere"); err != nil {
			t.Fatalf("CancelarRenglon %d: %v", l.ID, err)
		}
	}
	if err := svc.CancelarConDevolucion(tctx, app.CancelacionCmd{OrderID: ord.ID, Motivo: "Se equivocó el pedido", ActorID: cajero}); err != nil {
		t.Fatalf("cancelar el pedido: %v", err)
	}

	cases := []struct {
		name string
		id   int64
		want string
	}{
		{"el repuesto al quitarlo no se repone otra vez", refresco, antes[refresco]},
		{"el consumido no se repone al cancelar", frappe, pesos(antes[frappe]).Sub(pesos("1")).String()},
		{"el que seguía vivo se repone al cancelar", queda, antes[queda]},
	}
	for _, c := range cases {
		if e := existencias(t, st, c.id); e.String() != c.want {
			t.Errorf("%s: existencias = %s, quiere %s", c.name, e, c.want)
		}
	}
}

// PARTIR UN RENGLÓN CONSERVA DE QUÉ EXTRA O PAQUETE SALIÓ CADA DESCUENTO.
//
// «Quitar los que faltan» parte el renglón y reparte sus movimientos en pares. Si el par no copia
// `modifier_option_id` ni `component_of_product_id`, la perla extra y lo que lleva el paquete quedan
// como si fueran el producto mismo: el reporte de unidades por extra y por paquete deja de cuadrar, y
// reponer después ese renglón revertiría otro origen.
func TestSplittingALineKeepsTheOriginOfEachMovement(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_parte_con_origen", "cajero")
	abrirCajaPrincipal(t, st, cajero)

	milk := makeIngredient(t, st, "Leche partida")
	pearl := makeIngredient(t, st, "Perla partida")
	frappe := makeProduct(t, st, "Frappé con perla partida", pesos("70"), false)
	soda := makeProduct(t, st, "Refresco del paquete partido", pesos("30"), true)
	if _, err := st.Pool.Exec(ctx, `update products set recipe_id = $1 where id = $2`,
		makeRecipe(t, st, map[int64]string{milk: "200"}), frappe); err != nil {
		t.Fatal(err)
	}
	pearlExtra := opcionConTope(t, st, "Extras partidos", "Perla extra partida", pesos("10"), 3)
	if _, err := st.Pool.Exec(ctx, `update modifier_options set recipe_id = $1 where id = $2`,
		makeRecipe(t, st, map[int64]string{pearl: "50"}), pearlExtra); err != nil {
		t.Fatal(err)
	}
	pkg := makeProduct(t, st, "Paquete partido", pesos("120"), false)
	if _, err := st.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, pkg); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `
		with s as (insert into combo_slots (combo_id, name, min_select, max_select) values ($1, 'hueco', 2, 2) returning id)
		insert into combo_slot_products (slot_id, product_id, is_default) select id, $2, true from s`, pkg, soda); err != nil {
		t.Fatal(err)
	}

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)

	ord, err := svc.Create(tctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{
			{ProductID: frappe, Qty: pesos("3"), Modifiers: []domain.OrderModInput{{OptionID: pearlExtra, Qty: 1}}},
			{ProductID: pkg, Qty: pesos("2")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	frappeLine, pkgLine := ord.Lines[0].ID, ord.Lines[1].ID
	for _, l := range []int64{frappeLine, pkgLine} {
		if err := svc.DeliverLine(tctx, ord.ID, l, pesos("1")); err != nil {
			t.Fatalf("entregar una pieza: %v", err)
		}
	}
	if _, err := svc.CancelPending(tctx, ord.ID, cajero, "Ya no lo quiere"); err != nil {
		t.Fatalf("CancelPending: %v", err)
	}

	splitOff := func(orig int64) int64 {
		t.Helper()
		var id int64
		if err := st.Pool.QueryRow(ctx, `
			select id from order_lines where order_id = $1 and product_id = (select product_id from order_lines where id = $2)
			   and id <> $2`, ord.ID, orig).Scan(&id); err != nil {
			t.Fatalf("el renglón %d no se partió: %v", orig, err)
		}
		return id
	}
	cases := []struct {
		name string
		line int64
		want map[movimiento]string
	}{
		{"lo entregado del frappé", frappeLine, map[movimiento]string{
			{itemType: "ingrediente", item: milk, branchOK: true}:                      "-200",
			{itemType: "ingrediente", item: pearl, option: pearlExtra, branchOK: true}: "-50",
		}},
		{"lo quitado del frappé", splitOff(frappeLine), map[movimiento]string{
			{itemType: "ingrediente", item: milk, branchOK: true}:                      "-400",
			{itemType: "ingrediente", item: pearl, option: pearlExtra, branchOK: true}: "-100",
		}},
		{"lo entregado del paquete", pkgLine, map[movimiento]string{
			{itemType: "producto", item: soda, componentOf: pkg, branchOK: true}: "-2",
		}},
		{"lo quitado del paquete", splitOff(pkgLine), map[movimiento]string{
			{itemType: "producto", item: soda, componentOf: pkg, branchOK: true}: "-2",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := movimientosDelRenglon(t, st, c.line)
			for k, v := range got {
				if _, ok := c.want[k]; !ok && !v.IsZero() {
					t.Errorf("sobra %s de %+v: el par perdió de qué extra o paquete salió", v, k)
				}
			}
			for k, v := range c.want {
				if !got[k].Equal(pesos(v)) {
					t.Errorf("%+v: quería %s, salió %s (todo: %v)", k, v, got[k], got)
				}
			}
		})
	}
}
