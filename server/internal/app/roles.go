package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// RolesService: los roles de cada empresa y lo que puede cada usuario (spec 068).
type RolesService struct {
	store *store.Store
}

// NewRolesService arma el servicio de roles sobre el store de la aplicación.
func NewRolesService(st *store.Store) *RolesService { return &RolesService{store: st} }

// EffectivePermissions es lo que puede el usuario AHORA, leído de la base con la conexión de la
// empresa de la petición: quitarle un permiso a su rol aplica a su siguiente petición, no a la
// siguiente renovación de su sesión. Nunca devuelve nil.
func (s *RolesService) EffectivePermissions(ctx context.Context, userID int64) ([]domain.Permission, error) {
	return effectivePermissions(ctx, s.store.QC(ctx), userID)
}

func effectivePermissions(ctx context.Context, q *db.Queries, userID int64) ([]domain.Permission, error) {
	codes, err := q.UserEffectivePermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []domain.Permission{}
	for _, c := range codes {
		// Un permiso que una migración más nueva agregó y este binario no conoce no se ofrece.
		if p := domain.Permission(c); domain.KnownPermission(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// UserRoleInfo es el rol de un usuario como lo ve la sesión.
type UserRoleInfo struct {
	ID int64
	// Name es el que ve quien opera (el del producto en un sugerido).
	Name string
	// SystemCode es el código del sugerido, o "" en un rol propio.
	SystemCode domain.Role
}

// userRole lee el rol del usuario; sin rol (no debería pasar después de la 0108), uno vacío.
func userRole(ctx context.Context, q *db.Queries, userID int64) (UserRoleInfo, error) {
	r, err := q.UserRole(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRoleInfo{}, nil
	}
	if err != nil {
		return UserRoleInfo{}, err
	}
	return UserRoleInfo{ID: r.ID, Name: r.Name, SystemCode: domain.Role(r.SystemCode)}, nil
}

// UserRole es userRole con la conexión de la empresa de la petición.
func (s *RolesService) UserRole(ctx context.Context, userID int64) (UserRoleInfo, error) {
	return userRole(ctx, s.store.QC(ctx), userID)
}

// PermissionView es un permiso del catálogo como lo muestra la pantalla de roles.
type PermissionView struct {
	Code  domain.Permission `json:"code"`
	Area  string            `json:"area"`
	Label string            `json:"label"`
}

// Catalog es el catálogo de permisos en el orden del producto.
func (s *RolesService) Catalog() []PermissionView {
	out := []PermissionView{}
	for _, p := range domain.AllPermissions() {
		out = append(out, PermissionView{Code: p, Area: domain.PermissionArea(p), Label: domain.PermissionLabel(p)})
	}
	return out
}

// RoleView es un rol de la empresa con lo que puede y cómo se aparta de lo sugerido.
type RoleView struct {
	ID          int64               `json:"id"`
	SystemCode  string              `json:"systemCode"`
	Name        string              `json:"name"`
	Active      bool                `json:"active"`
	Version     int32               `json:"version"`
	ActiveUsers int64               `json:"activeUsers"`
	AllUsers    int64               `json:"allUsers"`
	Permissions []domain.Permission `json:"permissions"`
	Added       []domain.Permission `json:"added"`
	Removed     []domain.Permission `json:"removed"`
}

// roleState es lo que hace falta para editar un rol: su base y sus ajustes.
type roleState struct {
	row       db.GetRoleForUpdateRow
	system    *domain.Role
	base      []domain.Permission
	overrides map[domain.Permission]bool
}

func loadRoleState(ctx context.Context, q *db.Queries, id int64) (roleState, error) {
	row, err := q.GetRoleForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return roleState{}, domain.ErrNotFound
	}
	if err != nil {
		return roleState{}, err
	}
	st := roleState{row: row, overrides: map[domain.Permission]bool{}}
	if row.SystemCode != "" {
		r := domain.Role(row.SystemCode)
		st.system = &r
		sys, err := q.ListSystemRolePermissions(ctx)
		if err != nil {
			return roleState{}, err
		}
		for _, sp := range sys {
			if row.SystemRoleID != nil && sp.SystemRoleID == *row.SystemRoleID {
				st.base = append(st.base, domain.Permission(sp.Code))
			}
		}
	}
	ovs, err := q.ListRoleOverrides(ctx)
	if err != nil {
		return roleState{}, err
	}
	for _, o := range ovs {
		if o.RoleID == id {
			st.overrides[domain.Permission(o.Code)] = o.Granted
		}
	}
	return st, nil
}

// List devuelve los roles de la empresa (sugeridos primero), con cuántas personas los tienen.
func (s *RolesService) List(ctx context.Context) ([]RoleView, error) {
	q := s.store.QC(ctx)
	rows, err := q.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	sys, err := q.ListSystemRolePermissions(ctx)
	if err != nil {
		return nil, err
	}
	ovs, err := q.ListRoleOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RoleView, 0, len(rows))
	for _, r := range rows {
		var base []domain.Permission
		for _, sp := range sys {
			if r.SystemRoleID != nil && sp.SystemRoleID == *r.SystemRoleID {
				base = append(base, domain.Permission(sp.Code))
			}
		}
		ov := map[domain.Permission]bool{}
		for _, o := range ovs {
			if o.RoleID == r.ID {
				ov[domain.Permission(o.Code)] = o.Granted
			}
		}
		out = append(out, roleView(r.ID, r.SystemCode, r.Name, r.Active, r.Version, r.ActiveUsers, r.AllUsers, base, ov))
	}
	return out, nil
}

func roleView(id int64, code, name string, active bool, version int32, activeUsers, allUsers int64,
	base []domain.Permission, ov map[domain.Permission]bool) RoleView {
	v := RoleView{ID: id, SystemCode: code, Name: name, Active: active, Version: version,
		ActiveUsers: activeUsers, AllUsers: allUsers,
		Permissions: domain.EffectivePermissions(base, ov), Added: []domain.Permission{}, Removed: []domain.Permission{}}
	for _, p := range domain.AllPermissions() {
		granted, touched := ov[p]
		switch {
		case touched && granted && !slices.Contains(base, p):
			v.Added = append(v.Added, p)
		case touched && !granted && code != "" && slices.Contains(base, p):
			v.Removed = append(v.Removed, p)
		}
	}
	return v
}

// mutate corre un cambio de roles con el candado de la empresa tomado ANTES de leer, y al final
// exige que quede al menos una persona activa que administre roles.
func (s *RolesService) mutate(ctx context.Context, fn func(q *db.Queries) error) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompanyRoles(ctx); err != nil {
			return err
		}
		if err := fn(q); err != nil {
			return err
		}
		return ensureARoleAdminRemains(ctx, q)
	})
}

