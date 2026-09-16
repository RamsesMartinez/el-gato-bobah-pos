//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL SERVICIO, EJERCIDO COMO CORRE EN PRODUCCIÓN.
//
// Este archivo existe porque su ausencia dejó pasar un defecto crítico: el servicio consultaba por
// el pool crudo en vez de por la conexión con el tenant, y **ningún test lo tocaba**. Los de
// `menus_plataforma_test.go` prueban el ESQUEMA con SQL directo, que es otra cosa: pasan en verde
// mientras la feature no funciona.
//
// La regla que sale de ahí: un servicio multi-tenant se prueba llamándolo, bajo `appRoleStore`, y
// con DOS empresas. Con una sola, el `alter database ... set app.company_id` del harness enmascara
// exactamente el defecto que se busca.

// lectorFalso sustituye a la plataforma. No hay red: lo que se prueba es el servicio.
type lectorFalso struct {
	items []domain.ItemDePlataforma
	err   error
	// espera deja simular una lectura lenta, para ver qué pasa mientras corre.
	espera   time.Duration
	llamadas int
	mu       sync.Mutex
}

func (l *lectorFalso) LeerMenu(ctx context.Context, storeID string) ([]domain.ItemDePlataforma, error) {
	l.mu.Lock()
	l.llamadas++
	l.mu.Unlock()
	if l.espera > 0 {
		select {
		case <-time.After(l.espera):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return l.items, l.err
}

func (l *lectorFalso) ClaseDeFallo(err error) domain.ClaseDeFallo {
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.FalloTiempoAgotado
	}
	return domain.FalloRespuestaInvalida
}

func (l *lectorFalso) cuantasVeces() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.llamadas
}

func menuFalso() []domain.ItemDePlataforma {
	return []domain.ItemDePlataforma{
		{ID: "Chamoyada_de_Mango", Clase: domain.ItemPlatillo, Nombre: "Chamoyada de Mango 🥭", Centavos: 9900, Activo: true},
		{ID: "Chamoyada_de_Fresa", Clase: domain.ItemPlatillo, Nombre: "Chamoyada de Fresa 🍓", Centavos: 9900, Activo: true},
		{ID: "Dedos_de_queso", Clase: domain.ItemPlatillo, Nombre: "Dedos de queso", Centavos: 7500, Activo: true},
		{ID: "Con_hielo", Clase: domain.ItemOpcion, Nombre: "Con hielo", Centavos: 0, Activo: true},
	}
}

// servicioDePrueba arma el servicio sobre el rol de APP y devuelve un ctx con tenant, que es
// exactamente lo que el middleware le da a un handler.
func servicioDePrueba(t *testing.T, companyID int64, lector app.LectorDeMenu) (*app.MenusDePlataformaService, context.Context, *store.Store) {
	t.Helper()
	appSt := appRoleStore(t)
	lectores := map[string]app.LectorDeMenu{}
	if lector != nil {
		lectores["Uber Eats"] = lector
	}
	svc := app.NewMenusDePlataformaService(appSt, lectores, nil)
	tctx, soltar, err := appSt.AcquireTenant(context.Background(), companyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	t.Cleanup(soltar)
	return svc, tctx, appSt
}

func plataformaUber(t *testing.T, st *store.Store, companyID int64) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(),
		`select id from delivery_platforms where company_id = $1 and name = 'Uber Eats' limit 1`,
		companyID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Da de alta una conexión y deja una lectura `ok` con el menú falso, que es el punto de partida de
// casi todo lo demás.
func conexionConLectura(t *testing.T, svc *app.MenusDePlataformaService, ctx context.Context, st *store.Store, companyID int64, lector *lectorFalso, tienda string) int64 {
	t.Helper()
	id, err := svc.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, companyID), ExternalStoreID: tienda, Label: "Tienda " + tienda,
	})
	if err != nil {
		t.Fatalf("crear conexión: %v", err)
	}
	if _, _, err := svc.DispararLectura(ctx, companyID, id); err != nil {
		t.Fatalf("disparar lectura: %v", err)
	}
	esperarLectura(t, svc, ctx, id, lector)
	return id
}

