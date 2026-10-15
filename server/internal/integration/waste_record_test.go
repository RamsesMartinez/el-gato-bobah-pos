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

// LO QUE SE ECHÓ A PERDER (spec 039, historia 1): registrar resta del almacén en ese momento, con su
// motivo, quién y cuándo, como el rol de la aplicación y sin alcanzar a otra empresa.

// asWaste es el servicio como el rol de la aplicación, con la empresa puesta.
func asWaste(t *testing.T, company int64) (*app.BackofficeService, context.Context) {
	t.Helper()
	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.NewBackofficeService(as, clock), ctx
}

func reasonNamed(t *testing.T, st *store.Store, company int64, name string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from waste_reasons where company_id = $1 and name = $2`, company, name).Scan(&id); err != nil {
		t.Fatalf("motivo %s: %v", name, err)
	}
	return id
}

func qty(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestRecordingWasteTakesItOffTheShelf(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_merma", "cajero")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")

	t.Run("un insumo resta lo registrado y guarda motivo, quién y su costo", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Leche de la merma")
		adjust(t, st, "ingrediente", ing, "10")
		if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 24 where id = $1`, ing); err != nil {
			t.Fatal(err)
		}
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("2"), Actor: cajero})
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != string(domain.WasteApplied) {
			t.Fatalf("estado %s; sin tope resta de una vez", res.Status)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(qty("8")) {
			t.Fatalf("quedaron %s; quiere 8", got)
		}
		var reason, by int64
		var total decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `select reason_id, recorded_by, total_cost from waste_records where id = $1`, res.ID).Scan(&reason, &by, &total); err != nil {
			t.Fatal(err)
		}
		if reason != caduco || by != cajero || !total.Equal(qty("48")) {
			t.Fatalf("motivo %d, quién %d, costo %s; quiere %d, %d, 48", reason, by, total, caduco, cajero)
		}
	})

	t.Run("en empaque resta lo que trae el empaque", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Vasos de la merma")
		adjust(t, st, "ingrediente", ing, "100")
		var pres int64
		if err := st.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
			values ($1, $2, 'Paquete', 50) returning id`, defaultCompanyID, ing).Scan(&pres); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, PresentationID: &pres, Qty: qty("1"), Actor: cajero}); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(qty("50")) {
			t.Fatalf("quedaron %s; un paquete de 50 deja 50", got)
		}
	})

	t.Run("un producto resta lo mismo que venderlo y no es una venta", func(t *testing.T) {
		admin := makeUser(t, st, "admin_receta_merma", "admin")
		adminSvc, adminCtx := asApp(t, defaultCompanyID)
		prod := makeProduct(t, st, "Frappé de la merma", qty("60"), false)
		ing := pieceIngredient(t, st, "Vaso del frappé de la merma")
		adjust(t, st, "ingrediente", ing, "10")
		if err := adminSvc.SaveComposition(adminCtx, app.CompositionOfProduct, prod, app.CompositionRequest{Mode: "items", Items: pieces(t, ing, "1", st)}, admin); err != nil {
			t.Fatal(err)
		}
		var ordersBefore int
		if err := st.Pool.QueryRow(ctx, `select count(*) from orders`).Scan(&ordersBefore); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, ProductID: &prod, Qty: qty("2"), Actor: cajero}); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(qty("8")) {
			t.Fatalf("el vaso quedó en %s; dos frappés tirados restan dos vasos", got)
		}
		var ordersAfter int
		if err := st.Pool.QueryRow(ctx, `select count(*) from orders`).Scan(&ordersAfter); err != nil {
			t.Fatal(err)
		}
		if ordersAfter != ordersBefore {
			t.Fatal("la merma creó un pedido: aparecería en Ventas")
		}
	})

	t.Run("un producto sin nada que restar no se registra", func(t *testing.T) {
		prod := makeProduct(t, st, "Producto sin receta de la merma", qty("30"), false)
		_, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, ProductID: &prod, Qty: qty("1"), Actor: cajero})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v; quiere ErrValidation (capturar su receta o registrar los insumos)", err)
		}
	})

	t.Run("cantidad cero, negativa o un motivo archivado se rechazan", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Insumo de las cantidades malas")
		for _, q := range []string{"0", "-1", "2000000"} {
			if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty(q), Actor: cajero}); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("cantidad %s = %v; quiere ErrValidation", q, err)
			}
		}
		var archived int64
		if err := st.Pool.QueryRow(ctx, `insert into waste_reasons (company_id, name, description, archived_at)
			values ($1, 'Archivado de prueba', 'ya no se usa', now()) returning id`, defaultCompanyID).Scan(&archived); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: archived, IngredientID: &ing, Qty: qty("1"), Actor: cajero}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("motivo archivado = %v; quiere ErrValidation", err)
		}
	})

	t.Run("un motivo nuevo con nombre repetido devuelve el que existe", func(t *testing.T) {
		_, err := svc.CreateWasteReason(ctx, "  CADUCO ", "otra vez", cajero)
		var exists domain.WasteReasonExists
		if !errors.As(err, &exists) || exists.ID != caduco || !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("= %v; quiere el motivo existente %d como conflicto", err, caduco)
		}
		id, err := svc.CreateWasteReason(ctx, "Se derritió", "Hielo fuera del refri", cajero)
		if err != nil || id == 0 {
			t.Fatalf("motivo nuevo = %d, %v", id, err)
		}
	})
}

// Ni el motivo, ni el insumo, ni el producto de otra empresa; y ninguna lectura sin empresa.
func TestWasteIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "otra-merma")
	cajero := makeUser(t, st, "cajero_merma_aislada", "cajero")
	svc, ctx := asWaste(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Insumo de la dueña")
	adjust(t, st, "ingrediente", ing, "5")
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero}); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, c context.Context) {
		s := app.NewBackofficeService(as, clock)
		if rows, err := s.WasteToday(c); err == nil && len(rows) > 0 {
			t.Fatal("se vieron las mermas de otra empresa")
		}
		_, err := s.RecordWaste(c, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero})
		if err == nil {
			t.Fatal("una sesión de otra empresa (o sin empresa) registró merma con el motivo y el insumo de la dueña")
		}
	})
	if got := onHandIngredient(t, st, ing); !got.Equal(qty("4")) {
		t.Fatalf("el insumo de la dueña quedó en %s; quiere 4", got)
	}
}
