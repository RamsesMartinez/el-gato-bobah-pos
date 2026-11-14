//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// «SE ACABÓ» (spec 084, historia 1): deja la existencia en 0 con un ajuste fijo, sin cantidad, abre
// el aviso y se puede deshacer 90 segundos; repetir la misma petición no hace nada dos veces.

func asStockOut(t *testing.T, company int64) (*app.StockOutService, context.Context) {
	t.Helper()
	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.NewStockOutService(as, clock), ctx
}

func reportOut(t *testing.T, svc *app.StockOutService, ctx context.Context, ing, actor int64) app.OutReportResult {
	t.Helper()
	res, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: actor})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func outMovements(t *testing.T, st *store.Store, ing int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from stock_movements where ingredient_id = $1 and reason in ('se acabó', 'se acabó (deshecho)')`, ing).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReportingOutLeavesZeroAndOpensTheNotification(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084", "cajero")
	svc, ctx := asStockOut(t, defaultCompanyID)

	t.Run("deja 0 con un ajuste «se acabó» que no es venta, abre el aviso y lo audita", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Queso crema 084")
		adjust(t, st, "ingrediente", ing, "3")
		res := reportOut(t, svc, ctx, ing, cajero)
		if got := ingredientOnHand(t, st, ing); !got.IsZero() {
			t.Fatalf("quedaron %s; «se acabó» deja 0", got)
		}
		var kind string
		if err := st.Pool.QueryRow(ctx, `select movement_type from stock_movements where reason = 'se acabó' and ingredient_id = $1`, ing).Scan(&kind); err != nil || kind != "ajuste" {
			t.Fatalf("el movimiento es %q (%v); un «se acabó» es ajuste, nunca consumo", kind, err)
		}
		var opener int64
		if err := st.Pool.QueryRow(ctx, `select opened_by from notifications where id = $1 and status = 'open' and kind = 'stock_out'`, res.NotificationID).Scan(&opener); err != nil || opener != cajero {
			t.Fatalf("aviso abierto por %d (%v)", opener, err)
		}
		var audits int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'stock.out_reported' and entity_id = $1`, res.ID).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("bitácora: %d (%v)", audits, err)
		}
	})

	t.Run("funciona sin alerta ni mínimo", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Servilletas 084")
		adjust(t, st, "ingrediente", ing, "5")
		res := reportOut(t, svc, ctx, ing, cajero)
		if res.NotificationID == 0 || res.Kind != "report" {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("la misma clave dos veces es un solo reporte", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Bolsas 084")
		adjust(t, st, "ingrediente", ing, "7")
		key := uuid.New()
		a, created1, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: key, Actor: cajero})
		if err != nil {
			t.Fatal(err)
		}
		b, created2, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: key, Actor: cajero})
		if err != nil {
			t.Fatal(err)
		}
		if a.ID != b.ID || !created1 || created2 || outMovements(t, st, ing) != 1 {
			t.Fatalf("doble envío: %d/%d creados %v/%v, ajustes %d", a.ID, b.ID, created1, created2, outMovements(t, st, ing))
		}
	})

	t.Run("un segundo reporte del mismo artículo suma confirmación sin otro ajuste ni otro aviso", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Platos 084")
		adjust(t, st, "ingrediente", ing, "7")
		otro := makeUser(t, st, "mesero_084", "mesero")
		first := reportOut(t, svc, ctx, ing, cajero)
		second := reportOut(t, svc, ctx, ing, otro)
		if second.Kind != "confirmation" || second.NotificationID != first.NotificationID || second.Confirmations != 2 {
			t.Fatalf("%+v", second)
		}
		if outMovements(t, st, ing) != 1 {
			t.Fatalf("ajustes: %d", outMovements(t, st, ing))
		}
	})

	t.Run("con la existencia en negativo, el ajuste la sube a 0", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Jamón 084")
		adjust(t, st, "ingrediente", ing, "-4")
		res := reportOut(t, svc, ctx, ing, cajero)
		if got := ingredientOnHand(t, st, ing); !got.IsZero() || res.Delta == nil || !res.Delta.Equal(qty("4")) {
			t.Fatalf("existencia %s, delta %v", got, res.Delta)
		}
	})

	t.Run("en 0 no hay ajuste, pero sí aviso", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Tapas 084")
		res := reportOut(t, svc, ctx, ing, cajero)
		if res.Delta != nil || outMovements(t, st, ing) != 0 || res.NotificationID == 0 {
			t.Fatalf("%+v ajustes %d", res, outMovements(t, st, ing))
		}
	})

	t.Run("no acepta un artículo archivado", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Archivado 084")
		if _, err := st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, ing); err != nil {
			t.Fatal(err)
		}
		_, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: cajero})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%v", err)
		}
	})
}

