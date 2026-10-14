//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// CONTEO (spec 038, historia 3): lo heredado de FUDO se corrige contando, de aquí en adelante.

type countKit struct {
	*purchaseKit
	leche int64 // en ml, nadie la cuenta
}

func newCountKit(t *testing.T) *countKit {
	t.Helper()
	k := &countKit{purchaseKit: newPurchaseKit(t)}
	k.leche = ingredientIn(t, k.st, defaultCompanyID, "Leche conteo", "ml")
	// Lo heredado: el azúcar quedó en -1,200 g y la leche en 500 ml.
	for ing, q := range map[int64]string{k.azucar: "-1200", k.leche: "500"} {
		if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
			values ($1, 'ingrediente', $2, 'ajuste', $3)`, defaultCompanyID, ing, q); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

// sale simula lo que una venta resta mientras se cuenta.
func (k *countKit) sale(t *testing.T, ing int64, q string) {
	t.Helper()
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'venta', $3)`, defaultCompanyID, ing, q); err != nil {
		t.Fatal(err)
	}
}

func TestACountAdjustsAgainstWhatThereWasWhenCaptured(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.azucar, dd("2000"), k.user); err != nil {
		t.Fatal(err)
	}
	// Recapturar: manda la lectura nueva.
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.azucar, dd("3000"), k.user); err != nil {
		t.Fatal(err)
	}
	// Se vendieron 100 g después de contar: deben quedar restados al cerrar.
	k.sale(t, k.azucar, "-100")

	preview, err := k.svc.PreviewCount(k.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := k.svc.CloseCount(k.ctx, c.ID, k.user)
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("2900")) {
		t.Fatalf("azúcar = %s; quiere 2900 (3,000 contados menos 100 vendidos después)", got)
	}
	if got := onHandIngredient(t, k.st, k.leche); !got.Equal(dd("500")) {
		t.Errorf("la leche no se contó y cambió a %s", got)
	}
	if len(closed.Adjusted) != 1 || !closed.Adjusted[0].Delta.Equal(dd("4200")) || !closed.Adjusted[0].System.Equal(dd("-1200")) {
		t.Errorf("ajustes = %+v; quiere uno de +4200 sobre -1200", closed.Adjusted)
	}
	if len(closed.NotCounted) != 1 || closed.NotCounted[0].Name != "Leche conteo" {
		t.Errorf("no contados = %+v; quiere la leche", closed.NotCounted)
	}
	pj, _ := json.Marshal(preview)
	cj, _ := json.Marshal(closed)
	if string(pj) != string(cj) {
		t.Errorf("la vista previa no es lo que hizo el cierre:\n%s\n%s", pj, cj)
	}
	// SC-003: el ajuste dice de qué conteo viene, quién lo cerró y en qué sucursal.
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements
		where stock_count_id = $1 and movement_type = 'ajuste' and user_id = $2 and branch_id is not null`, c.ID, k.user).Scan(&n); err != nil || n != 1 {
		t.Errorf("movimientos del conteo = %d (%v); quiere 1 con conteo, quién y sucursal", n, err)
	}
	var events int
	_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = 'stock.counted' and entity_id = $1`, c.ID).Scan(&events)
	if events != 1 {
		t.Errorf("eventos stock.counted = %d; quiere 1", events)
	}
	// Cerrado no acepta más cifras.
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.leche, dd("1"), k.user); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("capturar en un conteo cerrado = %v; quiere ErrConflict", err)
	}
	last, err := k.svc.LastClosedCount(k.ctx)
	if err != nil || last == nil {
		t.Errorf("último conteo cerrado = %v (%v); quiere su fecha", last, err)
	}
}

