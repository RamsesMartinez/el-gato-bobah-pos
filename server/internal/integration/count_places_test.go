//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LUGARES PARA CONTAR (spec 040): una lista por empresa, con orden y archivo, en vez del texto libre.

// placeIngredient da de alta un insumo en la empresa, como owner.
func placeIngredient(t *testing.T, st *store.Store, company int64, name string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into ingredients (company_id, name, base_unit_id) select $1, $2, id from units where code = 'pieza' returning id`,
		company, name).Scan(&id); err != nil {
		t.Fatalf("insumo %s: %v", name, err)
	}
	return id
}

func placeNames(t *testing.T, ctx context.Context, svc *app.BackofficeService) []string {
	t.Helper()
	ps, err := svc.ListCountPlaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

func TestChoosingAPlaceFromTheList(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-elegir")
	svc, ctx := asWaste(t, co)
	ing := placeIngredient(t, st, co, "Leche de lugares")

	refri, err := svc.CreateCountPlace(ctx, "  Refri ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateCountPlace(ctx, "Barra"); err != nil {
		t.Fatal(err)
	}
	if got := placeNames(t, ctx, svc); len(got) != 2 || got[0] != "Refri" || got[1] != "Barra" {
		t.Fatalf("lista = %v; quiere [Refri Barra] en el orden en que se crearon, sin espacios de más", got)
	}

	t.Run("un nombre que ya existe con otra mayúscula o acento no crea otro y dice cuál es", func(t *testing.T) {
		_, err := svc.CreateCountPlace(ctx, " REFRÍ ")
		var exists domain.CountPlaceExists
		if !errors.As(err, &exists) || exists.ID != refri {
			t.Fatalf("= %v; quiere el lugar %d que ya existe", err, refri)
		}
	})
	t.Run("asignar y quitar", func(t *testing.T) {
		if err := svc.SetCountPlace(ctx, ing, &refri); err != nil {
			t.Fatal(err)
		}
		var got *int64
		if err := st.Pool.QueryRow(ctx, `select count_place_id from ingredients where id = $1`, ing).Scan(&got); err != nil || got == nil || *got != refri {
			t.Fatalf("lugar = %v, %v", got, err)
		}
		if err := svc.SetCountPlace(ctx, ing, nil); err != nil {
			t.Fatal(err)
		}
		if err := st.Pool.QueryRow(ctx, `select count_place_id from ingredients where id = $1`, ing).Scan(&got); err != nil || got != nil {
			t.Fatalf("tras quitarlo = %v, %v", got, err)
		}
	})
	t.Run("nombre vacío o de más de 30 letras", func(t *testing.T) {
		for _, n := range []string{"   ", "1234567890123456789012345678901"} {
			if _, err := svc.CreateCountPlace(ctx, n); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("%q = %v; quiere ErrValidation", n, err)
			}
		}
	})
}

// Un lugar de otra empresa no se ve ni se asigna; uno archivado no se asigna.
func TestAPlaceOfAnotherCompanyOrArchivedCannotBeAssigned(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-duenos")
	other := makeCompany(t, st, "lugares-otros")
	svc, ctx := asWaste(t, co)
	place, err := svc.CreateCountPlace(ctx, "Bodega de la dueña")
	if err != nil {
		t.Fatal(err)
	}
	archived, err := svc.CreateCountPlace(ctx, "Archivado")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveCountPlace(ctx, archived); err != nil {
		t.Fatal(err)
	}
	ing := placeIngredient(t, st, co, "Insumo de la dueña")
	if err := svc.SetCountPlace(ctx, ing, &archived); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("asignar uno archivado = %v; quiere conflicto", err)
	}

	theirs := placeIngredient(t, st, other, "Insumo de la otra")
	inTheThreeCases(t, co, other, func(t *testing.T, s *store.Store, c context.Context) {
		svc := app.NewBackofficeService(s, clock)
		ps, err := svc.ListCountPlaces(c)
		if err == nil {
			for _, p := range ps {
				if p.ID == place {
					t.Fatal("otra sesión ve los lugares de la dueña")
				}
			}
		}
		if err := svc.SetCountPlace(c, theirs, &place); err == nil {
			t.Fatal("otra sesión asignó el lugar de la dueña")
		}
	})
}

// Asignar un lugar mientras otra tableta lo archiva: sin candado, el artículo quedaba en un lugar
// archivado que ya no se ofrece (revisión de arquitectura, E5).
func TestAssigningWhileArchivingNeverLeavesAnArchivedPlace(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-carrera")
	svcA, ctxA := asWaste(t, co)
	svcB, ctxB := asWaste(t, co)
	place, err := svcA.CreateCountPlace(ctxA, "Refri en disputa")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		ing := placeIngredient(t, st, co, "Insumo en carrera "+string(rune('a'+i)))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = svcA.SetCountPlace(ctxA, ing, &place) }()
		go func() { defer wg.Done(); _ = svcB.ArchiveCountPlace(ctxB, place) }()
		wg.Wait()
		var bad int
		if err := st.Pool.QueryRow(context.Background(), `
			select count(*) from ingredients i join count_places p on p.id = i.count_place_id
			 where p.archived_at is not null`).Scan(&bad); err != nil {
			t.Fatal(err)
		}
		if bad != 0 {
			t.Fatalf("vuelta %d: %d artículos quedaron en un lugar archivado", i, bad)
		}
		if place, err = svcA.CreateCountPlace(ctxA, fmt.Sprintf("Refri en disputa %d", i)); err != nil {
			t.Fatal(err)
		}
	}
}