func TestUndoingAnOutReport(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084u", "cajero")
	otro := makeUser(t, st, "cajera_084u", "cajero")
	svc, ctx := asStockOut(t, defaultCompanyID)

	t.Run("dentro de 90 s repone lo que restó y descarta el aviso", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Leche 084u")
		adjust(t, st, "ingrediente", ing, "3")
		res := reportOut(t, svc, ctx, ing, cajero)
		if err := svc.UndoOut(ctx, res.ID, cajero, false); err != nil {
			t.Fatal(err)
		}
		if got := ingredientOnHand(t, st, ing); !got.Equal(qty("3")) {
			t.Fatalf("quedaron %s", got)
		}
		var status, reason string
		if err := st.Pool.QueryRow(ctx, `select status, resolved_reason from notifications where id = $1`, res.NotificationID).Scan(&status, &reason); err != nil {
			t.Fatal(err)
		}
		if status != "dismissed" || reason != "undone" {
			t.Fatalf("%s %s", status, reason)
		}
		var audits int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'stock.out_undone' and entity_id = $1`, res.ID).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("bitácora del deshacer: %d %v", audits, err)
		}
	})

	t.Run("pasados los 90 s (reloj de la base) ya no", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Azúcar 084u")
		adjust(t, st, "ingrediente", ing, "3")
		res := reportOut(t, svc, ctx, ing, cajero)
		if _, err := st.Pool.Exec(ctx, `update stock_out_reports set reported_at = now() - interval '91 seconds' where id = $1`, res.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.UndoOut(ctx, res.ID, cajero, false); !errors.Is(err, domain.ErrUndoExpired) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("con un movimiento posterior no procede", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Vasos 084u")
		adjust(t, st, "ingrediente", ing, "3")
		res := reportOut(t, svc, ctx, ing, cajero)
		adjust(t, st, "ingrediente", ing, "10")
		if err := svc.UndoOut(ctx, res.ID, cajero, false); !errors.Is(err, domain.ErrUndoAfterMovement) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("solo quien reportó, o quien configura el almacén", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Sirope 084u")
		adjust(t, st, "ingrediente", ing, "3")
		res := reportOut(t, svc, ctx, ing, cajero)
		if err := svc.UndoOut(ctx, res.ID, otro, false); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("otra cajera deshizo: %v", err)
		}
		if err := svc.UndoOut(ctx, res.ID, otro, true); err != nil {
			t.Fatalf("quien configura no pudo: %v", err)
		}
	})

	t.Run("deshacer el primer reporte cuando otro lo confirmó no repone nada", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Crema 084u")
		adjust(t, st, "ingrediente", ing, "3")
		first := reportOut(t, svc, ctx, ing, cajero)
		reportOut(t, svc, ctx, ing, otro)
		if err := svc.UndoOut(ctx, first.ID, cajero, false); err != nil {
			t.Fatal(err)
		}
		if got := ingredientOnHand(t, st, ing); !got.IsZero() {
			t.Fatalf("otra persona confirmó que se acabó y la existencia volvió a %s", got)
		}
		var status string
		if err := st.Pool.QueryRow(ctx, `select status from notifications where id = $1`, first.NotificationID).Scan(&status); err != nil || status != "open" {
			t.Fatalf("%s %v", status, err)
		}
	})

	t.Run("deshacer una confirmación deja el aviso abierto", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Tapioca 084u")
		adjust(t, st, "ingrediente", ing, "3")
		first := reportOut(t, svc, ctx, ing, cajero)
		second := reportOut(t, svc, ctx, ing, otro)
		if err := svc.UndoOut(ctx, second.ID, otro, false); err != nil {
			t.Fatal(err)
		}
		var status string
		if err := st.Pool.QueryRow(ctx, `select status from notifications where id = $1`, first.NotificationID).Scan(&status); err != nil || status != "open" {
			t.Fatalf("%s %v", status, err)
		}
		if got := ingredientOnHand(t, st, ing); !got.IsZero() {
			t.Fatalf("deshacer una confirmación movió la existencia: %s", got)
		}
	})
}

// Un tope de 20 reportes por hora y persona.
func TestReportLimitPerHour(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084l", "cajero")
	svc, ctx := asStockOut(t, defaultCompanyID)
	for i := 0; i < 20; i++ {
		if _, _, err := svc.ReportNotFound(ctx, "cosa "+itoa(i), uuid.New(), cajero); err != nil {
			t.Fatalf("reporte %d: %v", i+1, err)
		}
	}
	if _, _, err := svc.ReportNotFound(ctx, "una más", uuid.New(), cajero); !errors.Is(err, domain.ErrReportLimit) {
		t.Fatalf("el reporte 21 en la hora: %v", err)
	}
}

// Dos tabletas reportan el mismo artículo a la vez: un ajuste y una confirmación.
func TestConcurrentStockOutReports(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	a := makeUser(t, st, "cajero_084c", "cajero")
	b := makeUser(t, st, "cajera_084c", "cajero")
	ing := pieceIngredient(t, st, "Bolsa concurrida 084")
	adjust(t, st, "ingrediente", ing, "9")
	svcA, ctxA := asStockOut(t, defaultCompanyID)
	svcB, ctxB := asStockOut(t, defaultCompanyID)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, run := range []func() error{
		func() error {
			_, _, err := svcA.ReportOut(ctxA, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: a})
			return err
		},
		func() error {
			_, _, err := svcB.ReportOut(ctxB, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: b})
			return err
		},
	} {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = run() }()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil && !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("reporte simultáneo: %v", err)
		}
	}
	if n := outMovements(t, st, ing); n != 1 {
		t.Fatalf("ajustes: %d", n)
	}
	var opens int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from notifications where ingredient_id = $1 and kind = 'stock_out'`, ing).Scan(&opens); err != nil || opens != 1 {
		t.Fatalf("avisos: %d %v", opens, err)
	}
}

