//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MERMA ESPERADA QUE DESCUENTA INVENTARIO (spec 096, subida 2). Un frappé lleva 100 g de leche y la
// empresa tiene 10 % general. Con la opción encendida, vender dos descuenta 200 por venta y 20 por
// merma esperada, en dos movimientos. Quitar el renglón repone las dos cosas, y quitar una pieza parte
// las dos: lo que se queda conserva su merma. Apagada, la venta descuenta como siempre.
type deductKit struct {
	st            *store.Store
	svc           *app.OrdersService
	cajero, leche int64
	frappe        int64
}

func newDeductKit(t *testing.T, deducts bool) deductKit {
	t.Helper()
	st := newTestStore(t)
	k := deductKit{st: st, svc: app.NewOrdersService(st, clock)}
	k.cajero = makeUser(t, st, "cajero_merma_desc", "cajero")
	k.leche = ingredientIn(t, st, defaultCompanyID, "Leche merma desc", "g")
	k.frappe = makeProduct(t, st, "Frappé merma desc", pesos("60"), false)
	sinPreparacion(t, st, k.frappe)
	recipe := makeRecipe(t, st, map[int64]string{k.leche: "100"})
	if _, err := st.Pool.Exec(context.Background(), `update products set recipe_id = $2, cost_source = 'receta' where id = $1`, k.frappe, recipe); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(context.Background(), `update business_settings set expected_waste_pct = 10, expected_waste_deducts = $1`, deducts); err != nil {
		t.Fatal(err)
	}
	abrirCajaPrincipal(t, st, k.cajero)
	return k
}

// moved suma lo que se movió de la leche por tipo de movimiento.
func (k deductKit) moved(t *testing.T) map[string]decimal.Decimal {
	t.Helper()
	rows, err := k.st.Pool.Query(context.Background(),
		`select movement_type::text, sum(quantity) from stock_movements where ingredient_id = $1 group by 1`, k.leche)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]decimal.Decimal{}
	for rows.Next() {
		var kind string
		var q decimal.Decimal
		if err := rows.Scan(&kind, &q); err != nil {
			t.Fatal(err)
		}
		out[kind] = q
	}
	return out
}

func (k deductKit) sell(t *testing.T, qty string) *app.OrderView {
	t.Helper()
	o, err := k.svc.Create(context.Background(), app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.cajero,
		Lines: []domain.OrderLineInput{{ProductID: k.frappe, Qty: pesos(qty)}}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestExpectedWasteDeductsOnlyWhenOn(t *testing.T) {
	t.Parallel()
	off := newDeductKit(t, false)
	off.sell(t, "2")
	if m := off.moved(t); !m["venta"].Equal(pesos("-200")) || !m["merma_esperada"].IsZero() {
		t.Fatalf("apagada descontó %v; quiere solo la venta de -200", m)
	}

	on := newDeductKit(t, true)
	on.sell(t, "2")
	if m := on.moved(t); !m["venta"].Equal(pesos("-200")) || !m["merma_esperada"].Equal(pesos("-20")) {
		t.Fatalf("encendida descontó %v; quiere -200 de venta y -20 de merma esperada", m)
	}
}

// Revisión de arquitectura (R14 H2): cancelar el renglón dejaba la merma esperada descontada para
// siempre, y el conteo la habría leído como faltante.
func TestCancellingALineGivesBackTheExpectedWaste(t *testing.T) {
	t.Parallel()
	k := newDeductKit(t, true)
	o := k.sell(t, "2")
	if _, err := k.svc.CancelarRenglon(context.Background(), o.ID, o.Lines[0].ID, k.cajero, "el cliente cambió de opinión"); err != nil {
		t.Fatal(err)
	}
	total := decimal.Zero
	for _, q := range k.moved(t) {
		total = total.Add(q)
	}
	if !total.IsZero() {
		t.Fatalf("tras cancelar el renglón, la leche quedó movida %s (%v): la merma esperada no volvió", total, k.moved(t))
	}
	// Revisión de código: la merma vuelve como merma_esperada, no como cancelación. Si volvía como
	// cancelación, el reporte contra ventas leía «vendido −20» y la merma real un consumo negativo.
	m := k.moved(t)
	if sold := m["venta"].Add(m["cancelacion"]); !sold.IsZero() || !m["merma_esperada"].IsZero() {
		t.Fatalf("tras cancelar, vendido neto %s y merma esperada neta %s (%v); los dos deben quedar en 0", sold, m["merma_esperada"], m)
	}
}

func TestRemovingOnePieceSplitsTheExpectedWaste(t *testing.T) {
	t.Parallel()
	k := newDeductKit(t, true)
	o := k.sell(t, "2")
	uno := pesos("1")
	if _, err := k.svc.RemovePieces(context.Background(), o.ID, o.Lines[0].ID, k.cajero, "sobró uno", &uno); err != nil {
		t.Fatal(err)
	}
	total := decimal.Zero
	for _, q := range k.moved(t) {
		total = total.Add(q)
	}
	// Se queda un frappé: 100 de venta y 10 de merma.
	if !total.Equal(pesos("-110")) {
		t.Fatalf("tras quitar una pieza la leche quedó en %s (%v); quiere -110", total, k.moved(t))
	}
	var merma decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `
		select coalesce(sum(quantity), 0) from stock_movements
		 where ingredient_id = $1 and movement_type = 'merma_esperada' and order_line_id = $2`, k.leche, o.Lines[0].ID).Scan(&merma); err != nil {
		t.Fatal(err)
	}
	if !merma.Equal(pesos("-10")) {
		t.Fatalf("el renglón que se queda tiene %s de merma esperada; quiere -10 (partirla la volvía venta)", merma)
	}
}

// Encender la opción queda en la bitácora, solo toca lo que se venda después y otra empresa no la
// cambia (los tres casos de RLS).
func TestTurningOnTheDeductionIsAuditedAndIsolated(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	if err := app.NewCostingService(st).SetExpectedWasteDeducts(ctx, true, k.admin); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := owner.Pool.QueryRow(context.Background(), `select after::text from audit_events
		where action = 'settings.expected_waste_deducts' and entity_id = $1 order by id desc limit 1`, k.company).Scan(&after); err != nil || after == "" {
		t.Fatalf("encender la opción no quedó en la bitácora (%v)", err)
	}
	other := makeCompany(t, owner, "otra-descuenta")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		_ = app.NewCostingService(st).SetExpectedWasteDeducts(ctx, false, k.admin)
	})
	var on bool
	if err := owner.Pool.QueryRow(context.Background(), `select expected_waste_deducts from business_settings where company_id = $1`, defaultCompanyID).Scan(&on); err != nil || !on {
		t.Fatalf("otra sesión apagó la opción de la empresa dueña (%v, %v)", on, err)
	}
}
