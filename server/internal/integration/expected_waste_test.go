//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// MERMA ESPERADA (spec 096) bajo el rol de la API. El catálogo de costos: el frappé cuesta $4
// (200 g de base a $0.02) sin merma. La base es un preparado: el % general y el del producto NO
// entran a su receta (R2), así que con 5 % general el frappé sube 5 % exacto: $4.20.

func wpct(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

func lastAudit(t *testing.T, st *store.Store, action domain.AuditAction, entity int64) (before, after string) {
	t.Helper()
	if err := st.Pool.QueryRow(context.Background(), `
		select coalesce(before::text, ''), coalesce(after::text, '') from audit_events
		 where action = $1 and entity_id = $2 order by id desc limit 1`, string(action), entity).Scan(&before, &after); err != nil {
		t.Fatalf("no quedó %s en la bitácora: %v", action, err)
	}
	return before, after
}

func TestGeneralExpectedWasteMovesRecipeCosts(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewCostingService(st)

	if err := svc.SetGeneralExpectedWaste(ctx, wpct("5"), k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé con 5 % general", "products", k.frappe, "4.2")
	wantCost(t, owner, "el preparado no recibe el general", "ingredients", k.base, "0.02")
	wantCost(t, owner, "extra de leche (artículo → general)", "modifier_options", k.extraLeche, "2.1")
	if before, after := lastAudit(t, owner, domain.AuditSettingsExpectedWaste, k.company); before == "" || after == "" {
		t.Fatalf("la bitácora no dice de cuánto a cuánto: %q → %q", before, after)
	}

	if err := svc.SetGeneralExpectedWaste(ctx, nil, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé al vaciar el general", "products", k.frappe, "4")

	if err := svc.SetGeneralExpectedWaste(ctx, wpct("100"), k.admin); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("un 100 %% general = %v; quiere ErrValidation (400), no un 500 de la columna", err)
	}
}

func TestProductExpectedWasteReplacesTheGeneral(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewCostingService(st)

	if err := svc.SetGeneralExpectedWaste(ctx, wpct("5"), k.admin); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetProductExpectedWaste(ctx, k.frappe, wpct("10"), k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé con 10 % propio y 5 % general (10, no 15)", "products", k.frappe, "4.4")
	wantCost(t, owner, "paquete de dos frappés", "products", k.paquete, "8.8")
	if _, after := lastAudit(t, owner, domain.AuditProductExpectedWaste, k.frappe); after == "" {
		t.Fatal("el cambio del producto no quedó en la bitácora")
	}

	if err := svc.SetProductExpectedWaste(ctx, k.frappe, wpct("0"), k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé con 0 % propio no suma el general", "products", k.frappe, "4")

	if err := svc.SetProductExpectedWaste(ctx, k.frappe, nil, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé sin % propio vuelve al general", "products", k.frappe, "4.2")
}

func TestArticleExpectedWasteZeroIsNotEmpty(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	svc := app.NewCostingService(st)

	if err := svc.SetGeneralExpectedWaste(ctx, wpct("5"), k.admin); err != nil {
		t.Fatal(err)
	}
	// La base es lo único que lleva el frappé: capturada en 0 a propósito, el general no le suma.
	if err := svc.SetArticleExpectedWaste(ctx, k.base, wpct("0"), k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé cuyo único artículo está capturado en 0", "products", k.frappe, "4")
	var pct *decimal.Decimal
	if err := owner.Pool.QueryRow(context.Background(), `select expected_waste_pct from ingredients where id = $1`, k.base).Scan(&pct); err != nil || pct == nil || !pct.IsZero() {
		t.Fatalf("0 capturado se guardó como %v (%v): se confundiría con «sin captura»", pct, err)
	}

	if err := svc.SetArticleExpectedWaste(ctx, k.base, nil, k.admin); err != nil {
		t.Fatal(err)
	}
	wantCost(t, owner, "frappé con el artículo sin captura vuelve al general", "products", k.frappe, "4.2")
	if _, after := lastAudit(t, owner, domain.AuditIngredientExpectedWaste, k.base); after == "" {
		t.Fatal("el cambio del artículo no quedó en la bitácora")
	}
}

// Ninguna venta cerrada cambia su costo al cambiar un % (SC-004): el renglón guarda su costo al
// venderse y el recálculo solo escribe el costo vigente del catálogo.
func TestExpectedWasteDoesNotRewritePastSales(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, k.company, k.admin)
	var order, line int64
	if err := owner.Pool.QueryRow(context.Background(), `
		insert into orders (company_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by)
		values ($1, gen_random_uuid(), current_date, 9601, 'mostrador', 60, 60, $2) returning id`, k.company, k.admin).Scan(&order); err != nil {
		t.Fatalf("sembrar el pedido: %v", err)
	}
	if err := owner.Pool.QueryRow(context.Background(), `
		insert into order_lines (company_id, order_id, product_id, product_name, quantity, unit_price, unit_cost, line_total)
		values ($1, $2, $3, 'Frappé', 1, 60, 4, 60) returning id`, k.company, order, k.frappe).Scan(&line); err != nil {
		t.Fatalf("sembrar el renglón: %v", err)
	}
	if err := app.NewCostingService(st).SetGeneralExpectedWaste(ctx, wpct("5"), k.admin); err != nil {
		t.Fatal(err)
	}
	var cost decimal.Decimal
	if err := owner.Pool.QueryRow(context.Background(), `select unit_cost from order_lines where id = $1`, line).Scan(&cost); err != nil {
		t.Fatal(err)
	}
	if !cost.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("la venta pasada ahora cuesta %s: el cambio de merma reescribió el pasado", cost)
	}
}

func TestExpectedWasteIsIsolated(t *testing.T) {
	owner := newTestStore(t)
	k := seedCostCatalog(t, owner, defaultCompanyID, "")
	other := makeCompany(t, owner, "otra-merma")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewCostingService(st)
		// 404 y nada más: ni un 500 de RLS ni un 409 que distinga «ajeno» de «no existe».
		if err := svc.SetProductExpectedWaste(ctx, k.frappe, wpct("10"), k.admin); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("merma de un producto ajeno = %v; quiere ErrNotFound", err)
		}
		if err := svc.SetArticleExpectedWaste(ctx, k.base, wpct("10"), k.admin); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("merma de un artículo ajeno = %v; quiere ErrNotFound", err)
		}
		_ = svc.SetGeneralExpectedWaste(ctx, wpct("10"), k.admin)
	})
	var general *decimal.Decimal
	if err := owner.Pool.QueryRow(context.Background(), `select expected_waste_pct from business_settings where company_id = $1`, defaultCompanyID).Scan(&general); err != nil {
		t.Fatal(err)
	}
	if general != nil {
		t.Fatalf("otra sesión le cambió el general a la empresa dueña: %s", general)
	}
	wantCost(t, owner, "frappé de la dueña", "products", k.frappe, "4")
}
