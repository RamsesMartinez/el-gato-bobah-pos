package app

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Densidad de un artículo (spec 058, US1). La fila guarda solo la vigente, que es la que convierte; la
// historia, con cada pesada, va en la bitácora. Cambiarla no recalcula nada ya convertido: las compras
// guardan la suya (expense_items.density_g_per_ml) y los conteos la que usaron.

// DensityInput es una captura: una pesada (lleno, vacío, contenido), la etiqueta (neto, contenido) o el
// valor escrito. Solo los campos de su método; uno de más es un cuerpo que no se entiende y se rechaza.
type DensityInput struct {
	Method    string           `json:"method"`
	FullG     *decimal.Decimal `json:"fullG"`
	EmptyG    *decimal.Decimal `json:"emptyG"`
	ContentMl *decimal.Decimal `json:"contentMl"`
	NetG      *decimal.Decimal `json:"netG"`
	Density   *decimal.Decimal `json:"density"`
}

// DensityView es la densidad vigente de un artículo, para la ficha.
type DensityView struct {
	Value     decimal.Decimal `json:"value"`
	Method    string          `json:"method"`
	SetByName string          `json:"setByName"`
	SetAt     time.Time       `json:"setAt"`
}

// resolve calcula la densidad y lo que se guarda en la bitácora sobre cómo salió.
func (in DensityInput) resolve() (decimal.Decimal, map[string]any, error) {
	only := func(want ...*decimal.Decimal) bool {
		all := []*decimal.Decimal{in.FullG, in.EmptyG, in.ContentMl, in.NetG, in.Density}
		n := 0
		for _, p := range all {
			if p != nil {
				n++
			}
		}
		for _, p := range want {
			if p == nil {
				return false
			}
		}
		return n == len(want)
	}
	bad := fmt.Errorf("%w: faltan o sobran datos para esa forma de capturar la densidad", domain.ErrValidation)
	switch in.Method {
	case "weighing":
		if !only(in.FullG, in.EmptyG, in.ContentMl) {
			return decimal.Zero, nil, bad
		}
		d, err := domain.DensityFromWeighing(*in.FullG, *in.EmptyG, *in.ContentMl)
		return d, map[string]any{"fullG": in.FullG, "emptyG": in.EmptyG, "contentMl": in.ContentMl}, err
	case "label":
		if !only(in.NetG, in.ContentMl) {
			return decimal.Zero, nil, bad
		}
		d, err := domain.DensityFromLabel(*in.NetG, *in.ContentMl)
		return d, map[string]any{"netG": in.NetG, "contentMl": in.ContentMl}, err
	case "manual":
		if !only(in.Density) {
			return decimal.Zero, nil, bad
		}
		return *in.Density, map[string]any{}, domain.ValidDensity(*in.Density)
	}
	return decimal.Zero, nil, fmt.Errorf("%w: forma de capturar la densidad desconocida", domain.ErrValidation)
}

// lockDensityArticle toma el artículo sin esperar con el mismo candado que una compra toma antes de
// leer sus renglones (`no key update`), así una compra y un cambio de densidad no se cruzan (revisión
// M1) y una venta, que solo toma la llave, no espera. Exige un artículo activo en g o ml.
func lockDensityArticle(ctx context.Context, q *db.Queries, id int64) (db.GetArticleRow, error) {
	if err := lockIngredients(ctx, q, []int64{id}); err != nil {
		return db.GetArticleRow{}, err
	}
	art, err := q.GetArticle(ctx, id)
	if err != nil {
		return art, notFound(err)
	}
	if !art.IsActive {
		return art, fmt.Errorf("%w: reactívalo antes de cambiar su densidad", domain.ErrConflict)
	}
	if art.BaseUnitKind != db.UnitKindMasa && art.BaseUnitKind != db.UnitKindVolumen {
		return art, fmt.Errorf("%w: la densidad solo aplica a lo que se mide en g o ml", domain.ErrValidation)
	}
	return art, nil
}

// SetDensity pone o rehace la densidad de un artículo. Último en guardar gana; la bitácora tiene las dos.
func (s *BackofficeService) SetDensity(ctx context.Context, id int64, in DensityInput) (DensityView, error) {
	actor, ok := ActorUserFrom(ctx)
	if !ok {
		return DensityView{}, fmt.Errorf("%w: capturar la densidad necesita a la persona que lo hace", domain.ErrForbidden)
	}
	d, detail, err := in.resolve()
	if err != nil {
		return DensityView{}, err
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		art, err := lockDensityArticle(ctx, q, id)
		if err != nil {
			return err
		}
		if err := q.SetArticleDensity(ctx, db.SetArticleDensityParams{ID: id, Density: &d, Method: in.Method, UserID: &actor}); err != nil {
			return err
		}
		detail["density"] = d
		detail["method"] = in.Method
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientDensitySet, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"density": art.DensityGPerMl, "method": art.DensityMethod},
			After:  detail,
		})
	})
	if err != nil {
		return DensityView{}, err
	}
	sheet, err := s.GetArticle(ctx, id)
	if err != nil {
		return DensityView{}, err
	}
	if sheet.Density == nil {
		return DensityView{}, fmt.Errorf("la densidad no quedó guardada")
	}
	return *sheet.Density, nil
}

// ClearDensity quita la densidad. Se niega mientras una compra entre peso y líquido por recibir la
// necesite para convertirse (revisión A1): sin ella el gasto entero dejaría de poder tocarse.
func (s *BackofficeService) ClearDensity(ctx context.Context, id int64) error {
	if _, ok := ActorUserFrom(ctx); !ok {
		return fmt.Errorf("%w: quitar la densidad necesita a la persona que lo hace", domain.ErrForbidden)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		art, err := lockDensityArticle(ctx, q, id)
		if err != nil {
			return err
		}
		if art.DensityGPerMl == nil {
			return nil
		}
		pending, err := q.PendingDensityLinesOf(ctx, &id)
		if err != nil {
			return err
		}
		if pending > 0 {
			return domain.ErrDensityInUse
		}
		if _, err := q.ClearArticleDensity(ctx, id); err != nil {
			return err
		}
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientDensityCleared, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"density": art.DensityGPerMl, "method": art.DensityMethod},
			After:  map[string]any{},
		})
	})
}
