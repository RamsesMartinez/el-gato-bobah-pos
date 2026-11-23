//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// adminAsApp es el servicio de administración como lo usa una petición: rol de la aplicación y
// empresa de la sesión, sujeto a RLS.
func adminAsApp(t *testing.T, owner *store.Store, company int64, suffix string) (*app.AdminService, context.Context) {
	t.Helper()
	who := makeUserIn(t, owner, company, "admin_sc_"+suffix, "admin")
	ast := appRoleStore(t)
	return app.NewAdminService(ast), tenantCtx(t, ast, company, who)
}

// productRow busca el producto en la lista de administración: es lo que la ficha lee.
func salesProductRow(t *testing.T, ctx context.Context, admin *app.AdminService, id int64) app.AdminProductView {
	t.Helper()
	page, err := admin.ListProducts(ctx, "", "", 0, "", "", "", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Items {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("el producto %d no salió en la lista", id)
	return app.AdminProductView{}
}

func updateWith(t *testing.T, ctx context.Context, admin *app.AdminService, st *store.Store, id int64, sales *app.SalesInput) error {
	t.Helper()
	var name string
	var price decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(), `select name::text, price from products where id = $1`, id).Scan(&name, &price); err != nil {
		t.Fatal(err)
	}
	return admin.UpdateProduct(ctx, app.UpdateProductInput{ID: id, Name: name, Price: price, Active: true, NeedsPrep: true, Sales: sales})
}

// DÓNDE SE VENDE CADA PRODUCTO (spec 092, US1). Lo que se rompe si no: un producto que nace fuera de
// la web, un PATCH viejo que borra la decisión, y un «apagar Web» que responde bien sin escribir.
func TestSalesChannelsOfAProduct(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin, ctx := adminAsApp(t, st, defaultCompanyID, "lugares")

	t.Run("un producto nuevo se vende en mostrador y web y nace con vender sin existencia", func(t *testing.T) {
		cat := makeProduct(t, st, "sc-cat-seed", dec("10"), false)
		var catID int64
		if err := st.Pool.QueryRow(context.Background(), `select category_id from products where id = $1`, cat).Scan(&catID); err != nil {
			t.Fatal(err)
		}
		id, err := admin.CreateProduct(ctx, "Agua de jamaica", catID, dec("30"), false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		p := salesProductRow(t, ctx, admin, id)
		if !p.SoldAtCounter || !p.SoldOnWeb || p.WebPrice != nil || !p.AllowOversell {
			t.Fatalf("nació con mostrador=%v web=%v precio web=%v vender sin existencia=%v", p.SoldAtCounter, p.SoldOnWeb, p.WebPrice, p.AllowOversell)
		}
	})

	id := makeProduct(t, st, "Agua de horchata", dec("35"), false)

	t.Run("guardar mostrador apagado y web encendida", func(t *testing.T) {
		if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: false, Web: true, AllowOversell: true, Description: "De arroz con canela"}); err != nil {
			t.Fatal(err)
		}
		p := salesProductRow(t, ctx, admin, id)
		if p.SoldAtCounter || !p.SoldOnWeb || p.Description != "De arroz con canela" {
			t.Fatalf("quedó mostrador=%v web=%v descripción=%q", p.SoldAtCounter, p.SoldOnWeb, p.Description)
		}
	})

	t.Run("un PATCH sin sales no toca lo que ya se decidió", func(t *testing.T) {
		if err := updateWith(t, ctx, admin, st, id, nil); err != nil {
			t.Fatal(err)
		}
		if p := salesProductRow(t, ctx, admin, id); p.SoldAtCounter || p.Description != "De arroz con canela" {
			t.Fatal("un cliente que no conoce los lugares de venta volvió a encender Mostrador o borró la descripción")
		}
	})

	t.Run("las dos apagadas se guardan", func(t *testing.T) {
		if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: false, Web: false, AllowOversell: true}); err != nil {
			t.Fatal(err)
		}
		if p := salesProductRow(t, ctx, admin, id); p.SoldAtCounter || p.SoldOnWeb {
			t.Fatal("se esperaban las dos apagadas")
		}
	})

	t.Run("duplicar copia lugares de venta y precio web", func(t *testing.T) {
		if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: false, Web: true, WebPrice: ptr(dec("39")), AllowOversell: true}); err != nil {
			t.Fatal(err)
		}
		dup, err := admin.DuplicateProduct(ctx, id, "Agua de horchata grande", nil)
		if err != nil {
			t.Fatal(err)
		}
		p := salesProductRow(t, ctx, admin, dup)
		if p.SoldAtCounter || !p.SoldOnWeb || p.WebPrice == nil || !p.WebPrice.Equal(dec("39")) {
			t.Fatalf("el duplicado quedó mostrador=%v web=%v precio=%v", p.SoldAtCounter, p.SoldOnWeb, p.WebPrice)
		}
	})

	t.Run("un producto de otra empresa no se toca", func(t *testing.T) {
		coB := makeCompany(t, st, "sc-otra")
		var other int64
		if err := st.Pool.QueryRow(context.Background(), `insert into products (company_id, name, category_id, price)
			values ($1, 'De otra', (select id from categories where company_id = $1 limit 1), 10) returning id`, coB).Scan(&other); err != nil {
			// Sin categoría propia, la crea.
			var cat int64
			if err := st.Pool.QueryRow(context.Background(), `insert into categories (company_id, name) values ($1, 'Cat B') returning id`, coB).Scan(&cat); err != nil {
				t.Fatal(err)
			}
			if err := st.Pool.QueryRow(context.Background(), `insert into products (company_id, name, category_id, price) values ($1, 'De otra', $2, 10) returning id`, coB, cat).Scan(&other); err != nil {
				t.Fatal(err)
			}
		}
		inTheThreeCases(t, coB, defaultCompanyID, func(t *testing.T, ast *store.Store, actx context.Context) {
			err := app.NewAdminService(ast).UpdateProduct(actx, app.UpdateProductInput{ID: other, Name: "De otra", Price: dec("10"), Active: true,
				Sales: &app.SalesInput{Counter: false, Web: false, AllowOversell: true}})
			if err == nil {
				var n int
				_ = st.Pool.QueryRow(context.Background(), `select count(*) from product_channels where product_id = $1`, other).Scan(&n)
				if n > 0 {
					t.Fatal("una sesión ajena escribió dónde se vende un producto de otra empresa")
				}
			}
		})
	})
}

