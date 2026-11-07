package domain

import (
	"errors"
	"slices"
	"testing"
)

// HERENCIA (decisión del dueño, 2026-11-06): un sugerido tiene lo que el producto le da, más lo que
// la empresa le agregó, menos lo que le quitó; un rol propio, solo lo que la empresa le dio.
func TestEffectivePermissions(t *testing.T) {
	base := []Permission{PermOrdersMoveLines, PermCashOperate}
	cases := []struct {
		name      string
		base      []Permission
		overrides map[Permission]bool
		want      []Permission
	}{
		{"sugerido sin ajustes", base, nil, base},
		{"sugerido con un agregado", base, map[Permission]bool{PermReportsView: true}, []Permission{PermOrdersMoveLines, PermCashOperate, PermReportsView}},
		{"sugerido con un quitado", base, map[Permission]bool{PermCashOperate: false}, []Permission{PermOrdersMoveLines}},
		// Un permiso nuevo del producto entra a la base y llega solo, salvo que se haya quitado.
		{"permiso nuevo del producto llega", append(base, PermSalesView), nil, []Permission{PermOrdersMoveLines, PermCashOperate, PermSalesView}},
		{"permiso nuevo ya quitado no llega", append(base, PermSalesView), map[Permission]bool{PermSalesView: false}, base},
		{"propio: solo lo agregado", nil, map[Permission]bool{PermSalesView: true}, []Permission{PermSalesView}},
		{"propio: un quitar no hace nada", nil, map[Permission]bool{PermSalesView: false}, []Permission{}},
		// Un permiso que el producto dejó de tener no rompe el cálculo: se ignora.
		{"permiso desconocido se ignora", base, map[Permission]bool{"orders.ghost": true}, base},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EffectivePermissions(c.base, c.overrides)
			if got == nil {
				t.Fatal("nil: la sesión lo mandaría como null")
			}
			if !samePerms(got, c.want) {
				t.Errorf("= %v, quiere %v", got, c.want)
			}
		})
	}
}

func samePerms(a, b []Permission) bool {
	if len(a) != len(b) {
		return false
	}
	for _, p := range a {
		if !slices.Contains(b, p) {
			return false
		}
	}
	return true
}

// GUARDAR POR DIFERENCIA: solo lo que la persona tocó. Agregar lo que el producto ya da o quitar lo
// que el rol no tenía no deja ajuste; así un sugerido sin cambios reales sigue heredando todo.
func TestApplyRoleEdit(t *testing.T) {
	base := []Permission{PermOrdersMoveLines, PermCashOperate}
	cajero := RoleCajero
	admin := RoleAdmin
	cases := []struct {
		name    string
		system  *Role
		base    []Permission
		current map[Permission]bool
		add     []Permission
		remove  []Permission
		want    map[Permission]bool
		err     error
	}{
		{"agregar a un sugerido", &cajero, base, nil, []Permission{PermReportsView}, nil, map[Permission]bool{PermReportsView: true}, nil},
		{"quitar a un sugerido", &cajero, base, nil, nil, []Permission{PermCashOperate}, map[Permission]bool{PermCashOperate: false}, nil},
		{"devolver lo quitado borra el ajuste", &cajero, base, map[Permission]bool{PermCashOperate: false}, []Permission{PermCashOperate}, nil, map[Permission]bool{}, nil},
		{"quitar lo agregado borra el ajuste", &cajero, base, map[Permission]bool{PermReportsView: true}, nil, []Permission{PermReportsView}, map[Permission]bool{}, nil},
		{"un ajuste que no se tocó se conserva", &cajero, base, map[Permission]bool{PermSalesView: true}, []Permission{PermReportsView}, nil, map[Permission]bool{PermSalesView: true, PermReportsView: true}, nil},
		{"propio: agregar", nil, nil, nil, []Permission{PermSalesView}, nil, map[Permission]bool{PermSalesView: true}, nil},
		{"propio: quitar borra", nil, nil, map[Permission]bool{PermSalesView: true}, nil, []Permission{PermSalesView}, map[Permission]bool{}, nil},
		{"el admin no pierde la administración de roles", &admin, AllPermissions(), nil, nil, []Permission{PermRolesManage}, nil, ErrRoleAdminKeepsRoles},
		{"al admin sí se le quita otra cosa", &admin, AllPermissions(), nil, nil, []Permission{PermReportsView}, map[Permission]bool{PermReportsView: false}, nil},
		{"permiso que no existe", &cajero, base, nil, []Permission{"orders.ghost"}, nil, nil, ErrValidation},
		{"el mismo en agregar y quitar", &cajero, base, nil, []Permission{PermSalesView}, []Permission{PermSalesView}, nil, ErrValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ApplyRoleEdit(c.system, c.base, c.current, c.add, c.remove)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("err = %v, quiere %v", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("= %v, quiere %v", got, c.want)
			}
			for p, v := range c.want {
				if g, ok := got[p]; !ok || g != v {
					t.Errorf("[%s] = %v (%v), quiere %v", p, g, ok, v)
				}
			}
		})
	}
}

