//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// VISTA PREVIA DE LA MERMA DE UN INSUMO (spec 099). La leche entra en la base (preparado), que entra
// en el frappé, que entra en el paquete y es un extra; y entra directo en el extra de leche.
// Con leche en 10 %: base 0.021, frappé 4.2, paquete 8.4, extra de leche 2.2, extra frappé 4.2.

func usesByKey(p app.ExpectedWastePreview) map[string]app.WasteUse {
	m := map[string]app.WasteUse{}
	for _, u := range p.Uses {
		m[u.Kind+":"+u.Name] = u
	}
	return m
}

func TestExpectedWastePreviewShowsBeforeAndAfterWithoutWriting(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewCostingService(st)

	p, err := svc.ArticleExpectedWastePreview(ctx, k.leche, wpct("10"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Current.Pct != nil || p.Current.EffectiveCost != nil {
		t.Errorf("la leche no tiene captura y salió %v / %v; «sin captura» no es 0", p.Current.Pct, p.Current.EffectiveCost)
	}
	if p.Next.Pct == nil || !p.Next.Pct.Equal(*wpct("10")) || p.Next.EffectiveCost == nil || *p.Next.EffectiveCost != 0.022 {
		t.Errorf("después = %+v; quiere 10 %% y 0.022 (0.02 × 1.10)", p.Next)
	}
	if !p.Fallback.ProductOrGeneral {
		t.Error("con un lado sin captura la vista previa debe decir que hereda del producto o la general")
	}
	want := map[string][2]float64{
		"prep:Base costo":       {0.02, 0.021},
		"product:Frappé costo":  {4, 4.2},
		"product:Paquete costo": {8, 8.4},
		"option:Extra leche":    {2, 2.2},
		"option:Extra frappé":   {4, 4.2},
	}
	got := usesByKey(p)
	if len(got) != len(want) {
		t.Errorf("usos = %+v; quiere %d", p.Uses, len(want))
	}
	for k2, w := range want {
		u, ok := got[k2]
		if !ok {
			t.Errorf("falta %s", k2)
			continue
		}
		if u.Before != w[0] || u.After != w[1] {
			t.Errorf("%s = %v → %v, quiere %v → %v", k2, u.Before, u.After, w[0], w[1])
		}
	}
	wantCost(t, owner, "la vista previa no escribe el costo del frappé", "products", k.frappe, "4")
	var stored *string
	if err := owner.Pool.QueryRow(context.Background(), `select expected_waste_pct::text from ingredients where id = $1`, k.leche).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("la vista previa guardó el %% (%v, %v)", stored, err)
	}

	if _, err := svc.ArticleExpectedWastePreview(ctx, k.leche, wpct("100")); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("100 %% en la vista previa = %v; quiere ErrValidation", err)
	}
	same, err := svc.ArticleExpectedWastePreview(ctx, k.te, nil)
	if err != nil || !same.Unchanged {
		t.Fatalf("sin captura → sin captura = %+v, %v; quiere unchanged", same, err)
	}
}

// EL MOTIVO DEL MCP VA A LA BITÁCORA; la pantalla, que no lo manda, sigue igual.
func TestArticleExpectedWasteReasonGoesToTheAudit(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewCostingService(st)

	if err := svc.SetArticleExpectedWaste(ctx, k.leche, wpct("8"), k.admin, "  rota poco  "); err != nil {
		t.Fatal(err)
	}
	if _, after := lastAudit(t, owner, "ingredient.expected_waste", k.leche); !strings.Contains(after, `"reason": "rota poco"`) {
		t.Fatalf("after = %s; quiere el motivo sin espacios de más", after)
	}
	if err := svc.SetArticleExpectedWaste(ctx, k.leche, wpct("9"), k.admin, ""); err != nil {
		t.Fatal(err)
	}
	if _, after := lastAudit(t, owner, "ingredient.expected_waste", k.leche); strings.Contains(after, "reason") {
		t.Fatalf("sin motivo (la pantalla) el after = %s; no debe inventar uno", after)
	}
	if err := svc.SetArticleExpectedWaste(ctx, k.leche, wpct("9"), k.admin, strings.Repeat("x", 201)); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("motivo de 201 letras = %v; quiere ErrValidation", err)
	}
	var n int
	if err := owner.Pool.QueryRow(context.Background(),
		`select count(*) from audit_events where action = 'ingredient.expected_waste' and entity_id = $1`, k.leche).Scan(&n); err != nil || n != 2 {
		t.Fatalf("renglones = %d (%v); quiere 2: el rechazo no deja renglón", n, err)
	}
	if err := svc.SetArticleExpectedWaste(ctx, k.leche, wpct("9"), k.admin, "otra vez"); err != nil {
		t.Fatal(err)
	}
	if err := owner.Pool.QueryRow(context.Background(),
		`select count(*) from audit_events where action = 'ingredient.expected_waste' and entity_id = $1`, k.leche).Scan(&n); err != nil || n != 2 {
		t.Fatalf("renglones = %d; el mismo valor no debe dejar otro", n)
	}
}

// LA VISTA PREVIA NO LEE UN INSUMO AJENO en los tres casos donde RLS ya falló.
func TestExpectedWastePreviewIsIsolated(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	other := makeCompany(t, owner, "otra-merma-099")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		if _, err := app.NewCostingService(st).ArticleExpectedWastePreview(ctx, k.leche, wpct("10")); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("vista previa de un insumo ajeno = %v; quiere ErrNotFound", err)
		}
	})
}
