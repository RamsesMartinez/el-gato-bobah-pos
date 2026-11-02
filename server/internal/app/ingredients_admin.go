package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Gestión de artículos del almacén (spec 042): la lista con buscador y filtros, la ficha, editar,
// archivar, el historial de costos, la bitácora y el cambio de unidad.

// ArticleFilter son los filtros de la lista. Se combinan con «y».
type ArticleFilter struct {
	Q        string
	Archived bool
	PlaceID  *int64
	NoPlace  bool
	NoCost   bool
	Negative bool
}

// PresentationLabel es un empaque como se pinta en la lista: «Garrafa · 4 L».
type PresentationLabel struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	QtyInBase decimal.Decimal `json:"qtyInBase"`
	Brand     *string         `json:"brand"`
}

// ArticleRow es un renglón de la lista de artículos.
type ArticleRow struct {
	ID            int64               `json:"id"`
	Name          string              `json:"name"`
	IsActive      bool                `json:"isActive"`
	BaseUnitID    int16               `json:"baseUnitId"`
	BaseUnitCode  string              `json:"baseUnitCode"`
	BaseUnitKind  string              `json:"baseUnitKind"`
	CurrentCost   decimal.Decimal     `json:"currentCost"`
	MinStock      *decimal.Decimal    `json:"minStock"`
	IsPrep        bool                `json:"isPrep"`
	CategoryID    *int64              `json:"categoryId"`
	Category      *string             `json:"category"`
	CountPlaceID  *int64              `json:"countPlaceId"`
	CountPlace    *string             `json:"countPlace"`
	OnHand        decimal.Decimal     `json:"onHand"`
	Presentations []PresentationLabel `json:"presentations"`
	// CompositionStatus: "estimated" es una receta que vino estimada (de FUDO) y está por revisar.
	CompositionStatus string `json:"compositionStatus"`
}

// ListArticles es la lista de Almacén › Artículos. Los empaques de cada artículo se juntan aparte, en
// una sola lectura, en vez de una consulta por renglón.
func (s *BackofficeService) ListArticles(ctx context.Context, f ArticleFilter) ([]ArticleRow, error) {
	q := s.store.QC(ctx)
	var needle *string
	if t := strings.TrimSpace(f.Q); t != "" {
		needle = &t
	}
	rows, err := q.ListArticles(ctx, db.ListArticlesParams{
		Archived: f.Archived, Q: needle, PlaceID: f.PlaceID, NoPlace: f.NoPlace, NoCost: f.NoCost, Negative: f.Negative,
	})
	if err != nil {
		return nil, err
	}
	pres, err := q.ListPresentations(ctx, nil)
	if err != nil {
		return nil, err
	}
	byArticle := map[int64][]PresentationLabel{}
	for _, p := range pres {
		if p.Active && !p.IsApproximate {
			byArticle[p.IngredientID] = append(byArticle[p.IngredientID], PresentationLabel{ID: p.ID, Name: p.Name, QtyInBase: p.QtyInBase, Brand: p.Brand})
		}
	}
	out := make([]ArticleRow, len(rows))
	for i, r := range rows {
		ps := byArticle[r.ID]
		if ps == nil {
			// Un arreglo prometido sale como [] y no como null (AGENTS §1).
			ps = []PresentationLabel{}
		}
		out[i] = ArticleRow{
			ID: r.ID, Name: r.Name, IsActive: r.IsActive, BaseUnitID: r.BaseUnitID, BaseUnitCode: r.BaseUnitCode,
			BaseUnitKind: string(r.BaseUnitKind), CurrentCost: r.CurrentCost, MinStock: r.MinStock, IsPrep: r.IsPrep,
			CategoryID: r.CategoryID, Category: r.Category, CountPlaceID: r.CountPlaceID, CountPlace: r.CountPlace,
			OnHand: r.OnHand, Presentations: ps, CompositionStatus: textoDe(r.CompositionStatus),
		}
	}
	return out, nil
}

