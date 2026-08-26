package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Catálogo de artículos del almacén y sugerencias de mapeo.
//
// Hasta ahora los ingredientes solo entraban por el importador de FUDO: no había forma de
// listarlos ni crearlos desde la app, así que el almacén era un ledger sin catálogo consultable
// y el "buscador de artículo" del gasto no tenía contra qué buscar.

type UnitView struct {
	ID     int16           `json:"id"`
	Code   string          `json:"code"`
	Name   string          `json:"name"`
	Kind   string          `json:"kind"` // masa | volumen | pieza
	ToBase decimal.Decimal `json:"toBase"`
}

type IngredientView struct {
	ID           int64            `json:"id"`
	Name         string           `json:"name"`
	IsActive     bool             `json:"isActive"`
	TrackStock   bool             `json:"trackStock"`
	IsPackaging  bool             `json:"isPackaging"`
	MinStock     *decimal.Decimal `json:"minStock"`
	CurrentCost  decimal.Decimal  `json:"currentCost"`
	BaseUnitID   int16            `json:"baseUnitId"`
	BaseUnitCode string           `json:"baseUnitCode"`
	BaseUnitKind string           `json:"baseUnitKind"`
	Category     *string          `json:"category"`
	OnHand       decimal.Decimal  `json:"onHand"`
}

// ArticleView es una entrada del buscador único: ingredientes y productos con control de stock
// en una sola lista, para que el operador no tenga que decidir primero "¿es ingrediente o
// producto?" antes de buscar.
type ArticleView struct {
	ItemType string `json:"itemType"` // ingrediente | producto
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	UnitCode string `json:"unitCode"`
	UnitKind string `json:"unitKind"`
}

type IngredientInput struct {
	Name        string
	BaseUnitID  int16
	CategoryID  *int64
	MinStock    *decimal.Decimal
	TrackStock  *bool
	IsPackaging *bool
}

// maxArticleResults es un techo defensivo, no una paginación: el front carga el catálogo una
// vez al abrir el diálogo del gasto y el picker filtra en local (así el buscador responde sin
// round-trip por tecla). Un local maneja cientos de insumos, no miles.
const maxArticleResults = 500

func (s *BackofficeService) Units(ctx context.Context) ([]UnitView, error) {
	rows, err := s.store.QC(ctx).ListUnits(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]UnitView, len(rows))
	for i, r := range rows {
		out[i] = UnitView{ID: r.ID, Code: r.Code, Name: r.Name, Kind: string(r.Kind), ToBase: r.ToBase}
	}
	return out, nil
}

func (s *BackofficeService) Ingredients(ctx context.Context, onlyActive bool) ([]IngredientView, error) {
	var active *bool
	if onlyActive {
		active = &onlyActive
	}
	rows, err := s.store.QC(ctx).ListIngredients(ctx, active)
	if err != nil {
		return nil, err
	}
	out := make([]IngredientView, len(rows))
	for i, r := range rows {
		out[i] = IngredientView{
			ID: r.ID, Name: string(r.Name), IsActive: r.IsActive, TrackStock: r.TrackStock,
			IsPackaging: r.IsPackaging, MinStock: r.MinStock, CurrentCost: r.CurrentCost,
			BaseUnitID: r.BaseUnitID, BaseUnitCode: r.BaseUnitCode, BaseUnitKind: string(r.BaseUnitKind),
			Category: r.Category, OnHand: r.OnHand,
		}
	}
	return out, nil
}

// CreateIngredient da de alta un insumo con lo mínimo: nombre y unidad base. Receta, merma y
// proveedor se editan después — el alta tiene que caber en el diálogo del gasto sin sacar al
// operador del flujo de captura.
func (s *BackofficeService) CreateIngredient(ctx context.Context, in IngredientInput) (IngredientView, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || in.BaseUnitID == 0 {
		return IngredientView{}, domain.ErrValidation
	}
	if in.MinStock != nil {
		v := domain.Round4(*in.MinStock)
		if v.IsNegative() || v.GreaterThan(domain.MaxStockQty) {
			return IngredientView{}, domain.ErrValidation
		}
		in.MinStock = &v
	}
	r, err := s.store.QC(ctx).CreateIngredient(ctx, db.CreateIngredientParams{
		Name: name, BaseUnitID: in.BaseUnitID, CategoryID: in.CategoryID,
		MinStock: in.MinStock, TrackStock: in.TrackStock, IsPackaging: in.IsPackaging,
	})
	if err != nil {
		// El unique (company_id, name) protege contra dos "Leche entera" en la misma empresa;
		// llega como 409 accionable en vez de un 500 opaco.
		if isUniqueViolation(err) {
			return IngredientView{}, domain.ErrConflict
		}
		return IngredientView{}, err
	}
	// Los campos de join (unidad, categoría, existencia) quedan vacíos a propósito: el alta no
	// los consulta y el picker solo necesita id+nombre para agregar la opción recién creada.
	return IngredientView{
		ID: r.ID, Name: string(r.Name), BaseUnitID: r.BaseUnitID, IsActive: r.IsActive,
		TrackStock: r.TrackStock, IsPackaging: r.IsPackaging,
		MinStock: r.MinStock, CurrentCost: r.CurrentCost,
	}, nil
}

