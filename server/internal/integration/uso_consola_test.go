//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL MAPA QUE LEE LA CONSOLA (US1), por el router real.

type mapaDeUso struct {
	Periodo struct {
		Desde string `json:"desde"`
		Hasta string `json:"hasta"`
	} `json:"periodo"`
	Pantallas []struct {
		Pantalla  string `json:"pantalla"`
		Aperturas int64  `json:"aperturas"`
		Acciones  []struct {
			Accion string `json:"accion"`
			Veces  int64  `json:"veces"`
		} `json:"acciones"`
		PorRol []struct {
			Rol   *string `json:"rol"`
			Veces int64   `json:"veces"`
		} `json:"porRol"`
	} `json:"pantallas"`
}

func routerConMapa(t *testing.T, st *store.Store) (http.Handler, *auth.Manager, *auth.ManagerDePlataforma) {
	t.Helper()
	prepararRolDePlataforma(t, st)
	plataforma := platformRoleStore(t)

	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	pjm := auth.NewManagerDePlataforma(secretoDeLaConsolaEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:         jm,
		Auth:        app.NewAuthService(st, jm, clock),
		PlatformJWT: pjm,
		Platform:    app.NewPlatformService(plataforma, pjm, clock),
		// Dos servicios de uso, uno por conexión: el POS escribe con la del negocio y la consola
		// lee con la de plataforma, que no alcanza el grano fino.
		Usage:        app.NewUsageService(st),
		UsageConsola: app.NewUsageService(plataforma),
	})
	return httpapi.Router(config.Config{}, jm, h, st), jm, pjm
}

