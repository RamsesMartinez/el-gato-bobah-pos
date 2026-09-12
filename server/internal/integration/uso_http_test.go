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

// LA MEDICIÓN POR EL ROUTER REAL, con la base de verdad detrás.
//
// Lo que solo se ve aquí: que la ruta quedó dentro del grupo con tenant —sin eso, RLS no tiene
// empresa y el insert falla—, y que lo que el cliente manda de más no llega a ninguna columna.
func TestLaMedicionPorElRouter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Usage: app.NewUsageService(st)})
	r := httpapi.Router(config.Config{}, jm, h, st)

	// Dos cajeros: así el corte por rol se conserva y se puede verificar que llega el del TOKEN.
	id := makeUser(t, st, "cajero_router_uno", "cajero")
	makeUser(t, st, "cajero_router_dos", "cajero")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "Cajero", Role: domain.RoleCajero})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	t.Run("sin token no se mide", func(t *testing.T) {
		cuerpo, _ := json.Marshal(map[string]any{"eventos": []map[string]string{{"pantalla": "pos"}}})
		if w := do(t, r, http.MethodPost, "/api/v1/usage", "", cuerpo, "application/json"); w.Code != http.StatusUnauthorized {
			t.Fatalf("sin token = %d, quiere 401: la empresa y el rol salen del token", w.Code)
		}
	})

	// El cuerpo trae de más: un `detail` con datos de cliente y un `rol` inventado. Ninguno de los
	// dos puede terminar en la base.
	cuerpo, _ := json.Marshal(map[string]any{
		"eventos": []map[string]any{
			{"pantalla": "pos", "detail": map[string]string{"cliente": "Juan Pérez"}, "rol": "admin"},
			{"pantalla": "caja", "accion": "cerrar-turno"},
		},
	})
	if w := do(t, r, http.MethodPost, "/api/v1/usage", tok, cuerpo, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("registrar = %d, quiere 204: %s", w.Code, w.Body.String())
	}

	var conDetalle, conRolInventado, total int
	if err := st.Pool.QueryRow(ctx, `
		select count(*) filter (where detail is not null),
		       count(*) filter (where role = 'admin'),
		       count(*)
		  from usage_events`).Scan(&conDetalle, &conRolInventado, &total); err != nil {
		t.Fatalf("leer los eventos: %v", err)
	}
	if total != 2 {
		t.Fatalf("se guardaron %d eventos de 2", total)
	}
	if conDetalle != 0 {
		t.Fatal("el `detail` del cuerpo llegó a la columna: esa puerta se abre desde adentro (coordenadas, FR-013) o no se abre — llena desde el cliente es por donde entra lo que FR-003 prohíbe")
	}
	if conRolInventado != 0 {
		t.Fatal("el rol del CUERPO se guardó: el cliente puede decir de qué rol es y el corte por rol deja de significar nada")
	}
	var conRolDelToken int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_events where role = 'cajero'`).Scan(&conRolDelToken); err != nil {
		t.Fatalf("contar por rol: %v", err)
	}
	if conRolDelToken != 2 {
		t.Fatalf("%d eventos con el rol del token y son 2", conRolDelToken)
	}
}
