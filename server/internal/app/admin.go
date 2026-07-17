package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

type AdminService struct {
	store *store.Store
}

func NewAdminService(s *store.Store) *AdminService { return &AdminService{store: s} }

// AdminProductView expone las fechas de disponibilidad como "YYYY-MM-DD" o null
// (JSON limpio para el front) en vez del pgtype.Date crudo.
type AdminProductView struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Price          float64 `json:"price"`
	CurrentCost    float64 `json:"current_cost"`
	Type           string  `json:"type"`
	IsActive       bool    `json:"is_active"`
	IsFavorite     bool    `json:"is_favorite"`
	Category       string  `json:"category"`
	AvailableFrom  *string `json:"availableFrom"`
	AvailableUntil *string `json:"availableUntil"`
}

const dateFmt = "2006-01-02"

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(dateFmt)
	return &s
}

func parseDate(s *string) (pgtype.Date, error) {
	if s == nil || *s == "" {
		return pgtype.Date{Valid: false}, nil
	}
	t, err := time.Parse(dateFmt, *s)
	if err != nil {
		return pgtype.Date{}, domain.ErrValidation
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func (s *AdminService) ListProducts(ctx context.Context) ([]AdminProductView, error) {
	rows, err := s.store.Q.AdminListProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminProductView, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminProductView{
			ID: r.ID, Name: r.Name, Price: r.Price, CurrentCost: r.CurrentCost,
			Type: string(r.Type), IsActive: r.IsActive, IsFavorite: r.IsFavorite,
			Category: r.Category, AvailableFrom: dateStr(r.AvailableFrom), AvailableUntil: dateStr(r.AvailableUntil),
		})
	}
	return out, nil
}

type UpdateProductInput struct {
	ID             int64
	Name           string
	Price          float64
	Favorite       bool
	Active         bool
	AvailableFrom  *string
	AvailableUntil *string
}

func (s *AdminService) UpdateProduct(ctx context.Context, in UpdateProductInput) error {
	if in.Name == "" || in.Price < 0 {
		return domain.ErrValidation
	}
	from, err := parseDate(in.AvailableFrom)
	if err != nil {
		return err
	}
	until, err := parseDate(in.AvailableUntil)
	if err != nil {
		return err
	}
	return s.store.Q.AdminUpdateProduct(ctx, db.AdminUpdateProductParams{
		ID:             in.ID,
		Name:           in.Name,
		Price:          domain.Round2(in.Price),
		IsFavorite:     in.Favorite,
		IsActive:       in.Active,
		AvailableFrom:  from,
		AvailableUntil: until,
	})
}
