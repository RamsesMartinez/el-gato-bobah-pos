//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func productAltNames(t *testing.T, st *store.Store, id int64) []string {
	t.Helper()
	var alts []string
	if err := st.Pool.QueryRow(context.Background(), `select alt_names from products where id = $1`, id).Scan(&alts); err != nil {
		t.Fatal(err)
	}
	return alts
}

func requireAltTaken(t *testing.T, err error, name, owner string) domain.AltNameTaken {
	t.Helper()
	var taken domain.AltNameTaken
	if !errors.As(err, &taken) {
		t.Fatalf("quería AltNameTaken por «%s», tuve %v", name, err)
	}
	if taken.Name != name || taken.OwnerName != owner {
		t.Fatalf("chocó «%s» con «%s», quería «%s» con «%s»", taken.Name, taken.OwnerName, name, owner)
	}
	return taken
}

// T011: guardar alternativos de un producto y el borde de unicidad.
func TestProductAltNames(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)
	price := decimal.RequireFromString("60")
	ice := makeProduct(t, st, "Ice Coffee", price, false)
	latte := makeProduct(t, st, "Latte", price, false)
	frappe := makeProduct(t, st, "Frappé de café", price, false)
	capu := makeProduct(t, st, "Capuchino", price, false)

	t.Run("guarda limpios y en orden", func(t *testing.T) {
		got, err := admin.SetProductAltNames(ctx, ice, []string{"  Café   frío ", "Café helado"})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"Café frío", "Café helado"}
		if !slices.Equal(got, want) || !slices.Equal(productAltNames(t, st, ice), want) {
			t.Fatalf("respondió %v y guardó %v, quería %v", got, productAltNames(t, st, ice), want)
		}
	})

	t.Run("choca con el nombre de otro producto, sin acentos", func(t *testing.T) {
		_, err := admin.SetProductAltNames(ctx, ice, []string{"Café frío", "frappe de cafe"})
		taken := requireAltTaken(t, err, "frappe de cafe", "Frappé de café")
		if taken.OwnerAlt || taken.OwnerID != frappe {
			t.Fatalf("debía chocar con el NOMBRE de %d: %+v", frappe, taken)
		}
		if !slices.Equal(productAltNames(t, st, ice), []string{"Café frío", "Café helado"}) {
			t.Fatal("un choque guardó parte de la lista")
		}
	})

	t.Run("choca con un alternativo de otro producto, otro espaciado y sin acento", func(t *testing.T) {
		if _, err := admin.SetProductAltNames(ctx, latte, []string{"Café con leche"}); err != nil {
			t.Fatal(err)
		}
		_, err := admin.SetProductAltNames(ctx, capu, []string{"cafe  con LECHE"})
		taken := requireAltTaken(t, err, "cafe con LECHE", "Latte")
		if !taken.OwnerAlt {
			t.Fatalf("debía decir que es OTRO nombre de Latte: %+v", taken)
		}
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatal("no es un conflicto")
		}
	})

	t.Run("un producto inactivo sigue reservando y el aviso lo dice", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `update products set is_active = false where id = $1`, frappe); err != nil {
			t.Fatal(err)
		}
		_, err := admin.SetProductAltNames(ctx, capu, []string{"Frappé de café"})
		taken := requireAltTaken(t, err, "Frappé de café", "Frappé de café")
		if !taken.OwnerArchived || taken.Error() != "«Frappé de café» ya es el nombre de «Frappé de café» (inactivo)" {
			t.Fatalf("no dice inactivo: %q", taken.Error())
		}
	})

	t.Run("quitar todos guarda vacío, no nulo", func(t *testing.T) {
		got, err := admin.SetProductAltNames(ctx, latte, nil)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %#v, %v", got, err)
		}
		var empty bool
		if err := st.Pool.QueryRow(ctx, `select alt_names = '{}' from products where id = $1`, latte).Scan(&empty); err != nil || !empty {
			t.Fatalf("no quedó '{}': %v", err)
		}
	})

	t.Run("el producto genérico de plataforma no lleva alternativos", func(t *testing.T) {
		var generic int64
		if err := st.Pool.QueryRow(ctx, `select id from products where system_kind = 'platform_unpaired' and company_id = $1`, defaultCompanyID).Scan(&generic); err != nil {
			t.Skip("la empresa de prueba no tiene producto genérico")
		}
		if _, err := admin.SetProductAltNames(ctx, generic, []string{"Otro"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("quería 404, tuve %v", err)
		}
	})

	t.Run("un guardado espera el candado y ve lo que otro guardó mientras", func(t *testing.T) {
		a := makeProduct(t, st, "Americano frío", price, false)
		b := makeProduct(t, st, "Cold brew", price, false)
		holdLockWhile(t, st, 28063, func(saved <-chan error) {
			// Mientras el guardado de A espera el candado, otra tableta deja el mismo nombre en B.
			if _, err := st.Pool.Exec(ctx, `update products set alt_names = array['Café de filtro frío'] where id = $1`, b); err != nil {
				t.Fatal(err)
			}
		}, func() error {
			_, err := admin.SetProductAltNames(ctx, a, []string{"Café de filtro frío"})
			return err
		}, func(err error) {
			if !errors.Is(err, domain.ErrAltNameTaken) {
				t.Fatalf("guardó sin ver el choque (sin candado, las dos quedan): %v", err)
			}
		})
	})
}

