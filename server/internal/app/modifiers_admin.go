package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// isUniqueViolation detecta el 23505 de Postgres (p. ej. nombre de grupo duplicado)
// para devolver un 409 amable en vez de un 500 opaco.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- Grupos (catálogo global reutilizable) ---------------------------------

type GroupView struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	IsActive      bool   `json:"isActive"`
	DefaultMin    int    `json:"defaultMin"`
	DefaultMax    int    `json:"defaultMax"`
	OptionCount   int    `json:"optionCount"`
	ProductCount  int    `json:"productCount"`
	OverrideCount int    `json:"overrideCount"` // productos que sobrescriben el default
}

// validMinMax: min ≥ 0, max ≥ 1, min ≤ max.
func validMinMax(min, max int) bool { return min >= 0 && max >= 1 && min <= max }

type GroupsPage struct {
	Items  []GroupView   `json:"items"`
	Total  int           `json:"total"`
	Counts ProductCounts `json:"counts"`
}

func (s *AdminService) ListGroups(ctx context.Context, status, search, sort, dir string, limit, offset int32) (GroupsPage, error) {
	rows, err := s.store.Q.AdminListGroups(ctx, db.AdminListGroupsParams{
		Status: status, Search: search, Sort: sort, Dir: dir, Lim: limit, Off: offset,
	})
	if err != nil {
		return GroupsPage{}, err
	}
	out := make([]GroupView, 0, len(rows))
	total := 0
	for _, r := range rows {
		total = int(r.Total)
		out = append(out, GroupView{
			ID: r.ID, Name: r.Name, IsActive: r.IsActive,
			DefaultMin: int(r.DefaultMinSelect), DefaultMax: int(r.DefaultMaxSelect),
			OptionCount: int(r.OptionCount), ProductCount: int(r.ProductCount), OverrideCount: int(r.OverrideCount),
		})
	}
	c, err := s.store.Q.AdminGroupCounts(ctx)
	if err != nil {
		return GroupsPage{}, err
	}
	return GroupsPage{Items: out, Total: total, Counts: ProductCounts{Act: int(c.Active), Inact: int(c.Inactive)}}, nil
}

func (s *AdminService) CreateGroup(ctx context.Context, name string, defMin, defMax int) (int64, error) {
	if name == "" || !validMinMax(defMin, defMax) {
		return 0, domain.ErrValidation
	}
	id, err := s.store.Q.AdminCreateGroup(ctx, db.AdminCreateGroupParams{
		Name: name, DefaultMinSelect: int16(defMin), DefaultMaxSelect: int16(defMax),
	})
	if isUniqueViolation(err) {
		return 0, domain.ErrConflict
	}
	return id, err
}

func (s *AdminService) UpdateGroup(ctx context.Context, id int64, name string, active bool, defMin, defMax int) error {
	if name == "" || !validMinMax(defMin, defMax) {
		return domain.ErrValidation
	}
	err := s.store.Q.AdminUpdateGroup(ctx, db.AdminUpdateGroupParams{
		ID: id, Name: name, IsActive: active, DefaultMinSelect: int16(defMin), DefaultMaxSelect: int16(defMax),
	})
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}

// --- Opciones de un grupo --------------------------------------------------

type GroupOptionView struct {
	ID          int64           `json:"id"`
	GroupID     int64           `json:"groupId"`
	Name        string          `json:"name"`
	PriceDelta  decimal.Decimal `json:"priceDelta"`
	MaxPerLine  int             `json:"maxPerLine"`
	CurrentCost decimal.Decimal `json:"currentCost"`
	Favorite    bool            `json:"favorite"`
	Active      bool            `json:"active"`
}

func (s *AdminService) GroupOptions(ctx context.Context, groupID int64) ([]GroupOptionView, error) {
	rows, err := s.store.Q.AdminGroupOptions(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupOptionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupOptionView{
			ID: r.ID, GroupID: r.GroupID, Name: r.Name, PriceDelta: r.PriceDelta,
			MaxPerLine: int(r.MaxPerLine), CurrentCost: r.CurrentCost, Favorite: r.IsFavorite, Active: r.IsActive,
		})
	}
	return out, nil
}

func (s *AdminService) CreateOption(ctx context.Context, groupID int64, name string, priceDelta decimal.Decimal, maxPerLine int) (int64, error) {
	if name == "" || maxPerLine < 1 {
		return 0, domain.ErrValidation
	}
	id, err := s.store.Q.AdminCreateOption(ctx, db.AdminCreateOptionParams{
		GroupID: groupID, Name: name, PriceDelta: domain.Round2(priceDelta), MaxPerLine: int16(maxPerLine),
	})
	if isUniqueViolation(err) {
		return 0, domain.ErrConflict
	}
	return id, err
}

