package domain

import (
	"slices"
	"strings"
	"testing"
)

// El mapa de hoy, rol por rol. Si alguien le quita `orders.cancel_pending` a un rol, un pedido sin
// productos vuelve a quedarse sin salida para quien atiende el mostrador; si le da `payments.void`
// a un cajero, cualquiera puede sacar dinero cobrado del corte.
func TestPermissionsForEachRoleOfToday(t *testing.T) {
	all := []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending}
	cases := []struct {
		role Role
		want []Permission
	}{
		{RoleAdmin, all},
		{RoleGerente, all},
		{RoleCajero, []Permission{PermOrdersMoveLines, PermOrdersCancelPending}},
		{RoleMesero, []Permission{PermOrdersMoveLines, PermOrdersCancelPending}},
		{Role("dueño"), nil},
		{Role(""), nil},
	}
	for _, c := range cases {
		t.Run(string(c.role), func(t *testing.T) {
			got := PermissionsFor(c.role)
			if got == nil {
				t.Fatalf("PermissionsFor(%q) = nil: la sesión lo mandaría como null y no como []", c.role)
			}
			for _, p := range all {
				if slices.Contains(got, p) != slices.Contains(c.want, p) {
					t.Errorf("PermissionsFor(%q) contiene %q = %v, quiere %v",
						c.role, p, slices.Contains(got, p), slices.Contains(c.want, p))
				}
			}
			if len(got) != len(c.want) {
				t.Errorf("PermissionsFor(%q) = %v, quiere %v", c.role, got, c.want)
			}
		})
	}
}

// Cada permiso dice qué no puede hacer el usuario, y ninguno nombra un rol: con roles propios por
// empresa, «gerente» puede no existir, y un texto que lo nombre manda a buscar a alguien que no hay.
func TestEveryPermissionHasADeniedMessageWithoutRoleNames(t *testing.T) {
	want := map[Permission]string{
		PermPaymentsVoid:        "Tu usuario no puede devolver pagos",
		PermOrdersCancel:        "Tu usuario no puede cancelar pedidos",
		PermOrdersMoveLines:     "Tu usuario no puede pasar productos",
		PermOrdersCancelPending: "Tu usuario no puede quitar productos",
	}
	for _, p := range AllPermissions() {
		msg := PermissionDeniedMessage(p)
		if msg != want[p] {
			t.Errorf("PermissionDeniedMessage(%q) = %q, quiere %q", p, msg, want[p])
		}
		for _, r := range []Role{RoleAdmin, RoleGerente, RoleCajero, RoleMesero} {
			if strings.Contains(strings.ToLower(msg), string(r)) {
				t.Errorf("PermissionDeniedMessage(%q) nombra el rol %q: %q", p, r, msg)
			}
		}
	}
	if len(AllPermissions()) != len(want) {
		t.Errorf("AllPermissions() = %v, quiere los %d de la tabla", AllPermissions(), len(want))
	}
}
