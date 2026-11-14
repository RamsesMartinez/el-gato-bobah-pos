//go:build integration

package integration

import (
	"context"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LO QUE CAMBIA SIN MOVIMIENTO TAMBIÉN EVALÚA (spec 084, FR-007): un conteo que confirma la cifra,
// archivar, unir, cambiar de unidad y cambiar el mínimo en la ficha. Sin movimiento no hay trigger.

func notificationState(t *testing.T, st *store.Store, ing int64, kind string) (status, reason string) {
	t.Helper()
	_ = st.Pool.QueryRow(context.Background(), `
		select status, coalesce(resolved_reason, '') from notifications
		 where ingredient_id = $1 and kind = $2 order by id desc limit 1`, ing, kind).Scan(&status, &reason)
	return
}

func TestPathsWithoutMovementEvaluate(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	user := makeUser(t, st, "gerente_084p", "gerente")
	svc, ctx := asTenant(t, defaultCompanyID)
	ctx = app.WithActorUser(ctx, user)
	out, outCtx := asStockOut(t, defaultCompanyID)

	t.Run("cambiar el mínimo en la ficha abre al instante", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Ficha 084p")
		adjust(t, st, "ingrediente", ing, "3")
		alertOn(t, st, ing, "", true, false)
		a, err := svc.GetArticle(ctx, ing)
		if err != nil {
			t.Fatal(err)
		}
		min := qty("5")
		if _, err := svc.UpdateArticle(ctx, ing, app.ArticleUpdate{ExpectedUpdatedAt: a.UpdatedAt, MinStock: app.Field[decimal.Decimal]{Set: true, Value: &min}}); err != nil {
			t.Fatal(err)
		}
		if status, _ := notificationState(t, st, ing, "stock_low"); status != "open" {
			t.Fatalf("subir el mínimo sobre la existencia no abrió el aviso: %q", status)
		}
	})

	t.Run("archivar cierra lo abierto", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Archivo 084p")
		adjust(t, st, "ingrediente", ing, "3")
		alertOn(t, st, ing, "5", true, false)
		adjust(t, st, "ingrediente", ing, "-3")
		if err := svc.ArchiveArticle(ctx, ing, true); err != nil {
			t.Fatal(err)
		}
		if status, reason := notificationState(t, st, ing, "stock_low"); status != "resolved" || reason != "archived" {
			t.Fatalf("%s %s", status, reason)
		}
	})

	t.Run("un conteo que confirma la cifra cierra el «se acabó» como contado", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Contado 084p")
		adjust(t, st, "ingrediente", ing, "4")
		if _, _, err := out.ReportOut(outCtx, app.ReportOutCmd{ItemType: "ingrediente", ID: ing, ClientKey: uuid.New(), Actor: user}); err != nil {
			t.Fatal(err)
		}
		c, err := svc.OpenCount(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.CaptureCount(ctx, c.ID, ing, qty("0"), user); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CloseCount(ctx, c.ID, user); err != nil {
			t.Fatal(err)
		}
		if status, reason := notificationState(t, st, ing, "stock_out"); status != "resolved" || reason != "counted" {
			t.Fatalf("%s %s", status, reason)
		}
	})

	t.Run("un conteo cerrado deja de pedir conteo en lo que abra después", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Recién contado 084p")
		adjust(t, st, "ingrediente", ing, "10")
		c, err := svc.OpenCount(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.CaptureCount(ctx, c.ID, ing, qty("10"), user); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CloseCount(ctx, c.ID, user); err != nil {
			t.Fatal(err)
		}
		alertOn(t, st, ing, "9", true, false)
		adjust(t, st, "ingrediente", ing, "-2")
		var needs bool
		if err := st.Pool.QueryRow(context.Background(), `select needs_count from notifications where ingredient_id = $1 and status = 'open'`, ing).Scan(&needs); err != nil || needs {
			t.Fatalf("pide conteo de algo contado hace un momento: %v %v", needs, err)
		}
	})

	t.Run("cambiar de unidad cierra lo abierto y evalúa ya convertido", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Unidad 084p")
		adjust(t, st, "ingrediente", ing, "4")
		alertOn(t, st, ing, "5", true, false)
		adjust(t, st, "ingrediente", ing, "-1")
		if status, _ := notificationState(t, st, ing, "stock_low"); status != "open" {
			t.Fatalf("preparación: %q", status)
		}
		// 1 pieza = 20 g: 3 piezas son 60 g y el mínimo pasa a 100 g, sigue abajo.
		if err := svc.ChangeUnit(ctx, ing, app.UnitChangeRequest{BaseUnitID: unitID(t, st, "g"), Factor: qty("20")}); err != nil {
			t.Fatal(err)
		}
		var closedReason string
		var open int
		if err := st.Pool.QueryRow(context.Background(), `
			select coalesce(min(resolved_reason) filter (where status = 'resolved'), ''), count(*) filter (where status = 'open')
			  from notifications where ingredient_id = $1 and kind = 'stock_low'`, ing).Scan(&closedReason, &open); err != nil {
			t.Fatal(err)
		}
		if closedReason != "unit_changed" || open != 1 {
			t.Fatalf("cerrado como %q y %d abiertos; el viejo se cierra y el convertido se vuelve a abrir", closedReason, open)
		}
		var minSeen string
		if err := st.Pool.QueryRow(context.Background(), `select min_seen::text from notifications where ingredient_id = $1 and status = 'open'`, ing).Scan(&minSeen); err != nil || minSeen != "100.0000" {
			t.Fatalf("mínimo visto %s (%v)", minSeen, err)
		}
	})
}

// Unir (spec 056 + 084): la casilla del destino se fusiona con OR, el aviso del origen se cierra como
// unido y el destino se evalúa con la existencia ya traspasada.
func TestMergeOrsLowStockAndClosesSourceNotifications(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	// B con alerta y bajo su mínimo; A sin alerta.
	alertOn(t, k.st, k.b, "1000", true, false)
	adjust(t, k.st, "ingrediente", k.b, "-1")
	if status, _ := notificationState(t, k.st, k.b, "stock_low"); status != "open" {
		t.Fatalf("preparación: %q", status)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `update ingredients set min_stock = 1000 where id = $1`, k.a); err != nil {
		t.Fatal(err)
	}
	k.merge(t, k.a, k.req(k.b))
	if status, reason := notificationState(t, k.st, k.b, "stock_low"); status != "resolved" || reason != "merged" {
		t.Fatalf("aviso del origen: %s %s", status, reason)
	}
	var enabled bool
	if err := k.st.Pool.QueryRow(context.Background(), `select low_stock_enabled from ingredients where id = $1`, k.a).Scan(&enabled); err != nil || !enabled {
		t.Fatalf("el destino perdió la alerta que tenía el origen: %v %v", enabled, err)
	}
	// 500 + 299 = 799 g, bajo el mínimo de 1000: el destino avisa.
	if status, _ := notificationState(t, k.st, k.a, "stock_low"); status != "open" {
		t.Fatalf("el destino no avisó con la existencia traspasada: %q", status)
	}
}
