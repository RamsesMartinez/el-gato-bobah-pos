//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// LA LISTA DE CLIENTES, Y SOBRE TODO LO QUE NO TRAE (US2: FR-007, FR-008, FR-015, FR-016).
//
// Los tres «no trae» se afirman explícitamente y no se dan por buenos porque nadie los escribió:
// una respuesta se llena sola el día que alguien agrega un campo «de paso», y la primera vez que
// se note será mirando la pantalla de un cliente en la computadora de quien vende el sistema.
func TestLaConsolaListaLasEmpresasSinMirarSuOperacion(t *testing.T) {
	st := newTestStore(t)
	r, _, pjm := routerConConsola(t, st)

	makeCompany(t, st, "segunda-empresa")
	crearOperador(t, st, "soporte_lista", "Contrasena-De-Plataforma-1!", true)

	tok := tokenDeOperador(t, st, pjm, "soporte_lista")
	w := do(t, r, http.MethodGet, "/api/v1/platform/companies", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("lista de empresas = %d: %s", w.Code, w.Body.String())
	}

	var res struct {
		Items []struct {
			ID        int64  `json:"id"`
			Slug      string `json:"slug"`
			Name      string `json:"name"`
			CreatedAt string `json:"createdAt"`
		} `json:"items"`
		Schema struct {
			Version int64 `json:"version"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("respuesta: %v (%s)", err, w.Body.String())
	}

	// Con dos empresas salen las dos: es FR-007 y es la mitad que la política de RLS hace posible.
	if len(res.Items) != 2 {
		t.Fatalf("la consola ve %d empresas y hay 2: si es una, falta la política de RLS y la lista de clientes sale incompleta sin avisar", len(res.Items))
	}
	if res.Items[0].Slug == "" || res.Items[0].CreatedAt == "" {
		t.Fatalf("la lista no trae lo mínimo para reconocer a un cliente: %+v", res.Items[0])
	}

	// La versión de esquema viaja UNA vez, para la instalación. Como columna por empresa diría lo
	// mismo en todos los renglones —el esquema es de la base, no del cliente— y aparentaría
	// informar algo que no es por cliente.
	if res.Schema.Version < 68 {
		t.Fatalf("schema.version = %d: la instalación está al menos en la 68", res.Schema.Version)
	}

	// Y lo que NO puede aparecer, buscado en el JSON crudo: un campo agregado «de paso» no
	// rompería el decodificador de arriba, así que no se vería.
	crudo := w.Body.String()
	for _, prohibido := range []string{
		"total", "ventas", "sales", "revenue", "ingresos", // FR-008: ni una cifra de dinero
		"users", "usuarios", "empleados", "email", // FR-016: nada de la gente del cliente
		"lastActivity", "ultimaActividad", // FR-007b: llega con la spec 017, no antes
	} {
		if strings.Contains(strings.ToLower(crudo), strings.ToLower(prohibido)) {
			t.Errorf("la respuesta trae %q: %s", prohibido, crudo)
		}
	}
}

// Cero empresas: la respuesta lo dice con una lista vacía, no con `null` (FR-015).
//
// La diferencia importa donde se ve: `null` no se recorre en el front y la pantalla truena en vez
// de decir que todavía no hay clientes.
func TestLaConsolaConCeroEmpresas(t *testing.T) {
	st := newTestStore(t)
	r, _, pjm := routerConConsola(t, st)
	crearOperador(t, st, "soporte_vacio", "Contrasena-De-Plataforma-1!", true)
	tok := tokenDeOperador(t, st, pjm, "soporte_vacio")

	// El dueño borra la empresa sembrada: el rol de la consola no puede, y esa es la barrera.
	if _, err := st.Pool.Exec(context.Background(), `delete from companies`); err != nil {
		t.Fatalf("dejar la instalación sin empresas: %v", err)
	}

	w := do(t, r, http.MethodGet, "/api/v1/platform/companies", tok, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("lista vacía = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("sin empresas la respuesta debe traer una lista vacía, no null: %s", w.Body.String())
	}
}