func esperarLectura(t *testing.T, svc *app.MenusDePlataformaService, ctx context.Context, conexionID int64, lector *lectorFalso) {
	t.Helper()
	for i := 0; i < 100; i++ {
		lecturas, err := svc.ListarLecturas(ctx, conexionID, 1)
		if err == nil && len(lecturas) == 1 && lecturas[0].Estado != "en_curso" {
			if lecturas[0].Estado != "ok" && lector.err == nil {
				t.Fatalf("la lectura terminó en %q", lecturas[0].Estado)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("la lectura no terminó")
}

// --- Aislamiento: el defecto que este archivo existe para que no vuelva ---

func TestLoQueVeElServicioEsSoloDeSuEmpresa(t *testing.T) {
	st := newTestStore(t)
	otra := makeCompany(t, st, "vecina-servicio")
	lector := &lectorFalso{items: menuFalso()}

	svc, ctx, appSt := servicioDePrueba(t, defaultCompanyID, lector)
	conexionConLectura(t, svc, ctx, st, defaultCompanyID, lector, "tienda-propia")

	// La MISMA instancia del servicio, con el ctx de la OTRA empresa. Si consultara por el pool
	// crudo vería la conexión ajena: el `alter database` del harness le da company_id=1 a cualquier
	// conexión suelta, y por eso pedir el tenant de la segunda empresa es lo único que lo desnuda.
	octx, soltar, err := appSt.AcquireTenant(context.Background(), otra)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()

	ajenas, err := svc.ListarConexiones(octx)
	if err != nil {
		t.Fatalf("listar como la otra empresa: %v", err)
	}
	if len(ajenas) != 0 {
		t.Fatalf("la empresa %d vio %d conexiones ajenas: el servicio no está usando la conexión con el tenant", otra, len(ajenas))
	}
}

// Y el catálogo del otro lado de la comparación tampoco cruza: `ListActiveProductsForCompare` es un
// `select ... from products` sin filtro, así que quien lo aísla es RLS y nada más.
func TestLaComparacionNoTraeProductosDeOtraEmpresa(t *testing.T) {
	st := newTestStore(t)
	otra := makeCompany(t, st, "vecina-catalogo")
	sembrarProductoEn(t, st, otra, "Producto secreto de la vecina")
	sembrarProductoEn(t, st, defaultCompanyID, "Chamoyada")

	lector := &lectorFalso{items: menuFalso()}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)
	id := conexionConLectura(t, svc, ctx, st, defaultCompanyID, lector, "tienda-catalogo")

	cmp, err := svc.Diferencias(ctx, id, nil)
	if err != nil {
		t.Fatalf("diferencias: %v", err)
	}
	for _, d := range cmp.Diferencias {
		if strings.Contains(d.NombreAbajo, "secreta") || strings.Contains(d.NombreAbajo, "vecina") {
			t.Fatalf("la comparación trajo un producto de otra empresa: %q", d.NombreAbajo)
		}
	}
}

// --- La lectura ---

// Dos disparos a la vez dejan UNA sola lectura. El chequeo previo y el insert son dos statements,
// así que quien lo impide de verdad es el índice único parcial del esquema.
func TestDosDisparosSimultaneosDejanUnaSolaLectura(t *testing.T) {
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso(), espera: 300 * time.Millisecond}
	svc, ctx, appSt := servicioDePrueba(t, defaultCompanyID, lector)

	id, err := svc.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, defaultCompanyID), ExternalStoreID: "tienda-carrera", Label: "Carrera",
	})
	if err != nil {
		t.Fatal(err)
	}

	// CADA GOROUTINE CON SU PROPIA CONEXIÓN DE TENANT, como cada request en producción: la del
	// middleware es una sola y no es concurrente —compartirla da `conn busy`, que es un artefacto
	// de la prueba y no el defecto que se busca.
	var wg sync.WaitGroup
	errores := make([]error, 8)
	for i := range errores {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			propio, soltar, err := appSt.AcquireTenant(context.Background(), defaultCompanyID)
			if err != nil {
				errores[n] = err
				return
			}
			defer soltar()
			_, _, errores[n] = svc.DispararLectura(propio, defaultCompanyID, id)
		}(i)
	}
	wg.Wait()

	var aceptados int
	for _, e := range errores {
		if e == nil {
			aceptados++
		} else if !errors.Is(e, domain.ErrLecturaEnCurso) {
			t.Errorf("el rechazo debería ser ErrLecturaEnCurso, para que la pantalla lo explique: %v", e)
		}
	}
	if aceptados != 1 {
		t.Fatalf("%d disparos entraron a la vez; cada uno gasta un token contra la plataforma y escribe la misma foto", aceptados)
	}
	esperarLectura(t, svc, ctx, id, lector)
	if n := lector.cuantasVeces(); n != 1 {
		t.Fatalf("se habló %d veces con la plataforma para una sola lectura", n)
	}
}

