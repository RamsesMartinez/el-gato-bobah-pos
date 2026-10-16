//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/shopspring/decimal"
)

// Defectos que encontró la revisión de código del 039: los tres restan o devuelven de más cuando una
// merma se cruza con un conteo abierto, o guardan un costo de otra unidad.

// Aprobar una pendiente que un conteo abierto ya vio la restaba, y al cerrar el conteo la restaba
// otra vez: 2 kg perdidos bajaban 4.
func TestApprovingWasteAnOpenCountSawSubtractsItOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_aprueba_abierto", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	tope := "1"
	setWasteThreshold(t, ctx, svc, actor, &tope)
	ing := pieceIngredient(t, st, "Insumo visto por el conteo abierto")
	adjust(t, st, "ingrediente", ing, "10")
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 50 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	r, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("2"), Actor: actor})
	if err != nil || r.Status != string(domain.WastePending) {
		t.Fatalf("= %+v, %v", r, err)
	}
	c, err := svc.OpenCount(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CaptureCount(ctx, c.ID, ing, qty("8"), actor); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveWaste(ctx, r.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloseCount(ctx, c.ID, actor); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, st, ing); !got.Equal(qty("8")) {
		t.Fatalf("quedaron %s; se contaron 8 y la merma se restó dos veces", got)
	}
}

// Deshacer una merma que un conteo abierto ya capturó devolvía lo que el conteo nunca vio: al
// cerrarlo, el almacén quedaba con piezas inventadas.
func TestUndoingWasteAnOpenCountCapturedIsRefused(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_deshace_abierto", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo deshecho con conteo abierto")
	adjust(t, st, "ingrediente", ing, "10")
	r, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("2"), Actor: actor})
	if err != nil || r.Status != string(domain.WasteApplied) {
		t.Fatalf("= %+v, %v", r, err)
	}
	c, err := svc.OpenCount(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CaptureCount(ctx, c.ID, ing, qty("8"), actor); err != nil {
		t.Fatal(err)
	}
	_ = svc.UndoWaste(ctx, r.ID, actor, true)
	if _, err := svc.CloseCount(ctx, c.ID, actor); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, st, ing); !got.Equal(qty("8")) {
		t.Fatalf("quedaron %s; se contaron 8 y el deshacer inventó piezas", got)
	}
}

// Un producto con receta de un solo insumo copiaba al movimiento del insumo el costo por pieza del
// producto: el gramo quedaba valuado como el frappé entero.
func TestWasteOfAProductKeepsTheIngredientCostOnItsMovement(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_costo_merma", "admin")
	svc, ctx := asWaste(t, defaultCompanyID)
	adminSvc, adminCtx := asApp(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	prod := makeProduct(t, st, "Frappé del costo de la merma", qty("60"), false)
	ing := pieceIngredient(t, st, "Vaso del costo de la merma")
	adjust(t, st, "ingrediente", ing, "10")
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 3 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	if err := adminSvc.SaveComposition(adminCtx, app.CompositionOfProduct, prod, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "2", st)}, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update products set current_cost = 6 where id = $1`, prod); err != nil {
		t.Fatal(err)
	}
	r, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, ProductID: &prod, Qty: qty("1"), Actor: admin})
	if err != nil {
		t.Fatal(err)
	}
	var cost *decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select unit_cost from stock_movements where waste_record_id = $1 and ingredient_id = $2`, r.ID, ing).Scan(&cost); err != nil {
		t.Fatal(err)
	}
	if cost != nil && !cost.Equal(qty("3")) {
		t.Fatalf("el vaso quedó a $%s por pieza; cuesta $3 (el frappé entero cuesta $6)", cost)
	}
}

