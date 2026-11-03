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

func articleAltNames(t *testing.T, st *store.Store, id int64) []string {
	t.Helper()
	var alts []string
	if err := st.Pool.QueryRow(context.Background(), `select alt_names from ingredients where id = $1`, id).Scan(&alts); err != nil {
		t.Fatal(err)
	}
	return alts
}

func actorCtx(t *testing.T, st *store.Store, name string) context.Context {
	t.Helper()
	return app.WithActorUser(context.Background(), makeUser(t, st, name, "admin"))
}

// T039: guardar alternativos de un artículo y el borde de unicidad.
func TestArticleAltNames(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := actorCtx(t, st, "admin_alt_art")
	svc := app.NewBackofficeService(st, clock)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche entera", "ml")
	lala := ingredientIn(t, st, defaultCompanyID, "Leche Lala", "ml")
	crema := ingredientIn(t, st, defaultCompanyID, "Crema", "ml")
	makeProduct(t, st, "Leche", decimal.RequireFromString("20"), false)

	t.Run("guarda, devuelve la versión nueva de la ficha y deja evento", func(t *testing.T) {
		got, err := svc.SetArticleAltNames(ctx, leche, []string{"Leche de vaca", "Leche"})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.AltNames, []string{"Leche de vaca", "Leche"}) {
			t.Fatalf("got %v", got.AltNames)
		}
		var after time.Time
		var events int
		if err := st.Pool.QueryRow(ctx, `select updated_at, (select count(*) from audit_events where action = 'ingredient.updated'
			and entity_id = $1 and after ? 'altNames') from ingredients where id = $1`, leche).Scan(&after, &events); err != nil {
			t.Fatal(err)
		}
		// El trigger de la 0009 mueve updated_at en todo update; la ficha sigue editando con esta
		// versión. Sin ella, el siguiente autoguardado de la misma ficha respondería STALE.
		if !got.UpdatedAt.Equal(after) {
			t.Fatalf("devolvió la versión %v; la ficha quedó en %v", got.UpdatedAt, after)
		}
		if events != 1 {
			t.Fatalf("%d eventos con altNames; quería 1", events)
		}
	})

	t.Run("un producto y un artículo con el mismo nombre no chocan", func(t *testing.T) {
		if got := articleAltNames(t, st, leche); !slices.Contains(got, "Leche") {
			t.Fatalf("no guardó «Leche» aunque solo es producto: %v", got)
		}
	})

	t.Run("choca con el nombre o un alternativo de otro artículo", func(t *testing.T) {
		_, err := svc.SetArticleAltNames(ctx, crema, []string{"leche lala"})
		requireAltTaken(t, err, "leche lala", "Leche Lala")
		_, err = svc.SetArticleAltNames(ctx, crema, []string{"LECHE DE VACA"})
		if taken := requireAltTaken(t, err, "LECHE DE VACA", "Leche entera"); !taken.OwnerAlt {
			t.Fatal("no dice que es otro nombre")
		}
	})

	t.Run("un archivado reserva y el aviso lo dice", func(t *testing.T) {
		old := ingredientIn(t, st, defaultCompanyID, "Crema vieja", "ml")
		if _, err := st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, old); err != nil {
			t.Fatal(err)
		}
		_, err := svc.SetArticleAltNames(ctx, crema, []string{"Crema vieja"})
		if taken := requireAltTaken(t, err, "Crema vieja", "Crema vieja"); taken.Error() != "«Crema vieja» ya es el nombre de «Crema vieja» (archivado)" {
			t.Fatalf("mensaje: %q", taken.Error())
		}
		if _, err := svc.SetArticleAltNames(ctx, old, []string{"Crema de antes"}); err != nil {
			t.Fatalf("un archivado no deja editar sus alternativos: %v", err)
		}
	})

	t.Run("el origen de una unión no reserva nada (FR-021)", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `insert into ingredient_replacements (old_ingredient_id, new_ingredient_id, factor, user_id, kind)
			values ($1, $2, 1, (select min(id) from users), 'merge')`, lala, leche); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, lala); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetArticleAltNames(ctx, leche, []string{"Leche de vaca", "Leche", "Leche Lala"}); err != nil {
			t.Fatalf("después de unir «Leche Lala» no se puede capturar como alternativo del destino: %v", err)
		}
	})

	t.Run("dos guardados: el segundo espera el candado y ve el choque", func(t *testing.T) {
		a := ingredientIn(t, st, defaultCompanyID, "Azúcar", "g")
		b := ingredientIn(t, st, defaultCompanyID, "Azúcar morena", "g")
		holdLockWhile(t, st, 28037, func(<-chan error) {
			if _, err := st.Pool.Exec(ctx, `update ingredients set alt_names = array['Endulzante'] where id = $1`, b); err != nil {
				t.Fatal(err)
			}
		}, func() error {
			_, err := svc.SetArticleAltNames(ctx, a, []string{"Endulzante"})
			return err
		}, func(err error) {
			if !errors.Is(err, domain.ErrAltNameTaken) {
				t.Fatalf("guardó sin ver el choque: %v", err)
			}
		})
	})
}

