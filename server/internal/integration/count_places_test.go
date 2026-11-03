//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
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
		if err := svc.RenameCountPlace(c, place, "Tomado"); err == nil {
			t.Fatal("otra sesión renombró el lugar de la dueña")
		}
		if err := svc.MoveCountPlace(c, place, 0); err == nil {
			t.Fatal("otra sesión movió el lugar de la dueña")
		}
		if err := svc.ArchiveCountPlaceMoving(c, place, nil); err == nil {
			t.Fatal("otra sesión archivó el lugar de la dueña")
		}
	})
}

// Asignar un lugar mientras otra tableta lo archiva: sin el `for share` de la asignación, leía el lugar
// todavía activo y el artículo quedaba en un lugar archivado que ya no se ofrece (E5). Determinista:
// el archivo queda a medias en otra transacción, la asignación tiene que esperarlo, y al confirmarse
// el archivo la asignación responde conflicto.
func TestAssigningWhileArchivingNeverLeavesAnArchivedPlace(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "lugares-carrera")
	svc, ctx := asWaste(t, co)
	place, err := svc.CreateCountPlace(ctx, "Refri en disputa")
	if err != nil {
		t.Fatal(err)
	}
	ing := placeIngredient(t, st, co, "Insumo en carrera")

	tx, err := st.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `select 1 from count_places where id = $1 for update`, place); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), `update count_places set archived_at = now() where id = $1`, place); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- svc.SetCountPlace(ctx, ing, &place) }()
	select {
	case err := <-done:
		t.Fatalf("la asignación no esperó al archivo en curso: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("asignar un lugar que se acaba de archivar = %v; quiere conflicto", err)
	}
	var got *int64
	if err := st.Pool.QueryRow(context.Background(), `select count_place_id from ingredients where id = $1`, ing).Scan(&got); err != nil || got != nil {
		t.Fatalf("el artículo quedó en %v, %v; quiere sin lugar", got, err)
	}
}

// UBICACIONES (spec 043, FR-005): crear una pide configurar el almacén; quien solo cuenta elige entre
// las que hay. Ningún rol sugerido de hoy tiene contar sin configurar, así que la prueba arma ese rol
// con el mapa de permisos inyectado: con un rol propio por empresa ese caso existirá.
func TestCreatingALocationNeedsConfigure(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	soloCuenta := func(r domain.Role) []domain.Permission {
		if r == domain.RoleCajero {
			return []domain.Permission{domain.PermInventoryCount}
		}
		return domain.PermissionsFor(r)
	}
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(st, nil), MenuCache: cache.NewMenuCache(""),
		Broker: realtime.NewBroker(), Permissions: soloCuenta})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	cuenta, admin := token("ubic_cuenta", "cajero"), token("ubic_admin", "admin")
	const js = "application/json"

	if w := do(t, r, http.MethodPost, "/api/v1/stock/count-places", cuenta, []byte(`{"name":"Bodega de quien cuenta"}`), js); w.Code != http.StatusForbidden {
		t.Fatalf("crear sin configurar = %d %s; quiere 403", w.Code, w.Body.String())
	}
	w := do(t, r, http.MethodPost, "/api/v1/stock/count-places", admin, []byte(`{"name":"Bodega 043 permisos"}`), js)
	if w.Code != http.StatusCreated {
		t.Fatalf("crear con configurar = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/count-places", admin, []byte(`{"name":"  bódega 043 PERMISOS "}`), js); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"existingId"`) {
		t.Fatalf("repetida por acentos y mayúsculas = %d %s; quiere 409 con la que existe", w.Code, w.Body.String())
	}
	var place struct{ ID int64 }
	_ = json.Unmarshal(w.Body.Bytes(), &place)
	item := placeIngredient(t, st, defaultCompanyID, "Artículo de quien cuenta")
	if w := do(t, r, http.MethodPut, "/api/v1/stock/ingredients/"+strconv.FormatInt(item, 10)+"/place", cuenta, []byte(`{"placeId":`+strconv.FormatInt(place.ID, 10)+`}`), js); w.Code/100 != 2 {
		t.Fatalf("quien cuenta asigna una que existe = %d %s", w.Code, w.Body.String())
	}

	t.Run("crear no ve ni toca las ubicaciones de otra empresa, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "ubicacion-ajena"), func(t *testing.T, st *store.Store, ctx context.Context) {
			_, err := app.NewBackofficeService(st, clock).CreateCountPlace(ctx, "Bodega 043 permisos")
			var exists domain.CountPlaceExists
			if errors.As(err, &exists) && exists.ID == place.ID {
				t.Fatal("la ubicación de otra empresa contó como repetida: se vio entre empresas")
			}
		})
		var n int
		if err := st.Pool.QueryRow(context.Background(), `select count(*) from count_places where company_id = $1 and name ilike '%043 permisos%'`, defaultCompanyID).Scan(&n); err != nil || n != 1 {
			t.Fatalf("la empresa dueña tiene %d ubicaciones con ese nombre %v; quiere 1", n, err)
		}
	})
}
