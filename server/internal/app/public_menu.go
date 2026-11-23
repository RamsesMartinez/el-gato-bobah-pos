package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// PublicMenuService es el menú público de cada negocio (spec 097): lo que cualquiera ve en
// menu.elgatobobah.com/<nombre>, y su nombre, enlace, QR y color en Ajustes.
type PublicMenuService struct {
	store   *store.Store
	baseURL string
}

// NewPublicMenuService arma el servicio; baseURL es MENU_BASE_URL, sin / final.
func NewPublicMenuService(s *store.Store, baseURL string) *PublicMenuService {
	return &PublicMenuService{store: s, baseURL: baseURL}
}

// PublicMenu es TODO lo que sale por la puerta pública (FR-019). Un campo nuevo aquí es un dato más
// que cualquiera puede leer sin sesión: TestThePublicMenuExposesOnlyThePublicFields lo detiene.
type PublicMenu struct {
	Kind       string           `json:"kind"`
	Name       string           `json:"name"`
	QRPath     string           `json:"qrPath"`
	Business   PublicBusiness   `json:"business"`
	Brand      PublicBrand      `json:"brand"`
	Template   string           `json:"template"`
	Categories []PublicCategory `json:"categories"`
}

// PublicBusiness es el encabezado del menú.
type PublicBusiness struct {
	Name string `json:"name"`
}

// PublicBrand es el estilo de marca, separado de los datos para que otra plantilla (#330) lo use igual.
type PublicBrand struct {
	Palette string `json:"palette"`
}

// PublicCategory es una categoría principal: la barra, la columna y el mosaico.
type PublicCategory struct {
	ID            int64               `json:"id"`
	Name          string              `json:"name"`
	Family        *string             `json:"family"`
	ImageURL      *string             `json:"imageUrl"`
	Products      []PublicProduct     `json:"products"`
	Subcategories []PublicSubcategory `json:"subcategories"`
}

// PublicSubcategory se pinta como subtítulo dentro de su principal.
type PublicSubcategory struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Family   *string         `json:"family"`
	Products []PublicProduct `json:"products"`
}

// PublicProduct: nombre, descripción, precio efectivo, foto y si está agotado. Nunca una cantidad.
type PublicProduct struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Price       decimal.Decimal `json:"price"`
	ImageURL    *string         `json:"imageUrl"`
	SoldOut     bool            `json:"soldOut"`
}

// PublicMoved dice a qué nombre vigente lleva un nombre anterior o un código de QR.
type PublicMoved struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

var qrTokenShape = regexp.MustCompile(`^[0-9a-f]{12}$`)

// Menu resuelve un nombre SIN sesión: el vigente devuelve el menú; uno anterior, a dónde llevar. Un
// nombre que no existe, con forma imposible o de una empresa inactiva es el mismo ErrNotFound.
func (s *PublicMenuService) Menu(ctx context.Context, name string) (*PublicMenu, *PublicMoved, error) {
	if !domain.ValidMenuNameShape(name) {
		return nil, nil, domain.ErrNotFound
	}
	// La vista de la 0121 resuelve con el rol de la aplicación sin empresa: el pool, no la conexión
	// de una sesión (aquí no la hay, y si la hubiera sería de otra empresa).
	route, err := s.store.Q.ResolvePublicMenuName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !route.CompanyActive) {
		return nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if !route.IsCurrent {
		return nil, &PublicMoved{Kind: "moved", Name: route.CurrentName}, nil
	}
	var out *PublicMenu
	err = s.store.WithTenant(ctx, route.CompanyID, func(q *db.Queries) error {
		m, err := s.build(ctx, q, route.CompanyID)
		out = m
		return err
	})
	return out, nil, err
}

// ByToken resuelve el código del QR al nombre vigente.
func (s *PublicMenuService) ByToken(ctx context.Context, token string) (*PublicMoved, error) {
	if !qrTokenShape.MatchString(token) {
		return nil, domain.ErrNotFound
	}
	r, err := s.store.Q.ResolvePublicMenuToken(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !r.CompanyActive) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &PublicMoved{Kind: "moved", Name: r.CurrentName}, nil
}

