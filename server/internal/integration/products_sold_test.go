//go:build integration

package integration

import (
	"context"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LO VENDIDO POR PRODUCTO CUENTA LO QUE SALIÓ DENTRO DE UN PAQUETE (spec 028, historia 4).
//
// Sin esto, «cuántas crepas salieron» contesta solo las sueltas: un paquete «Frappé + crepa» se
// reporta como un renglón con su propio nombre, y la crepa que llevaba no aparece en ningún lado.
// Lo de un renglón cancelado no cuenta, igual que en la venta suelta.
func TestProductsSoldCountsWhatWentInsidePackages(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_vendidos", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	crepe := makeProduct(t, st, "Crepa vendidos", decimal.RequireFromString("60"), true)
	soda := makeProduct(t, st, "Refresco vendidos", decimal.RequireFromString("30"), true)
	pkg := makeProduct(t, st, "Paquete vendidos", decimal.RequireFromString("80"), false)
	if _, err := st.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, pkg); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []struct {
		product int64
		min     int
	}{{crepe, 1}, {soda, 2}} {
		if _, err := st.Pool.Exec(ctx, `
			with s as (insert into combo_slots (combo_id, name, min_select, max_select) values ($1, 'hueco', $3, $3) returning id)
			insert into combo_slot_products (slot_id, product_id, is_default) select id, $2, true from s`,
			pkg, slot.product, slot.min); err != nil {
			t.Fatal(err)
		}
	}

	order, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{
			{ProductID: crepe, Qty: decimal.RequireFromString("1")},
			{ProductID: pkg, Qty: decimal.RequireFromString("2")},
			{ProductID: pkg, Qty: decimal.RequireFromString("1")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// El paquete ×1 se cancela antes de cocina: lo que llevaba no se vendió.
	var cancelled int64
	for _, l := range order.Lines {
		if l.ProductName == "Paquete vendidos" && l.Quantity.Equal(decimal.RequireFromString("1")) {
			cancelled = l.ID
		}
	}
	if _, err := st.Pool.Exec(ctx, `update order_lines set enviado_a_cocina_at = null where id = $1`, cancelled); err != nil {
		t.Fatal(err)
	}
	if _, err := orders.CancelarRenglon(ctx, order.ID, cancelled, cashier, "no lo quiso"); err != nil {
		t.Fatal(err)
	}

	day := fixedNow
	rows, err := back.ProductsSold(ctx, day, day, 50)
	if err != nil {
		t.Fatal(err)
	}
	type sold struct{ alone, inPackages string }
	got := map[string]sold{}
	for _, r := range rows {
		got[r.ProductName] = sold{r.Alone.String(), r.InPackages.String()}
	}
	if g := got["Crepa vendidos"]; g != (sold{"1", "2"}) {
		t.Fatalf("crepa: 1 suelta y 2 en paquetes (el cancelado no cuenta), salió %+v", g)
	}
	if g := got["Refresco vendidos"]; g != (sold{"0", "4"}) {
		t.Fatalf("refresco: 0 suelto y 4 en paquetes, salió %+v", g)
	}

	// Lo filtra RLS, no la consulta: desde otra empresa, una conexión reciclada o sin empresa no
	// aparece nada de lo vendido aquí.
	other := makeCompany(t, st, "empresa-otra-vendidos")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		rows, err := app.NewBackofficeService(st, clock).ProductsSold(ctx, day, day, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.ProductName == "Crepa vendidos" || r.ProductName == "Refresco vendidos" {
				t.Fatalf("se vio lo vendido por otra empresa: %+v", r)
			}
		}
	})
}

// UN INSUMO PREPARADO DESCUENTA LO QUE LO COMPONE (spec 028, historia 5).
func TestAPrepIngredientDepletesItsComponents(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, st, "cajero_jarabe", "cajero")
	abrirCajaPrincipal(t, st, cashier)
	orders := app.NewOrdersService(st, clock)

	sugar := makeIngredient(t, st, "Azúcar jarabe")
	water := makeIngredient(t, st, "Agua jarabe")
	syrup := makeIngredient(t, st, "Jarabe natural")
	if _, err := st.Pool.Exec(ctx, `update ingredients set is_prep = true, recipe_id = $1, yield_qty = 1000 where id = $2`,
		makeRecipe(t, st, map[int64]string{sugar: "500", water: "500"}), syrup); err != nil {
		t.Fatal(err)
	}
	tea := makeProduct(t, st, "Té jarabe", decimal.RequireFromString("40"), false)
	if _, err := st.Pool.Exec(ctx, `update products set recipe_id = $1 where id = $2`,
		makeRecipe(t, st, map[int64]string{syrup: "30"}), tea); err != nil {
		t.Fatal(err)
	}
	if _, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cashier,
		Lines: []domain.OrderLineInput{{ProductID: tea, Qty: decimal.RequireFromString("2")}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, st, sugar); !got.Equal(decimal.RequireFromString("-30")) {
		t.Fatalf("2 tés × 30 ml de jarabe = 60 ml; el jarabe es mitad azúcar: quería -30, hay %s", got)
	}
	if got := onHandIngredient(t, st, syrup); !got.IsZero() {
		t.Fatalf("el jarabe se descuenta en lo que lo compone, no él mismo: hay %s", got)
	}
}
