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

// LAS BARRERAS DE «QUÉ LLEVA» QUE NO TENÍAN PRUEBA (auditoría de seguridad de la spec 028).
func TestCompositionGuards(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	admin := makeUser(t, owner, "admin_barreras", "admin")
	g := unitID(t, owner, "g")
	crepe := makeProduct(t, owner, "Crepa barreras", decimal.RequireFromString("60"), false)
	soda := makeProduct(t, owner, "Refresco barreras", decimal.RequireFromString("30"), true)
	water := makeProduct(t, owner, "Agua barreras", decimal.RequireFromString("20"), true)
	syrup := makeIngredient(t, owner, "Jarabe barreras")

	other := makeCompany(t, owner, "empresa-otra-barreras")
	var otherCat, foreignProduct, foreignIngredient int64
	if err := owner.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'Ajena') returning id`, other).Scan(&otherCat); err != nil {
		t.Fatal(err)
	}
	if err := owner.Pool.QueryRow(ctx, `insert into products (company_id, name, category_id, price) values ($1, 'Producto ajeno', $2, 10) returning id`,
		other, otherCat).Scan(&foreignProduct); err != nil {
		t.Fatal(err)
	}
	if err := owner.Pool.QueryRow(ctx, `
		insert into ingredients (company_id, name, base_unit_id) select $1, 'Insumo ajeno barreras', id from units where code = 'g' returning id`,
		other).Scan(&foreignIngredient); err != nil {
		t.Fatal(err)
	}

	// Un paquete que deja elegir: un hueco con refresco o agua.
	choice := makeProduct(t, owner, "Paquete con opción barreras", decimal.RequireFromString("80"), false)
	if _, err := owner.Pool.Exec(ctx, `
		update products set type = 'combo' where id = $1;`, choice); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx, `
		with s as (insert into combo_slots (combo_id, name) values ($1, 'Bebida') returning id)
		insert into combo_slot_products (slot_id, product_id, is_default) select id, p, p = $2 from s, unnest(array[$2, $3]::bigint[]) p`,
		choice, soda, water); err != nil {
		t.Fatal(err)
	}

	st := appRoleStore(t)
	tctx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(st)
	one := decimal.RequireFromString("1")

	t.Run("un paquete que deja elegir no se reescribe en silencio", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfProduct, choice,
			app.CompositionRequest{Items: []app.CompositionInputItem{{IngredientID: syrup, Quantity: one, UnitID: g}}}, admin)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("debe rechazarse como conflicto: %v", err)
		}
		var slots, products int
		if err := owner.Pool.QueryRow(ctx, `
			select count(distinct cs.id), count(csp.product_id) from combo_slots cs
			  join combo_slot_products csp on csp.slot_id = cs.id where cs.combo_id = $1`, choice).Scan(&slots, &products); err != nil {
			t.Fatal(err)
		}
		if slots != 1 || products != 2 {
			t.Fatalf("las opciones del paquete se tocaron: %d huecos, %d productos", slots, products)
		}
	})

	// Un hueco sin producto por omisión no sale en «Qué lleva»: guardar lo borraría sin que nadie lo
	// viera y el paquete quedaría confirmado a medias. Lo encontró la revisión de código.
	t.Run("un paquete con un hueco vacío no se reescribe en silencio", func(t *testing.T) {
		gap := makeProduct(t, owner, "Paquete con hueco barreras", decimal.RequireFromString("80"), false)
		if _, err := owner.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, gap); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Pool.Exec(ctx, `
			with a as (insert into combo_slots (combo_id, name, position) values ($1, 'Crepa', 0) returning id),
			     b as (insert into combo_slots (combo_id, name, position) values ($1, 'Bebida', 1) returning id)
			insert into combo_slot_products (slot_id, product_id, is_default)
			select id, $2::bigint, true from a union all select id, $3::bigint, false from b`, gap, crepe, soda); err != nil {
			t.Fatal(err)
		}
		err := svc.SaveComposition(tctx, app.CompositionOfProduct, gap,
			app.CompositionRequest{Components: []app.CompositionComponent{{ProductID: crepe, Quantity: 1}}}, admin)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("debe rechazarse como conflicto: %v", err)
		}
		// Y la hoja lo dice al abrir, no al guardar.
		v, err := svc.Composition(tctx, app.CompositionOfProduct, gap)
		if err != nil {
			t.Fatal(err)
		}
		if v.Editable || v.Reason != "package_choices" {
			t.Fatalf("la hoja debe avisar que este paquete no se captura aquí: %+v", v)
		}
	})

	t.Run("un producto de otra empresa no entra en un paquete", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfProduct, crepe,
			app.CompositionRequest{Components: []app.CompositionComponent{{ProductID: foreignProduct, Quantity: 1}}}, admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("debe rechazarse: %v", err)
		}
	})

	t.Run("un insumo de otra empresa no entra en un insumo preparado", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfIngredient, syrup, app.CompositionRequest{
			Items: []app.CompositionInputItem{{IngredientID: foreignIngredient, Quantity: one, UnitID: g}}, Yield: &one,
		}, admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("debe rechazarse: %v", err)
		}
	})

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		err := app.NewAdminService(st).SaveComposition(ctx, app.CompositionOfProduct, crepe,
			app.CompositionRequest{Components: []app.CompositionComponent{{ProductID: soda, Quantity: 1}}}, admin)
		if err == nil {
			t.Fatal("se convirtió en paquete el producto de otra empresa")
		}
		var typ string
		if err := owner.Pool.QueryRow(context.Background(), `select type::text from products where id = $1`, crepe).Scan(&typ); err != nil {
			t.Fatal(err)
		}
		if typ != "simple" {
			t.Fatalf("el producto de la empresa dueña cambió a %s", typ)
		}
	})
}