// build arma el menú con la empresa ya fijada: la oferta web (la regla única de #326), agrupada en
// dos niveles como el catálogo.
func (s *PublicMenuService) build(ctx context.Context, q *db.Queries, company int64) (*PublicMenu, error) {
	menu, err := q.GetPublicMenu(ctx, company)
	if err != nil {
		return nil, err
	}
	business, err := q.CompanyDisplayName(ctx, company)
	if err != nil {
		return nil, err
	}
	items, err := offer(ctx, q)
	if err != nil {
		return nil, err
	}
	cats, err := q.PublicMenuCategories(ctx)
	if err != nil {
		return nil, err
	}
	return &PublicMenu{
		Kind: "menu", Name: menu.Name, QRPath: "/q/" + menu.QrToken,
		Business: PublicBusiness{Name: business}, Brand: PublicBrand{Palette: menu.BrandPalette},
		Template: menu.TemplateKey, Categories: groupPublicMenu(cats, items),
	}, nil
}

// groupPublicMenu reparte la oferta en principales y subcategorías, en el orden del catálogo. Una
// hija cuya principal está apagada se vuelve principal; las vacías no salen. Los arreglos nunca son
// nil: la plantilla los recorre sin preguntar.
func groupPublicMenu(cats []db.PublicMenuCategoriesRow, items []WebOfferItem) []PublicCategory {
	byID := make(map[int64]db.PublicMenuCategoriesRow, len(cats))
	for _, c := range cats {
		byID[c.ID] = c
	}
	family := func(c db.PublicMenuCategoriesRow) *string {
		if c.FoodFamily != "" {
			f := c.FoodFamily
			return &f
		}
		return nil
	}
	prods := map[int64][]PublicProduct{}
	for _, it := range items {
		prods[it.CategoryID] = append(prods[it.CategoryID], PublicProduct{
			ID: it.ProductID, Name: it.Name, Description: it.Description, Price: domain.Round2(it.Price),
			ImageURL: it.ImageURL, SoldOut: it.SoldOut,
		})
	}
	isTop := func(c db.PublicMenuCategoriesRow) bool {
		if c.ParentID == nil {
			return true
		}
		_, parentActive := byID[*c.ParentID]
		return !parentActive
	}
	out := []PublicCategory{}
	for _, top := range cats { // ya vienen en el orden del catálogo
		if !isTop(top) {
			continue
		}
		pc := PublicCategory{ID: top.ID, Name: top.Name, Family: family(top), ImageURL: top.ImageUrl,
			Products: nonNil(prods[top.ID]), Subcategories: []PublicSubcategory{}}
		for _, sub := range cats {
			if sub.ParentID == nil || *sub.ParentID != top.ID || len(prods[sub.ID]) == 0 {
				continue
			}
			f := family(sub)
			if f == nil {
				f = pc.Family
			}
			pc.Subcategories = append(pc.Subcategories, PublicSubcategory{ID: sub.ID, Name: sub.Name, Family: f, Products: prods[sub.ID]})
		}
		if len(pc.Products) > 0 || len(pc.Subcategories) > 0 {
			out = append(out, pc)
		}
	}
	return out
}

func nonNil(p []PublicProduct) []PublicProduct {
	if p == nil {
		return []PublicProduct{}
	}
	return p
}