// Una empresa nacida sin catálogo de lugares de venta (un insert crudo, como algunas del pasado)
// también puede apagar «Web»: el servicio asegura el canal. Nunca responde bien sin escribir.
func TestSalesChannelsInACompanyBornWithoutThem(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	var co, cat, prod int64
	if err := st.Pool.QueryRow(ctx, `insert into companies (slug, name) values ('sc-cruda', 'Cruda') returning id`).Scan(&co); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `delete from channels where company_id = $1`, co); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'Aguas') returning id`, co).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into products (company_id, name, category_id, price) values ($1, 'Limonada', $2, 25) returning id`, co, cat).Scan(&prod); err != nil {
		t.Fatal(err)
	}
	ast := appRoleStore(t)
	actx, release, err := ast.AcquireTenant(ctx, co)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := app.NewAdminService(ast).UpdateProduct(actx, app.UpdateProductInput{ID: prod, Name: "Limonada", Price: dec("25"), Active: true,
		Sales: &app.SalesInput{Counter: true, Web: false, AllowOversell: true}}); err != nil {
		t.Fatal(err)
	}
	var vis string
	if err := st.Pool.QueryRow(ctx, `select pc.visibility::text from product_channels pc join channels ch on ch.id = pc.channel_id
		 where pc.product_id = $1 and ch.code = 'online'`, prod).Scan(&vis); err != nil {
		t.Fatalf("apagar Web no escribió nada en una empresa sin catálogo: %v", err)
	}
	if vis != "oculto" {
		t.Fatalf("Web quedó %q", vis)
	}
}