// ArticleCreate es lo que se captura en «Nuevo artículo». Lo opcional va en nil.
type ArticleCreate struct {
	Name         string
	BaseUnitID   int16
	CategoryID   *int64
	PlaceID      *int64
	MinStock     *decimal.Decimal
	InitialCost  *decimal.Decimal
	InitialStock *decimal.Decimal
}

// NewArticle es un artículo que nace dentro de otra captura (un gasto o una receta): solo nombre y
// en qué se mide. Se crea en la misma transacción que guarda esa captura (FR-003).
type NewArticle struct {
	Name       string `json:"name"`
	BaseUnitID int16  `json:"baseUnitId"`
}

// CreateArticle da de alta un artículo desde la lista, sin gasto ni receta (FR-002).
func (s *BackofficeService) CreateArticle(ctx context.Context, in ArticleCreate) (int64, error) {
	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		id, err = createArticleTx(ctx, q, in)
		return err
	})
	return id, err
}

// createArticleTx es la única alta de artículos: la lista, el gasto y la receta pasan por aquí, con
// la misma regla de nombres repetidos, el mismo candado y el mismo evento de bitácora.
func createArticleTx(ctx context.Context, q *db.Queries, in ArticleCreate) (int64, error) {
	in.Name = domain.CleanArticleName(in.Name)
	// Se redondea ANTES de validar y de decidir si hay existencia o costo: 0.00001 pasaba la validación
	// y se volvía 0 al insertar, que la columna no admite (500).
	if in.InitialStock != nil {
		v := domain.Round4(*in.InitialStock)
		in.InitialStock = &v
	}
	if in.InitialCost != nil {
		v := in.InitialCost.Round(6)
		in.InitialCost = &v
	}
	if err := domain.ValidateArticle(domain.ArticleInput{Name: in.Name, MinStock: in.MinStock, Cost: in.InitialCost, InitialStock: in.InitialStock}); err != nil {
		return 0, err
	}
	u, err := q.GetUnit(ctx, in.BaseUnitID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%w: unidad desconocida", domain.ErrValidation)
		}
		return 0, err
	}
	// Se cuenta en la unidad base de su clase (g, ml, cm, pieza); lo que llega en kg o L es un empaque.
	if !u.ToBase.Equal(decimal.NewFromInt(1)) {
		return 0, fmt.Errorf("%w: el artículo se mide en g, ml, cm o piezas; lo que llega en kg o L se captura como empaque", domain.ErrValidation)
	}
	if err := categoryOfCompany(ctx, q, in.CategoryID); err != nil {
		return 0, err
	}
	if err := q.LockIngredientNames(ctx); err != nil {
		return 0, err
	}
	switch found, err := q.FindArticleByName(ctx, in.Name); {
	case err == nil:
		return 0, domain.ArticleNameTaken{ID: found.ID, Archived: !found.IsActive}
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, err
	}
	var minStock *decimal.Decimal
	if in.MinStock != nil {
		v := domain.Round4(*in.MinStock)
		minStock = &v
	}
	row, err := q.CreateIngredient(ctx, db.CreateIngredientParams{
		Name: in.Name, BaseUnitID: in.BaseUnitID, CategoryID: in.CategoryID, MinStock: minStock,
	})
	if err != nil {
		return 0, err
	}
	id := row.ID
	if in.PlaceID != nil {
		if err := activePlace(ctx, q, *in.PlaceID); err != nil {
			return 0, err
		}
		if _, err := q.SetCountPlace(ctx, db.SetCountPlaceParams{ID: id, PlaceID: in.PlaceID}); err != nil {
			return 0, err
		}
	}
	actor, hasActor := ActorUserFrom(ctx)
	if in.InitialCost != nil && in.InitialCost.IsPositive() {
		if !hasActor {
			return 0, fmt.Errorf("%w: corregir un costo necesita a la persona que lo hace", domain.ErrForbidden)
		}
		cost := in.InitialCost.Round(6)
		prev, err := q.SetArticleCostManual(ctx, db.SetArticleCostManualParams{ID: id, Cost: cost})
		if err != nil {
			return 0, err
		}
		if err := q.InsertCostCorrection(ctx, db.InsertCostCorrectionParams{IngredientID: id, PreviousCost: prev, NewCost: cost, UserID: actor}); err != nil {
			return 0, err
		}
	}
	if in.InitialStock != nil && in.InitialStock.IsPositive() {
		var by *int64
		if hasActor {
			by = &actor
		}
		if err := q.InsertInitialStock(ctx, db.InsertInitialStockParams{IngredientID: &id, Quantity: domain.Round4(*in.InitialStock), UserID: by}); err != nil {
			return 0, err
		}
	}
	return id, recordAudit(ctx, q, domain.AuditEvent{
		Action: domain.AuditIngredientCreated, EntityType: "ingredient", EntityID: id,
		After: map[string]any{"name": in.Name, "baseUnit": u.Code, "initialStock": in.InitialStock, "initialCost": in.InitialCost},
	})
}