func ensureARoleAdminRemains(ctx context.Context, q *db.Queries) error {
	n, err := q.CountActiveRoleAdmins(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrLastRoleAdmin
	}
	return nil
}

// bump sube la versión del rol solo si la pantalla vio la vigente.
func bump(ctx context.Context, q *db.Queries, id int64, version int32) error {
	n, err := q.BumpRoleVersion(ctx, db.BumpRoleVersionParams{ID: id, Version: version})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrRoleChanged
	}
	return nil
}

func writeOverrides(ctx context.Context, q *db.Queries, id int64, ov map[domain.Permission]bool) error {
	if err := q.DeleteRoleOverrides(ctx, id); err != nil {
		return err
	}
	for _, p := range domain.AllPermissions() {
		if g, ok := ov[p]; ok {
			if err := q.InsertRoleOverride(ctx, db.InsertRoleOverrideParams{RoleID: id, Code: string(p), Granted: g}); err != nil {
				return err
			}
		}
	}
	return nil
}

func roleAudit(ctx context.Context, q *db.Queries, action domain.AuditAction, id int64, before, after any) error {
	return recordAudit(ctx, q, domain.AuditEvent{Action: action, EntityType: "role", EntityID: id, Before: before, After: after})
}

// withinRole: nadie toca un rol que puede más que él (ni para quitarle).
func withinRole(actor []domain.Permission, st roleState) error {
	return domain.WithinPermissions(actor, domain.EffectivePermissions(st.base, st.overrides))
}

// takenNames son los nombres con los que no puede chocar el rol `except` (0 al crear).
func takenNames(ctx context.Context, q *db.Queries, except int64) ([]string, error) {
	return q.RoleNamesExcept(ctx, except)
}

// Create da de alta un rol propio con sus permisos.
//
// `actor` es lo que puede quien crea: no se da a un rol lo que uno no tiene.
func (s *RolesService) Create(ctx context.Context, actor []domain.Permission, name string, perms []domain.Permission) (int64, error) {
	if err := domain.WithinPermissions(actor, perms); err != nil {
		return 0, err
	}
	var id int64
	err := s.mutate(ctx, func(q *db.Queries) error {
		taken, err := takenNames(ctx, q, 0)
		if err != nil {
			return err
		}
		clean, err := domain.ValidateRoleName(name, taken)
		if err != nil {
			return err
		}
		ov, err := domain.ApplyRoleEdit(nil, nil, nil, perms, nil)
		if err != nil {
			return err
		}
		if id, err = q.InsertRole(ctx, &clean); err != nil {
			return err
		}
		if err := writeOverrides(ctx, q, id, ov); err != nil {
			return err
		}
		return roleAudit(ctx, q, "role.created", id, nil, map[string]any{"name": clean, "permissions": perms})
	})
	return id, err
}