// SearchArticles alimenta el picker del gasto. q vacío devuelve el inicio del catálogo (para que
// el picker abra con algo, no en blanco).
func (s *BackofficeService) SearchArticles(ctx context.Context, q string) ([]ArticleView, error) {
	var needle *string
	if t := strings.TrimSpace(q); t != "" {
		needle = &t
	}
	rows, err := s.store.QC(ctx).SearchArticles(ctx, db.SearchArticlesParams{Q: needle, Lim: maxArticleResults})
	if err != nil {
		return nil, err
	}
	out := make([]ArticleView, len(rows))
	for i, r := range rows {
		out[i] = ArticleView{
			ItemType: r.ItemType, ID: r.ID, Name: r.Name,
			UnitCode: r.UnitCode, UnitKind: string(r.UnitKind),
		}
	}
	return out, nil
}

// ---- Sugerencias de mapeo ----

// ArticleSuggestion es una propuesta para un renglón de documento. Nunca se aplica sola: el
// operador confirma, y al confirmar se aprende (learnSupplierItems).
// sourcePersonal marca el renglón que ya se decidió que no es del local. No trae artículo: es
// una respuesta ("esto es de la casa"), no una sugerencia de qué es.
const sourcePersonal = "personal"

type ArticleSuggestion struct {
	// Source dice de dónde salió, porque cambia cuánto confiar:
	// "aprendido" = ya se mapeó ese mismo renglón con ese proveedor → autollenar.
	// "otro_proveedor" / "catalogo" = parecido → sugerir y esperar confirmación.
	// "personal" = aprendido como de la casa → se marca y no suma al gasto.
	Source        string           `json:"source"`
	ItemType      string           `json:"itemType"`
	ItemID        int64            `json:"itemId"`
	ItemName      string           `json:"itemName"`
	Score         float64          `json:"score"` // 1 = coincidencia exacta aprendida
	MatchedVia    string           `json:"matchedVia"`
	PackQtyInBase *decimal.Decimal `json:"packQtyInBase"`
	UnitID        *int16           `json:"unitId"`
}