// PublicMenuSettings es la sección «Venta en web» de Ajustes.
type PublicMenuSettings struct {
	// BusinessName va en la tarjeta para imprimir.
	BusinessName  string   `json:"businessName"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	QRURL         string   `json:"qrUrl"`
	Palette       string   `json:"palette"`
	PreviousNames []string `json:"previousNames"`
}

// PublicMenuUpdate cambia el nombre, el color o los dos; ausente = sin cambio.
type PublicMenuUpdate struct {
	Name    *string `json:"name"`
	Palette *string `json:"palette"`
}

// Settings lee los ajustes del menú de la empresa de la sesión, creándolo si falta (una empresa que
// la versión anterior creó durante el traslape nació sin él).
func (s *PublicMenuService) Settings(ctx context.Context) (PublicMenuSettings, error) {
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return PublicMenuSettings{}, domain.ErrUnauthorized
	}
	var out PublicMenuSettings
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := s.ensure(ctx, q, company); err != nil {
			return err
		}
		var err error
		out, err = s.settings(ctx, q, company)
		return err
	})
	return out, err
}

// Update cambia nombre y color. Toma primero el menú de la empresa (for update): dos cambios a la vez
// se forman y el segundo planea sobre lo que dejó el primero, en vez de chocar con el índice de «un
// vigente» y responder un «no disponible» falso.
func (s *PublicMenuService) Update(ctx context.Context, u PublicMenuUpdate) (PublicMenuSettings, error) {
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return PublicMenuSettings{}, domain.ErrUnauthorized
	}
	if u.Palette != nil && !domain.ValidPalette(*u.Palette) {
		return PublicMenuSettings{}, fmt.Errorf("%w: color desconocido", domain.ErrValidation)
	}
	var out PublicMenuSettings
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := s.ensure(ctx, q, company); err != nil {
			return err
		}
		locked, err := q.LockPublicMenu(ctx, company)
		if err != nil {
			return err
		}
		if u.Name != nil {
			if err := s.rename(ctx, q, company, locked.ID, domain.SlugifyMenuName(*u.Name)); err != nil {
				return err
			}
		}
		if u.Palette != nil {
			if err := q.SetMenuPalette(ctx, db.SetMenuPaletteParams{Palette: *u.Palette, CompanyID: company}); err != nil {
				return err
			}
		}
		out, err = s.settings(ctx, q, company)
		return err
	})
	return out, err
}

func (s *PublicMenuService) rename(ctx context.Context, q *db.Queries, company, menuID int64, requested string) error {
	names, err := q.PublicMenuNames(ctx, company)
	if err != nil {
		return err
	}
	in := domain.MenuRenameInput{Requested: requested}
	for _, n := range names {
		if !n.RetiredAt.Valid {
			in.Current = n.Name
		} else {
			in.Retired = append(in.Retired, n.Name)
		}
	}
	if in.TakenElsewhere, err = q.MenuNameTakenElsewhere(ctx, db.MenuNameTakenElsewhereParams{Name: requested, CompanyID: company}); err != nil {
		return err
	}
	action, err := domain.PlanMenuRename(in)
	if err != nil || action == domain.MenuRenameNone {
		return err
	}
	if _, err := q.RetireCurrentMenuName(ctx, company); err != nil {
		return err
	}
	if action == domain.MenuRenameRevive {
		n, err := q.ReviveMenuName(ctx, db.ReviveMenuNameParams{CompanyID: company, Name: requested})
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("revivir %q tocó %d filas", requested, n)
		}
		return nil
	}
	return q.InsertMenuName(ctx, db.InsertMenuNameParams{CompanyID: company, MenuID: menuID, Name: requested})
}

func (s *PublicMenuService) ensure(ctx context.Context, q *db.Queries, company int64) error {
	if _, err := q.GetPublicMenu(ctx, company); err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	display, err := q.CompanyDisplayName(ctx, company)
	if err != nil {
		return err
	}
	name, err := q.FreeMenuNameFor(ctx, db.FreeMenuNameForParams{Display: display, CompanyID: company})
	if err != nil {
		return err
	}
	return q.EnsurePublicMenu(ctx, db.EnsurePublicMenuParams{CompanyID: company, Name: name})
}

func (s *PublicMenuService) settings(ctx context.Context, q *db.Queries, company int64) (PublicMenuSettings, error) {
	m, err := q.GetPublicMenu(ctx, company)
	if err != nil {
		return PublicMenuSettings{}, err
	}
	names, err := q.PublicMenuNames(ctx, company)
	if err != nil {
		return PublicMenuSettings{}, err
	}
	business, err := q.CompanyDisplayName(ctx, company)
	if err != nil {
		return PublicMenuSettings{}, err
	}
	prev := []string{}
	for _, n := range names {
		if n.RetiredAt.Valid {
			prev = append(prev, n.Name)
		}
	}
	return PublicMenuSettings{
		BusinessName: business, Name: m.Name, URL: s.baseURL + "/" + m.Name, QRURL: s.baseURL + "/q/" + m.QrToken,
		Palette: m.BrandPalette, PreviousNames: prev,
	}, nil
}
