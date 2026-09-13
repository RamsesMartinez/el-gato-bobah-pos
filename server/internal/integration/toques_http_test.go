//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// EL TOQUE ENTRA POR EL MISMO ENDPOINT que la medición de la 017, y este test mira lo que solo se
// ve desde el router: que la ruta sigue dentro del grupo con tenant —sin eso RLS no tiene empresa y
// el insert falla— y, sobre todo, **que un punto con precisión de píxel no tiene a dónde llegar**.
func TestElToquePorElRouterNoGuardaCoordenadas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Usage: app.NewUsageService(st)})
	r := httpapi.Router(config.Config{}, jm, h, st)

	id := makeUser(t, st, "cajero_toque_router_uno", "cajero")
	makeUser(t, st, "cajero_toque_router_dos", "cajero")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "Cajero", Role: domain.RoleCajero})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// EL CUERPO TRAE EL PUNTO EXACTO, que es justo lo que esta feature promete que no existe en el
	// servidor. Un cliente viejo, un `curl` o una versión modificada del front pueden mandarlo; lo
	// que garantiza la promesa no es que nadie lo mande, es que no haya dónde ponerlo.
	cuerpo, _ := json.Marshal(map[string]any{
		"toques": []map[string]any{
			{"pantalla": "pos", "celda": 7, "orientacion": "horizontal", "x": 412.5, "y": 233.75},
			{"pantalla": "pos", "celda": 7, "orientacion": "horizontal", "rol": "admin"},
		},
	})
	if w := do(t, r, http.MethodPost, "/api/v1/usage", tok, cuerpo, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("registrar toques = %d, quiere 204: %s", w.Code, w.Body.String())
	}

	// Ninguna columna de la tabla puede contener un punto: se comprueba contra el catálogo y no
	// contra una lista escrita a mano, para que una columna agregada mañana rompa este test.
	var columnas []string
	filas, err := st.Pool.Query(ctx, `
		select column_name from information_schema.columns
		 where table_name = 'usage_touches_daily'
		 order by ordinal_position`)
	if err != nil {
		t.Fatalf("leer el catálogo: %v", err)
	}
	defer filas.Close()
	for filas.Next() {
		var c string
		if err := filas.Scan(&c); err != nil {
			t.Fatalf("escanear: %v", err)
		}
		columnas = append(columnas, c)
	}
	esperadas := map[string]bool{"day": true, "screen": true, "orientation": true, "cell": true, "role": true, "hits": true, "company_id": true}
	for _, c := range columnas {
		if !esperadas[c] {
			t.Fatalf("la tabla ganó la columna %q: cualquier columna nueva aquí es un lugar donde cabe el punto exacto o el instante, y las dos deshacen el anonimato", c)
		}
	}

	var total, conRolInventado int
	if err := st.Pool.QueryRow(ctx, `
		select coalesce(sum(hits), 0),
		       coalesce(sum(hits) filter (where role = 'admin'), 0)
		  from usage_touches_daily`).Scan(&total, &conRolInventado); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if total != 2 {
		t.Fatalf("se contaron %d toques de 2: lo de más en el cuerpo no puede tirar lo válido", total)
	}
	if conRolInventado != 0 {
		t.Fatal("el rol del CUERPO se guardó: el cliente puede decir de qué rol es y el corte deja de significar nada")
	}
}