func TestOneOpenCountPerBranchAndBlindWhileOpen(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	if last, err := k.svc.LastClosedCount(k.ctx); err != nil || last != nil {
		t.Fatalf("sin conteos, último cerrado = %v (%v); quiere nada", last, err)
	}
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.OpenCount(k.ctx, k.user); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("segundo conteo abierto = %v; quiere ErrConflict", err)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.azucar, dd("-1"), k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("cifra negativa = %v; quiere ErrValidation", err)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.azucar, dd("0"), k.user); err != nil {
		t.Errorf("cero («no hay») = %v; quiere aceptarlo", err)
	}
	cur, err := k.svc.CurrentCount(k.ctx)
	if err != nil || cur == nil || cur.ID != c.ID {
		t.Fatalf("conteo actual = %+v (%v)", cur, err)
	}
	// A ciegas: lo que el sistema tiene no viaja mientras se cuenta.
	b, _ := json.Marshal(cur)
	if strings.Contains(string(b), "-1200") || strings.Contains(strings.ToLower(string(b)), "system") {
		t.Errorf("el conteo abierto deja ver lo que el sistema tiene: %s", b)
	}
	if err := k.svc.UncaptureCount(k.ctx, c.ID, k.azucar); err != nil {
		t.Fatal(err)
	}
	// Descartar no toca lo guardado.
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.azucar, dd("5000"), k.user); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.DiscardCount(k.ctx, c.ID, k.user); err != nil {
		t.Fatal(err)
	}
	if got := k.onHand(t); !got.Equal(dd("-1200")) {
		t.Errorf("descartar movió lo guardado a %s", got)
	}
	if cur, _ := k.svc.CurrentCount(k.ctx); cur != nil {
		t.Error("descartado y sigue como conteo actual")
	}
	if _, err := k.svc.OpenCount(k.ctx, k.user); err != nil {
		t.Errorf("tras descartar, abrir otro = %v", err)
	}
}

func TestCountPlaceOrdersTheCount(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	refri := "Refri"
	if err := k.svc.SetCountPlace(k.ctx, k.leche, &refri); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 31)
	if err := k.svc.SetCountPlace(k.ctx, k.leche, &long); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("lugar de 31 letras = %v; quiere ErrValidation", err)
	}
	if _, err := k.svc.OpenCount(k.ctx, k.user); err != nil {
		t.Fatal(err)
	}
	cur, err := k.svc.CurrentCount(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Los que tienen lugar primero, los sin lugar al final.
	if len(cur.Items) < 2 || cur.Items[0].IngredientID != k.leche || cur.Items[0].Place == nil {
		t.Errorf("orden del conteo = %+v; quiere primero la leche (Refri)", cur.Items)
	}
	for _, it := range cur.Items {
		if it.IngredientID == k.azucar && (len(it.Presentations) != 1 || it.Presentations[0].Name != "Bulto") {
			t.Errorf("el azúcar no trae su empaque: %+v", it.Presentations)
		}
	}
}

func TestACountWithTwoBranchesAndNoSelectorIsNotGuessed(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	addBranch(t, k.st, defaultCompanyID, "NORTE")
	if _, err := k.svc.OpenCount(k.ctx, k.user); !errors.Is(err, domain.ErrBranchAmbiguous) {
		t.Fatalf("con dos sucursales = %v; quiere ErrBranchAmbiguous", err)
	}
}

func TestCountsAreIsolated(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.azucar, dd("3000"), k.user); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, k.st, "conteo-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		if cur, err := svc.CurrentCount(ctx); err == nil && cur != nil && cur.ID == c.ID {
			t.Fatal("se ve el conteo de otra empresa")
		}
		if err := svc.CaptureCount(ctx, c.ID, k.azucar, dd("1"), k.user); err == nil {
			t.Fatal("capturó en el conteo de otra empresa")
		}
		if _, err := svc.CloseCount(ctx, c.ID, k.user); err == nil {
			t.Fatal("cerró el conteo de otra empresa")
		}
	})
	if got := k.onHand(t); !got.Equal(dd("-1200")) {
		t.Errorf("otra empresa movió lo guardado a %s", got)
	}
}
