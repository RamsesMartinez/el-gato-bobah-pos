//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA REJILLA QUE LEE LA CONSOLA (US1 de la 019), por el router real.

type rejillaDeToques struct {
	Pantalla    string `json:"pantalla"`
	Orientacion string `json:"orientacion"`
	Rejilla     struct {
		Columnas int `json:"columnas"`
		Filas    int `json:"filas"`
	} `json:"rejilla"`
	Periodo struct {
		Desde string `json:"desde"`
		Hasta string `json:"hasta"`
	} `json:"periodo"`
	Celdas []struct {
		Celda int   `json:"celda"`
		Veces int64 `json:"veces"`
	} `json:"celdas"`
	PorRol []struct {
		Rol   *string `json:"rol"`
		Veces int64   `json:"veces"`
	} `json:"porRol"`
}

// rangoVigente es un periodo que CABE en la retención de la rejilla, calculado contra hoy.
//
// Con fechas fijas el test caduca solo: la retención son 92 días, así que un `desde=2026-01-01`
// escrito hoy empieza a devolver 400 dentro de unos meses y el fallo se lee como un defecto.
func rangoVigente() string {
	desde := time.Now().AddDate(0, 0, -30).Format(time.DateOnly)
	return "&desde=" + desde + "&hasta=" + hoy()
}

func hoy() string { return time.Now().Format(time.DateOnly) }

