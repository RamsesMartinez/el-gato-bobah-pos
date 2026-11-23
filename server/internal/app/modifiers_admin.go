package app

import (
	"context"
	"errors"
	"fmt"

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

// isUniqueViolationOf distingue cuál restricción se repitió cuando una tabla tiene varias: el alta de
// un empleado decía «ese PIN ya lo usa otra persona» cuando lo repetido era el usuario.
func isUniqueViolationOf(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// isForeignKeyViolation: la llave compuesta por empresa frenó una referencia a una fila que esta
// sesión no puede ver.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// --- Grupos (catálogo global reutilizable) ---------------------------------

type GroupView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	IsActive    bool   `json:"isActive"`
	DefaultMin  int    `json:"defaultMin"`
	DefaultMax  int    `json:"defaultMax"`
	OptionCount int    `json:"optionCount"`
	// OptionPreview son las primeras opciones por su orden real, para que la tarjeta diga QUÉ
	// tiene el grupo y no solo cuántas: antes había que expandir cada uno para saber si era el
	// que se buscaba.
	OptionPreview string `json:"optionPreview"`
	ProductCount  int    `json:"productCount"`
	// ActiveProductCount: de esos, los activos; en ellos deja de ofrecerse al archivar el grupo.
	ActiveProductCount int `json:"activeProductCount"`
	OverrideCount      int `json:"overrideCount"` // productos que sobrescriben el default
}

// validMinMax: min ≥ 0, max ≥ 1, min ≤ max.
// Los nombres repetidos dicen qué se repite: un «conflicto» a secas no le dice nada a quien captura ni
// al agente (spec 090).
var (
	errGroupNameTaken  = fmt.Errorf("%w: ya hay un grupo con ese nombre", domain.ErrConflict)
	errOptionNameTaken = fmt.Errorf("%w: ya hay un extra con ese nombre en el grupo", domain.ErrConflict)
)

func validMinMax(min, max int) bool { return domain.ValidSelectRange(min, max) }

type GroupsPage struct {
	Items  []GroupView   `json:"items"`
	Total  int           `json:"total"`
	Counts ProductCounts `json:"counts"`
}

func (s *AdminService) ListGroups(ctx context.Context, status, search, sort, dir string, limit, offset int32) (GroupsPage, error) {
	rows, err := s.store.QC(ctx).AdminListGroups(ctx, db.AdminListGroupsParams{
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
			OptionCount: int(r.OptionCount), OptionPreview: r.OptionPreview,
			ProductCount: int(r.ProductCount), ActiveProductCount: int(r.ActiveProductCount), OverrideCount: int(r.OverrideCount),
		})
	}
	c, err := s.store.QC(ctx).AdminGroupCounts(ctx)
	if err != nil {
		return GroupsPage{}, err
	}
	return GroupsPage{Items: out, Total: total, Counts: ProductCounts{Act: int(c.Active), Inact: int(c.Inactive)}}, nil
}

func (s *AdminService) CreateGroup(ctx context.Context, name string, defMin, defMax int) (int64, error) {
	if name == "" || !validMinMax(defMin, defMax) {
		return 0, domain.ErrValidation
	}
	id, err := s.store.QC(ctx).AdminCreateGroup(ctx, db.AdminCreateGroupParams{
		Name: name, DefaultMinSelect: int16(defMin), DefaultMaxSelect: int16(defMax),
	})
	if isUniqueViolation(err) {
		return 0, errGroupNameTaken
	}
	return id, err
}

func (s *AdminService) UpdateGroup(ctx context.Context, id int64, name string, active bool, defMin, defMax int) error {
	if name == "" || !validMinMax(defMin, defMax) {
		return domain.ErrValidation
	}
	err := s.store.QC(ctx).AdminUpdateGroup(ctx, db.AdminUpdateGroupParams{
		ID: id, Name: name, IsActive: active, DefaultMinSelect: int16(defMin), DefaultMaxSelect: int16(defMax),
	})
	if isUniqueViolation(err) {
		return errGroupNameTaken
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
	// CompositionStatus: "" sin capturar, "estimated" o "confirmed" (spec 028).
	CompositionStatus string `json:"compositionStatus"`
}

func (s *AdminService) GroupOptions(ctx context.Context, groupID int64) ([]GroupOptionView, error) {
	rows, err := s.store.QC(ctx).AdminGroupOptions(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupOptionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupOptionView{
			ID: r.ID, GroupID: r.GroupID, Name: r.Name, PriceDelta: r.PriceDelta,
			MaxPerLine: int(r.MaxPerLine), CurrentCost: r.CurrentCost, Favorite: r.IsFavorite, Active: r.IsActive,
			CompositionStatus: textoDe(r.CompositionStatus),
		})
	}
	return out, nil
}

func (s *AdminService) CreateOption(ctx context.Context, groupID int64, name string, priceDelta decimal.Decimal, maxPerLine int) (int64, error) {
	if name == "" || !domain.ValidMaxPerLine(maxPerLine) || !domain.ValidPriceDelta(priceDelta) {
		return 0, domain.ErrValidation
	}
	id, err := s.store.QC(ctx).AdminCreateOption(ctx, db.AdminCreateOptionParams{
		GroupID: groupID, Name: name, PriceDelta: domain.Round2(priceDelta), MaxPerLine: int16(maxPerLine),
	})
	if isUniqueViolation(err) {
		return 0, errOptionNameTaken
	}
	return id, err
}

