package domain

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Roles por empresa (spec 068, decisiones del dueño del 2026-11-06). Los cuatro sugeridos existen en
// toda empresa; lo que puede un sugerido es lo que el producto le da más lo que la empresa le agregó
// menos lo que le quitó, y lo que puede un rol propio es solo lo que la empresa le dio.

var (
	ErrRoleNameTaken = fmt.Errorf("%w: ya existe un rol con ese nombre", ErrConflict)
	ErrRoleSystem    = fmt.Errorf("%w: los roles sugeridos no se borran ni se renombran", ErrConflict)
	// ErrRoleAdminKeepsRoles: el admin no pierde la administración de roles ni se inactiva; sin él,
	// una empresa podría quedarse sin nadie que corrija los roles.
	ErrRoleAdminKeepsRoles = fmt.Errorf("%w: el administrador siempre puede administrar roles", ErrConflict)
	ErrLastRoleAdmin       = fmt.Errorf("%w: debe quedar al menos una persona activa que administre roles", ErrConflict)
	ErrRoleChanged         = fmt.Errorf("%w: el rol cambió mientras lo editabas; vuelve a abrirlo", ErrConflict)
	ErrRoleInactive        = fmt.Errorf("%w: ese rol está inactivo; elige uno activo", ErrConflict)
	// ErrRoleInUse se envuelve con cuántas personas lo tienen (RoleInUse); su propio texto es solo el
	// del conflicto, para que el mensaje que ve quien opera empiece por lo que importa.
	ErrRoleInUse = fmt.Errorf("%w", ErrConflict)
)

// RoleInUse arma el rechazo con cuántas personas lo tienen, que es lo que quien opera necesita.
func RoleInUse(people int, activeOnly bool) error {
	if activeOnly {
		if people == 1 {
			return fmt.Errorf("%w: Lo tiene 1 persona activa; reasígnala primero", ErrRoleInUse)
		}
		return fmt.Errorf("%w: Lo tienen %d personas activas; reasígnalas primero", ErrRoleInUse, people)
	}
	if people == 1 {
		return fmt.Errorf("%w: Lo tiene asignado 1 persona (activa o dada de baja); solo se puede inactivar", ErrRoleInUse)
	}
	return fmt.Errorf("%w: Lo tienen asignado %d personas (activas o dadas de baja); solo se puede inactivar", ErrRoleInUse, people)
}

// MaxRoleName es el largo máximo del nombre de un rol propio.
const MaxRoleName = 40

// EffectivePermissions calcula lo que puede un rol. `base` es lo que el producto le da (nil en un
// propio); `overrides` lo que la empresa agregó (true) o quitó (false). Un permiso que el producto
// ya no tiene se ignora. Nunca devuelve nil.
func EffectivePermissions(base []Permission, overrides map[Permission]bool) []Permission {
	out := []Permission{}
	for _, p := range AllPermissions() {
		granted, touched := overrides[p]
		inBase := slices.Contains(base, p)
		switch {
		case touched && granted:
			out = append(out, p)
		case touched && !granted:
			// quitado (o un «quitar» en un propio, que no tiene efecto)
		case inBase:
			out = append(out, p)
		}
	}
	return out
}

// ApplyRoleEdit aplica lo que la persona tocó (`add`, `remove`) a los ajustes actuales y devuelve
// los ajustes nuevos, mínimos: lo que coincide con la base no deja ajuste, así un sugerido sigue
// heredando lo que el producto agregue después. `system` es el sugerido, o nil en un propio.
func ApplyRoleEdit(system *Role, base []Permission, current map[Permission]bool, add, remove []Permission) (map[Permission]bool, error) {
	out := map[Permission]bool{}
	for p, v := range current {
		out[p] = v
	}
	for _, p := range add {
		if !KnownPermission(p) || slices.Contains(remove, p) {
			return nil, fmt.Errorf("%w: permiso %q", ErrValidation, p)
		}
		if system != nil && slices.Contains(base, p) {
			delete(out, p)
		} else {
			out[p] = true
		}
	}
	for _, p := range remove {
		if !KnownPermission(p) {
			return nil, fmt.Errorf("%w: permiso %q", ErrValidation, p)
		}
		if system != nil && *system == RoleAdmin && p == PermRolesManage {
			return nil, ErrRoleAdminKeepsRoles
		}
		if system != nil && slices.Contains(base, p) {
			out[p] = false
		} else {
			delete(out, p)
		}
	}
	return out, nil
}

// ValidateRoleName limpia el nombre de un rol propio y lo rechaza si está vacío, si pasa de
// MaxRoleName o si choca con otro de la empresa (`taken`, incluidos los nombres de los sugeridos)
// sin distinguir mayúsculas, acentos ni espacios.
func ValidateRoleName(name string, taken []string) (string, error) {
	clean := strings.Join(strings.Fields(name), " ")
	if clean == "" {
		return "", fmt.Errorf("%w: el rol necesita un nombre", ErrValidation)
	}
	if utf8.RuneCountInString(clean) > MaxRoleName {
		return "", fmt.Errorf("%w: el nombre del rol cabe en %d caracteres", ErrValidation, MaxRoleName)
	}
	key := CatalogNameKey(clean)
	for _, t := range taken {
		if CatalogNameKey(t) == key {
			return "", ErrRoleNameTaken
		}
	}
	return clean, nil
}

// CanDeactivateRole: el admin nunca; un rol con personas activas, tampoco (primero se reasignan).
func CanDeactivateRole(system *Role, activeUsers int) error {
	if system != nil && *system == RoleAdmin {
		return ErrRoleAdminKeepsRoles
	}
	if activeUsers > 0 {
		return RoleInUse(activeUsers, true)
	}
	return nil
}

// CanDeleteRole: solo un rol propio que nadie tiene asignado (los dados de baja cuentan).
func CanDeleteRole(system *Role, anyUsers int) error {
	if system != nil {
		return ErrRoleSystem
	}
	if anyUsers > 0 {
		return RoleInUse(anyUsers, false)
	}
	return nil
}

// ErrBeyondOwnPermissions: quien actúa intentó dar, quitar o asignar algo que su usuario no puede.
var ErrBeyondOwnPermissions = fmt.Errorf("%w: Tu usuario no puede administrar permisos que no tiene", ErrForbidden)

// WithinPermissions exige que `target` (lo que puede un rol o una persona) quepa en lo que puede quien
// actúa. Es la barrera contra escalar: sin ella, quien solo administra empleados se asignaba el rol
// admin o reseteaba la contraseña del admin, y quien administra roles se daba cualquier permiso.
func WithinPermissions(actor, target []Permission) error {
	for _, p := range target {
		if !slices.Contains(actor, p) {
			return ErrBeyondOwnPermissions
		}
	}
	return nil
}
