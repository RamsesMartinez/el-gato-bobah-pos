package app

import (
	"context"
	"errors"

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
	Name          string
	Username      *string
	Role          domain.Role
	PIN           string  // opcional
	Password      string  // OBLIGATORIO y fuerte (política + HIBP)
	RecoveryEmail *string // opcional (email externo para recuperación)
}

// Create da de alta un empleado en la empresa del tenant (RLS auto-sella company_id). Password
// obligatorio y validado; PIN opcional. must_change_password=true: el admin fija una contraseña
// inicial y el empleado la cambia en su primer login.
func (s *UsersService) Create(ctx context.Context, in CreateUserInput) (domain.User, error) {
	if in.Name == "" || !in.Role.Valid() {
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
		Name: in.Name, Username: in.Username, Role: string(in.Role),
		PasswordHash: &pwHash, RecoveryEmail: in.RecoveryEmail, MustChangePassword: true,
	}
	if in.PIN != "" {
		pinHash, err := hashPIN(in.PIN)
		if err != nil {
			return domain.User{}, err
		}
		params.PinHash = pinHash
	}
	u, err := s.store.QC(ctx).CreateUser(ctx, params)
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(u), nil
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
	ID            int64
	Name          string
	Role          domain.Role
	IsActive      bool
	RecoveryEmail *string
}

// Update edita datos no-secretos de un empleado (nombre, rol, alta/baja, email de recuperación).
func (s *UsersService) Update(ctx context.Context, in UpdateUserInput) (domain.User, error) {
	if in.Name == "" || !in.Role.Valid() {
		return domain.User{}, domain.ErrValidation
	}
	u, err := s.store.QC(ctx).UpdateUser(ctx, db.UpdateUserParams{
		ID: in.ID, Name: in.Name, Role: string(in.Role), IsActive: in.IsActive, RecoveryEmail: in.RecoveryEmail,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
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
	soloPin, pepper := s.politicaDePin(ctx)
	if err := domain.ValidarPin(pin, soloPin); err != nil {
		return err
	}
	pinHash, err := hashPIN(pin)
	if err != nil {
		return err
	}

	// La huella determinista solo se guarda si hay secreto. Sin él no se puede comparar por
	// igualdad, y guardar un HMAC con clave vacía sería una huella invertible por cualquiera.
	var lookup *string
	if pepper != "" {
		l := domain.PinLookup(pin, pepper)
		lookup = &l
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

// politicaDePin: si el negocio usa solo-PIN, y con qué secreto se calcula la huella.
func (s *UsersService) politicaDePin(ctx context.Context) (bool, string) {
	ajustes, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	if err != nil {
		return false, s.pinPepper
	}
	return ajustes.PinOnlyUnlock, s.pinPepper
}
