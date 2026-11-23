//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// MANDAR A MERMA UN LOTE DESDE CADUCIDADES (spec 098, historia 4).
func TestWastingALotTakesFromThatLot(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_lote_merma", "cajero")
	gerente := makeUser(t, st, "gerente_lote_merma", "gerente")
	svc, sctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Yogur lotes")
	var today time.Time
	if err := st.Pool.QueryRow(ctx, `select business_today($1)`, defaultCompanyID).Scan(&today); err != nil {
		t.Fatal(err)
	}
	old, good := today.AddDate(0, 0, -1).Format(time.DateOnly), today.AddDate(0, 0, 5).Format(time.DateOnly)
	for _, m := range [][2]string{{"6", old}, {"4", good}} {
		if _, err := st.Pool.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, expires_on)
			values ('ingrediente', $1, 'compra', $2::numeric, $3::date)`, ing, m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}
	lot := func(d string) decimal.Decimal {
		var v decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `select qty from stock_lots where ingredient_id = $1 and expires_on = $2::date`, ing, d).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	// Parcial: 2 del caducado, y el bueno no se toca.
	if _, err := svc.RecordWaste(sctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("2"), Actor: cajero, LotExpiresOn: &old}); err != nil {
		t.Fatal(err)
	}
	if v := lot(old); !v.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("caducado = %s, quería 4", v)
	}
	if v := lot(good); !v.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("bueno = %s, quería 4 (la merma de un lote no toca otro)", v)
	}

	// Con aprobación: el lote baja al aprobar, aunque la aprobación la haga la versión anterior (sin
	// fecha en el movimiento: la toma del registro, R12 A2).
	tope := "0.01"
	setWasteThreshold(t, sctx, svc, gerente, &tope)
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 5 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	res, err := svc.RecordWaste(sctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("4"), Actor: cajero, LotExpiresOn: &old})
	if err != nil || res.Status != string(domain.WastePending) {
		t.Fatalf("= %+v, %v", res, err)
	}
	if v := lot(old); !v.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("una merma por aprobar ya restó del lote: %s", v)
	}
	if err := svc.ApproveWaste(sctx, res.ID, gerente); err != nil {
		t.Fatal(err)
	}
	if v := lot(old); !v.IsZero() {
		t.Fatalf("caducado tras aprobar = %s, quería 0", v)
	}

	// Deshacer la merma la devuelve al mismo lote.
	if err := svc.UndoWaste(sctx, res.ID, gerente, true); err != nil {
		t.Fatal(err)
	}
	if v := lot(old); !v.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("caducado tras deshacer = %s, quería 4", v)
	}

	// Un lote solo es de un artículo, y su fecha se valida.
	prod := makeProduct(t, st, "Lata lotes", qty("20"), true)
	if _, err := svc.RecordWaste(sctx, app.RecordWasteCmd{ReasonID: caduco, ProductID: &prod, Qty: qty("1"), Actor: cajero, LotExpiresOn: &old}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("lote en un producto = %v, quería validación", err)
	}
	bad := "1999-01-01"
	if _, err := svc.RecordWaste(sctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero, LotExpiresOn: &bad}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fecha absurda = %v, quería validación", err)
	}
}

// Deshacer una merma de un lote que tomó parte de lo sin fecha devuelve a cada uno lo suyo
// (revisión de base de datos, spec 098).
func TestUndoingALotWasteGivesBackWhatEachLent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_deshacer_lote", "cajero")
	gerente := makeUser(t, st, "gerente_deshacer_lote", "gerente")
	svc, sctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Leche deshacer lote")
	var today time.Time
	if err := st.Pool.QueryRow(ctx, `select business_today($1)`, defaultCompanyID).Scan(&today); err != nil {
		t.Fatal(err)
	}
	d := today.AddDate(0, 0, -1).Format(time.DateOnly)
	adjust(t, st, "ingrediente", ing, "5")
	if _, err := st.Pool.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, expires_on)
		values ('ingrediente', $1, 'compra', 5, $2::date)`, ing, d); err != nil {
		t.Fatal(err)
	}
	res, err := svc.RecordWaste(sctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("8"), Actor: cajero, LotExpiresOn: &d})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UndoWaste(sctx, res.ID, gerente, true); err != nil {
		t.Fatal(err)
	}
	var lot decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select qty from stock_lots where ingredient_id = $1`, ing).Scan(&lot); err != nil || !lot.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("lote tras deshacer = %s (%v), quería 5: lo que el lote prestó, no la merma entera", lot, err)
	}
}