// T012: el nombre principal no puede ser alternativo de otro, duplicar no copia y el PATCH viejo no borra.
func TestProductPrincipalNameAgainstAltNames(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)
	price := decimal.RequireFromString("60")
	ice := makeProduct(t, st, "Ice Coffee", price, false)
	other := makeProduct(t, st, "Moka", price, false)
	if _, err := admin.SetProductAltNames(ctx, ice, []string{"Café frío"}); err != nil {
		t.Fatal(err)
	}
	cat := categoriaDeEmpresa(t, st, defaultCompanyID, "Bebidas de prueba")

	t.Run("alta con el nombre de un alternativo", func(t *testing.T) {
		_, err := admin.CreateProduct(ctx, "Cafe frio", cat, price, false, false, nil)
		taken := requireAltTaken(t, err, "Cafe frio", "Ice Coffee")
		if !taken.OwnerAlt {
			t.Fatal("no dice que es otro nombre")
		}
	})

	t.Run("renombrar a un alternativo de otro", func(t *testing.T) {
		err := admin.UpdateProduct(ctx, app.UpdateProductInput{ID: other, Name: "café frío", Price: price, Active: true})
		requireAltTaken(t, err, "café frío", "Ice Coffee")
	})

	t.Run("duplicar con el nombre de un alternativo", func(t *testing.T) {
		_, err := admin.DuplicateProduct(ctx, other, "CAFÉ FRÍO", nil)
		requireAltTaken(t, err, "CAFÉ FRÍO", "Ice Coffee")
	})

	t.Run("duplicar no copia los alternativos", func(t *testing.T) {
		copyID, err := admin.DuplicateProduct(ctx, ice, "Ice Coffee grande", nil)
		if err != nil {
			t.Fatal(err)
		}
		if alts := productAltNames(t, st, copyID); len(alts) != 0 {
			t.Fatalf("la copia trae %v", alts)
		}
	})

	t.Run("la edición de siempre, sin alternativos, no los borra", func(t *testing.T) {
		if err := admin.UpdateProduct(ctx, app.UpdateProductInput{ID: ice, Name: "Ice Coffee", Price: decimal.RequireFromString("65"), Active: true}); err != nil {
			t.Fatal(err)
		}
		if alts := productAltNames(t, st, ice); !slices.Equal(alts, []string{"Café frío"}) {
			t.Fatalf("el PATCH dejó %v", alts)
		}
	})

	t.Run("renombrar a sí mismo con su propio alternativo choca con el propio", func(t *testing.T) {
		// Un producto no puede llamarse igual que uno de sus alternativos: el alternativo sobraría.
		err := admin.UpdateProduct(ctx, app.UpdateProductInput{ID: ice, Name: "Café frío", Price: price, Active: true})
		if err == nil {
			t.Fatal("se renombró a su propio alternativo")
		}
	})
}

// T013: aislamiento entre empresas en los tres casos donde RLS ya falló.
func TestProductAltNamesIsolation(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	price := decimal.RequireFromString("60")
	other := makeCompany(t, owner, "otra-alternativos")
	theirCat := categoriaDeEmpresa(t, owner, other, "Bebidas ajenas")
	var theirs int64
	if err := owner.Pool.QueryRow(ctx, `insert into products (company_id, name, category_id, price, alt_names)
		values ($1, 'Ice Coffee ajeno', $2, 60, array['Café frío']) returning id`, other, theirCat).Scan(&theirs); err != nil {
		t.Fatal(err)
	}
	mine := makeProduct(t, owner, "Iced latte", price, false)

	t.Run("el alternativo de otra empresa no choca ni se nombra", func(t *testing.T) {
		appSt := appRoleStore(t)
		tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		svc := app.NewAdminService(appSt)
		if _, err := svc.SetProductAltNames(tctx, mine, []string{"Café frío"}); err != nil {
			t.Fatalf("chocó con otra empresa: %v", err)
		}
		similar, err := svc.SimilarProductAltNames(tctx, mine, "Ice Coffee")
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range similar {
			if s.ID == theirs {
				t.Fatalf("el aviso de parecidos nombra un producto de otra empresa: %+v", s)
			}
		}
	})

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAdminService(st)
		if _, err := svc.SetProductAltNames(ctx, mine, []string{"Otro nombre"}); err == nil {
			t.Fatal("escribió los alternativos de un producto de otra empresa")
		}
		similar, err := svc.SimilarProductAltNames(ctx, 0, "Iced latte")
		if err == nil {
			for _, s := range similar {
				if s.ID == mine {
					t.Fatalf("ve el producto de la otra empresa: %+v", s)
				}
			}
		}
		if alts := productAltNames(t, owner, mine); !slices.Equal(alts, []string{"Café frío"}) {
			t.Fatalf("cambió los alternativos ajenos: %v", alts)
		}
	})
}

