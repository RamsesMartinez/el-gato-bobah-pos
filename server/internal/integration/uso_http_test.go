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
	// dos puede terminar en la base — el primero ni siquiera tiene columna a la que llegar, y el
	// segundo se ignora porque el rol lo pone el servidor desde el token.
	cuerpo, _ := json.Marshal(map[string]any{
		"eventos": []map[string]any{
			{"pantalla": "pos", "detail": map[string]string{"cliente": "Juan Pérez"}, "rol": "admin"},
			{"pantalla": "caja", "accion": "cerrar-turno"},
		},
	})
	if w := do(t, r, http.MethodPost, "/api/v1/usage", tok, cuerpo, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("registrar = %d, quiere 204: %s", w.Code, w.Body.String())
	}

	var conRolInventado, conRolDelToken, total int
	if err := st.Pool.QueryRow(ctx, `
		select coalesce(sum(hits) filter (where role = 'admin'), 0),
		       coalesce(sum(hits) filter (where role = 'cajero'), 0),
		       coalesce(sum(hits), 0)
		  from usage_daily`).Scan(&conRolInventado, &conRolDelToken, &total); err != nil {
		t.Fatalf("leer el agregado: %v", err)
	}
	if total != 2 {
		t.Fatalf("se contaron %d eventos de 2", total)
	}
	if conRolInventado != 0 {
		t.Fatal("el rol del CUERPO se guardó: el cliente puede decir de qué rol es y el corte por rol deja de significar nada")
	}
	if conRolDelToken != 2 {
		t.Fatalf("%d eventos con el rol del token y son 2", conRolDelToken)
	}
}
