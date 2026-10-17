//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// ADMINISTRAR LOS LUGARES (spec 040, historia 2): renombrar, ordenar el recorrido y archivar.

func TestRenamingAPlaceRenamesItForAllItsItems(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-renombrar")
	svc, ctx := asWaste(t, co)
	refri, _ := svc.CreateCountPlace(ctx, "Refri")
	barra, _ := svc.CreateCountPlace(ctx, "Barra")
	for i := range 3 {
		ing := placeIngredient(t, st, co, fmt.Sprintf("En el refri %d", i))
		if err := svc.SetCountPlace(ctx, ing, &refri); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.RenameCountPlace(ctx, refri, "Refrigerador"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from ingredients i join count_places p on p.id = i.count_place_id where p.name = 'Refrigerador'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("%d artículos dicen «Refrigerador», %v; quiere 3", n, err)
	}
	var exists domain.CountPlaceExists
	if err := svc.RenameCountPlace(ctx, refri, " barra"); !errors.As(err, &exists) || exists.ID != barra {
		t.Fatalf("renombrar como otro = %v; quiere conflicto con %d", err, barra)
	}
}

func TestMovingAPlaceChangesTheCountRoute(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-mover")
	svc, ctx := asWaste(t, co)
	ids := map[string]int64{}
	for _, n := range []string{"Refri", "Barra", "Bodega"} {
		id, err := svc.CreateCountPlace(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = id
	}
	if err := svc.MoveCountPlace(ctx, ids["Bodega"], 0); err != nil {
		t.Fatal(err)
	}
	if got := placeNames(t, ctx, svc); !slices.Equal(got, []string{"Bodega", "Refri", "Barra"}) {
		t.Fatalf("recorrido = %v", got)
	}
	for _, to := range []int{-1, 3} {
		if err := svc.MoveCountPlace(ctx, ids["Refri"], to); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("mover a %d = %v; quiere ErrValidation", to, err)
		}
	}

	t.Run("el conteo recorre por el orden de los lugares, los sin lugar al final", func(t *testing.T) {
		mk := func(name, place string) {
			ing := placeIngredient(t, st, co, name)
			if _, err := st.Pool.Exec(ctx, `update ingredients set track_stock = true where id = $1`, ing); err != nil {
				t.Fatal(err)
			}
			if place != "" {
				id := ids[place]
				if err := svc.SetCountPlace(ctx, ing, &id); err != nil {
					t.Fatal(err)
				}
			}
		}
		mk("A en barra", "Barra")
		mk("B sin lugar", "")
		mk("C en bodega", "Bodega")
		mk("D en refri", "Refri")
		c, err := svc.OpenCount(ctx, makeUserIn(t, st, co, "gerente_recorrido", "gerente"))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, it := range c.Items {
			got = append(got, it.Name)
		}
		if !slices.Equal(got, []string{"C en bodega", "D en refri", "A en barra", "B sin lugar"}) {
			t.Fatalf("recorrido del conteo = %v", got)
		}
	})
}

// Crear y mover a la vez repetían posición: el orden del conteo quedaba al azar (E7). Dos renombres
// a la vez no pueden dejar dos lugares con el mismo nombre.
func TestConcurrentPlaceChangesKeepOneOrderAndUniqueNames(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-concurrencia")
	svc, ctx := asWaste(t, co)
	first, _ := svc.CreateCountPlace(ctx, "Primero")
	// Cada rutina con su propia sesión: una sesión es una sola conexión y no se comparte.
	type sess struct {
		svc *app.BackofficeService
		ctx context.Context
	}
	sessions := make([]sess, 12)
	for i := range sessions {
		s, c := asWaste(t, co)
		sessions[i] = sess{s, c}
	}
	var wg sync.WaitGroup
	var created int32
	var mu sync.Mutex
	for i := range 6 {
		a, b := sessions[2*i], sessions[2*i+1]
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := a.svc.CreateCountPlace(a.ctx, fmt.Sprintf("Alta %d", i)); err == nil {
				mu.Lock()
				created++
				mu.Unlock()
			} else {
				t.Errorf("alta %d: %v", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := b.svc.MoveCountPlace(b.ctx, first, 0); err != nil {
				t.Errorf("mover: %v", err)
			}
		}()
	}
	wg.Wait()
	var dup, active int
	if err := st.Pool.QueryRow(context.Background(), `
		select (select count(*) from (select position from count_places where company_id = $1 and archived_at is null
		                               group by position having count(*) > 1) d),
		       (select count(*) from count_places where company_id = $1 and archived_at is null)`, co).Scan(&dup, &active); err != nil {
		t.Fatal(err)
	}
	if dup != 0 || active != 7 || created != 6 {
		t.Fatalf("%d posiciones repetidas, %d lugares (quiere 7), %d altas", dup, active, created)
	}

	x, _ := svc.CreateCountPlace(ctx, "Equis")
	y, _ := svc.CreateCountPlace(ctx, "Ye")
	a, b := sessions[0], sessions[1]
	var okA, okB error
	wg.Add(2)
	go func() { defer wg.Done(); okA = a.svc.RenameCountPlace(a.ctx, x, "Mismo nombre") }()
	go func() { defer wg.Done(); okB = b.svc.RenameCountPlace(b.ctx, y, "mismo  nombre") }()
	wg.Wait()
	if (okA == nil) == (okB == nil) {
		t.Fatalf("renombres = %v y %v; quiere que gane uno y el otro responda conflicto", okA, okB)
	}
	var same int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from count_places where company_id = $1 and lower(name) like 'mismo%'`, co).Scan(&same); err != nil || same != 1 {
		t.Fatalf("%d lugares con el mismo nombre, %v; quiere 1", same, err)
	}
}

func TestArchivingAPlaceMovesItsItems(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-archivar")
	svc, ctx := asWaste(t, co)
	refri, _ := svc.CreateCountPlace(ctx, "Refri")
	barra, _ := svc.CreateCountPlace(ctx, "Barra")
	active := placeIngredient(t, st, co, "Activo en refri")
	inactive := placeIngredient(t, st, co, "Inactivo en refri")
	for _, ing := range []int64{active, inactive} {
		if err := svc.SetCountPlace(ctx, ing, &refri); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, inactive); err != nil {
		t.Fatal(err)
	}

	var has app.CountPlaceHasIngredients
	if err := svc.ArchiveCountPlace(ctx, refri); !errors.As(err, &has) || has.Ingredients != 2 {
		t.Fatalf("archivar sin destino = %v; quiere conflicto con 2 artículos (también el inactivo)", err)
	}
	if err := svc.ArchiveCountPlaceMoving(ctx, refri, &refri); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("pasarlos al mismo lugar = %v; quiere ErrValidation", err)
	}
	if err := svc.ArchiveCountPlaceMoving(ctx, refri, &barra); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from ingredients where count_place_id = $1`, barra).Scan(&n); err != nil || n != 2 {
		t.Fatalf("%d en Barra, %v; quiere 2, el inactivo incluido", n, err)
	}
	if got := placeNames(t, ctx, svc); !slices.Equal(got, []string{"Barra"}) {
		t.Fatalf("lista = %v; el archivado no se ofrece", got)
	}
	if err := svc.ArchiveCountPlaceMoving(ctx, barra, &refri); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("pasarlos a uno archivado = %v; quiere conflicto", err)
	}
	if err := svc.ArchiveCountPlaceMoving(ctx, barra, nil); err != nil {
		t.Fatalf("pasarlos a «Sin lugar» = %v", err)
	}
}