// T040: el nombre principal contra los alternativos, la captura de un gasto o receta, y el cambio de
// unidad por reemplazo.
func TestArticlePrincipalNameAgainstAltNames(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := actorCtx(t, st, "admin_alt_art2")
	svc := app.NewBackofficeService(st, clock)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche entera", "ml")
	if _, err := svc.SetArticleAltNames(ctx, leche, []string{"Leche Lala"}); err != nil {
		t.Fatal(err)
	}
	ml := unitID(t, st, "ml")

	t.Run("alta con el nombre de un alternativo", func(t *testing.T) {
		_, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "leche lala", BaseUnitID: ml})
		var taken domain.ArticleNameTaken
		if !errors.As(err, &taken) || taken.ID != leche || taken.ViaAlt == "" {
			t.Fatalf("quería NAME_TAKEN de «Leche entera» por su alternativo, tuve %v (%+v)", err, taken)
		}
	})

	t.Run("renombrar a un alternativo de otro", func(t *testing.T) {
		other := ingredientIn(t, st, defaultCompanyID, "Leche light", "ml")
		sheet, err := svc.GetArticle(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		name := "Leche  Lala"
		_, err = svc.UpdateArticle(ctx, other, app.ArticleUpdate{ExpectedUpdatedAt: sheet.UpdatedAt, Name: &name})
		var taken domain.ArticleNameTaken
		if !errors.As(err, &taken) || taken.ID != leche {
			t.Fatalf("quería NAME_TAKEN, tuve %v", err)
		}
	})

	t.Run("cambiar la unidad por reemplazo lleva los alternativos al nuevo", func(t *testing.T) {
		vaso := ingredientIn(t, st, defaultCompanyID, "Vaso grande", "pieza")
		if _, err := svc.SetArticleAltNames(ctx, vaso, []string{"Vaso 16 oz"}); err != nil {
			t.Fatal(err)
		}
		// Una compra registrada: con ella el cambio ya no reescribe, reemplaza.
		if _, err := st.Pool.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, reason)
			values ('ingrediente', $1, 'compra', 10, 'prueba 063')`, vaso); err != nil {
			t.Fatal(err)
		}
		if err := svc.ChangeUnit(ctx, vaso, app.UnitChangeRequest{BaseUnitID: unitID(t, st, "g"), Factor: decimal.RequireFromString("20")}); err != nil {
			t.Fatal(err)
		}
		var newID int64
		if err := st.Pool.QueryRow(ctx, `select new_ingredient_id from ingredient_replacements where old_ingredient_id = $1`, vaso).Scan(&newID); err != nil {
			t.Fatal(err)
		}
		if got := articleAltNames(t, st, newID); !slices.Equal(got, []string{"Vaso 16 oz"}) {
			t.Fatalf("el nuevo trae %v", got)
		}
		if got := articleAltNames(t, st, vaso); len(got) != 0 {
			t.Fatalf("el archivado se quedó con %v: los reservaría", got)
		}
	})
}

// T041: lecturas y búsqueda de artículos con alternativos, aisladas por empresa.
func TestArticleReadsCarryAltNames(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	leche := ingredientIn(t, owner, defaultCompanyID, "Leche entera", "ml")
	ingredientIn(t, owner, defaultCompanyID, "Lalo's sal", "g")
	if _, err := owner.Pool.Exec(ctx, `update ingredients set alt_names = array['Leche de vaca', 'Leche Lala'] where id = $1`, leche); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, owner, "otra-articulos-alt")
	ingredientIn(t, owner, other, "Leche ajena", "ml")

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewBackofficeService(appSt, clock)

	t.Run("buscar en Artículos por alternativo dice cuál", func(t *testing.T) {
		rows, err := svc.ListArticles(tctx, app.ArticleFilter{Q: "lala"})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range rows {
			if r.ID == leche {
				found = true
				if r.MatchedAlt != "Leche Lala" {
					t.Fatalf("matchedAlt %q", r.MatchedAlt)
				}
			}
		}
		if !found {
			t.Fatal("no encontró «Leche entera» por «lala»")
		}
	})

	t.Run("ficha, insumos, buscador del gasto y conteo traen la lista, nunca null", func(t *testing.T) {
		sheet, err := svc.GetArticle(tctx, leche)
		if err != nil || !slices.Equal(sheet.AltNames, []string{"Leche de vaca", "Leche Lala"}) {
			t.Fatalf("ficha: %v %v", sheet.AltNames, err)
		}
		ings, err := svc.Ingredients(tctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range ings {
			if i.AltNames == nil {
				t.Fatalf("insumo %s con altNames nil", i.Name)
			}
		}
		arts, err := svc.SearchArticles(tctx, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range arts {
			if a.AltNames == nil {
				t.Fatalf("artículo %s con altNames nil", a.Name)
			}
			if a.ID == leche && !slices.Contains(a.AltNames, "Leche Lala") {
				t.Fatalf("el buscador del gasto no trae los alternativos: %v", a.AltNames)
			}
		}
		view, err := svc.OpenCount(tctx, makeUser(t, owner, "contador_alt", "admin"))
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range view.Items {
			if it.AltNames == nil {
				t.Fatalf("renglón de conteo %s con altNames nil", it.Name)
			}
			if it.IngredientID == leche && !slices.Contains(it.AltNames, "Leche Lala") {
				t.Fatal("el conteo no trae los alternativos")
			}
		}
	})

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		rows, err := app.NewBackofficeService(st, clock).ListArticles(ctx, app.ArticleFilter{Q: "leche"})
		if err != nil {
			return
		}
		for _, r := range rows {
			if r.ID == leche {
				t.Fatal("ve un artículo de otra empresa")
			}
		}
	})
}

// FR-014: un gasto que crea un artículo con el nombre de un alternativo usa el existente.
func TestAnExpenseCreatingAnAltNameUsesTheExistingArticle(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	g := gramos(t, k.st)
	azucar := ingredientIn(t, k.st, defaultCompanyID, "Azúcar estándar", "g")
	if _, err := k.svc.SetArticleAltNames(ctx, azucar, []string{"Azúcar Zulka"}); err != nil {
		t.Fatal(err)
	}
	q := qty("1")
	if _, err := k.svc.CreateExpense(ctx, app.ExpenseInput{
		CategoryID: k.category, Amount: qty("40"), Status: domain.ExpensePendiente, UserID: k.user,
		Items: []app.ExpenseItemInput{{ItemType: "ingrediente", NewArticle: &app.NewArticle{Name: "azucar  zulka", BaseUnitID: g},
			Description: "Azúcar", Quantity: q, UnitID: &g, Amount: qty("40")}},
	}); err != nil {
		t.Fatal(err)
	}
	if n := articleCount(t, k.st, defaultCompanyID, "azucar  zulka"); n != 0 {
		t.Fatalf("creó otro artículo (%d) en vez de usar «Azúcar estándar»", n)
	}
	var item int64
	if err := k.st.Pool.QueryRow(context.Background(), `select ingredient_id from expense_items where description = 'Azúcar' order by id desc limit 1`).Scan(&item); err != nil || item != azucar {
		t.Fatalf("el renglón apunta a %d, quería %d (%v)", item, azucar, err)
	}
}
