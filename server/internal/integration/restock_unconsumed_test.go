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