// Una lectura que se quedó `en_curso` porque el proceso murió NO bloquea la tienda para siempre.
// Sin esto, todo disparo futuro responde «ya hay una en curso» y solo se arregla tocando la base.
func TestUnaLecturaColgadaNoBloqueaLaTiendaParaSiempre(t *testing.T) {
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso()}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)

	id, err := svc.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, defaultCompanyID), ExternalStoreID: "tienda-colgada", Label: "Colgada",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Una lectura vieja que nadie cerró, como la deja un despliegue a media descarga.
	if _, err := st.Pool.Exec(ctx,
		`insert into platform_menu_reads (company_id, connection_id, status, started_at)
		 values ($1, $2, 'en_curso', now() - interval '2 hours')`, defaultCompanyID, id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DispararLectura(ctx, defaultCompanyID, id); !errors.Is(err, domain.ErrLecturaEnCurso) {
		t.Fatalf("con una lectura colgada se esperaba ErrLecturaEnCurso, dio %v", err)
	}

	n, err := svc.CerrarLecturasColgadas(ctx)
	if err != nil {
		t.Fatalf("cerrar colgadas: %v", err)
	}
	if n != 1 {
		t.Fatalf("cerró %d lecturas colgadas, esperaba 1", n)
	}
	if _, _, err := svc.DispararLectura(ctx, defaultCompanyID, id); err != nil {
		t.Fatalf("tras cerrar la colgada la tienda tiene que volver a leerse: %v", err)
	}
	esperarLectura(t, svc, ctx, id, lector)
}

// Una lectura que vuelve vacía se guarda como FALLIDA, no como un menú sin productos: compararla
// diría que sobra todo el catálogo.
func TestUnaLecturaVaciaQuedaComoFallida(t *testing.T) {
	st := newTestStore(t)
	lector := &lectorFalso{err: errors.New("vacío")}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)

	id, err := svc.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, defaultCompanyID), ExternalStoreID: "tienda-vacia-svc", Label: "Vacía",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DispararLectura(ctx, defaultCompanyID, id); err != nil {
		t.Fatal(err)
	}
	esperarLectura(t, svc, ctx, id, lector)

	lecturas, err := svc.ListarLecturas(ctx, id, 1)
	if err != nil || len(lecturas) != 1 {
		t.Fatalf("listar lecturas: %v", err)
	}
	if lecturas[0].Estado != "fallida" {
		t.Fatalf("la lectura quedó en %q; una que no sirvió tiene que quedar fallida", lecturas[0].Estado)
	}
	// Y sin lectura válida no se compara: devolver una lista vacía haría creer que todo coincide.
	if _, err := svc.Diferencias(ctx, id, nil); !errors.Is(err, domain.ErrSinLecturaValida) {
		t.Fatalf("se esperaba ErrSinLecturaValida, dio %v", err)
	}
}

// Una plataforma sin credenciales NO es un error del operador ni una comparación vacía: es «esta
// tienda no está conectada», y son cosas distintas en pantalla.
func TestSinCredencialesNoSeLeeYSeDice(t *testing.T) {
	st := newTestStore(t)
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, nil) // sin lectores

	id, err := svc.CrearConexion(ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, st, defaultCompanyID), ExternalStoreID: "tienda-sin-cred", Label: "Sin acceso",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DispararLectura(ctx, defaultCompanyID, id); !errors.Is(err, domain.ErrPlataformaSinCredenciales) {
		t.Fatalf("se esperaba ErrPlataformaSinCredenciales, dio %v", err)
	}
	cons, err := svc.ListarConexiones(ctx)
	if err != nil || len(cons) != 1 {
		t.Fatalf("listar: %v", err)
	}
	if cons[0].Configurada {
		t.Error("la conexión se reporta como configurada y no hay lector para su plataforma")
	}
}

// --- Emparejamiento ---

