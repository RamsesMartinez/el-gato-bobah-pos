package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// GetPreference devuelve el valor JSON crudo de una preferencia del usuario, o nil si no existe.
func (s *UsersService) GetPreference(ctx context.Context, userID int64, key string) ([]byte, error) {
	v, err := s.store.Q.GetUserPreference(ctx, db.GetUserPreferenceParams{UserID: userID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

// SetPreference guarda (upsert) el valor JSON de una preferencia del usuario. value debe ser JSON válido.
func (s *UsersService) SetPreference(ctx context.Context, userID int64, key string, value []byte) error {
	return s.store.Q.SetUserPreference(ctx, db.SetUserPreferenceParams{UserID: userID, Key: key, Value: value})
}

type UsersService struct {
	store *store.Store
}

func NewUsersService(s *store.Store) *UsersService { return &UsersService{store: s} }

type CreateUserInput struct {
	Name     string
	Username *string
	Role     domain.Role
	PIN      string
	Password string
}

func (s *UsersService) Create(ctx context.Context, in CreateUserInput) (domain.User, error) {
	if in.Name == "" || !in.Role.Valid() {
		return domain.User{}, domain.ErrValidation
	}
	params := db.CreateUserParams{Name: in.Name, Username: in.Username, Role: string(in.Role)}
	if in.PIN != "" {
		h, err := auth.HashSecret(in.PIN)
		if err != nil {
			return domain.User{}, err
		}
		params.PinHash = &h
	}
	if in.Password != "" {
		h, err := auth.HashSecret(in.Password)
		if err != nil {
			return domain.User{}, err
		}
		params.PasswordHash = &h
	}
	u, err := s.store.Q.CreateUser(ctx, params)
	if err != nil {
		return domain.User{}, err
	}
	return toDomainUser(u), nil
}

func (s *UsersService) List(ctx context.Context) ([]domain.User, error) {
	rows, err := s.store.Q.ListActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, len(rows))
	for i, u := range rows {
		out[i] = toDomainUser(u)
	}
	return out, nil
}

func (s *UsersService) SetPIN(ctx context.Context, userID int64, pin string) error {
	if len(pin) < 4 {
		return domain.ErrValidation
	}
	h, err := auth.HashSecret(pin)
	if err != nil {
		return err
	}
	return s.store.Q.SetUserPin(ctx, db.SetUserPinParams{ID: userID, PinHash: &h})
}