// ensureArticleTx es el alta de un artículo que nace dentro de un gasto o una receta: si ya existe uno
// activo con ese nombre se usa ése, como hacía la pantalla al recibir el 409. Uno archivado no se
// reactiva en silencio: se responde el conflicto para que se decida en la lista.
func ensureArticleTx(ctx context.Context, q *db.Queries, na NewArticle) (int64, error) {
	id, err := createArticleTx(ctx, q, ArticleCreate{Name: na.Name, BaseUnitID: na.BaseUnitID})
	var taken domain.ArticleNameTaken
	if errors.As(err, &taken) && !taken.Archived {
		return taken.ID, nil
	}
	return id, err
}

// ArticleSheet es la ficha de un artículo. UpdatedAt es su versión: editar la devuelve.
type ArticleSheet struct {
	ArticleRow
	CostSource string    `json:"costSource"`
	UpdatedAt  time.Time `json:"updatedAt"`
	// MergedInto: a qué insumo se unió (spec 056); nulo si no es origen de una unión.
	MergedInto *MergedInto `json:"mergedInto"`
	// MergedSources: cuántos insumos se unieron en éste.
	MergedSources int `json:"mergedSources"`
}

// MergedInto es el destino de una unión, para la ficha del origen.
type MergedInto struct {
	ID   int64     `json:"id"`
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

// GetArticle es la ficha de un artículo, con sus empaques activos.
func (s *BackofficeService) GetArticle(ctx context.Context, id int64) (ArticleSheet, error) {
	q := s.store.QC(ctx)
	r, err := q.GetArticle(ctx, id)
	if err != nil {
		return ArticleSheet{}, notFound(err)
	}
	pres, err := q.ListPresentations(ctx, &id)
	if err != nil {
		return ArticleSheet{}, err
	}
	ps := []PresentationLabel{}
	for _, p := range pres {
		if p.Active && !p.IsApproximate {
			ps = append(ps, PresentationLabel{ID: p.ID, Name: p.Name, QtyInBase: p.QtyInBase, Brand: p.Brand})
		}
	}
	var into *MergedInto
	switch t, err := q.MergeTargetOf(ctx, id); {
	case err == nil:
		into = &MergedInto{ID: t.ID, Name: t.Name, At: t.CreatedAt}
	case !errors.Is(err, pgx.ErrNoRows):
		return ArticleSheet{}, err
	}
	sources, err := q.CountMergedSources(ctx, id)
	if err != nil {
		return ArticleSheet{}, err
	}
	return ArticleSheet{
		ArticleRow: ArticleRow{
			ID: r.ID, Name: r.Name, IsActive: r.IsActive, BaseUnitID: r.BaseUnitID, BaseUnitCode: r.BaseUnitCode,
			BaseUnitKind: string(r.BaseUnitKind), CurrentCost: r.CurrentCost, MinStock: r.MinStock, IsPrep: r.IsPrep,
			CategoryID: r.CategoryID, Category: r.Category, CountPlaceID: r.CountPlaceID, CountPlace: r.CountPlace,
			OnHand: r.OnHand, Presentations: ps, CompositionStatus: textoDe(r.CompositionStatus),
		},
		CostSource: r.CostSource, UpdatedAt: r.UpdatedAt, MergedInto: into, MergedSources: int(sources),
	}, nil
}

// ArticleUpdate son los campos de la ficha que cambian. Desde el autoguardado (spec 043) llega uno a
// la vez; lo que no viene se queda como estaba. Name nil es «no vino».
type ArticleUpdate struct {
	ExpectedUpdatedAt time.Time
	Name              *string
	CategoryID        Field[int64]
	PlaceID           Field[int64]
	MinStock          Field[decimal.Decimal]
}

// UpdateArticle guarda los campos que vienen si nadie cambió la ficha desde que se vio (FR-004), deja
// en la bitácora solo lo que cambió y devuelve la versión nueva.
func (s *BackofficeService) UpdateArticle(ctx context.Context, id int64, in ArticleUpdate) (time.Time, error) {
	var updatedAt time.Time
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		before, err := q.GetArticle(ctx, id)
		if err != nil {
			return notFound(err)
		}
		in := mergeArticleUpdate(in, before)
		// El nombre se valida y se busca repetido solo si cambió: cada autoguardado manda la ficha
		// completa, y un artículo heredado con un nombre que hoy no pasaría (repetido por acentos, de la
		// carga de FUDO) no podría guardar ni su mínimo. Tampoco toma el candado de nombres de la empresa.
		if *in.Name != before.Name {
			if err := domain.ValidateArticle(domain.ArticleInput{Name: *in.Name, MinStock: in.MinStock.Value}); err != nil {
				return err
			}
			if err := q.LockIngredientNames(ctx); err != nil {
				return err
			}
			switch found, err := q.FindOtherArticleByName(ctx, db.FindOtherArticleByNameParams{ID: id, Name: *in.Name}); {
			case err == nil:
				return domain.ArticleNameTaken{ID: found.ID, Archived: !found.IsActive}
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		} else if m := in.MinStock.Value; m != nil && !domain.ValidStockQtyOrZero(*m) {
			return fmt.Errorf("%w: el mínimo no puede ser negativo", domain.ErrValidation)
		}
		if err := categoryOfCompany(ctx, q, in.CategoryID.Value); err != nil {
			return err
		}
		if p := in.PlaceID.Value; p != nil && (before.CountPlaceID == nil || *before.CountPlaceID != *p) {
			if err := activePlace(ctx, q, *p); err != nil {
				return err
			}
		}
		var minStock *decimal.Decimal
		if in.MinStock.Value != nil {
			v := domain.Round4(*in.MinStock.Value)
			minStock = &v
		}
		updatedAt, err = q.UpdateArticle(ctx, db.UpdateArticleParams{
			ID: id, ExpectedUpdatedAt: in.ExpectedUpdatedAt, Name: *in.Name,
			CategoryID: in.CategoryID.Value, PlaceID: in.PlaceID.Value, MinStock: minStock,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrArticleStale
		}
		if err != nil {
			return err
		}
		was, now := map[string]any{}, map[string]any{}
		if before.Name != *in.Name {
			was["name"], now["name"] = before.Name, *in.Name
		}
		if !samePtr(before.CategoryID, in.CategoryID.Value) {
			was["categoryId"], now["categoryId"] = before.CategoryID, in.CategoryID.Value
		}
		if !samePtr(before.CountPlaceID, in.PlaceID.Value) {
			was["placeId"], now["placeId"] = before.CountPlaceID, in.PlaceID.Value
		}
		if !sameDec(before.MinStock, minStock) {
			was["minStock"], now["minStock"] = before.MinStock, minStock
		}
		if len(now) == 0 {
			return nil
		}
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientUpdated, EntityType: "ingredient", EntityID: id, Before: was, After: now,
		})
	})
	return updatedAt, err
}

