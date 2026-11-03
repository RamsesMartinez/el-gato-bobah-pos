//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Cambiar qué resta un producto o un extra mueve existencias (spec 037): lo decide el permiso
// `menu.configure`, no el nombre del rol, y aunque la petición no venga de la pantalla (FR-016).
func TestCompositionRoutesAreGatedByMenuConfigure(t *testing.T) {
	t.Parallel()
	soloCajero := func(r domain.Role) []domain.Permission {
		if r == domain.RoleCajero {
			return []domain.Permission{domain.PermMenuConfigure}
		}
		return []domain.Permission{}
	}
	for _, c := range []struct {
		name   string
		perms  httpapi.PermissionResolver
		role   string
		denied bool
	}{
		{"el cajero de hoy no puede", nil, "cajero", true},
		{"el gerente de hoy sí", nil, "gerente", false},
		{"un cajero con el permiso sí: decide el permiso", soloCajero, "cajero", false},
		{"un admin sin el permiso no: decide el permiso", soloCajero, "admin", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := newTestStore(t)
			jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
			h := httpapi.NewHandlers(httpapi.Deps{
				JWT: jm, Admin: app.NewAdminService(st), MenuCache: cache.NewMenuCache(""),
				Broker: realtime.NewBroker(), Permissions: c.perms,
			})
			r := httpapi.Router(config.Config{}, jm, h, st)
			id := makeUser(t, st, "u_"+c.role, c.role)
			tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: c.role, Role: domain.Role(c.role)})
			if err != nil {
				t.Fatal(err)
			}
			prod := makeProduct(t, st, "Producto permiso", decimal.RequireFromString("10"), false)
			opt := opcionConTope(t, st, "Grupo permiso", "Extra permiso", decimal.Zero, 1, prod)
			for _, path := range []string{
				"/api/v1/admin/products/" + itoa(int(prod)) + "/composition",
				"/api/v1/admin/modifier-options/" + itoa(int(opt)) + "/composition",
			} {
				put := do(t, r, http.MethodPut, path, tok, []byte(`{"mode":"nothing"}`), "application/json")
				confirm := do(t, r, http.MethodPost, path+"/confirm", tok, nil, "")
				for verb, w := range map[string]int{"PUT": put.Code, "POST confirm": confirm.Code} {
					if c.denied && w != http.StatusForbidden {
						t.Errorf("%s %s = %d, quiere 403", verb, path, w)
					}
					if !c.denied && w == http.StatusForbidden {
						t.Errorf("%s %s = 403 con el permiso", verb, path)
					}
				}
			}
		})
	}
}
