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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA CONFIGURACIÓN DE ALERTAS (spec 084, historia 3): la vista previa dice qué pasará sin guardar; al
// guardar se audita y se evalúa al instante (por estado, no solo por cruce).

func lowStockIn(enabled bool, min string) app.LowStockInput {
	in := app.LowStockInput{Enabled: enabled, OnMin: true, OnOut: true}
	if min != "" {
		m := decimal.RequireFromString(min)
		in.MinStock = &m
	}
	return in
}

func TestLowStockConfigPreviewAndSave(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	gerente := makeUser(t, st, "gerente_084c", "gerente")
	svc, _, ctx := asNotifications(t, defaultCompanyID)

	t.Run("la vista previa anuncia el aviso y no guarda nada", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Vista previa 084c")
		adjust(t, st, "ingrediente", ing, "2")
		res, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, lowStockIn(true, "5"), gerente, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Effect.Opens) != 1 || res.Effect.Opens[0] != domain.KindStockLow || !res.OnHand.Equal(qty("2")) {
			t.Fatalf("efecto %+v", res)
		}
		var enabled bool
		if err := st.Pool.QueryRow(ctx, `select low_stock_enabled from ingredients where id = $1`, ing).Scan(&enabled); err != nil || enabled {
			t.Fatalf("la vista previa guardó: %v %v", enabled, err)
		}
		if k := openKinds(t, st, ing); len(k) != 0 {
			t.Fatalf("la vista previa abrió %v", k)
		}
	})

	t.Run("guardar abre al instante, marca la pregunta como hecha y deja bitácora", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Guardar 084c")
		adjust(t, st, "ingrediente", ing, "2")
		res, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, lowStockIn(true, "5"), gerente, false)
		if err != nil {
			t.Fatal(err)
		}
		if k := openKinds(t, st, ing); len(k) != 1 || k[0] != domain.KindStockLow {
			t.Fatalf("abiertos %v; efecto %+v", k, res.Effect)
		}
		var asked *time.Time
		if err := st.Pool.QueryRow(ctx, `select low_stock_asked_at from ingredients where id = $1`, ing).Scan(&asked); err != nil || asked == nil {
			t.Fatalf("la pregunta no quedó hecha: %v", err)
		}
		var audits int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'stock.low_stock_configured' and entity_id = $1`, ing).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("bitácora %d %v", audits, err)
		}
	})

	t.Run("apagar cierra lo abierto", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Apagar 084c")
		adjust(t, st, "ingrediente", ing, "2")
		if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, lowStockIn(true, "5"), gerente, false); err != nil {
			t.Fatal(err)
		}
		res, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, lowStockIn(false, "5"), gerente, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Effect.Closes) != 1 || len(openKinds(t, st, ing)) != 0 {
			t.Fatalf("efecto %+v abiertos %v", res.Effect, openKinds(t, st, ing))
		}
	})

	t.Run("una versión vieja de la ficha es STALE", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Vieja 084c")
		in := lowStockIn(true, "5")
		old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		in.ExpectedUpdatedAt = &old
		if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, in, gerente, false); !errors.Is(err, domain.ErrArticleStale) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("el mínimo de la sucursal gana sobre el del artículo", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Sucursal 084c")
		adjust(t, st, "ingrediente", ing, "7")
		in := lowStockIn(true, "5")
		bm := qty("10")
		in.BranchMinStock = app.Field[decimal.Decimal]{Set: true, Value: &bm}
		if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, in, gerente, false); err != nil {
			t.Fatal(err)
		}
		if k := openKinds(t, st, ing); len(k) != 1 {
			t.Fatalf("con mínimo de sucursal 10 y 7 en existencia no avisó: %v", k)
		}
	})

	t.Run("valores absurdos son 400", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Absurdo 084c")
		in := lowStockIn(true, "-1")
		if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, in, gerente, true); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("mínimo negativo: %v", err)
		}
		in = lowStockIn(true, "5")
		days := 0
		in.CoverageDays = &days
		if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, in, gerente, true); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("cobertura 0: %v", err)
		}
	})

	t.Run("un producto sin existencia propia no tiene alertas", func(t *testing.T) {
		p := makeProduct(t, st, "Frappé sin existencia 084c", qty("50"), false)
		if _, err := svc.ConfigureLowStock(ctx, "producto", p, lowStockIn(true, "5"), gerente, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("«No, por ahora» marca la pregunta sin encender nada", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Pregunta 084c")
		if err := svc.LowStockAsked(ctx, ing); err != nil {
			t.Fatal(err)
		}
		cfg, err := svc.LowStockConfig(ctx, "ingrediente", ing)
		if err != nil || !cfg.Asked || cfg.Enabled {
			t.Fatalf("%+v %v", cfg, err)
		}
	})
}

func TestLowStockBatch(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	gerente := makeUser(t, st, "gerente_084b", "gerente")
	svc, _, ctx := asNotifications(t, defaultCompanyID)
	a := pieceIngredient(t, st, "Lote A 084b")
	b := pieceIngredient(t, st, "Lote B 084b")
	adjust(t, st, "ingrediente", a, "2")
	adjust(t, st, "ingrediente", b, "50")
	items := []app.LowStockBatchItem{
		{ItemType: "ingrediente", ID: a, Enabled: true, MinStock: decp("5")},
		{ItemType: "ingrediente", ID: b, Enabled: true, MinStock: decp("5")},
	}
	prev, err := svc.ConfigureBatch(ctx, items, gerente, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(prev) != 2 || len(prev[0].Effect.Opens) != 1 || len(prev[1].Effect.Opens) != 0 {
		t.Fatalf("vista previa %+v", prev)
	}
	if len(openKinds(t, st, a)) != 0 {
		t.Fatal("la vista previa del lote abrió avisos")
	}
	if _, err := svc.ConfigureBatch(ctx, items, gerente, false); err != nil {
		t.Fatal(err)
	}
	if len(openKinds(t, st, a)) != 1 || len(openKinds(t, st, b)) != 0 {
		t.Fatalf("a %v b %v", openKinds(t, st, a), openKinds(t, st, b))
	}
	if _, err := svc.ConfigureBatch(ctx, make([]app.LowStockBatchItem, 101), gerente, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("101 renglones: %v", err)
	}
}

// La configuración de alertas de una empresa no se lee ni se cambia desde otra.
func TestLowStockConfigIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "otra-084c")
	gerente := makeUser(t, st, "gerente_084ci", "gerente")
	ing := pieceIngredient(t, st, "Aislada 084c")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewNotificationsService(as, clock)
		if _, err := svc.LowStockConfig(ctx, "ingrediente", ing); err == nil {
			t.Fatal("otra empresa leyó la configuración")
		}
		if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, lowStockIn(true, "5"), gerente, false); err == nil {
			t.Fatal("otra empresa cambió la configuración")
		}
	})
}