func TestLaRejillaDeToquesPorElRouter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, jm, pjm := routerConMapa(t, st)

	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_rejilla_uno", "cajero")
	makeUser(t, st, "cajero_rejilla_dos", "cajero")
	if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Toques: []domain.Toque{
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 0, Orientacion: domain.OrientacionHorizontal},
	}}); err != nil {
		t.Fatalf("sembrar toques: %v", err)
	}

	crearOperador(t, st, "soporte_rejilla", "Contrasena-De-Plataforma-1!", true)
	tok := tokenDeOperador(t, st, pjm, "soporte_rejilla")
	ruta := "/api/v1/platform/touches?pantalla=pos" + rangoVigente()

	w := do(t, r, http.MethodGet, ruta, tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("la rejilla = %d: %s", w.Code, w.Body.String())
	}
	var rej rejillaDeToques
	if err := json.Unmarshal(w.Body.Bytes(), &rej); err != nil {
		t.Fatalf("respuesta: %v (%s)", err, w.Body.String())
	}

	// LAS 84 CELDAS VIAJAN SIEMPRE, incluidas las de cero: «qué parte no toca nadie» es la mitad de
	// la pregunta que esta feature vino a responder, y una celda omitida se pinta como un hueco.
	if len(rej.Celdas) != domain.CeldasDeLaRejilla {
		t.Fatalf("llegaron %d celdas y son %d: las de cero también viajan", len(rej.Celdas), domain.CeldasDeLaRejilla)
	}
	for i, c := range rej.Celdas {
		if c.Celda != i {
			t.Fatalf("la celda %d llegó en la posición %d: la rejilla se pinta por orden", c.Celda, i)
		}
	}
	if rej.Celdas[37].Veces != 2 || rej.Celdas[0].Veces != 1 {
		t.Fatalf("los conteos llegaron mal: celda 37 = %d (quiere 2), celda 0 = %d (quiere 1)", rej.Celdas[37].Veces, rej.Celdas[0].Veces)
	}
	if rej.Rejilla.Columnas != 12 || rej.Rejilla.Filas != 7 {
		t.Fatalf("la rejilla llegó %d×%d y en horizontal es 12×7", rej.Rejilla.Columnas, rej.Rejilla.Filas)
	}
	if rej.Orientacion != domain.OrientacionHorizontal {
		t.Fatalf("la orientación por defecto llegó %q", rej.Orientacion)
	}
	// El reparto por rol es del TOTAL de la pantalla, no de una celda.
	var total int64
	for _, p := range rej.PorRol {
		total += p.Veces
	}
	if total != 3 {
		t.Fatalf("el reparto por rol suma %d de 3 toques", total)
	}

	t.Run("con sesión del negocio da 401", func(t *testing.T) {
		// La consola es otra superficie con otra firma. Un token del POS —aunque sea de admin— no
		// abre esta puerta: es la barrera que la spec 016 puso y que aquí se vuelve a comprobar
		// porque la ruta es nueva y una ruta nueva no hereda nada.
		id := makeUser(t, st, "admin_curioso_toques", "admin")
		tokDelNegocio, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "Admin", Role: domain.RoleAdmin})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if w := do(t, r, http.MethodGet, ruta, tokDelNegocio, nil, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("con token del negocio = %d, quiere 401", w.Code)
		}
	})

	t.Run("un rango más viejo que la retención se rechaza", func(t *testing.T) {
		// No se recorta en silencio a lo que hay: devolver otro periodo del que se pidió es una
		// pantalla que se ve bien y reporta un número que nadie pidió (principio V).
		viejo := "/api/v1/platform/touches?pantalla=pos&desde=2020-01-01&hasta=" + hoy()
		if w := do(t, r, http.MethodGet, viejo, tok, nil, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("un rango de hace años = %d, quiere 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("una pantalla no instrumentada se rechaza", func(t *testing.T) {
		// Y no devuelve una rejilla vacía: una rejilla en ceros se lee como «aquí nadie toca», que
		// es exactamente la conclusión equivocada.
		mala := "/api/v1/platform/touches?pantalla=caja" + rangoVigente()
		if w := do(t, r, http.MethodGet, mala, tok, nil, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("una pantalla sin toques instrumentados = %d, quiere 400", w.Code)
		}
	})

	t.Run("empresa ausente suma todas", func(t *testing.T) {
		// El layout es el mismo software para todos los clientes, así que juntar tabletas da mejor
		// muestra para decidir dónde va un control. Lo hace posible la política de plataforma; sin
		// ella esta misma consulta devolvería cero filas sin fallar.
		// Se siembra por SQL directo y con el dueño: `RegistrarToques` escribe siempre en la
		// empresa del contexto, y lo que este caso mide es que la consola cruza esa frontera.
		//
		// EL DÍA SE PASA DESDE GO, no se toma de `current_date`. Postgres corre en UTC y el rango de
		// la consulta se arma con la fecha local: después de las 18:00 en México son días distintos,
		// así que la fila sembrada caía FUERA del rango y el test fallaba con «0 toques» a partir de
		// esa hora. Es la misma trampa que la migración 0038 arregló para la venta, reaparecida en
		// el andamio de un test.
		otra := makeCompany(t, st, "otra-empresa-rejilla")
		if _, err := st.Pool.Exec(ctx,
			`insert into usage_touches_daily (day, screen, orientation, cell, role, hits, company_id)
			 values ($1::date, 'pos', 'horizontal', 37, 'cajero', 5, $2)`, hoy(), otra); err != nil {
			t.Fatalf("sembrar la otra empresa: %v", err)
		}

		sola := "/api/v1/platform/touches?pantalla=pos" + rangoVigente() + "&empresa=" + itoa(int(otra))
		w := do(t, r, http.MethodGet, sola, tok, nil, "")
		var deUna rejillaDeToques
		if err := json.Unmarshal(w.Body.Bytes(), &deUna); err != nil {
			t.Fatalf("respuesta de una empresa: %v", err)
		}
		if deUna.Celdas[37].Veces != 5 {
			t.Fatalf("la otra empresa tiene %d toques en la celda 37 y sembramos 5", deUna.Celdas[37].Veces)
		}

		w = do(t, r, http.MethodGet, ruta, tok, nil, "")
		var juntas rejillaDeToques
		if err := json.Unmarshal(w.Body.Bytes(), &juntas); err != nil {
			t.Fatalf("respuesta de todas: %v", err)
		}
		if juntas.Celdas[37].Veces != 7 {
			t.Fatalf("sin `empresa` la celda 37 suma %d y son 7 (2 de una empresa + 5 de la otra)", juntas.Celdas[37].Veces)
		}
	})
}

// LAS DOS ORIENTACIONES NUNCA SE SUMAN (FR-016).
//
// Mezclarlas pinta una rejilla que nadie tocó nunca: la celda 37 está a la derecha del centro en
// horizontal y abajo del centro en vertical. Y el error sería invisible —la rejilla se ve normal,
// solo que describe un lugar que no existe—.
func TestLaRejillaNoSumaLasDosOrientaciones(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, _, pjm := routerConMapa(t, st)

	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_dos_formas_uno", "cajero")
	makeUser(t, st, "cajero_dos_formas_dos", "cajero")
	lote := []domain.Toque{
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionVertical},
	}
	if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Toques: lote}); err != nil {
		t.Fatalf("sembrar: %v", err)
	}

	crearOperador(t, st, "soporte_dos_formas", "Contrasena-De-Plataforma-1!", true)
	tok := tokenDeOperador(t, st, pjm, "soporte_dos_formas")

	leer := func(orientacion string) rejillaDeToques {
		t.Helper()
		ruta := "/api/v1/platform/touches?pantalla=pos" + rangoVigente() + "&orientacion=" + orientacion
		w := do(t, r, http.MethodGet, ruta, tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("la rejilla %s = %d: %s", orientacion, w.Code, w.Body.String())
		}
		var rej rejillaDeToques
		if err := json.Unmarshal(w.Body.Bytes(), &rej); err != nil {
			t.Fatalf("respuesta: %v", err)
		}
		return rej
	}

	h := leer(domain.OrientacionHorizontal)
	if h.Celdas[37].Veces != 3 {
		t.Fatalf("en horizontal la celda 37 tiene %d y son 3: se le sumó la otra forma", h.Celdas[37].Veces)
	}
	v := leer(domain.OrientacionVertical)
	if v.Celdas[37].Veces != 1 {
		t.Fatalf("en vertical la celda 37 tiene %d y es 1", v.Celdas[37].Veces)
	}
	if v.Rejilla.Columnas != 7 || v.Rejilla.Filas != 12 {
		t.Fatalf("en vertical la rejilla llegó %d×%d y es 7×12", v.Rejilla.Columnas, v.Rejilla.Filas)
	}

	t.Run("una orientación inventada se rechaza", func(t *testing.T) {
		ruta := "/api/v1/platform/touches?pantalla=pos" + rangoVigente() + "&orientacion=landscape"
		if w := do(t, r, http.MethodGet, ruta, tok, nil, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("orientación inventada = %d, quiere 400: caer a horizontal en silencio pinta datos de otra forma de pantalla", w.Code)
		}
	})
}
