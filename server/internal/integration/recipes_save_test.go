//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GUARDAR UNA RECETA: a los extras que se llaman igual, y sin pisar lo que otra persona guardó.
func TestSavingARecipeForTwinsAndAgainstAStaleCopy(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	admin := makeUser(t, owner, "admin_gemelos", "admin")
	pearl := makeIngredient(t, owner, "Perla gemelos")
	g := unitID(t, owner, "g")
	topping := opcionConTope(t, owner, "Toppings gemelos", "Perla", decimal.RequireFromString("10"), 3)
	twin := opcionConTope(t, owner, "Toppings frappé gemelos", "Perla", decimal.RequireFromString("10"), 3)
	stranger := opcionConTope(t, owner, "Salsas gemelos", "BBQ", decimal.RequireFromString("10"), 3)

	st := appRoleStore(t)
	tctx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(st)
	item := []app.CompositionInputItem{{IngredientID: pearl, Quantity: decimal.RequireFromString("45"), UnitID: g}}

	t.Run("la misma receta queda en el extra que se llama igual", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, topping,
			app.CompositionRequest{Items: item, AlsoOptionIDs: []int64{twin}}, admin); err != nil {
			t.Fatal(err)
		}
		v, err := svc.Composition(tctx, app.CompositionOfOption, twin)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Items) != 1 || v.Status != "confirmed" {
			t.Fatalf("el gemelo debe quedar con la receta: %+v", v)
		}
		// Y el buscador de insumos sabe que la perla va en dos recetas: los más usados van arriba.
		ings, err := app.NewBackofficeService(st, clock).Ingredients(tctx, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, in := range ings {
			if in.ID == pearl && in.RecipeUses != 2 {
				t.Fatalf("la perla aparece en 2 recetas, dice %d", in.RecipeUses)
			}
		}
	})
	t.Run("no se aplica a un extra que se llama distinto", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfOption, topping,
			app.CompositionRequest{Items: item, AlsoOptionIDs: []int64{stranger}}, admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("debe rechazarse: %v", err)
		}
	})
	// Repetido, un mismo gemelo crea una receta nueva por cada vez y deja las anteriores huérfanas,
	// todo con el candado de la empresa tomado: un PUT de un megabyte bloqueaba cualquier captura.
	t.Run("un extra repetido se rechaza", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfOption, topping,
			app.CompositionRequest{Items: item, AlsoOptionIDs: []int64{twin, twin}}, admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("debe rechazarse: %v", err)
		}
	})
	// El aviso de copia vieja cubre también al extra que se llama igual: si alguien guardó el de
	// frappé mientras otra persona editaba el de Toppings, copiarle la receta lo pisaba sin avisar.
	t.Run("el extra del mismo nombre que otra persona cambió no se pisa", func(t *testing.T) {
		opened, err := svc.Composition(tctx, app.CompositionOfOption, topping)
		if err != nil {
			t.Fatal(err)
		}
		if len(opened.SameName) != 1 || opened.SameName[0].Stamp == "" {
			t.Fatalf("la pantalla necesita la marca del gemelo: %+v", opened.SameName)
		}
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, twin, app.CompositionRequest{}, admin); err != nil {
			t.Fatal(err)
		}
		err = svc.SaveComposition(tctx, app.CompositionOfOption, topping, app.CompositionRequest{
			Items: item, BasedOn: &opened.Stamp, AlsoOptionIDs: []int64{twin},
			AlsoBasedOn: map[int64]string{twin: opened.SameName[0].Stamp},
		}, admin)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("debe avisar que el gemelo cambió: %v", err)
		}
		after, err := svc.Composition(tctx, app.CompositionOfOption, twin)
		if err != nil {
			t.Fatal(err)
		}
		if len(after.Items) != 0 {
			t.Fatal("se pisó lo que la otra persona guardó en el gemelo")
		}
	})
	t.Run("si otra persona la cambió mientras se editaba, se avisa y no se pisa", func(t *testing.T) {
		before, err := svc.Composition(tctx, app.CompositionOfOption, topping)
		if err != nil {
			t.Fatal(err)
		}
		// Ana guarda primero, sobre la misma copia.
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, topping,
			app.CompositionRequest{Items: item, BasedOn: &before.Stamp}, admin); err != nil {
			t.Fatal(err)
		}
		// Luego guarda quien abrió la misma copia antes que ella.
		err = svc.SaveComposition(tctx, app.CompositionOfOption, topping,
			app.CompositionRequest{BasedOn: &before.Stamp}, admin)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("debe avisar que otra persona la cambió: %v", err)
		}
		after, err := svc.Composition(tctx, app.CompositionOfOption, topping)
		if err != nil {
			t.Fatal(err)
		}
		if len(after.Items) != 1 {
			t.Fatal("la receta de Ana se pisó")
		}
	})
}