func TestElEmparejamientoDeExtremoAExtremo(t *testing.T) {
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso()}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)
	chamoyada := sembrarProductoEn(t, st, defaultCompanyID, "Chamoyada")
	otroProd := sembrarProductoEn(t, st, defaultCompanyID, "Frappé")
	quien := makeUser(t, st, "empareja", "gerente")
	id := conexionConLectura(t, svc, ctx, st, defaultCompanyID, lector, "tienda-emparejar")

	alta := func(ext string, local int64, reemplazar bool) error {
		return svc.GuardarPareja(ctx, app.AltaDePareja{
			ConexionID: id, ExternalID: ext, Clase: domain.ItemPlatillo,
			LocalID: local, ClaseLocal: domain.LocalProducto, Reemplazar: reemplazar,
			UsuarioID: quien,
		})
	}

	// FR-010: un producto del POS puede ser VARIOS platillos arriba. Es el caso real.
	if err := alta("Chamoyada_de_Mango", chamoyada, false); err != nil {
		t.Fatalf("primera pareja: %v", err)
	}
	if err := alta("Chamoyada_de_Fresa", chamoyada, false); err != nil {
		t.Fatalf("segunda pareja al mismo producto: %v", err)
	}

	// Una pareja CONFIRMADA no se pisa sin pedirlo: el emparejamiento es trabajo manual.
	if err := alta("Chamoyada_de_Mango", otroProd, false); !errors.Is(err, domain.ErrParejaOcupada) {
		t.Fatalf("se esperaba ErrParejaOcupada, dio %v", err)
	}
	if err := alta("Chamoyada_de_Mango", otroProd, true); err != nil {
		t.Fatalf("con reemplazo explícito debería pasar: %v", err)
	}

	// Un id que no está en la última lectura se rechaza: no hay FK que lo impida, a propósito.
	if err := alta("Item_que_no_existe", chamoyada, false); !errors.Is(err, domain.ErrItemInexistente) {
		t.Fatalf("se esperaba ErrItemInexistente, dio %v", err)
	}
	// Y cruzar los niveles, tampoco: una opción no se empareja con un producto.
	err := svc.GuardarPareja(ctx, app.AltaDePareja{
		ConexionID: id, ExternalID: "Con_hielo", Clase: domain.ItemOpcion,
		LocalID: chamoyada, ClaseLocal: domain.LocalProducto,
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("emparejar una opción con un producto debería ser ErrValidation, dio %v", err)
	}

	// Deshacer, y que la comparación lo refleje de inmediato.
	if err := svc.BorrarPareja(ctx, id, "Chamoyada_de_Fresa"); err != nil {
		t.Fatalf("deshacer: %v", err)
	}
	emp, err := svc.Emparejar(ctx, id, domain.ItemPlatillo)
	if err != nil {
		t.Fatalf("emparejar: %v", err)
	}
	for _, it := range emp.Items {
		if it.ID == "Chamoyada_de_Fresa" && it.Pareja != nil && it.Pareja.Confirmada {
			t.Error("la pareja deshecha sigue confirmada")
		}
	}
}

// LA MARCA DE CUÁNDO SE CONFIRMÓ viaja hasta la pantalla, y es lo único que hace correcto un
// «deshacer el último»: la lista llega ordenada por NOMBRE, así que sin ella hay que adivinar — y
// adivinar aquí deshace la pareja equivocada sin que nadie lo note.
func TestLaParejaDiceCuandoSeConfirmo(t *testing.T) {
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso()}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)
	p := sembrarProductoEn(t, st, defaultCompanyID, "Chamoyada")
	quien := makeUser(t, st, "marca-quien", "gerente")
	id := conexionConLectura(t, svc, ctx, st, defaultCompanyID, lector, "tienda-marca")

	if err := svc.GuardarPareja(ctx, app.AltaDePareja{
		ConexionID: id, ExternalID: "Dedos_de_queso", Clase: domain.ItemPlatillo,
		LocalID: p, ClaseLocal: domain.LocalProducto, UsuarioID: quien,
	}); err != nil {
		t.Fatal(err)
	}
	emp, err := svc.Emparejar(ctx, id, domain.ItemPlatillo)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range emp.Items {
		if it.ID != "Dedos_de_queso" {
			continue
		}
		if it.Pareja == nil || !it.Pareja.Confirmada {
			t.Fatal("la pareja no llegó confirmada")
		}
		if it.Pareja.ConfirmadaEn == nil {
			t.Fatal("la pareja no dice cuándo se confirmó: «deshacer el último» tendría que adivinar")
		}
		return
	}
	t.Fatal("no se encontró el item emparejado")
}

// --- La poda ---

// Conserva SIEMPRE la última de cada conexión. Sin esa excepción, una tienda abandonada se queda
// sin ninguna fila y la pantalla la muestra igual que una que nunca se leyó.
func TestLaPodaDelServicioConservaLaUltima(t *testing.T) {
	st := newTestStore(t)
	lector := &lectorFalso{items: menuFalso()}
	svc, ctx, _ := servicioDePrueba(t, defaultCompanyID, lector)
	id := conexionConLectura(t, svc, ctx, st, defaultCompanyID, lector, "tienda-poda-svc")

	if _, err := st.Pool.Exec(ctx,
		`update platform_menu_reads set started_at = now() - interval '300 days',
		        finished_at = now() - interval '300 days' where connection_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PodarLecturas(ctx); err != nil {
		t.Fatalf("podar: %v", err)
	}
	var quedan int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from platform_menu_reads where connection_id = $1`, id).Scan(&quedan); err != nil {
		t.Fatal(err)
	}
	if quedan != 1 {
		t.Fatalf("quedaron %d lecturas; la última se conserva aunque sea vieja", quedan)
	}
}