// mergeArticleUpdate llena lo que no vino con lo que la ficha ya tiene, y limpia el nombre que sí vino.
// Un nombre que solo difiere del guardado en espacios es el mismo: se conserva tal cual.
func mergeArticleUpdate(in ArticleUpdate, before db.GetArticleRow) ArticleUpdate {
	if in.Name == nil || domain.CleanArticleName(*in.Name) == domain.CleanArticleName(before.Name) {
		in.Name = &before.Name
	} else {
		name := domain.CleanArticleName(*in.Name)
		in.Name = &name
	}
	if !in.CategoryID.Set {
		in.CategoryID.Value = before.CategoryID
	}
	if !in.PlaceID.Set {
		in.PlaceID.Value = before.CountPlaceID
	}
	if !in.MinStock.Set {
		in.MinStock.Value = before.MinStock
	}
	return in
}

func samePtr(a, b *int64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func sameDec(a, b *decimal.Decimal) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

// CorrectArticleCost corrige a mano el costo de un artículo que se compra hecho (FR-005). Cuenta desde
// ahora y no recalcula lo vendido (order_lines.unit_cost es copia); queda en el historial con quién lo hizo.
func (s *BackofficeService) CorrectArticleCost(ctx context.Context, id int64, cost decimal.Decimal) error {
	if !domain.ValidMoney(cost, true) {
		return fmt.Errorf("%w: costo inválido", domain.ErrValidation)
	}
	actor, ok := ActorUserFrom(ctx)
	if !ok {
		return fmt.Errorf("%w: corregir un costo necesita a la persona que lo hace", domain.ErrForbidden)
	}
	cost = cost.Round(6)
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		// El candado del recálculo antes que la fila del insumo: al revés, una captura de receta que
		// ya lo tiene y cuya llave foránea pide el insumo se esperaban una a la otra.
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		a, err := q.GetArticle(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if a.IsPrep {
			return domain.ErrPrepCost
		}
		prev, err := q.SetArticleCostManual(ctx, db.SetArticleCostManualParams{ID: id, Cost: cost})
		if err != nil {
			return err
		}
		if err := q.InsertCostCorrection(ctx, db.InsertCostCorrectionParams{IngredientID: id, PreviousCost: prev, NewCost: cost, UserID: actor}); err != nil {
			return err
		}
		if err := recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientCostCorrected, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"cost": prev}, After: map[string]any{"cost": cost},
		}); err != nil {
			return err
		}
		// Los preparados, productos, paquetes y extras que lo llevan cambian de costo con él.
		return recomputeCosts(ctx, q)
	})
}

