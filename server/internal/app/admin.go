package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

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
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Price          decimal.Decimal `json:"price"`
	CurrentCost    decimal.Decimal `json:"current_cost"`
	Type           string          `json:"type"`
	IsActive       bool            `json:"is_active"`
	IsFavorite     bool            `json:"is_favorite"`
	Category       string          `json:"category"`
	AvailableFrom  *string         `json:"availableFrom"`
	AvailableUntil *string         `json:"availableUntil"`
	GroupCount     int             `json:"groupCount"`    // grupos de modificadores activos ligados al producto
	OverrideCount  int             `json:"overrideCount"` // grupos con min/max personalizado en este producto
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

// ProductsPage: página del catálogo (items + total del filtro) más los conteos por
// estado para las pestañas. total sirve al paginador; counts es independiente de la búsqueda.
type ProductsPage struct {
	Items  []AdminProductView `json:"items"`
	Total  int                `json:"total"`
	Counts ProductCounts      `json:"counts"`
}

type ProductCounts struct {
	Act   int `json:"act"`
	Inact int `json:"inact"`
}

// CategoryView es una categoría para pickers/filtros del admin (id, nombre, categoría padre).
type CategoryView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parentId"`
}

// Categories lista las categorías activas (para el filtro y el alta de productos). Reutiliza la
// lectura del menú (MenuCategories); aquí solo interesan id/nombre/padre.
func (s *AdminService) Categories(ctx context.Context) ([]CategoryView, error) {
	rows, err := s.store.QC(ctx).MenuCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryView, len(rows))
	for i, r := range rows {
		out[i] = CategoryView{ID: r.ID, Name: r.Name, ParentID: r.ParentID}
	}
	return out, nil
}

// CreateProduct da de alta un producto mínimo (activo, tipo simple). categoryID debe existir
// (FK); el costo/receta/canales se configuran después.
func (s *AdminService) CreateProduct(ctx context.Context, name string, categoryID int64, price decimal.Decimal, favorite, trackStock bool) (int64, error) {
	if name == "" || categoryID == 0 || !domain.ValidMoney(domain.Round2(price), true) {
		return 0, domain.ErrValidation
	}
	return s.store.QC(ctx).AdminCreateProduct(ctx, db.AdminCreateProductParams{
		Name: name, CategoryID: categoryID, Price: domain.Round2(price), IsFavorite: favorite, TrackStock: trackStock,
	})
}

// ListProducts pagina el catálogo en el backend. status: ""=todos | "act" | "inact".
// categoryID=0 → todas; sort/dir ordenan por columna (ver AdminListProducts).
func (s *AdminService) ListProducts(ctx context.Context, status, search string, categoryID int64, groups, sort, dir string, limit, offset int32) (ProductsPage, error) {
	rows, err := s.store.QC(ctx).AdminListProducts(ctx, db.AdminListProductsParams{
		Status: status, Search: search, CategoryID: categoryID, Groups: groups, Sort: sort, Dir: dir, Lim: limit, Off: offset,
	})
	if err != nil {
		return ProductsPage{}, err
	}
	out := make([]AdminProductView, 0, len(rows))
	total := 0
	for _, r := range rows {
		total = int(r.Total) // igual en todas las filas (window count)
		out = append(out, AdminProductView{
			ID: r.ID, Name: r.Name, Price: r.Price, CurrentCost: r.CurrentCost,
			Type: string(r.Type), IsActive: r.IsActive, IsFavorite: r.IsFavorite,
			Category: r.Category, AvailableFrom: dateStr(r.AvailableFrom), AvailableUntil: dateStr(r.AvailableUntil),
			GroupCount: int(r.GroupCount), OverrideCount: int(r.OverrideCount),
		})
	}
	c, err := s.store.QC(ctx).AdminProductCounts(ctx)
	if err != nil {
		return ProductsPage{}, err
	}
	return ProductsPage{Items: out, Total: total, Counts: ProductCounts{Act: int(c.Active), Inact: int(c.Inactive)}}, nil
}

type UpdateProductInput struct {
	ID             int64
	Name           string
	Price          decimal.Decimal
	Favorite       bool
	Active         bool
	AvailableFrom  *string
	AvailableUntil *string
}

// AdminOptionView: opción de modificador con su grupo, para gestionar (favorito/activo) en el admin.
type AdminOptionView struct {
	ID         int64           `json:"id"`
	GroupID    int64           `json:"groupId"`
	GroupName  string          `json:"groupName"`
	Name       string          `json:"name"`
	PriceDelta decimal.Decimal `json:"priceDelta"`
	Favorite   bool            `json:"favorite"`
	Active     bool            `json:"active"`
}

// OptionsPage: página de opciones (items + total del filtro) más los conteos por estado
// para las pestañas. Misma forma que ProductsPage; reutiliza ProductCounts ({act, inact}).
type OptionsPage struct {
	Items  []AdminOptionView `json:"items"`
	Total  int               `json:"total"`
	Counts ProductCounts     `json:"counts"`
}

// ListModifierOptions pagina las opciones en el backend. status: ""=todas | "act" | "inact".
func (s *AdminService) ListModifierOptions(ctx context.Context, status, search string, limit, offset int32) (OptionsPage, error) {
	rows, err := s.store.QC(ctx).AdminListModifierOptions(ctx, db.AdminListModifierOptionsParams{
		Status: status, Search: search, Lim: limit, Off: offset,
	})
	if err != nil {
		return OptionsPage{}, err
	}
	out := make([]AdminOptionView, 0, len(rows))
	total := 0
	for _, r := range rows {
		total = int(r.Total) // igual en todas las filas (window count)
		out = append(out, AdminOptionView{
			ID: r.ID, GroupID: r.GroupID, GroupName: r.GroupName, Name: r.Name,
			PriceDelta: r.PriceDelta, Favorite: r.IsFavorite, Active: r.IsActive,
		})
	}
	c, err := s.store.QC(ctx).AdminModifierOptionCounts(ctx)
	if err != nil {
		return OptionsPage{}, err
	}
	return OptionsPage{Items: out, Total: total, Counts: ProductCounts{Act: int(c.Active), Inact: int(c.Inactive)}}, nil
}

func (s *AdminService) SetOptionFavorite(ctx context.Context, id int64, fav bool) error {
	return s.store.QC(ctx).AdminSetOptionFavorite(ctx, db.AdminSetOptionFavoriteParams{ID: id, IsFavorite: fav})
}

func (s *AdminService) SetOptionActive(ctx context.Context, id int64, active bool) error {
	return s.store.QC(ctx).AdminSetOptionActive(ctx, db.AdminSetOptionActiveParams{ID: id, IsActive: active})
}

func (s *AdminService) UpdateProduct(ctx context.Context, in UpdateProductInput) error {
	if in.Name == "" || in.Price.IsNegative() {
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
	return s.store.QC(ctx).AdminUpdateProduct(ctx, db.AdminUpdateProductParams{
		ID:             in.ID,
		Name:           in.Name,
		Price:          domain.Round2(in.Price),
		IsFavorite:     in.Favorite,
		IsActive:       in.Active,
		AvailableFrom:  from,
		AvailableUntil: until,
	})
}