// holdLockWhile retiene el candado consultivo `key` de la empresa por omisión desde otra conexión,
// arranca `call` (que debe quedarse esperando ese candado), corre `meanwhile` y suelta el candado.
// Si `call` termina antes de soltarlo, no tomó el candado: la prueba truena ahí.
func holdLockWhile(t *testing.T, st *store.Store, key int, meanwhile func(<-chan error), call func() error, check func(error)) {
	t.Helper()
	ctx := context.Background()
	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1, $2)`, key, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		t.Fatalf("terminó sin esperar el candado %d: %v", key, err)
	case <-time.After(300 * time.Millisecond):
	}
	meanwhile(done)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		check(err)
	case <-time.After(10 * time.Second):
		t.Fatal("el guardado no terminó al soltar el candado")
	}
}

// T034: la lista de Productos busca por nombre y por alternativo, sin acentos, y dice cuál coincidió.
func TestProductListSearchesAltNames(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	price := decimal.RequireFromString("60")
	admin := app.NewAdminService(owner)
	ice := makeProduct(t, owner, "Ice Coffee", price, false)
	olla := makeProduct(t, owner, "Café frío de olla", price, false)
	makeProduct(t, owner, "Té verde 100%", price, false)
	if _, err := admin.SetProductAltNames(ctx, ice, []string{"Café helado", "Café frío"}); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, owner, "otra-lista-alt")
	theirCat := categoriaDeEmpresa(t, owner, other, "Bebidas ajenas lista")
	if _, err := owner.Pool.Exec(ctx, `insert into products (company_id, name, category_id, price, alt_names)
		values ($1, 'Frío ajeno', $2, 60, array['Café frío']) returning id`, other, theirCat); err != nil {
		t.Fatal(err)
	}
	search := func(t *testing.T, svc *app.AdminService, ctx context.Context, q string) app.ProductsPage {
		t.Helper()
		page, err := svc.ListProducts(ctx, "", q, 0, "", "", "", "", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(appSt)

	t.Run("por alternativo, con el que coincidió y sin acento", func(t *testing.T) {
		page := search(t, svc, tctx, "cafe fri")
		got := map[int64]string{}
		for _, p := range page.Items {
			got[p.ID] = p.MatchedAlt
		}
		if m, ok := got[ice]; !ok || m != "Café frío" {
			t.Fatalf("Ice Coffee no salió por «Café frío»: %v", got)
		}
		if m, ok := got[olla]; !ok || m != "" {
			t.Fatalf("«Café frío de olla» coincide por nombre y no lleva alternativo: %v", got)
		}
		if page.Total != 2 || len(got) != 2 {
			t.Fatalf("total %d, filas %v: quería las dos de esta empresa", page.Total, got)
		}
	})

	t.Run("el primero en el orden capturado", func(t *testing.T) {
		page := search(t, svc, tctx, "café")
		for _, p := range page.Items {
			if p.ID == ice && p.MatchedAlt != "Café helado" {
				t.Fatalf("señaló %q; el primero capturado es «Café helado»", p.MatchedAlt)
			}
		}
	})

	t.Run("% no es comodín y todo producto trae su lista", func(t *testing.T) {
		page := search(t, svc, tctx, "%")
		if len(page.Items) != 1 || page.Items[0].Name != "Té verde 100%" {
			t.Fatalf("«%%» trajo %d productos", len(page.Items))
		}
		for _, p := range search(t, svc, tctx, "").Items {
			if p.AltNames == nil {
				t.Fatalf("%s trae altNames nil: saldría null en el JSON", p.Name)
			}
		}
	})

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		for _, p := range search(t, app.NewAdminService(st), ctx, "frío").Items {
			if p.ID == ice || p.ID == olla {
				t.Fatalf("ve un producto de otra empresa: %s", p.Name)
			}
		}
	})
}