// Un producto apagado para mostrador no se vende en mostrador, pero sí entra en un pedido de
// plataforma (la plataforma ya lo vendió).
func TestHiddenAtCounterIsNotSoldAtTheCounter(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin, actx := adminAsApp(t, st, defaultCompanyID, "oculto")
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_sc", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	id := makeProduct(t, st, "Solo en web", dec("50"), false)
	if err := updateWith(t, actx, admin, st, id, &app.SalesInput{Counter: false, Web: true, AllowOversell: true}); err != nil {
		t.Fatal(err)
	}

	_, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: id, Qty: dec("1")}}})
	if !errors.Is(err, domain.ErrProductNotSell) {
		t.Fatalf("un producto apagado para mostrador se vendió en mostrador (o falló por otra cosa): %v", err)
	}

	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	if _, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "domicilio", OpenedBy: cajero,
		DeliveryPlatformID: &uber, Lines: []domain.OrderLineInput{{ProductID: id, Qty: dec("1")}}}); err != nil {
		t.Fatalf("un pedido de plataforma con un producto apagado para mostrador se rechazó: %v", err)
	}

	doc, err := app.NewMenuService(st, clock).Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range doc.Products {
		if p.ID == id {
			t.Fatal("el menú de Vender trae un producto apagado para mostrador")
		}
	}
}

// EL PRECIO WEB (spec 092, US3): no toca el de mostrador ni los de plataforma; vacío hereda; cero o
// absurdo se rechaza como validación, no como un 500 del check de la base.
func TestWebPrice(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin, ctx := adminAsApp(t, st, defaultCompanyID, "precio")
	id := makeProduct(t, st, "Frappé de taro", dec("65"), false)

	if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: true, Web: true, WebPrice: ptr(dec("69")), AllowOversell: true}); err != nil {
		t.Fatal(err)
	}
	var counter decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(), `select price from products where id = $1`, id).Scan(&counter); err != nil {
		t.Fatal(err)
	}
	if !counter.Equal(dec("65")) {
		t.Fatalf("el precio de mostrador cambió a %s al capturar el web", counter)
	}
	var platform int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from product_platform_prices where product_id = $1`, id).Scan(&platform); err != nil {
		t.Fatal(err)
	}
	if platform != 0 {
		t.Fatal("el precio web se guardó como precio de plataforma")
	}

	for _, bad := range []string{"0", "-1", "69.999"} {
		err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: true, Web: true, WebPrice: ptr(dec(bad)), AllowOversell: true})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("un precio web de %s se aceptó o falló sin ser validación: %v", bad, err)
		}
	}
	if p := salesProductRow(t, ctx, admin, id); p.WebPrice == nil || !p.WebPrice.Equal(dec("69")) {
		t.Fatal("un precio rechazado cambió el que ya estaba")
	}

	if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: true, Web: true, WebPrice: nil, AllowOversell: true}); err != nil {
		t.Fatal(err)
	}
	if p := salesProductRow(t, ctx, admin, id); p.WebPrice != nil {
		t.Fatalf("vaciar el precio web dejó %s", p.WebPrice)
	}
}

func ptr[T any](v T) *T { return &v }

// Revisión de código de la 092: una descripción con un byte NUL es 400 y no un 500 de la base;
// «vender sin existencia» apagado en algo que no descuenta nada se guarda encendido; y el cambio de
// dónde se vende deja evento en la bitácora.
func TestSalesInputEdges(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin, ctx := adminAsApp(t, st, defaultCompanyID, "bordes")
	id := makeProduct(t, st, "Agua del día", dec("20"), false)

	err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: true, Web: true, AllowOversell: true, Description: "a\x00b"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("una descripción con NUL debía ser validación, fue %v", err)
	}
	if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: true, Web: true, AllowOversell: true, Description: "Fresca\ndel día"}); err != nil {
		t.Fatalf("una descripción de dos renglones se rechazó: %v", err)
	}

	if err := updateWith(t, ctx, admin, st, id, &app.SalesInput{Counter: false, Web: true, AllowOversell: false}); err != nil {
		t.Fatal(err)
	}
	if p := salesProductRow(t, ctx, admin, id); !p.AllowOversell {
		t.Fatal("un producto que no descuenta nada quedó con «vender sin existencia» apagado: se agotaría el día que gane receta")
	}
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = 'product.sales_changed' and entity_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("apagar Mostrador dejó %d eventos de bitácora, se esperaba 1", n)
	}
}