// historyPage es cuántos renglones trae cada página del historial y de la bitácora.
const historyPage = 20

// CostHistoryItem es un renglón del historial: una compra o una corrección.
type CostHistoryItem struct {
	Kind         string           `json:"kind"` // purchase | correction
	At           time.Time        `json:"at"`
	ID           int64            `json:"id"`
	UnitCost     decimal.Decimal  `json:"unitCost"`
	Presentation *string          `json:"presentation"`
	PackQty      *decimal.Decimal `json:"packQty"`
	// PackCost es lo pagado por empaque («$30 por Bolsa · 5 kg»); lo calcula el servidor.
	PackCost *decimal.Decimal `json:"packCost"`
	Supplier *string          `json:"supplier"`
	User     *string          `json:"user"`
	// Brand es la marca con la que se compró (copiada en la compra, 0104).
	Brand *string `json:"brand"`
	// FromName es el insumo unido del que viene el renglón (spec 056); nulo si es del propio.
	FromName *string `json:"fromName"`
}

// HistoryCursor es dónde sigue una página: el último renglón que ya se mostró.
type HistoryCursor struct {
	At   time.Time
	Kind string
	ID   int64
}

// CostHistoryPage es una página del historial. TrendPct va solo en la primera y solo con 2 compras o
// más: compara el costo actual contra la compra más vieja de las últimas 20, y no cambia con «Ver más».
type CostHistoryPage struct {
	Items    []CostHistoryItem `json:"items"`
	Next     *string           `json:"next"`
	TrendPct *decimal.Decimal  `json:"trendPct"`
}