// SuggestForLine corre la cascada de mapeo para un renglón de documento:
//  1. llave exacta aprendida con ESE proveedor → autollenar.
//  2. parecido contra renglones ya mapeados de OTROS proveedores.
//  3. parecido contra el catálogo propio.
//
// Devuelve lista vacía cuando nada supera el umbral de similitud: ahí la UI ofrece crear el
// artículo con el nombre que sugirió el extractor.
func (s *BackofficeService) SuggestForLine(ctx context.Context, supplierID int64, rawCode, rawName string) ([]ArticleSuggestion, error) {
	if strings.TrimSpace(rawName) == "" && strings.TrimSpace(rawCode) == "" {
		return []ArticleSuggestion{}, nil
	}
	key := domain.SupplierItemKey(rawCode, rawName)
	needle := domain.NormalizeItemName(rawName)

	// 1. Exacta aprendida. Un 'ignorado' también es una respuesta aprendida (el operador ya dijo
	// que ese renglón no es inventariable), así que se devuelve para no volver a preguntar.
	if supplierID != 0 {
		learned, err := s.store.QC(ctx).LookupSupplierItem(ctx, db.LookupSupplierItemParams{
			SupplierID: supplierID, ItemKey: key,
		})
		switch {
		// El estado se mira ANTES del artículo: un renglón marcado como de la casa no debe
		// proponer el artículo al que alguna vez estuvo mapeado.
		case err == nil && learned.Status == domain.SupplierItemPersonal:
			return []ArticleSuggestion{{Source: sourcePersonal, Score: 1, MatchedVia: learned.RawName}}, nil
		case err == nil && learned.ItemType != nil:
			return []ArticleSuggestion{{
				Source: "aprendido", ItemType: string(*learned.ItemType),
				ItemID:   itemID(learned.IngredientID, learned.ProductID),
				ItemName: derefStr(firstName(learned.IngredientName, learned.ProductName)),
				Score:    1, MatchedVia: learned.RawName,
				PackQtyInBase: learned.PackQtyInBase, UnitID: learned.UnitID,
			}}, nil
		case err == nil:
			return []ArticleSuggestion{}, nil // aprendido como no inventariable
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
	}

	out := []ArticleSuggestion{}
	if needle == "" {
		return out, nil
	}

	// 2. Lo aprendido con otros proveedores: mapear "Coca Cola 600 ml" en una tienda hace que el
	// "COCA COLA 600ML" de otra pegue solo.
	cross, err := s.store.QC(ctx).SuggestFromSupplierItems(ctx, db.SuggestFromSupplierItemsParams{
		Needle: needle, SupplierID: supplierID, Lim: maxSuggestions,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range cross {
		if r.ItemType == nil {
			continue
		}
		out = append(out, ArticleSuggestion{
			Source: "otro_proveedor", ItemType: string(*r.ItemType),
			ItemID:   itemID(r.IngredientID, r.ProductID),
			ItemName: derefStr(firstName(r.IngredientName, r.ProductName)),
			Score:    float64(r.Score), MatchedVia: r.MatchedVia,
			PackQtyInBase: r.PackQtyInBase, UnitID: r.UnitID,
		})
	}

	// 3. El catálogo propio, para un proveedor nuevo sin nada aprendido.
	direct, err := s.store.QC(ctx).SuggestArticlesByName(ctx, db.SuggestArticlesByNameParams{
		Needle: needle, Lim: maxSuggestions,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range direct {
		out = append(out, ArticleSuggestion{
			Source: "catalogo", ItemType: r.ItemType, ItemID: r.ID,
			ItemName: r.ItemName, Score: float64(r.Score), MatchedVia: r.ItemName,
		})
	}
	return out, nil
}

// maxSuggestions: el operador elige de un vistazo en un tablet de 7". Más de un puñado de
// candidatos es peor que ninguno.
const maxSuggestions = 5

func itemID(ingredientID, productID *int64) int64 {
	if ingredientID != nil {
		return *ingredientID
	}
	if productID != nil {
		return *productID
	}
	return 0
}

// ---- Catálogo aprendido (revisión) ----

// SupplierItemView es una fila del mapeo aprendido: qué decía el papel y a qué artículo se
// resolvió. Es la superficie donde se revisa lo que el sistema aprendió y se corrige un mapeo
// equivocado — sin ella, un artículo mal asignado se repite en cada compra sin que nadie lo vea.
type SupplierItemView struct {
	ID            int64            `json:"id"`
	SupplierID    int64            `json:"supplierId"`
	Supplier      string           `json:"supplier"`
	RawCode       *string          `json:"rawCode"`
	RawName       string           `json:"rawName"`
	Status        string           `json:"status"`
	ItemType      *string          `json:"itemType"`
	ItemName      *string          `json:"itemName"`
	PackQtyInBase *decimal.Decimal `json:"packQtyInBase"`
	LastCost      *decimal.Decimal `json:"lastCost"`
	LastSeenAt    time.Time        `json:"lastSeenAt"`
}

// SupplierItems lista el catálogo aprendido, opcionalmente filtrado por estado o proveedor.
func (s *BackofficeService) SupplierItems(ctx context.Context, status string, supplierID int64, limit, offset int32) ([]SupplierItemView, int64, error) {
	var st *string
	if domain.ValidSupplierItemStatus(status) {
		st = &status
	}
	var sup *int64
	if supplierID != 0 {
		sup = &supplierID
	}
	total, err := s.store.QC(ctx).CountSupplierItems(ctx, db.CountSupplierItemsParams{Status: st, SupplierID: sup})
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.store.QC(ctx).ListSupplierItems(ctx, db.ListSupplierItemsParams{
		Status: st, SupplierID: sup, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]SupplierItemView, len(rows))
	for i, r := range rows {
		v := SupplierItemView{
			ID: r.ID, SupplierID: r.SupplierID, Supplier: r.Supplier, RawCode: r.RawCode,
			RawName: r.RawName, Status: r.Status, ItemName: firstName(r.IngredientName, r.ProductName),
			PackQtyInBase: r.PackQtyInBase, LastCost: r.LastCost, LastSeenAt: r.LastSeenAt,
		}
		if r.ItemType != nil {
			t := string(*r.ItemType)
			v.ItemType = &t
		}
		out[i] = v
	}
	return out, total, nil
}

// ForgetSupplierItem borra un mapeo aprendido. Es "deshacer": la próxima compra vuelve a
// sugerir desde cero en vez de arrastrar el error a cada ticket del mismo proveedor.
func (s *BackofficeService) ForgetSupplierItem(ctx context.Context, id int64) error {
	n, err := s.store.QC(ctx).DeleteSupplierItem(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
