package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// prefDefaultTerminal es la preferencia de usuario con su terminal por omisión (punto 8). Vive en
// user_preferences: si se archiva o es de otra sucursal, se ignora al cobrar.
const prefDefaultTerminal = "card_terminal"

// TerminalsService administra las terminales de tarjeta por sucursal (spec 032, puntos 8 y 9).
type TerminalsService struct{ store *store.Store }

// NewTerminalsService construye el servicio.
func NewTerminalsService(s *store.Store) *TerminalsService { return &TerminalsService{store: s} }

// TerminalView es una terminal en Configuración y al cobrar.
type TerminalView struct {
	ID         int64  `json:"id"`
	BranchID   int64  `json:"branchId"`
	BranchName string `json:"branchName"`
	Name       string `json:"name"`
	Archived   bool   `json:"archived"`
}

func terminalName(s string) (string, error) {
	n := strings.Join(strings.Fields(s), " ")
	if n == "" || len([]rune(n)) > 40 {
		return "", fmt.Errorf("%w: escribe el nombre de la terminal (hasta 40 letras)", domain.ErrValidation)
	}
	return n, nil
}

// List devuelve las terminales de la empresa, activas primero.
func (s *TerminalsService) List(ctx context.Context) ([]TerminalView, error) {
	rows, err := s.store.QC(ctx).ListCardTerminals(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TerminalView, 0, len(rows))
	for _, r := range rows {
		out = append(out, TerminalView{ID: r.ID, BranchID: r.BranchID, BranchName: r.BranchName, Name: r.Name, Archived: r.Archived})
	}
	return out, nil
}

// Create agrega una terminal a una sucursal.
func (s *TerminalsService) Create(ctx context.Context, branchID int64, name string) (TerminalView, error) {
	n, err := terminalName(name)
	if err != nil {
		return TerminalView{}, err
	}
	t, err := s.store.QC(ctx).InsertCardTerminal(ctx, db.InsertCardTerminalParams{BranchID: branchID, Name: n})
	switch {
	case isUniqueViolation(err):
		return TerminalView{}, fmt.Errorf("%w: ya hay una terminal con ese nombre en la sucursal", domain.ErrConflict)
	case isFKViolation(err):
		return TerminalView{}, fmt.Errorf("%w: esa sucursal no existe", domain.ErrValidation)
	case err != nil:
		return TerminalView{}, err
	}
	return TerminalView{ID: t.ID, BranchID: t.BranchID, Name: t.Name}, nil
}

// Rename cambia el nombre; los cobros pasados conservan el que tenían.
func (s *TerminalsService) Rename(ctx context.Context, id int64, name string) (TerminalView, error) {
	n, err := terminalName(name)
	if err != nil {
		return TerminalView{}, err
	}
	t, err := s.store.QC(ctx).RenameCardTerminal(ctx, db.RenameCardTerminalParams{ID: id, Name: n})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return TerminalView{}, domain.ErrNotFound
	case isUniqueViolation(err):
		return TerminalView{}, fmt.Errorf("%w: ya hay una terminal con ese nombre en la sucursal", domain.ErrConflict)
	case err != nil:
		return TerminalView{}, err
	}
	return TerminalView{ID: t.ID, BranchID: t.BranchID, Name: t.Name}, nil
}

// Archive deja de ofrecer la terminal; sus cobros la conservan.
func (s *TerminalsService) Archive(ctx context.Context, id int64) error {
	n, err := s.store.QC(ctx).ArchiveCardTerminal(ctx, id)
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

// SetUserDefault guarda (o borra, con nil) la terminal por omisión de un usuario.
func (s *TerminalsService) SetUserDefault(ctx context.Context, userID int64, terminalID *int64) error {
	v, err := json.Marshal(terminalID)
	if err != nil {
		return err
	}
	return s.store.QC(ctx).SetUserPreference(ctx, db.SetUserPreferenceParams{UserID: userID, Key: prefDefaultTerminal, Value: v})
}

// UserDefault lee la terminal por omisión de un usuario, o nil.
func userDefaultTerminal(ctx context.Context, q *db.Queries, userID int64) (*int64, error) {
	raw, err := q.GetUserPreference(ctx, db.GetUserPreferenceParams{UserID: userID, Key: prefDefaultTerminal})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var id *int64
	if json.Unmarshal(raw, &id) != nil {
		return nil, nil
	}
	return id, nil
}

// UserDefault devuelve la terminal por omisión del usuario.
func (s *TerminalsService) UserDefault(ctx context.Context, userID int64) (*int64, error) {
	return userDefaultTerminal(ctx, s.store.QC(ctx), userID)
}

// SetCardCountMode fija cómo se arquea la tarjeta en una sucursal (punto 9). Aplica desde el
// siguiente turno que se abra.
func (s *TerminalsService) SetCardCountMode(ctx context.Context, branchID int64, mode string) error {
	m, err := domain.ParseCardCountMode(mode)
	if err != nil {
		return err
	}
	n, err := s.store.QC(ctx).SetBranchCardCountMode(ctx, db.SetBranchCardCountModeParams{ID: branchID, CardCountMode: string(m)})
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

// BranchModeView es el modo de arqueo de una sucursal.
type BranchModeView struct {
	BranchID int64  `json:"branchId"`
	Name     string `json:"name"`
	Mode     string `json:"mode"`
}

// CardCountModes lista el modo de arqueo de cada sucursal activa.
func (s *TerminalsService) CardCountModes(ctx context.Context) ([]BranchModeView, error) {
	rows, err := s.store.QC(ctx).ListBranchCardCountModes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BranchModeView, 0, len(rows))
	for _, r := range rows {
		out = append(out, BranchModeView{BranchID: r.ID, Name: r.Name, Mode: r.CardCountMode})
	}
	return out, nil
}

// resolveTerminal decide la terminal de un cobro con tarjeta: la elegida, si es activa y de la
// sucursal de la caja; si no se eligió, la del usuario con la misma condición; si no, la única
// activa de la sucursal. Con varias y ninguna válida, se pide (punto 8).
func resolveTerminal(ctx context.Context, q *db.Queries, registerID, userID int64, chosen *int64) (db.ActiveTerminalsOfRegisterBranchRow, error) {
	activas, err := q.ActiveTerminalsOfRegisterBranch(ctx, registerID)
	if err != nil {
		return db.ActiveTerminalsOfRegisterBranchRow{}, err
	}
	find := func(id int64) (db.ActiveTerminalsOfRegisterBranchRow, bool) {
		for _, t := range activas {
			if t.ID == id {
				return t, true
			}
		}
		return db.ActiveTerminalsOfRegisterBranchRow{}, false
	}
	if chosen != nil {
		if t, ok := find(*chosen); ok {
			return t, nil
		}
		return db.ActiveTerminalsOfRegisterBranchRow{}, fmt.Errorf("%w: esa terminal no está disponible en esta sucursal", domain.ErrValidation)
	}
	def, err := userDefaultTerminal(ctx, q, userID)
	if err != nil {
		return db.ActiveTerminalsOfRegisterBranchRow{}, err
	}
	if def != nil {
		if t, ok := find(*def); ok {
			return t, nil
		}
	}
	if len(activas) == 1 {
		return activas[0], nil
	}
	return db.ActiveTerminalsOfRegisterBranchRow{}, domain.ErrCardTerminalRequired
}