// ArticleCostHistory es el historial de costos de un artículo (FR-006).
func (s *BackofficeService) ArticleCostHistory(ctx context.Context, id int64, before *HistoryCursor) (CostHistoryPage, error) {
	q := s.store.QC(ctx)
	arg := db.ArticleCostHistoryParams{IngredientID: id, Lim: historyPage + 1}
	if before != nil {
		arg.BeforeAt = pgtype.Timestamptz{Time: before.At, Valid: true}
		arg.BeforeKind, arg.BeforeID = &before.Kind, &before.ID
	}
	rows, err := q.ArticleCostHistory(ctx, arg)
	if err != nil {
		return CostHistoryPage{}, err
	}
	merged, err := q.MergedSourceNames(ctx, id)
	if err != nil {
		return CostHistoryPage{}, err
	}
	sourceNames := make(map[int64]string, len(merged))
	for _, m := range merged {
		sourceNames[m.ID] = m.Name
	}
	page := CostHistoryPage{Items: []CostHistoryItem{}}
	for i, r := range rows {
		if i == historyPage {
			last := rows[i-1]
			next := EncodeHistoryCursor(HistoryCursor{At: last.At, Kind: last.Kind, ID: last.ID})
			page.Next = &next
			break
		}
		it := CostHistoryItem{Kind: r.Kind, At: r.At, ID: r.ID, UnitCost: r.UnitCost, Presentation: r.Presentation,
			PackQty: r.PackQtyInBase, Supplier: r.Supplier, User: r.UserName, Brand: r.Brand}
		if name, ok := sourceNames[r.SourceID]; ok {
			it.FromName = &name
		}
		if r.PackQtyInBase != nil {
			v := domain.Round2(r.UnitCost.Mul(*r.PackQtyInBase))
			it.PackCost = &v
		}
		page.Items = append(page.Items, it)
	}
	if before == nil {
		ref, err := q.OldestOfLastPurchases(ctx, &id)
		if err != nil {
			return CostHistoryPage{}, err
		}
		if ref.Purchases >= 2 {
			a, err := q.GetArticle(ctx, id)
			if err != nil {
				return CostHistoryPage{}, notFound(err)
			}
			page.TrendPct = domain.TrendPct(a.CurrentCost, ref.OldestCost)
		}
	}
	return page, nil
}

// EncodeHistoryCursor y DecodeHistoryCursor viajan el cursor como texto opaco para la pantalla.
func EncodeHistoryCursor(c HistoryCursor) string {
	return c.At.UTC().Format(time.RFC3339Nano) + "|" + c.Kind + "|" + fmt.Sprint(c.ID)
}

// DecodeHistoryCursor rechaza un cursor malformado como 400: nunca cae a «la primera página».
func DecodeHistoryCursor(s string) (*HistoryCursor, error) {
	parts := strings.Split(s, "|")
	bad := fmt.Errorf("%w: página inválida", domain.ErrValidation)
	if len(parts) != 3 {
		return nil, bad
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, bad
	}
	var id int64
	if _, err := fmt.Sscan(parts[2], &id); err != nil || id <= 0 {
		return nil, bad
	}
	return &HistoryCursor{At: at, Kind: parts[1], ID: id}, nil
}