// OptionFieldsPatch es la edición de una opción: lo que viene cambia, lo que no viene se queda.
type OptionFieldsPatch struct {
	Name       *string
	PriceDelta *decimal.Decimal
	MaxPerLine *int
}

// UpdateOptionFields cambia solo lo que trae el parche (spec 090). Antes, mandar solo el nombre
// dejaba el precio en 0 y el tope en 1: el handler rellenaba lo ausente. Lee la opción con candado y
// escribe en la misma transacción, para no pisar una edición que llegó en medio.
func (s *AdminService) UpdateOptionFields(ctx context.Context, id int64, p OptionFieldsPatch) error {
	if p.Name == nil && p.PriceDelta == nil && p.MaxPerLine == nil {
		return nil
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		cur, err := q.AdminLockOption(ctx, id)
		if err != nil {
			return notFound(err)
		}
		name, price, maxPerLine := cur.Name, cur.PriceDelta, int(cur.MaxPerLine)
		if p.Name != nil {
			name = *p.Name
		}
		if p.PriceDelta != nil {
			price = *p.PriceDelta
		}
		if p.MaxPerLine != nil {
			maxPerLine = *p.MaxPerLine
		}
		if name == "" || !domain.ValidMaxPerLine(maxPerLine) || !domain.ValidPriceDelta(price) {
			return domain.ErrValidation
		}
		err = q.AdminUpdateOptionFields(ctx, db.AdminUpdateOptionFieldsParams{
			ID: id, Name: name, PriceDelta: domain.Round2(price), MaxPerLine: int16(maxPerLine),
		})
		if isUniqueViolation(err) {
			return errOptionNameTaken
		}
		return err
	})
}

// OptionView es una opción leída por id, con su grupo aunque esté inactivo (spec 090).
type OptionView struct {
	ID                int64           `json:"id"`
	GroupID           int64           `json:"groupId"`
	GroupName         string          `json:"groupName"`
	GroupActive       bool            `json:"groupActive"`
	Name              string          `json:"name"`
	PriceDelta        decimal.Decimal `json:"priceDelta"`
	MaxPerLine        int             `json:"maxPerLine"`
	Favorite          bool            `json:"favorite"`
	Active            bool            `json:"active"`
	CompositionStatus string          `json:"compositionStatus"`
}

// GetOption lee una opción por id. Bajo RLS, la de otra empresa no existe.
func (s *AdminService) GetOption(ctx context.Context, id int64) (OptionView, error) {
	r, err := s.store.QC(ctx).AdminGetOption(ctx, id)
	if err != nil {
		return OptionView{}, notFound(err)
	}
	return OptionView{ID: r.ID, GroupID: r.GroupID, GroupName: r.GroupName, GroupActive: r.GroupActive, Name: r.Name,
		PriceDelta: r.PriceDelta, MaxPerLine: int(r.MaxPerLine), Favorite: r.IsFavorite, Active: r.IsActive,
		CompositionStatus: textoDe(r.CompositionStatus)}, nil
}

// ReorderOptions fija el orden (sort_key) de las opciones de un grupo según la lista de ids.
// Se refleja en el catálogo y en el POS (bloque "Todas" del ModifierSheet).
func (s *AdminService) ReorderOptions(ctx context.Context, groupID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return s.store.QC(ctx).AdminReorderOptions(ctx, db.AdminReorderOptionsParams{GroupID: groupID, Ids: ids})
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
	rows, err := s.store.QC(ctx).AdminProductGroups(ctx, productID)
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
	Active     bool   `json:"active"`
	Required   bool   `json:"required"`
	Overridden bool   `json:"overridden"`
	MinSelect  int    `json:"minSelect"`
	MaxSelect  int    `json:"maxSelect"`
}

func (s *AdminService) GroupProducts(ctx context.Context, groupID int64) ([]GroupProductView, error) {
	rows, err := s.store.QC(ctx).AdminGroupProducts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupProductView, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupProductView{
			ID: r.ID, Name: r.Name, Active: r.IsActive, Required: r.MinSelect > 0, Overridden: r.Overridden,
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
	if !domain.ValidPosition(in.Position) {
		return domain.ErrValidation
	}
	if in.Override {
		if !validMinMax(in.MinSelect, in.MaxSelect) {
			return domain.ErrValidation
		}
		mn, mx := int16(in.MinSelect), int16(in.MaxSelect)
		minP, maxP = &mn, &mx
	}
	return s.store.QC(ctx).AdminAttachGroup(ctx, db.AdminAttachGroupParams{
		ProductID: in.ProductID, GroupID: in.GroupID, Title: in.Title,
		MinSelect: minP, MaxSelect: maxP, Position: int32(in.Position),
	})
}

func (s *AdminService) DetachGroup(ctx context.Context, productID, groupID int64) error {
	return s.store.QC(ctx).AdminDetachGroup(ctx, db.AdminDetachGroupParams{ProductID: productID, GroupID: groupID})
}
