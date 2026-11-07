package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// RolesService: los roles de cada empresa y lo que puede cada usuario (spec 068).
type RolesService struct {
	store *store.Store
}

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
