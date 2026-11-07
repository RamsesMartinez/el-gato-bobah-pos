package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/hibp"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// GetPreference devuelve el valor JSON crudo de una preferencia del usuario, o nil si no existe.
func (s *UsersService) GetPreference(ctx context.Context, userID int64, key string) ([]byte, error) {
	v, err := s.store.QC(ctx).GetUserPreference(ctx, db.GetUserPreferenceParams{UserID: userID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

// SetPreference guarda (upsert) el valor JSON de una preferencia del usuario. value debe ser JSON válido.
func (s *UsersService) SetPreference(ctx context.Context, userID int64, key string, value []byte) error {
	return s.store.QC(ctx).SetUserPreference(ctx, db.SetUserPreferenceParams{UserID: userID, Key: key, Value: value})
}

type UsersService struct {
	store       *store.Store
	hibp        *hibp.Client
	hibpEnabled bool
	// pinPepper: secreto con el que se calcula la huella determinista del PIN. Vacío = el modo de
	// solo-PIN no se puede encender, y no se guarda huella.
	pinPepper string
}

func NewUsersService(s *store.Store, hibpClient *hibp.Client, hibpEnabled bool, pinPepper string) *UsersService {
	return &UsersService{store: s, hibp: hibpClient, hibpEnabled: hibpEnabled, pinPepper: pinPepper}
}

func (s *UsersService) checkPassword(ctx context.Context, pw string) error {
	return checkPasswordStrength(ctx, pw, s.hibp, s.hibpEnabled)
}

func hashPIN(pin string) (*string, error) {
	h, err := auth.HashSecret(pin)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

type CreateUserInput struct {
	Name     string
	Username *string
	// RoleID es el rol de la empresa (spec 068). Sin él, Role (código de un sugerido) por
	// compatibilidad con la pantalla anterior durante la subida.
	RoleID *int64
	Role   domain.Role
	// ActorPerms es lo que puede quien da de alta: el rol que asigna no puede pasarse de eso.
	ActorPerms    []domain.Permission
	PIN           string  // opcional
	Password      string  // OBLIGATORIO y fuerte (política + HIBP)
	RecoveryEmail *string // opcional (email externo para recuperación)
}

// Create da de alta un empleado en la empresa del tenant (RLS auto-sella company_id). Password
// obligatorio y validado; PIN opcional. must_change_password=true: el admin fija una contraseña
// inicial y el empleado la cambia en su primer login.
func (s *UsersService) Create(ctx context.Context, in CreateUserInput) (domain.User, error) {
	if in.Name == "" {
		return domain.User{}, domain.ErrValidation
	}
	if err := s.checkPassword(ctx, in.Password); err != nil {
		return domain.User{}, err
	}
	pwHash, err := auth.HashSecret(in.Password)
	if err != nil {
		return domain.User{}, err
	}
	params := db.CreateUserParams{
		Name: in.Name, Username: in.Username,
		PasswordHash: &pwHash, RecoveryEmail: in.RecoveryEmail, MustChangePassword: true,
	}
	if in.PIN != "" {
		// La MISMA ruta que SetPIN, no `hashPIN` pelado. Cuando la validación se mudó a SetPIN,
		// esto se quedó atrás y volvió a aceptar "1234"; peor, no calculaba la huella, así que el
		// índice único —que es parcial sobre las filas con huella— no veía esta fila y se podía dar
		// de alta a alguien con el PIN de otro para recibir su sesión y su rol.
		pinHash, lookup, err := s.prepararPin(ctx, in.PIN)
		if err != nil {
			return domain.User{}, err
		}
		params.PinHash = pinHash
		params.PinLookup = lookup
	}
	var u db.User
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompanyRoles(ctx); err != nil {
			return err
		}
		roleID, perms, err := assignableRole(ctx, q, in.RoleID, in.Role, true)
		if err != nil {
			return err
		}
		if err := domain.WithinPermissions(in.ActorPerms, perms); err != nil {
			return err
		}
		params.RoleID = &roleID
		u, err = q.CreateUser(ctx, params)
		return err
	})
	if isUniqueViolation(err) {
		return domain.User{}, domain.ErrPinRepetido
	}
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(u), nil
}

// assignableRole resuelve el rol que se le asigna a un empleado: uno de la empresa (RLS) y activo si
// el empleado queda activo. Sin roleId, el sugerido del código (la pantalla anterior manda `role`).
// Devuelve también lo que puede ese rol, para que nadie asigne más de lo que él mismo puede.
func assignableRole(ctx context.Context, q *db.Queries, roleID *int64, code domain.Role, userActive bool) (int64, []domain.Permission, error) {
	var id int64
	switch {
	case roleID != nil:
		id = *roleID
	case code.Valid():
		var err error
		if id, err = q.SuggestedRoleID(ctx, string(code)); err != nil {
			return 0, nil, err
		}
	default:
		return 0, nil, fmt.Errorf("%w: elige un rol", domain.ErrValidation)
	}
	st, err := loadRoleState(ctx, q, id)
	if errors.Is(err, domain.ErrNotFound) {
		return 0, nil, fmt.Errorf("%w: ese rol no existe", domain.ErrValidation)
	}
	if err != nil {
		return 0, nil, err
	}
	if userActive && !st.row.Active {
		return 0, nil, domain.ErrRoleInactive
	}
	return id, domain.EffectivePermissions(st.base, st.overrides), nil
}

// userPermissions es lo que puede ahora un empleado (nada si está dado de baja o su rol inactivo).
func userPermissions(ctx context.Context, q *db.Queries, userID int64) ([]domain.Permission, error) {
	return effectivePermissions(ctx, q, userID)
}

// CanManageUser rechaza administrar (contraseña, PIN) a quien puede más que quien actúa.
func (s *UsersService) CanManageUser(ctx context.Context, actorPerms []domain.Permission, userID int64) error {
	target, err := userPermissions(ctx, s.store.QC(ctx), userID)
	if err != nil {
		return err
	}
	return domain.WithinPermissions(actorPerms, target)
}

func (s *UsersService) List(ctx context.Context) ([]domain.User, error) {
	rows, err := s.store.QC(ctx).ListActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, len(rows))
	for i, u := range rows {
		out[i] = toDomainUser(u)
	}
	return out, nil
}

type UpdateUserInput struct {
	ID     int64
	Name   string
	RoleID *int64
	Role   domain.Role
	// ActorPerms es lo que puede quien edita: no administra a quien puede más que él ni le da más.
	ActorPerms    []domain.Permission
	IsActive      bool
	RecoveryEmail *string
}

// Update edita datos no-secretos de un empleado (nombre, rol, alta/baja, email de recuperación).
//
// Bajo el candado de roles (28068): cambiar el rol o dar de baja al último que administra roles se
// rechaza, también si dos bajas llegan a la vez.
func (s *UsersService) Update(ctx context.Context, in UpdateUserInput) (domain.User, error) {
	if in.Name == "" {
		return domain.User{}, domain.ErrValidation
	}
	var u db.User
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompanyRoles(ctx); err != nil {
			return err
		}
		prev, err := q.GetUserByID(ctx, in.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Nadie edita a quien puede más que él (ni lo da de baja ni le cambia el rol).
		now, err := userPermissions(ctx, q, in.ID)
		if err != nil {
			return err
		}
		if err := domain.WithinPermissions(in.ActorPerms, now); err != nil {
			return err
		}
		// La pantalla anterior manda `role` con el código que ve; a alguien con rol propio el trigger
		// le puso «mesero». Ese mismo código no es un cambio de rol: se conserva el suyo.
		requested := in.RoleID
		if requested == nil && prev.RoleID != nil && string(in.Role) == prev.Role {
			requested = prev.RoleID
		}
		roleID, perms, err := assignableRole(ctx, q, requested, in.Role, in.IsActive)
		if err != nil {
			return err
		}
		if err := domain.WithinPermissions(in.ActorPerms, perms); err != nil {
			return err
		}
		u, err = q.UpdateUser(ctx, db.UpdateUserParams{
			ID: in.ID, Name: in.Name, RoleID: &roleID, IsActive: in.IsActive, RecoveryEmail: in.RecoveryEmail,
		})
		if err != nil {
			return err
		}
		roleChanged := prev.RoleID == nil || *prev.RoleID != roleID
		if roleChanged {
			subject := in.ID
			if err := recordAudit(ctx, q, domain.AuditEvent{Action: "user.role_changed", EntityType: "user", EntityID: in.ID,
				SubjectUserID: &subject, Before: map[string]any{"roleId": prev.RoleID}, After: map[string]any{"roleId": roleID}}); err != nil {
				return err
			}
		}
		// Solo si cambió el rol o el alta: una empresa que ya estaba sin quien administre roles (alta a
		// mano) no se queda sin poder corregir un nombre.
		if roleChanged || prev.IsActive != in.IsActive {
			return ensureARoleAdminRemains(ctx, q)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(u), nil
}

// AdminSetPassword: reset por admin (o por el CLI -reset-password, que llama esto mismo).
// Fuerza cambio en el próximo login (must_change_password).
func (s *UsersService) AdminSetPassword(ctx context.Context, userID int64, password string) error {
	if err := s.checkPassword(ctx, password); err != nil {
		return err
	}
	h, err := auth.HashSecret(password)
	if err != nil {
		return err
	}
	if err := s.store.QC(ctx).SetUserPassword(ctx, db.SetUserPasswordParams{ID: userID, PasswordHash: &h, MustChangePassword: true}); err != nil {
		return err
	}
	logging.SecurityEvent(ctx, "password_reset_by_admin", "user_id", userID)
	return nil
}

// ChangeOwnPassword: el empleado cambia su propia contraseña (verifica la actual). Quita el
// flag must_change_password. Sirve tanto para el flujo normal como para el primer login forzado.
func (s *UsersService) ChangeOwnPassword(ctx context.Context, userID int64, current, next string) error {
	u, err := s.store.QC(ctx).GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if u.PasswordHash == nil || !auth.CheckSecret(*u.PasswordHash, current) {
		return domain.ErrInvalidCredentials
	}
	if err := s.checkPassword(ctx, next); err != nil {
		return err
	}
	h, err := auth.HashSecret(next)
	if err != nil {
		return err
	}
	if err := s.store.QC(ctx).SetUserPassword(ctx, db.SetUserPasswordParams{ID: userID, PasswordHash: &h, MustChangePassword: false}); err != nil {
		return err
	}
	logging.SecurityEvent(ctx, "password_changed_self", "user_id", userID)
	return nil
}

// SetPIN fija/actualiza el PIN de un usuario (admin sobre cualquiera, o el propio empleado).
// PIN opcional en el sistema, pero si se establece debe pasar el filtro de PIN débil.
func (s *UsersService) SetPIN(ctx context.Context, userID int64, pin string) error {
	// El largo exigido depende del modo del negocio: con solo-PIN el PIN ES la identidad y necesita
	// seis dígitos; sin él, el nombre ya identifica y bastan cuatro.
	pinHash, lookup, err := s.prepararPin(ctx, pin)
	if err != nil {
		return err
	}

	err = s.store.QC(ctx).SetUserPin(ctx, db.SetUserPinParams{
		ID: userID, PinHash: pinHash, PinLookup: lookup,
	})
	// El índice único de la base es quien decide: entre validar y escribir cabe otra transacción
	// poniendo el mismo PIN. El mensaje NO dice de quién es — si lo dijera, este formulario sería
	// un oráculo para averiguar el PIN de un compañero probando números.
	if isUniqueViolation(err) {
		return domain.ErrPinRepetido
	}
	return err
}

// prepararPin valida un PIN y devuelve lo que hay que guardar: el hash con el que se verifica y la
// huella con la que se compara.
//
// Es la ÚNICA ruta por la que un PIN entra al sistema. Tenerla partida entre el alta y el cambio ya
// costó caro: la validación vivía en una y no en la otra, y el alta acabó aceptando PINs triviales
// y sin huella —lo que dejaba dar de alta a alguien con el PIN de otro para recibir su sesión.
func (s *UsersService) prepararPin(ctx context.Context, pin string) (*string, *string, error) {
	// El largo exigido depende del modo del negocio: con solo-PIN el PIN ES la identidad y necesita
	// seis dígitos; sin él, el nombre ya identifica y bastan cuatro.
	soloPin, pepper, err := s.politicaDePin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := domain.ValidarPin(pin, soloPin); err != nil {
		return nil, nil, err
	}
	pinHash, err := hashPIN(pin)
	if err != nil {
		return nil, nil, err
	}
	// La huella solo se guarda si hay secreto. Sin él no se puede comparar por igualdad, y un HMAC
	// con clave vacía sería una huella invertible por cualquiera.
	var lookup *string
	if pepper != "" {
		l := domain.PinLookup(pin, pepper)
		lookup = &l
	}
	return pinHash, lookup, nil
}

// politicaDePin: si el negocio usa solo-PIN, y con qué secreto se calcula la huella.
//
// Un error de lectura se PROPAGA en vez de caer a "no es solo-PIN". Caía al modo permisivo, así que
// un hipo de la consulta —un timeout de sentencia, una conexión que se cae— hacía que un negocio de
// seis dígitos aceptara un PIN de cuatro y le calculara su huella, que en ese modo es directamente
// desbloqueable. La empresa sin fila de ajustes es otra cosa: ahí el default del negocio es la
// respuesta correcta, no un fallo.
func (s *UsersService) politicaDePin(ctx context.Context) (bool, string, error) {
	ajustes, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultIdentity().PinOnlyUnlock, s.pinPepper, nil
	}
	if err != nil {
		return false, "", err
	}
	return ajustes.PinOnlyUnlock, s.pinPepper, nil
}