func TestElMapaDeUsoPorElRouter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, jm, pjm := routerConMapa(t, st)

	// Uso sembrado: 3 aperturas de pos, 2 cobros y 1 apertura de caja, con dos cajeros para que el
	// corte por rol se conserve.
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_mapa_uno", "cajero")
	makeUser(t, st, "cajero_mapa_dos", "cajero")
	if _, err := svc.Registrar(ctx, domain.RoleCajero, []domain.EventoDeUso{
		{Pantalla: "pos"}, {Pantalla: "pos"}, {Pantalla: "pos"},
		{Pantalla: "pos", Accion: "cobrar"}, {Pantalla: "pos", Accion: "cobrar"},
		{Pantalla: "caja"},
	}); err != nil {
		t.Fatalf("sembrar uso: %v", err)
	}

	crearOperador(t, st, "soporte_mapa", "Contrasena-De-Plataforma-1!", true)
	tok := tokenDeOperador(t, st, pjm, "soporte_mapa")

	w := do(t, r, http.MethodGet, "/api/v1/platform/usage?desde=2026-01-01&hasta=2030-01-01", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("el mapa = %d: %s", w.Code, w.Body.String())
	}
	var mapa mapaDeUso
	if err := json.Unmarshal(w.Body.Bytes(), &mapa); err != nil {
		t.Fatalf("respuesta: %v (%s)", err, w.Body.String())
	}

	// Ordenadas de más a menos usada: es la pregunta que la pantalla responde.
	if len(mapa.Pantallas) < 2 {
		t.Fatalf("llegaron %d pantallas y el catálogo tiene más: las de cero también viajan", len(mapa.Pantallas))
	}
	if mapa.Pantallas[0].Pantalla != "pos" {
		t.Fatalf("la primera es %q y debe ser la más usada (pos)", mapa.Pantallas[0].Pantalla)
	}

	var pos, caja, vacia *int
	for i := range mapa.Pantallas {
		switch mapa.Pantallas[i].Pantalla {
		case "pos":
			pos = &i
		case "caja":
			caja = &i
		case "gastos": // nadie la abrió
			vacia = &i
		}
	}
	if pos == nil || caja == nil {
		t.Fatal("faltan pantallas con uso en la respuesta")
	}
	p := mapa.Pantallas[*pos]
	if p.Aperturas != 3 {
		t.Fatalf("aperturas de pos = %d, quiere 3", p.Aperturas)
	}
	if len(p.Acciones) != 1 || p.Acciones[0].Veces != 2 {
		t.Fatalf("acciones de pos = %+v, quiere una con 2", p.Acciones)
	}

	// LA IGUALDAD QUE FIJA QUÉ INCLUYE CADA CIFRA (hallazgo del analyze): sin esto, quien lea la
	// respuesta suma dos de las tres y reporta un número que no existe.
	var porRol int64
	for _, r := range p.PorRol {
		porRol += r.Veces
	}
	var enAcciones int64
	for _, a := range p.Acciones {
		enAcciones += a.Veces
	}
	if porRol != p.Aperturas+enAcciones {
		t.Fatalf("sum(porRol)=%d y aperturas+acciones=%d: las cifras no dicen lo que el contrato promete", porRol, p.Aperturas+enAcciones)
	}

	// Una pantalla que nadie abrió viaja con cero y no se omite: el cero ES el dato que se busca.
	if vacia == nil {
		t.Fatal("las pantallas sin uso no viajan, y son la mitad de la pregunta: qué no usa nadie")
	}
	if mapa.Pantallas[*vacia].Aperturas != 0 {
		t.Fatalf("la pantalla sin uso trae %d aperturas", mapa.Pantallas[*vacia].Aperturas)
	}

	// Y lo que NO puede traer, buscado en los NOMBRES DE CAMPO del JSON: un campo agregado «de
	// paso» no rompería el decodificador de arriba, así que no se vería.
	//
	// Se miran las claves y no el texto crudo porque los VALORES incluyen nombres de pantalla —una
	// de ellas se llama `usuarios`— y buscar la palabra suelta convierte el guardia en ruido, que
	// es como termina apagado.
	var crudo any
	if err := json.Unmarshal(w.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("releer la respuesta: %v", err)
	}
	claves := map[string]bool{}
	recogerClaves(crudo, claves)
	for _, prohibida := range []string{"total", "importe", "monto", "ingreso", "ventas", "user", "usuario", "empleado", "email"} {
		if claves[prohibida] {
			t.Errorf("la respuesta trae un campo %q: esta pantalla dice qué se usa, nunca cuánto se vendió ni quién lo hizo", prohibida)
		}
	}

	t.Run("una sesión del negocio no lo ve", func(t *testing.T) {
		id := makeUser(t, st, "admin_mapa", "admin")
		tokNegocio, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "Admin", Role: domain.RoleAdmin})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		// 401 y no 403: a quien no debe ver la consola no se le confirma que la ruta exista.
		if w := do(t, r, http.MethodGet, "/api/v1/platform/usage?desde=2026-01-01&hasta=2030-01-01", tokNegocio, nil, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("con sesión de negocio = %d, quiere 401", w.Code)
		}
	})

	t.Run("un rango más viejo que la retención se rechaza", func(t *testing.T) {
		// No se recorta en silencio a lo que hay: una pantalla que devuelve otro rango del que se
		// pidió miente, y nadie la audita (principio V).
		w := do(t, r, http.MethodGet, "/api/v1/platform/usage?desde=2019-01-01&hasta=2030-01-01", tok, nil, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("rango imposible = %d, quiere 400", w.Code)
		}
	})

	t.Run("una fecha mal escrita se rechaza", func(t *testing.T) {
		if w := do(t, r, http.MethodGet, "/api/v1/platform/usage?desde=ayer&hasta=2030-01-01", tok, nil, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("fecha inválida = %d, quiere 400 — no un default en silencio", w.Code)
		}
	})
}

// recogerClaves junta los nombres de campo de un JSON cualquiera, en minúsculas.
func recogerClaves(v any, en map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, sub := range t {
			en[strings.ToLower(k)] = true
			recogerClaves(sub, en)
		}
	case []any:
		for _, sub := range t {
			recogerClaves(sub, en)
		}
	}
}
