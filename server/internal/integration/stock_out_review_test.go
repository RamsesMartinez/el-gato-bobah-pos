//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Lo que encontró la revisión de código de la 084 en «Se acabó»: cada caso es un fallo concreto.

// Dos tabletas reportan el mismo PRODUCTO a la vez: la segunda esperaba el candado de la existencia,
// no encontraba aviso sin autor y respondía 500. Tiene que quedar como confirmación.
func TestTwoReportsOfTheSameProductAtOnceConfirm(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	a := makeUser(t, st, "cajero_084rp", "cajero")
	b := makeUser(t, st, "cajera_084rp", "cajero")
	p := makeProduct(t, st, "Arizona 084rp", qty("30"), true)
	adjust(t, st, "producto", p, "6")
	svcA, ctxA := asStockOut(t, defaultCompanyID)
	svcB, ctxB := asStockOut(t, defaultCompanyID)
	var wg sync.WaitGroup
	res := make([]app.OutReportResult, 2)
	errs := make([]error, 2)
	for i, run := range []func() (app.OutReportResult, error){
		func() (app.OutReportResult, error) {
			r, _, err := svcA.ReportOut(ctxA, app.ReportOutCmd{ItemType: "producto", ID: p, ClientKey: uuid.New(), Actor: a})
			return r, err
		},
		func() (app.OutReportResult, error) {
			r, _, err := svcB.ReportOut(ctxB, app.ReportOutCmd{ItemType: "producto", ID: p, ClientKey: uuid.New(), Actor: b})
			return r, err
		},
	} {
		wg.Add(1)
		go func() { defer wg.Done(); res[i], errs[i] = run() }()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil && !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("reporte simultáneo de un producto: %v", err)
		}
	}
	if errs[0] == nil && errs[1] == nil && res[0].NotificationID != res[1].NotificationID {
		t.Fatalf("dos avisos para el mismo producto: %d y %d", res[0].NotificationID, res[1].NotificationID)
	}
}

// La misma llave con OTRO artículo: la respuesta se perdió, la tableta reusó la llave para la leche
// y el servidor devolvía el reporte del queso, que la pantalla anunciaba como «leche quedó en 0».
func TestAReusedKeyForAnotherItemIsRejected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084rk", "cajero")
	svc, ctx := asStockOut(t, defaultCompanyID)
	queso := pieceIngredient(t, st, "Queso 084rk")
	leche := pieceIngredient(t, st, "Leche 084rk")
	adjust(t, st, "ingrediente", queso, "3")
	adjust(t, st, "ingrediente", leche, "3")
	key := uuid.New()
	if _, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: queso, ClientKey: key, Actor: cajero}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: leche, ClientKey: key, Actor: cajero}); !errors.Is(err, domain.ErrClientKeyReused) {
		t.Fatalf("la llave del queso sirvió para la leche: %v", err)
	}
	if _, _, err := svc.ReportNotFound(ctx, "popotes", key, cajero); !errors.Is(err, domain.ErrClientKeyReused) {
		t.Fatalf("la llave del queso sirvió para «No lo encuentro»: %v", err)
	}
	nf := uuid.New()
	if _, _, err := svc.ReportNotFound(ctx, "popotes", nf, cajero); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ReportNotFound(ctx, "servilletas", nf, cajero); !errors.Is(err, domain.ErrClientKeyReused) {
		t.Fatalf("la llave de «popotes» sirvió para «servilletas»: %v", err)
	}
	if got := ingredientOnHand(t, st, leche); !got.Equal(qty("3")) {
		t.Fatalf("la leche quedó en %s", got)
	}
}

// Doble toque o reintento con la MISMA llave a la vez: chocaban en el único de la llave y salía 500.
func TestTheSameKeyTwiceAtOnceIsOneReport(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084rd", "cajero")
	ing := pieceIngredient(t, st, "Tapas 084rd")
	adjust(t, st, "ingrediente", ing, "4")
	key := uuid.New()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	ids := make([]int64, 2)
	for i := range 2 {
		svc, ctx := asStockOut(t, defaultCompanyID)
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: key, Actor: cajero})
			ids[i], errs[i] = r.ID, err
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("la misma llave dos veces a la vez: %v", err)
		}
	}
	if ids[0] != ids[1] || outMovements(t, st, ing) != 1 {
		t.Fatalf("reportes %d/%d, ajustes %d", ids[0], ids[1], outMovements(t, st, ing))
	}
}

// El tope de 20 por hora se pasaba mandando muchos a la vez: cada transacción contaba menos de 20.
func TestReportLimitHoldsUnderConcurrency(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084rl", "cajero")
	const n = 30
	ings := make([]int64, n)
	for i := range ings {
		ings[i] = pieceIngredient(t, st, "Ráfaga 084rl "+string(rune('A'+i)))
		adjust(t, st, "ingrediente", ings[i], "2")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, limited := 0, 0
	// Un solo pool, una conexión por petición: como la API.
	as := appRoleStore(t)
	svc := app.NewStockOutService(as, clock)
	for _, ing := range ings {
		ctx, release, err := as.AcquireTenant(context.Background(), defaultCompanyID)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer release()
			_, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: cajero})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrReportLimit):
				limited++
			default:
				t.Errorf("reporte: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 20 || limited != n-20 {
		t.Fatalf("aceptados %d, frenados %d; el tope es 20 por hora", ok, limited)
	}
}

// A reporta, B confirma, A deshace (no repone: B sostiene el aviso) y B deshace. Los dos se
// retractaron y la existencia se quedaba en 0 con el aviso cerrado como «deshecho».
func TestUndoingEveryReportRestoresTheStock(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	a := makeUser(t, st, "cajero_084ru", "cajero")
	b := makeUser(t, st, "cajera_084ru", "cajero")
	svc, ctx := asStockOut(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Crema 084ru")
	adjust(t, st, "ingrediente", ing, "5")
	ra := reportOut(t, svc, ctx, ing, a)
	rb := reportOut(t, svc, ctx, ing, b)
	if rb.Kind != "confirmation" {
		t.Fatalf("el segundo es %s", rb.Kind)
	}
	if err := svc.UndoOut(ctx, ra.ID, a, false); err != nil {
		t.Fatal(err)
	}
	if got := ingredientOnHand(t, st, ing); !got.IsZero() {
		t.Fatalf("con la confirmación de B viva, A no repone: quedaron %s", got)
	}
	if err := svc.UndoOut(ctx, rb.ID, b, false); err != nil {
		t.Fatal(err)
	}
	if got := ingredientOnHand(t, st, ing); !got.Equal(qty("5")) {
		t.Fatalf("los dos se retractaron y quedaron %s; tenía 5", got)
	}
}