// Juntar A con B y después B con C dejaba a A apuntando a B, que ya no se ofrece: lo de A salía
// bajo un motivo huérfano y el filtro por C no lo incluía. Juntar mueve también lo que ya apuntaba
// al que se junta, sin tocar los registros.
func TestMergingAMergedIntoReasonKeepsOneLevel(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_junta_cadena", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	mk := func(name string) int64 {
		t.Helper()
		id, err := svc.CreateWasteReason(ctx, name, "cadena de juntar", actor)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b, c := mk("Cadena uno"), mk("Cadena dos"), mk("Cadena tres")
	if err := svc.MergeWasteReason(ctx, a, b, actor); err != nil {
		t.Fatal(err)
	}
	if err := svc.MergeWasteReason(ctx, b, c, actor); err != nil {
		t.Fatal(err)
	}
	var into int64
	if err := st.Pool.QueryRow(ctx, `select merged_into_id from waste_reasons where id = $1`, a).Scan(&into); err != nil {
		t.Fatal(err)
	}
	if into != c {
		t.Fatalf("el primero apunta a %d; quiere %d, el que queda", into, c)
	}
}

// Aprobar, rechazar o deshacer por id no revisaba la sucursal: desde otra sucursal de la misma
// empresa se resolvía una merma ajena y se restaba de un almacén que no era el suyo.
func TestWasteOfAnotherBranchCannotBeResolved(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	company := makeCompany(t, st, "merma-otra-sucursal")
	actor := makeUserIn(t, st, company, "gerente_otra_sucursal", "gerente")
	svc, ctx := asWaste(t, company)
	caduco := reasonNamed(t, st, company, "Caducó")
	var ing int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into ingredients (company_id, name, base_unit_id, current_cost)
		select $1, 'Insumo de la matriz', id, 50 from units where code = 'pieza' returning id`, company).Scan(&ing); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(context.Background(), `insert into business_settings (company_id, business_name) values ($1, 'Otra sucursal')`, company); err != nil {
		t.Fatal(err)
	}
	tope := "1"
	setWasteThreshold(t, ctx, svc, actor, &tope)
	pending, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("2"), Actor: actor})
	if err != nil || pending.Status != string(domain.WastePending) {
		t.Fatalf("= %+v, %v", pending, err)
	}
	setWasteThreshold(t, ctx, svc, actor, nil)
	applied, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: actor})
	if err != nil {
		t.Fatal(err)
	}
	// La sesión pasa a resolver a otra sucursal: alta de la segunda y la matriz queda inactiva.
	addBranch(t, st, company, "SEGUNDA")
	if _, err := st.Pool.Exec(context.Background(),
		`update branches set is_active = false where company_id = $1 and is_headquarters`, company); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveWaste(ctx, pending.ID, actor); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aprobar desde otra sucursal = %v; quiere no encontrada", err)
	}
	if err := svc.RejectWaste(ctx, pending.ID, actor); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rechazar desde otra sucursal = %v; quiere no encontrada", err)
	}
	if err := svc.UndoWaste(ctx, applied.ID, actor, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deshacer desde otra sucursal = %v; quiere no encontrada", err)
	}
}

// «Hoy» mostraba lo capturado con la unidad del insumo: 1 kg de un insumo en gramos salía «1 g»,
// mil veces menos de lo que se restó, y quien revisa no sabe qué deshacer.
func TestWasteOfTodayShowsWhatWasSubtracted(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_hoy_kg", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	var ing int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into ingredients (name, base_unit_id) select 'Insumo en gramos de hoy', id from units where code = 'g' returning id`).Scan(&ing); err != nil {
		t.Fatal(err)
	}
	kg := unitID(t, st, "kg")
	r, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, UnitID: &kg, Qty: qty("1"), Actor: actor})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.WasteToday(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == r.ID {
			if row.UnitCode != "g" || !row.Quantity.Equal(qty("1000")) {
				t.Fatalf("«Hoy» dice %s %s; se restaron 1000 g", row.Quantity, row.UnitCode)
			}
			return
		}
	}
	t.Fatal("la merma no salió en «Hoy»")
}