// «Se acabó» y una venta del mismo artículo a la vez: ninguna muere por deadlock (el reporte toma la
// existencia antes que el aviso, como la venta).
func TestConcurrentStockOutReportAndSale(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_084v", "cajero")
	ing := pieceIngredient(t, st, "Vendido y acabado 084")
	adjust(t, st, "ingrediente", ing, "10")
	alertOn(t, st, ing, "9", true, true)
	svc, ctx := asStockOut(t, defaultCompanyID)
	sale := appRoleStore(t)
	for round := 0; round < 10; round++ {
		adjust(t, st, "ingrediente", ing, "5")
		var wg sync.WaitGroup
		var errSale, errOut error
		wg.Add(2)
		go func() {
			defer wg.Done()
			errSale = saleOf(sale, ing)
		}()
		go func() {
			defer wg.Done()
			_, _, errOut = svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: cajero})
		}()
		wg.Wait()
		for _, err := range []error{errSale, errOut} {
			if err != nil && isDeadlock(err) {
				t.Fatalf("vuelta %d: deadlock: %v", round, err)
			}
		}
	}
}

// saleOf vende una pieza del insumo como la API: en su transacción, con la empresa puesta.
func saleOf(as *store.Store, ing int64) error {
	ctx := context.Background()
	tx, err := as.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, itoa(defaultCompanyID)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
		values ('ingrediente', $1, 'venta', -1)`, ing); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isDeadlock(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "40P01"
}

// Una empresa no reporta ni encuentra lo de otra.
func TestStockOutIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "otra-084")
	ing := pieceIngredient(t, st, "Secreto 084")
	cajero := makeUser(t, st, "cajero_084i", "cajero")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewStockOutService(as, clock)
		if items, err := svc.Search(ctx, "secreto"); err == nil && len(items) > 0 {
			t.Fatalf("la búsqueda de otra empresa encontró %v", items)
		}
		if _, _, err := svc.ReportOut(ctx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: cajero}); err == nil {
			t.Fatal("otra empresa reportó el insumo ajeno")
		}
	})
}