// UpdateRoleCmd: lo que la pantalla tocó. Add y Remove son solo los permisos que la persona marcó o
// desmarcó: un permiso que el producto agregó mientras la hoja estaba abierta no se vuelve «quitado».
type UpdateRoleCmd struct {
	// Actor es lo que puede quien edita: no toca un rol que puede más que él ni le da lo que no tiene.
	Actor   []domain.Permission
	ID      int64
	Version int32
	Name    *string
	Add     []domain.Permission
	Remove  []domain.Permission
}

// Update renombra un rol propio y le aplica lo que la pantalla tocó.
func (s *RolesService) Update(ctx context.Context, cmd UpdateRoleCmd) error {
	return s.mutate(ctx, func(q *db.Queries) error {
		st, err := loadRoleState(ctx, q, cmd.ID)
		if err != nil {
			return err
		}
		if err := withinRole(cmd.Actor, st); err != nil {
			return err
		}
		if err := domain.WithinPermissions(cmd.Actor, cmd.Add); err != nil {
			return err
		}
		if err := bump(ctx, q, cmd.ID, cmd.Version); err != nil {
			return err
		}
		if cmd.Name != nil && *cmd.Name != st.row.Name {
			if st.system != nil {
				return domain.ErrRoleSystem
			}
			taken, err := takenNames(ctx, q, cmd.ID)
			if err != nil {
				return err
			}
			clean, err := domain.ValidateRoleName(*cmd.Name, taken)
			if err != nil {
				return err
			}
			if err := q.RenameRole(ctx, db.RenameRoleParams{ID: cmd.ID, Name: &clean}); err != nil {
				return err
			}
			if err := roleAudit(ctx, q, "role.renamed", cmd.ID, map[string]any{"name": st.row.Name}, map[string]any{"name": clean}); err != nil {
				return err
			}
		}
		if len(cmd.Add)+len(cmd.Remove) == 0 {
			return nil
		}
		ov, err := domain.ApplyRoleEdit(st.system, st.base, st.overrides, cmd.Add, cmd.Remove)
		if err != nil {
			return err
		}
		if err := writeOverrides(ctx, q, cmd.ID, ov); err != nil {
			return err
		}
		return roleAudit(ctx, q, "role.permissions_changed", cmd.ID, nil,
			map[string]any{"name": st.row.Name, "added": cmd.Add, "removed": cmd.Remove})
	})
}

// Reset devuelve un sugerido a lo que el producto le da.
func (s *RolesService) Reset(ctx context.Context, actor []domain.Permission, id int64, version int32) error {
	return s.mutate(ctx, func(q *db.Queries) error {
		st, err := loadRoleState(ctx, q, id)
		if err != nil {
			return err
		}
		if err := withinRole(actor, st); err != nil {
			return err
		}
		if err := domain.WithinPermissions(actor, st.base); err != nil {
			return err
		}
		if st.system == nil {
			return fmt.Errorf("%w: solo un rol sugerido vuelve a lo sugerido", domain.ErrValidation)
		}
		if err := bump(ctx, q, id, version); err != nil {
			return err
		}
		if err := q.DeleteRoleOverrides(ctx, id); err != nil {
			return err
		}
		return roleAudit(ctx, q, "role.reset", id, map[string]any{"name": st.row.Name}, nil)
	})
}

// SetActive inactiva o reactiva un rol. No se inactiva con personas activas ni el admin.
func (s *RolesService) SetActive(ctx context.Context, actor []domain.Permission, id int64, version int32, active bool) error {
	return s.mutate(ctx, func(q *db.Queries) error {
		st, err := loadRoleState(ctx, q, id)
		if err != nil {
			return err
		}
		if err := withinRole(actor, st); err != nil {
			return err
		}
		if !active {
			n, err := q.CountActiveUsersWithRole(ctx, id)
			if err != nil {
				return err
			}
			if err := domain.CanDeactivateRole(st.system, int(n)); err != nil {
				return err
			}
		}
		if err := bump(ctx, q, id, version); err != nil {
			return err
		}
		if err := q.SetRoleActive(ctx, db.SetRoleActiveParams{ID: id, Active: active}); err != nil {
			return err
		}
		action := domain.AuditAction("role.reactivated")
		if !active {
			action = "role.deactivated"
		}
		return roleAudit(ctx, q, action, id, map[string]any{"name": st.row.Name}, nil)
	})
}

// Delete borra un rol propio que nadie tiene ni tuvo.
func (s *RolesService) Delete(ctx context.Context, actor []domain.Permission, id int64) error {
	return s.mutate(ctx, func(q *db.Queries) error {
		st, err := loadRoleState(ctx, q, id)
		if err != nil {
			return err
		}
		if err := withinRole(actor, st); err != nil {
			return err
		}
		n, err := q.CountUsersWithRole(ctx, id)
		if err != nil {
			return err
		}
		if err := domain.CanDeleteRole(st.system, int(n)); err != nil {
			return err
		}
		n, err = q.DeleteRole(ctx, id)
		if err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrRoleSystem
		}
		return roleAudit(ctx, q, "role.deleted", id, map[string]any{"name": st.row.Name}, nil)
	})
}
