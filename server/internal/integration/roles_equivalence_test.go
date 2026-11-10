//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// NADA CAMBIA PARA QUIEN YA OPERA (spec 068, US1, SC-001).
//
// Pasar los RequireRole a permisos por capacidad no puede abrir ni cerrar una sola ruta para ningún
// rol sugerido. testdata/route_gates.golden se generó con el router de ANTES del cambio (rol por
// rol, ruta por ruta: «pasa» o «403») y este test exige que el de hoy diga exactamente lo mismo.
// Regenerarlo (UPDATE_ROUTE_GATES=1) es cambiar a propósito lo que puede un rol: va en su PR con
// el porqué.
//
// Solo se mira si la ruta responde 403 de permiso; lo que haga el handler después (con servicios
// vacíos truena o rechaza el cuerpo) no importa aquí.
func TestNoSuggestedRoleGainsOrLosesARoute(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Broker: realtime.NewBroker(), APITokens: app.NewAPITokensService(st)})
	router := httpapi.Router(config.Config{}, jm, h, st)

	roles := []domain.Role{domain.RoleAdmin, domain.RoleGerente, domain.RoleCajero, domain.RoleMesero}
	tokens := map[domain.Role]string{}
	for _, r := range roles {
		id := makeUser(t, st, "equiv_"+string(r), string(r))
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: string(r), Role: r})
		if err != nil {
			t.Fatal(err)
		}
		tokens[r] = tok
	}

	param := regexp.MustCompile(`\{[^}]+\}`)
	var lines []string
	err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/") || strings.HasPrefix(route, "/api/v1/platform") ||
			strings.HasPrefix(route, "/api/v1/auth/") || strings.HasSuffix(route, "/events") ||
			strings.HasPrefix(route, "/api/v1/webhooks") {
			return nil
		}
		path := strings.TrimSuffix(param.ReplaceAllString(route, "1"), "/*")
		var who []string
		for _, r := range roles {
			if !forbiddenByPermission(t, router, method, path, tokens[r]) {
				who = append(who, string(r))
			}
		}
		lines = append(lines, method+" "+route+" "+strings.Join(who, ","))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	got := strings.Join(lines, "\n") + "\n"

	golden := filepath.Join("testdata", "route_gates.golden")
	if os.Getenv("UPDATE_ROUTE_GATES") == "1" {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		wl := map[string]string{}
		for _, l := range strings.Split(strings.TrimSpace(string(want)), "\n") {
			wl[routeKey(l)] = l
		}
		for _, l := range lines {
			if w, ok := wl[routeKey(l)]; !ok {
				t.Errorf("ruta nueva sin referencia: %s", l)
			} else if w != l {
				t.Errorf("cambió quién pasa:\n  antes:   %s\n  después: %s", w, l)
			}
			delete(wl, routeKey(l))
		}
		for _, l := range wl {
			t.Errorf("ruta que desapareció: %s", l)
		}
	}
}

func routeKey(line string) string {
	f := strings.Fields(line)
	return f[0] + " " + f[1]
}

// forbiddenByPermission: la ruta respondió 403 de autorización (no un 403 de negocio del handler).
func forbiddenByPermission(t *testing.T, r http.Handler, method, path, tok string) bool {
	t.Helper()
	w := do(t, r, method, path, tok, []byte(`{}`), "application/json")
	if w.Code != http.StatusForbidden {
		return false
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code == "FORBIDDEN"
}