func (s *AdminService) UpdateOptionFields(ctx context.Context, id int64, name string, priceDelta decimal.Decimal, maxPerLine int) error {
	if name == "" || maxPerLine < 1 {
		return domain.ErrValidation
	}
	err := s.store.Q.AdminUpdateOptionFields(ctx, db.AdminUpdateOptionFieldsParams{
		ID: id, Name: name, PriceDelta: domain.Round2(priceDelta), MaxPerLine: int16(maxPerLine),
	})
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}

// ReorderOptions fija el orden (sort_key) de las opciones de un grupo según la lista de ids.
// Se refleja en el catálogo y en el POS (bloque "Todas" del ModifierSheet).
func (s *AdminService) ReorderOptions(ctx context.Context, groupID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return s.store.Q.AdminReorderOptions(ctx, db.AdminReorderOptionsParams{GroupID: groupID, Ids: ids})
}

// --- Producto ↔ grupos (aquí vive min/max/obligatorio, por producto) -------

type ProductGroupView struct {
	GroupID     int64  `json:"groupId"`
	GroupName   string `json:"groupName"`
	GroupActive bool   `json:"groupActive"`
	Title       string `json:"title"`
	MinSelect   int    `json:"minSelect"`  // efectivo (override o default del grupo)
	MaxSelect   int    `json:"maxSelect"`  // efectivo
	Overridden  bool   `json:"overridden"` // true = el producto sobrescribe el default del grupo
	DefaultMin  int    `json:"defaultMin"` // default del grupo (para mostrar y restablecer)
	DefaultMax  int    `json:"defaultMax"`
	Position    int    `json:"position"`
	OptionCount int    `json:"optionCount"`
}

func (s *AdminService) ProductGroups(ctx context.Context, productID int64) ([]ProductGroupView, error) {
	rows, err := s.store.Q.AdminProductGroups(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := make([]ProductGroupView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProductGroupView{
			GroupID: r.GroupID, GroupName: r.GroupName, GroupActive: r.GroupActive, Title: r.Title,
			MinSelect: int(r.MinSelect), MaxSelect: int(r.MaxSelect), Overridden: r.Overridden,
			DefaultMin: int(r.DefaultMinSelect), DefaultMax: int(r.DefaultMaxSelect),
			Position: int(r.Position), OptionCount: int(r.OptionCount),
		})
	}
	return out, nil
}

// GroupProductView: producto que usa un grupo; overridden = sobrescribe el default del grupo.
type GroupProductView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	Overridden bool   `json:"overridden"`
	MinSelect  int    `json:"minSelect"`
	MaxSelect  int    `json:"maxSelect"`
}

func (s *AdminService) GroupProducts(ctx context.Context, groupID int64) ([]GroupProductView, error) {
	rows, err := s.store.Q.AdminGroupProducts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupProductView, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupProductView{
			ID: r.ID, Name: r.Name, Required: r.MinSelect > 0, Overridden: r.Overridden,
			MinSelect: int(r.MinSelect), MaxSelect: int(r.MaxSelect),
		})
	}
	return out, nil
}

type AttachGroupInput struct {
	ProductID int64
	GroupID   int64
	Title     string
	Override  bool // false = hereda el default del grupo (min/max NULL)
	MinSelect int
	MaxSelect int
	Position  int
}

func (s *AdminService) AttachGroup(ctx context.Context, in AttachGroupInput) error {
	var minP, maxP *int16
	if in.Override {
		if !validMinMax(in.MinSelect, in.MaxSelect) {
			return domain.ErrValidation
		}
		mn, mx := int16(in.MinSelect), int16(in.MaxSelect)
		minP, maxP = &mn, &mx
	}
	return s.store.Q.AdminAttachGroup(ctx, db.AdminAttachGroupParams{
		ProductID: in.ProductID, GroupID: in.GroupID, Title: in.Title,
		MinSelect: minP, MaxSelect: maxP, Position: int32(in.Position),
	})
}

func (s *AdminService) DetachGroup(ctx context.Context, productID, groupID int64) error {
	return s.store.Q.AdminDetachGroup(ctx, db.AdminDetachGroupParams{ProductID: productID, GroupID: groupID})
}
