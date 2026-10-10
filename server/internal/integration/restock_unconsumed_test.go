//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Un producto sin preparación —el refresco embotellado— vuelve al almacén al quitarlo, aunque su
// renglón haya nacido «enviado a cocina» como todos. Antes solo reponía lo que no había salido a
// cocina, y como todo renglón nace enviado, en la práctica nunca reponía: cada refresco quitado
// era una merma inventada. Sin desmarcar la cocina a mano: así está en producción.
func TestRemovingAProductWithoutPrepRestocksIt(t *testing.T) {
	t.Parallel()
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
//     comida se hizo, y reponerla inventaría existencias;
//   - el renglón que sigue vivo y ya salió a cocina tampoco: cancelar el pedido sigue la MISMA regla
//     que quitarlo (spec 031, D11). Antes este caso esperaba reponerlo, que era el defecto.
func TestCancellingAfterRemovingALineRestocksOnce(t *testing.T) {
	t.Parallel()
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
	if err := svc.CancelarConDevolucion(tctx, app.CancelacionCmd{CardFolio: "F-1", OrderID: ord.ID, Motivo: "Se equivocó el pedido", ActorID: cajero}); err != nil {
		t.Fatalf("cancelar el pedido: %v", err)
	}

	cases := []struct {
		name string
		id   int64
		want string
	}{
		{"el repuesto al quitarlo no se repone otra vez", refresco, antes[refresco]},
		{"el consumido no se repone al cancelar", frappe, pesos(antes[frappe]).Sub(pesos("1")).String()},
		{"el que seguía vivo y ya salió a cocina no se repone al cancelar", queda, pesos(antes[queda]).Sub(pesos("1")).String()},
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
	t.Parallel()
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
	pearlExtra := opcionConTope(t, st, "Extras partidos", "Perla extra partida", pesos("10"), 3, frappe)
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
	// Lo que lleva el paquete se reparte igual: cada mitad dice cuántos refrescos lleva la suya. Sin
	// copiarlo, el renglón quitado sale como un paquete vacío y el entregado como si llevara los
	// cuatro, y «unidades vendidas dentro de paquetes» cuenta los del paquete que no se entregó.
	for _, l := range []int64{pkgLine, splitOff(pkgLine)} {
		var q decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `select coalesce(sum(quantity), 0) from order_line_components where order_line_id = $1 and product_id = $2`,
			l, soda).Scan(&q); err != nil {
			t.Fatal(err)
		}
		if !q.Equal(pesos("2")) {
			t.Errorf("el renglón %d del paquete lleva %s refrescos; cada mitad lleva 2", l, q)
		}
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

// QUITAR PIEZAS DE UN RENGLÓN: 1 DE 2 PARTE EL RENGLÓN, Y NO SE QUITAN MÁS DE LAS QUE FALTAN.
func TestRemovingSomePiecesSplitsTheLine(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	s := newSplitTable(t, st, "quitar_una_de_dos", "50")
	ctx := context.Background()
	// Un pedido aparte con un renglón de dos piezas.
	two, err := s.svc.Create(s.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: s.cashier,
		Lines: []domain.OrderLineInput{{ProductID: productOf(t, st, s.order.Lines[0].ID), Qty: pesos("2")}}})
	if err != nil {
		t.Fatal(err)
	}
	s.order = two
	if _, err := s.svc.RemovePieces(s.ctx, s.order.ID, s.order.Lines[0].ID, s.cashier, "Ya no lo quiere", ptrDec(pesos("3"))); !errors.Is(err, domain.ErrTooManyPieces) {
		t.Fatalf("quitar 3 de 2 = %v; quiere «No hay tantas piezas por quitar»", err)
	}
	if _, err := s.svc.RemovePieces(s.ctx, s.order.ID, s.order.Lines[0].ID, s.cashier, "Ya no lo quiere", ptrDec(pesos("1"))); err != nil {
		t.Fatalf("quitar 1 de 2: %v", err)
	}
	rows, err := st.Pool.Query(ctx, `select quantity, line_total, cancelled_at is not null from order_lines where order_id = $1 order by id`, s.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var q, total decimal.Decimal
		var cancelled bool
		if err := rows.Scan(&q, &total, &cancelled); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s:%s:%v", q.String(), total.StringFixed(2), cancelled))
	}
	if len(got) != 2 || got[0] != "1:50.00:false" || got[1] != "1:50.00:true" {
		t.Fatalf("renglones = %v; quiere uno vivo de 1 por 50 y uno quitado de 1 por 50", got)
	}
	var total decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select total from orders where id = $1`, s.order.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if !total.Equal(pesos("50")) {
		t.Fatalf("total %s, quiere 50", total)
	}
}

func ptrDec(d decimal.Decimal) *decimal.Decimal { return &d }

func productOf(t *testing.T, st *store.Store, line int64) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `select product_id from order_lines where id = $1`, line).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// CADA MITAD DE UN RENGLÓN PARTIDO REPONE SOLO LA SUYA.
//
// Dos refrescos sin preparación: quitar uno y luego el otro devuelve exactamente dos al refri. Si la
// primera reposición revirtiera todo el renglón, o la segunda repusiera otra vez lo de la primera,
// el almacén inventaría existencias.
func TestASplitLineRestocksEachHalfOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cashier := makeUser(t, st, "cajero_mitades", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	soda := makeProduct(t, st, "Refresco de dos", pesos("25"), true)
	sinPreparacion(t, st, soda)
	appSt := appRoleStore(t)
	ctx, release, err := appSt.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)
	before := existencias(t, st, soda)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: soda, Qty: pesos("2")}, {ProductID: soda, Qty: pesos("1")}}})
	if err != nil {
		t.Fatal(err)
	}
	line := ord.Lines[0].ID
	if _, err := svc.RemovePieces(ctx, ord.ID, line, cashier, "Ya no lo quiere", ptrDec(pesos("1"))); err != nil {
		t.Fatalf("quitar el primero: %v", err)
	}
	if got := existencias(t, st, soda); !got.Equal(before.Sub(pesos("2"))) {
		t.Fatalf("tras quitar uno: existencias %s, quiere %s", got, before.Sub(pesos("2")))
	}
	if _, err := svc.RemovePieces(ctx, ord.ID, line, cashier, "Ya no lo quiere", nil); err != nil {
		t.Fatalf("quitar el otro: %v", err)
	}
	if got := existencias(t, st, soda); !got.Equal(before.Sub(pesos("1"))) {
		t.Fatalf("tras quitar los dos: existencias %s, quiere %s (queda solo el tercer refresco vendido)", got, before.Sub(pesos("1")))
	}
}

// QUITAR LO QUE FALTA RESPETA LO PAGADO, CON LA MISMA REGLA QUE QUITAR UN PRODUCTO.
func TestCancelPendingRespectsPayments(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	r, token := ordersAPI(t, st, nil)
	_, tok := token("http_quitar_con_pagos", "cajero")
	s := newSplitTable(t, st, "pendiente_pagado", "50", "50")
	post := func(orderID int64) (int, string) {
		w := do(t, r, http.MethodPost, "/api/v1/orders/"+strconv.FormatInt(orderID, 10)+"/lines/cancel-pending", tok,
			[]byte(`{"reason":"Ya no lo quiere"}`), "application/json")
		var m map[string]map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		msg, _ := m["error"]["message"].(string)
		return w.Code, msg
	}
	s.pay(t, 0)
	if code, msg := post(s.order.ID); code != http.StatusConflict || msg != "Ese producto ya se pagó. Primero hay que devolver el pago" {
		t.Errorf("pendientes pagados = %d %q", code, msg)
	}

	m := newSplitTable(t, st, "pendiente_sobrepagado", "50", "50")
	if _, err := m.svc.Charge(m.ctx, app.ChargeCmd{OrderID: m.order.ID, MethodID: m.cash, Amount: pesos("80"), ActorID: m.cashier}); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.DeliverLine(m.ctx, m.order.ID, m.order.Lines[0].ID, pesos("1")); err != nil {
		t.Fatal(err)
	}
	if code, msg := post(m.order.ID); code != http.StatusConflict || msg != "Ya se cobró más de lo que quedaría. Primero hay que devolver un pago" {
		t.Errorf("dejaría el total bajo lo pagado = %d %q", code, msg)
	}
}

// QUITAR PIEZAS POR HTTP: qty de más es un 400 con su texto.
func TestRemovePiecesHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	r, token := ordersAPI(t, st, nil)
	_, tok := token("http_quitar_piezas", "cajero")
	s := newSplitTable(t, st, "quitar_http", "50")
	w := do(t, r, http.MethodPost, "/api/v1/orders/"+strconv.FormatInt(s.order.ID, 10)+"/lines/"+strconv.FormatInt(s.order.Lines[0].ID, 10)+"/cancel",
		tok, []byte(`{"reason":"Ya no lo quiere","qty":"2"}`), "application/json")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "No hay tantas piezas por quitar") {
		t.Fatalf("= %d %s", w.Code, w.Body.String())
	}
}