// ArticleAuditItem es un evento de la bitácora del artículo.
type ArticleAuditItem struct {
	ID     int64           `json:"id"`
	At     time.Time       `json:"at"`
	Action string          `json:"action"`
	Entity string          `json:"entity"`
	Actor  *string         `json:"actor"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
	// Changes cuenta los guardados seguidos que junta esta entrada (spec 043); 1 es un evento solo.
	Changes int64 `json:"changes"`
}

// ArticleAuditPage es una página de la bitácora del artículo.
type ArticleAuditPage struct {
	Items []ArticleAuditItem `json:"items"`
	Next  *string            `json:"next"`
}

// ArticleAudit es la bitácora de un artículo y de sus empaques (FR-016), de 20 en 20.
func (s *BackofficeService) ArticleAudit(ctx context.Context, id int64, before *HistoryCursor) (ArticleAuditPage, error) {
	arg := db.ArticleAuditParams{IngredientID: id, Lim: historyPage + 1}
	if before != nil {
		arg.BeforeAt = pgtype.Timestamptz{Time: before.At, Valid: true}
		arg.BeforeID = &before.ID
	}
	rows, err := s.store.QC(ctx).ArticleAudit(ctx, arg)
	if err != nil {
		return ArticleAuditPage{}, err
	}
	page := ArticleAuditPage{Items: []ArticleAuditItem{}}
	for i, r := range rows {
		if i == historyPage {
			last := rows[i-1]
			next := EncodeHistoryCursor(HistoryCursor{At: last.OccurredAt, Kind: "audit", ID: last.ID})
			page.Next = &next
			break
		}
		page.Items = append(page.Items, ArticleAuditItem{ID: r.ID, At: r.OccurredAt, Action: r.Action, Entity: r.EntityType,
			Actor: r.Actor, Before: rawOrNull(r.Before), After: rawOrNull(r.After), Changes: r.Changes})
	}
	return page, nil
}

func rawOrNull(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

// ArchiveCheck es lo que se muestra antes de archivar (FR-007).
type ArchiveCheck struct {
	OnHand          []BranchQty `json:"onHand"`
	ActiveRecipes   []RecipeUse `json:"activeRecipes"`
	OpenCount       bool        `json:"openCount"`
	PendingReceipts int         `json:"pendingReceipts"`
}

// BranchQty es lo que queda en una sucursal.
type BranchQty struct {
	Branch string          `json:"branch"`
	Qty    decimal.Decimal `json:"qty"`
}

// RecipeUse es una receta activa que resta el artículo: de un producto, de un extra o de un preparado.
type RecipeUse struct {
	Kind string `json:"kind"` // product | option | ingredient
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ArticleArchiveCheck dice qué queda y qué lo usa antes de archivar.
func (s *BackofficeService) ArticleArchiveCheck(ctx context.Context, id int64) (ArchiveCheck, error) {
	return archiveCheck(ctx, s.store.QC(ctx), id)
}

func archiveCheck(ctx context.Context, q *db.Queries, id int64) (ArchiveCheck, error) {
	if _, err := q.GetArticle(ctx, id); err != nil {
		return ArchiveCheck{}, notFound(err)
	}
	out := ArchiveCheck{OnHand: []BranchQty{}, ActiveRecipes: []RecipeUse{}}
	stock, err := q.ArticleOnHandByBranch(ctx, &id)
	if err != nil {
		return out, err
	}
	for _, r := range stock {
		out.OnHand = append(out.OnHand, BranchQty{Branch: r.Branch, Qty: r.OnHand})
	}
	uses, err := q.ActiveRecipesUsing(ctx, id)
	if err != nil {
		return out, err
	}
	for _, r := range uses {
		out.ActiveRecipes = append(out.ActiveRecipes, RecipeUse{Kind: r.Kind, ID: r.ID, Name: r.Name})
	}
	if out.OpenCount, err = q.OpenCountHasArticle(ctx, id); err != nil {
		return out, err
	}
	n, err := q.PendingReceiptsOf(ctx, &id)
	out.PendingReceipts = int(n)
	return out, err
}

// ArchiveArticle deja de ofrecer un artículo en el gasto, la receta y el conteo, sin tocar su historia.
// Se niega mientras una receta activa lo use o un conteo abierto lo tenga capturado (FR-007). Una
// compra por recibir no lo impide: cuando llegue, suma sin reactivarlo.
func (s *BackofficeService) ArchiveArticle(ctx context.Context, id int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		// Sin esperar: una venta en curso responde «vuelve a intentar» en vez de dejar la pantalla colgada.
		a, err := q.LockArticleNowait(ctx, id)
		if err != nil {
			return notFound(lockErr(err))
		}
		if !a.IsActive {
			return nil
		}
		check, err := archiveCheck(ctx, q, id)
		if err != nil {
			return err
		}
		if len(check.ActiveRecipes) > 0 {
			return fmt.Errorf("%w: lo usan %d recetas activas; quítalo de ellas antes de archivar", domain.ErrArticleInUse, len(check.ActiveRecipes))
		}
		if check.OpenCount {
			return fmt.Errorf("%w: un conteo abierto ya lo capturó; ciérralo o descártalo antes", domain.ErrArticleInUse)
		}
		if err := q.SetArticleActive(ctx, db.SetArticleActiveParams{ID: id, Active: false}); err != nil {
			return err
		}
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientArchived, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"active": true}, After: map[string]any{"active": false, "onHand": check.OnHand, "pendingReceipts": check.PendingReceipts},
		})
	})
}

// ReactivateArticle vuelve a ofrecer un artículo archivado, tal como estaba.
func (s *BackofficeService) ReactivateArticle(ctx context.Context, id int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		a, err := q.LockArticle(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if a.IsActive {
			return nil
		}
		// Un insumo unido a otro no se reactiva (spec 056, FR-011); la base también lo impide.
		switch t, err := q.MergeTargetOf(ctx, id); {
		case err == nil:
			return domain.IngredientMergedInto{Target: t.Name}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		// Otro activo pudo tomar su nombre mientras estaba archivado.
		art, err := q.GetArticle(ctx, id)
		if err != nil {
			return err
		}
		if err := q.LockIngredientNames(ctx); err != nil {
			return err
		}
		switch found, err := q.FindOtherArticleByName(ctx, db.FindOtherArticleByNameParams{ID: id, Name: art.Name}); {
		case err == nil && found.IsActive:
			return domain.ArticleNameTaken{ID: found.ID}
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if err := q.SetArticleActive(ctx, db.SetArticleActiveParams{ID: id, Active: true}); err != nil {
			return err
		}
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientReactivated, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"active": false}, After: map[string]any{"active": true},
		})
	})
}

// ArticleCategory es una categoría de artículos.
type ArticleCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ArticleCategories son las categorías de artículos de la empresa, para la ficha.
func (s *BackofficeService) ArticleCategories(ctx context.Context) ([]ArticleCategory, error) {
	rows, err := s.store.QC(ctx).ListIngredientCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ArticleCategory, len(rows))
	for i, r := range rows {
		out[i] = ArticleCategory{ID: r.ID, Name: string(r.Name)}
	}
	return out, nil
}

// CreateArticleCategory da de alta una categoría desde la ficha. Repetida es 409.
func (s *BackofficeService) CreateArticleCategory(ctx context.Context, name string) (ArticleCategory, error) {
	name = domain.CleanArticleName(name)
	if name == "" || len([]rune(name)) > domain.MaxArticleName {
		return ArticleCategory{}, fmt.Errorf("%w: la categoría lleva de 1 a %d letras", domain.ErrValidation, domain.MaxArticleName)
	}
	r, err := s.store.QC(ctx).CreateIngredientCategory(ctx, name)
	if err != nil {
		if isUniqueViolation(err) {
			return ArticleCategory{}, fmt.Errorf("%w: esa categoría ya existe", domain.ErrConflict)
		}
		return ArticleCategory{}, err
	}
	return ArticleCategory{ID: r.ID, Name: string(r.Name)}, nil
}

// PresentationSuggestion es un empaque que ya se usa: «Garrafa · 4 L».
type PresentationSuggestion struct {
	Name      string          `json:"name"`
	QtyInBase decimal.Decimal `json:"qtyInBase"`
}

// PresentationSuggestions son los empaques de artículos que se miden igual (FR-011).
func (s *BackofficeService) PresentationSuggestions(ctx context.Context, kind string) ([]PresentationSuggestion, error) {
	switch kind {
	case "masa", "volumen", "longitud", "pieza":
	default:
		return nil, fmt.Errorf("%w: clase de unidad desconocida", domain.ErrValidation)
	}
	rows, err := s.store.QC(ctx).PresentationSuggestions(ctx, db.UnitKind(kind))
	if err != nil {
		return nil, err
	}
	out := make([]PresentationSuggestion, len(rows))
	for i, r := range rows {
		out[i] = PresentationSuggestion{Name: r.Name, QtyInBase: r.QtyInBase}
	}
	return out, nil
}

// categoryOfCompany exige que la categoría sea de la empresa de la sesión: la lee bajo RLS. La llave
// de ingredients.category_id no lleva la empresa y una llave foránea salta RLS (revisión de la 042).
func categoryOfCompany(ctx context.Context, q *db.Queries, id *int64) error {
	if id == nil {
		return nil
	}
	if _, err := q.CategoryVisible(ctx, *id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: esa categoría no existe", domain.ErrValidation)
		}
		return err
	}
	return nil
}
