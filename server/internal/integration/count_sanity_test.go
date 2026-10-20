//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CONTAR SIN EQUIVOCARSE DE UNIDAD (spec 042, US7, FR-012).

// El aviso lo decide el servidor y no revela contra qué comparó: el conteo sigue a ciegas. Solo se
// evalúa si la pantalla lo pide (confirmable): una tableta con la pantalla anterior guarda como hoy.
func TestTheCountAsksToConfirmAnAbsurdFigure(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	// El azúcar está en -1,200 g, nunca contado, con su bulto de 25 kg: la referencia es 10 bultos.
	t.Run("sin confirmable guarda como hoy, aunque la cifra sea absurda (Revisión B1)", func(t *testing.T) {
		if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: qty("740000")}, k.user); err != nil {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("con confirmable, 740 kg pasa de 10 bultos: 409 sin guardar ni decir la referencia", func(t *testing.T) {
		_, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: qty("740000"), Confirmable: true}, k.user)
		if !errors.Is(err, domain.ErrCountNeedsConfirm) {
			t.Fatalf("= %v; quiere ErrCountNeedsConfirm", err)
		}
		if strings.Contains(err.Error(), "250000") || strings.Contains(err.Error(), "-1200") {
			t.Fatalf("el aviso revela la referencia: %v", err)
		}
	})
	t.Run("confirmada, se guarda", func(t *testing.T) {
		got, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: qty("740000"), Confirmable: true, Confirmed: true}, k.user)
		if err != nil || !got.Equal(qty("740000")) {
			t.Fatalf("= %s %v", got, err)
		}
	})
	t.Run("contar 0 nunca pide confirmar", func(t *testing.T) {
		if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.leche, app.CountCapture{Qty: qty("0"), Confirmable: true}, k.user); err != nil {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("una cifra razonable pasa sin preguntar", func(t *testing.T) {
		if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: qty("30000"), Confirmable: true}, k.user); err != nil {
			t.Fatalf("= %v", err)
		}
	})
}

// Con lo esperado en cero o negativo, la referencia es el último conteo cerrado (FR-012, punto 2).
func TestTheLastClosedCountIsTheReferenceWhenNothingIsExpected(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.leche, qty("12000"), k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CloseCount(k.ctx, c.ID, k.user); err != nil {
		t.Fatal(err)
	}
	k.sale(t, k.leche, "-13500") // queda en -1.5 L
	c2, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CaptureCountChecked(k.ctx, c2.ID, k.leche, app.CountCapture{Qty: qty("1000"), Confirmable: true}, k.user); !errors.Is(err, domain.ErrCountNeedsConfirm) {
		t.Fatalf("1 L contra un último conteo de 12 L = %v; quiere pedir confirmar (10 veces menor)", err)
	}
	if _, err := k.svc.CaptureCountChecked(k.ctx, c2.ID, k.leche, app.CountCapture{Qty: qty("10000"), Confirmable: true}, k.user); err != nil {
		t.Fatalf("10 L = %v", err)
	}
}

// Un artículo en piezas con un empaque aproximado por peso se puede contar pesándolo: el servidor
// convierte a piezas y lo guarda en piezas (FR-012, decisión del dueño).
func TestWeighingAPieceArticle(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	popotes := ingredientIn(t, k.st, defaultCompanyID, "Popotes delgados", "pieza")
	kg := unitID(t, k.st, "kg")
	if _, err := k.st.Pool.Exec(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, purchase_unit_id, is_approximate)
		values ($1, $2, 'Kilo', 400, $3, true)`, defaultCompanyID, popotes, kg); err != nil {
		t.Fatal(err)
	}
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	g := unitID(t, k.st, "g")
	got, err := k.svc.CaptureCountChecked(k.ctx, c.ID, popotes, app.CountCapture{Weight: new(qty("300")), WeightUnitID: &g}, k.user)
	if err != nil || !got.Equal(qty("120")) {
		t.Fatalf("300 g con «Kilo ≈ 400 pz» = %s %v; quiere 120 piezas", got, err)
	}
	t.Run("sin empaque aproximado no se puede pesar", func(t *testing.T) {
		vasos := ingredientIn(t, k.st, defaultCompanyID, "Vasos sin peso", "pieza")
		if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, vasos, app.CountCapture{Weight: new(qty("300")), WeightUnitID: &g}, k.user); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("la lista del conteo dice que se puede pesar y trae todos sus empaques", func(t *testing.T) {
		view, err := k.svc.CurrentCount(k.ctx)
		if err != nil || view == nil {
			t.Fatal(err)
		}
		for _, it := range view.Items {
			if it.IngredientID == popotes && !it.Weighable {
				t.Fatal("los popotes con «Kilo ≈ 400 pz» no se ofrecen para pesar")
			}
		}
	})
}

func TestCountCaptureHTTPConfirm(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin := token("conteo_aviso", "admin")
	hielo := ingredientIn(t, st, defaultCompanyID, "Hielo conteo http", "g")
	if _, err := st.Pool.Exec(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Bolsa', 15000)`, defaultCompanyID, hielo); err != nil {
		t.Fatal(err)
	}
	w := do(t, r, http.MethodPost, "/api/v1/stock/counts", admin, []byte(`{}`), "application/json")
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("abrir = %d %s", w.Code, w.Body.String())
	}
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `select id from stock_counts where status = 'open'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/stock/counts/" + strconv.FormatInt(id, 10) + "/lines/" + strconv.FormatInt(hielo, 10)
	w = do(t, r, http.MethodPut, path, admin, []byte(`{"countedQty":"740000","confirmable":true}`), "application/json")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "NEEDS_CONFIRM") {
		t.Fatalf("= %d %s; quiere 409 NEEDS_CONFIRM", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPut, path, admin, []byte(`{"countedQty":"740000","confirmable":true,"confirmed":true}`), "application/json"); w.Code >= 300 {
		t.Fatalf("confirmada = %d %s", w.Code, w.Body.String())
	}
}