// El nombre de un rol propio: único en la empresa sin distinguir mayúsculas, acentos ni espacios,
// también contra los nombres de los sugeridos.
func TestValidateRoleName(t *testing.T) {
	taken := []string{"Admin", "Gerente", "Cajero", "Mesero", "Encargado de turno"}
	cases := []struct {
		in   string
		want string
		err  error
	}{
		{"  Barista   de   día ", "Barista de día", nil},
		{"", "", ErrValidation},
		{"   ", "", ErrValidation},
		{"Cocinero con un nombre larguísimo que no cabe", "", ErrValidation},
		{"cajero", "", ErrRoleNameTaken},
		{"ENCARGADO  DE TURNO", "", ErrRoleNameTaken},
		{"Cájero", "", ErrRoleNameTaken},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ValidateRoleName(c.in, taken)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("err = %v, quiere %v", err, c.err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("= %q, %v; quiere %q", got, err, c.want)
			}
		})
	}
}

func TestCanDeactivateAndDelete(t *testing.T) {
	admin, cajero := RoleAdmin, RoleCajero
	if err := CanDeactivateRole(&admin, 0); !errors.Is(err, ErrRoleAdminKeepsRoles) {
		t.Errorf("inactivar al admin: %v", err)
	}
	if err := CanDeactivateRole(&cajero, 2); !errors.Is(err, ErrRoleInUse) {
		t.Errorf("inactivar con 2 activos: %v", err)
	}
	if err := CanDeactivateRole(nil, 0); err != nil {
		t.Errorf("inactivar propio sin activos: %v", err)
	}
	if err := CanDeleteRole(&cajero, 0); !errors.Is(err, ErrRoleSystem) {
		t.Errorf("borrar sugerido: %v", err)
	}
	if err := CanDeleteRole(nil, 1); !errors.Is(err, ErrRoleInUse) {
		t.Errorf("borrar propio con usuario: %v", err)
	}
	if err := CanDeleteRole(nil, 0); err != nil {
		t.Errorf("borrar propio sin usuarios: %v", err)
	}
}

// Nadie administra a quien puede más que él: sin esto, «administrar empleados» bastaba para
// asignarse el rol admin (auditoría de seguridad de la 068).
func TestWithinPermissions(t *testing.T) {
	actor := []Permission{PermEmployeesManage, PermSalesView}
	if err := WithinPermissions(actor, []Permission{PermSalesView}); err != nil {
		t.Errorf("menos que uno: %v", err)
	}
	if err := WithinPermissions(actor, []Permission{PermSalesView, PermRolesManage}); !errors.Is(err, ErrForbidden) {
		t.Errorf("más que uno = %v, quiere ErrForbidden", err)
	}
	if err := WithinPermissions(nil, []Permission{PermSalesView}); !errors.Is(err, ErrForbidden) {
		t.Errorf("sin permisos no se administra a nadie con permisos: %v", err)
	}
	if err := WithinPermissions(nil, nil); err != nil {
		t.Errorf("vacío contra vacío: %v", err)
	}
}

// El rechazo por personas asignadas se lee bien, en singular y en plural, y empieza como oración
// (la prueba con usuario nuevo encontró «el rol está en uso: lo tienen 1 persona activas»).
func TestRoleInUseReadsWell(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{CanDeactivateRole(nil, 1), "conflicto: Lo tiene 1 persona activa; reasígnala primero"},
		{CanDeactivateRole(nil, 3), "conflicto: Lo tienen 3 personas activas; reasígnalas primero"},
		{CanDeleteRole(nil, 1), "conflicto: Lo tiene o lo tuvo 1 persona; solo se puede inactivar"},
		{CanDeleteRole(nil, 2), "conflicto: Lo tienen o lo tuvieron 2 personas; solo se puede inactivar"},
	}
	for _, c := range cases {
		if !errors.Is(c.err, ErrRoleInUse) || c.err.Error() != c.want {
			t.Errorf("= %q, quiere %q", c.err, c.want)
		}
	}
}
