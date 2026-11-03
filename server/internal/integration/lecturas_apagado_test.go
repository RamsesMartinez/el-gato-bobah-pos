//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// EL APAGADO NO DEJA LECTURAS «EN CURSO» (spec 041).
//
// La lectura de menú corre en una goroutine fuera de la petición, y `http.Server.Shutdown` no la
// espera: el proceso moría con la fila `en_curso`, la pantalla decía «se está leyendo» y cada
// intento nuevo respondía «ya hay una lectura en curso» hasta la siguiente pasada de mantenimiento.
// Con dos empresas y bajo el rol de app: marcarlas sin la empresa de cada una no tocaría ninguna
// fila (RLS) y el test pasaría en verde sin que el apagado hiciera nada.

func TestShutdownWaitsForAMenuReadInFlight(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso(), espera: 200 * time.Millisecond}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)
	id, err := svc.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, defaultCompanyID), ExternalStoreID: "apagado-espera", Label: "Tienda",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DispararLectura(ctx, defaultCompanyID, id); err != nil {
		t.Fatal(err)
	}

	wctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Esperar(wctx); err != nil {
		t.Fatalf("Esperar con plazo de sobra: %v", err)
	}
	lecturas, err := svc.ListarLecturas(ctx, id, 1)
	if err != nil || len(lecturas) != 1 || lecturas[0].Estado != "ok" {
		t.Fatalf("tras esperar, la lectura quedó %+v (%v); quería ok", lecturas, err)
	}
}

func TestShutdownPastTheDeadlineFailsTheReadsOfEveryCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	otra := makeCompany(t, st, "vecina-apagado")
	lector := &lectorFalso{items: menuFalso(), espera: time.Minute}
	svc, ctx, appSt := servicioDePrueba(t, defaultCompanyID, lector)
	octx, soltar, err := appSt.AcquireTenant(context.Background(), otra)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()

	conexiones := map[int64]context.Context{}
	for empresa, c := range map[int64]context.Context{defaultCompanyID: ctx, otra: octx} {
		id, err := svc.CrearConexion(c, app.AltaDeConexion{
			PlatformID: plataformaUber(t, st, empresa), ExternalStoreID: "apagado-vencido", Label: "Tienda",
		})
		if err != nil {
			t.Fatalf("empresa %d: %v", empresa, err)
		}
		if _, _, err := svc.DispararLectura(c, empresa, id); err != nil {
			t.Fatalf("empresa %d: %v", empresa, err)
		}
		conexiones[id] = c
	}

	wctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := svc.Esperar(wctx); err == nil {
		t.Fatal("Esperar regresó sin error con lecturas vivas al vencer el plazo")
	}
	for id, c := range conexiones {
		lecturas, err := svc.ListarLecturas(c, id, 1)
		if err != nil || len(lecturas) != 1 {
			t.Fatalf("conexión %d: %+v (%v)", id, lecturas, err)
		}
		if lecturas[0].Estado != "fallida" {
			t.Fatalf("conexión %d quedó %q tras el apagado: la pantalla diría «se está leyendo» para siempre", id, lecturas[0].Estado)
		}
	}
}

// TestStartupMaintenanceLeavesTheOtherCopysReadAlone: durante el traslape, la copia nueva corre al
// arrancar CerrarLecturasColgadas mientras la vieja puede estar a media lectura. Solo cierra las de
// más de TiempoMaximoDeLectura; una viva de la otra copia no se toca.
func TestStartupMaintenanceLeavesTheOtherCopysReadAlone(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso(), espera: time.Minute}
	vieja, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)
	id, err := vieja.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, defaultCompanyID), ExternalStoreID: "traslape", Label: "Tienda",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := vieja.DispararLectura(ctx, defaultCompanyID, id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_ = vieja.Esperar(c)
	})

	// La copia nueva: mantenimiento como dueño, igual que RecortarPeriodicamente.
	nueva := app.NewMenusDePlataformaService(st, nil, nil)
	if _, err := nueva.CerrarLecturasColgadas(context.Background()); err != nil {
		t.Fatal(err)
	}
	lecturas, err := vieja.ListarLecturas(ctx, id, 1)
	if err != nil || len(lecturas) != 1 || lecturas[0].Estado != "en_curso" {
		t.Fatalf("el arranque de la copia nueva tocó una lectura viva de la vieja: %+v (%v)", lecturas, err)
	}
}

// TestFailingAReadDoesNotOverwriteOneThatFinished: el apagado toma la lista de lecturas vivas y
// después las cierra; si una termina bien entre las dos cosas, cerrarla como fallida escondería una
// lectura buena detrás de un «falló». Solo se cierra lo que sigue en curso.
func TestFailingAReadDoesNotOverwriteOneThatFinished(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso()}
	svc, ctx, appSt := servicioDePrueba(t, defaultCompanyID, lector)
	id := conexionConLectura(t, svc, ctx, st, defaultCompanyID, lector, "terminada-antes")
	lecturas, err := svc.ListarLecturas(ctx, id, 1)
	if err != nil || len(lecturas) != 1 {
		t.Fatal(lecturas, err)
	}
	clase := "tiempo_agotado"
	if err := appSt.WithTenant(context.Background(), defaultCompanyID, func(q *db.Queries) error {
		return q.FailMenuRead(context.Background(), db.FailMenuReadParams{ID: lecturas[0].ID, FailureKind: &clase})
	}); err != nil {
		t.Fatal(err)
	}
	despues, _ := svc.ListarLecturas(ctx, id, 1)
	if despues[0].Estado != "ok" {
		t.Fatalf("una lectura ya terminada bien quedó %q: la pantalla diría que falló", despues[0].Estado)
	}
}
