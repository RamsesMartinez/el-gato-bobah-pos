package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Empaques de compra por insumo (spec 038, historia 2): «Caja = 12 L» se captura con número y
// unidad y se guarda en la unidad del insumo. El contenido se copia al renglón de cada compra, así
// que editar o desactivar un empaque no reescribe compras pasadas.

// PresentationView es un empaque de compra de un insumo.
type PresentationView struct {
	ID           int64           `json:"id"`
	IngredientID int64           `json:"ingredientId"`
	Name         string          `json:"name"`
	QtyInBase    decimal.Decimal `json:"qtyInBase"`
	BaseUnitCode string          `json:"baseUnitCode"`
	Active       bool            `json:"active"`
	// Approximate: se compra en otro tipo de unidad que el insumo («Kilo ≈ 4 piezas»).
	Approximate    bool   `json:"approximate"`
	PurchaseUnitID *int16 `json:"purchaseUnitId"`
	// Brand es la marca o el proveedor (spec 056), opcional.
	Brand *string `json:"brand"`
	// TareG: lo que pesa vacío, en g (spec 058), para contar pesando con el producto adentro.
	TareG *decimal.Decimal `json:"tareG"`
}

// PresentationInput es un empaque tal como se captura: cuánto trae y en qué unidad. Active nulo
// crea el empaque activo y, al editar, conserva su estado.
type PresentationInput struct {
	Name        string
	Qty         decimal.Decimal
	UnitID      int16
	Approximate bool
	Active      *bool
	// Brand nulo crea sin marca y, al editar, conserva la que tiene; vacío la quita.
	Brand *string
	// Tare: lo que pesa vacío (spec 058). Sin Set, al editar se queda la que tiene (una tableta vieja
	// no la manda); con Set y sin valor, se quita.
	Tare Field[decimal.Decimal]
}

// Presentations lista los empaques de un insumo, o los de toda la empresa sin filtro. Nunca nil:
// la pantalla espera una lista.
func (s *BackofficeService) Presentations(ctx context.Context, ingredientID *int64) ([]PresentationView, error) {
	rows, err := s.store.QC(ctx).ListPresentations(ctx, ingredientID)
	if err != nil {
		return nil, err
	}
	out := make([]PresentationView, 0, len(rows))
	for _, r := range rows {
		out = append(out, PresentationView{
			ID: r.ID, IngredientID: r.IngredientID, Name: r.Name, QtyInBase: r.QtyInBase,
			BaseUnitCode: r.BaseUnitCode, Active: r.Active, Approximate: r.IsApproximate,
			PurchaseUnitID: r.PurchaseUnitID, Brand: r.Brand, TareG: r.TareG,
		})
	}
	return out, nil
}

// presentationTare valida lo que pesa vacío un empaque: solo en un artículo en g o ml.
func presentationTare(ctx context.Context, q *db.Queries, ingredientID int64, tare *decimal.Decimal) error {
	if tare == nil {
		return nil
	}
	if err := domain.ValidTare(*tare); err != nil {
		return err
	}
	ing, err := q.GetIngredientUnit(ctx, ingredientID)
	if err != nil {
		return notFound(err)
	}
	if ing.BaseUnitKind != db.UnitKindMasa && ing.BaseUnitKind != db.UnitKindVolumen {
		return fmt.Errorf("%w: lo que pesa vacío solo sirve en lo que se mide en g o ml", domain.ErrValidation)
	}
	return nil
}

// setTare guarda lo que pesa vacío y deja el evento si cambió.
func setTare(ctx context.Context, q *db.Queries, id int64, before, after *decimal.Decimal) error {
	if sameDec(before, after) {
		return nil
	}
	if err := q.SetPresentationTare(ctx, db.SetPresentationTareParams{ID: id, TareG: after}); err != nil {
		return err
	}
	return recordAudit(ctx, q, domain.AuditEvent{
		Action: domain.AuditPresentationTareSet, EntityType: "presentation", EntityID: id,
		Before: map[string]any{"tareG": before}, After: map[string]any{"tareG": after},
	})
}

// planPresentation convierte lo capturado a la unidad del insumo con las reglas de dominio.
func planPresentation(ctx context.Context, q *db.Queries, ingredientID int64, in PresentationInput) (domain.PresentationPlan, error) {
	if in.UnitID == 0 {
		return domain.PresentationPlan{}, fmt.Errorf("%w: falta la unidad del empaque", domain.ErrValidation)
	}
	ing, err := q.GetIngredientUnit(ctx, ingredientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PresentationPlan{}, domain.ErrNotFound
		}
		return domain.PresentationPlan{}, err
	}
	unit, err := q.GetUnit(ctx, in.UnitID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PresentationPlan{}, fmt.Errorf("%w: unidad desconocida", domain.ErrValidation)
		}
		return domain.PresentationPlan{}, err
	}
	units, err := q.ListUnits(ctx)
	if err != nil {
		return domain.PresentationPlan{}, err
	}
	names := make([]string, 0, 2*len(units))
	for _, u := range units {
		names = append(names, u.Code, u.Name)
	}
	return domain.PlanPresentation(domain.PresentationInput{
		Name: in.Name, Qty: in.Qty,
		Unit:        domain.PurchaseUnit{Kind: string(unit.Kind), ToBase: unit.ToBase},
		BaseKind:    string(ing.BaseUnitKind),
		Approximate: in.Approximate, UnitNames: names,
	})
}

// CreatePresentation da de alta un empaque. Un nombre repetido (sin mayúsculas ni acentos) entre
// los activos del mismo insumo responde 409.
func (s *BackofficeService) CreatePresentation(ctx context.Context, ingredientID int64, in PresentationInput) (PresentationView, error) {
	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		plan, err := planPresentation(ctx, q, ingredientID, in)
		if err != nil {
			return err
		}
		brand, err := domain.CleanPresentationBrand(in.Brand)
		if err != nil {
			return err
		}
		if err := presentationTare(ctx, q, ingredientID, in.Tare.Value); err != nil {
			return err
		}
		id, err = q.CreatePresentation(ctx, db.CreatePresentationParams{
			IngredientID: ingredientID, Name: plan.Name, QtyInBase: plan.QtyInBase,
			PurchaseUnitID: approxUnit(plan.Approximate, in.UnitID), IsApproximate: plan.Approximate, Brand: brand,
		})
		if err != nil {
			return err
		}
		if err := setTare(ctx, q, id, nil, in.Tare.Value); err != nil {
			return err
		}
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditPresentationCreated, EntityType: "presentation", EntityID: id,
			After: map[string]any{"name": plan.Name, "qtyInBase": plan.QtyInBase, "approximate": plan.Approximate, "brand": brand},
		})
	})
	if err != nil {
		return PresentationView{}, presentationErr(err)
	}
	return s.presentation(ctx, ingredientID, id)
}

// UpdatePresentation cambia nombre, contenido o estado de un empaque.
func (s *BackofficeService) UpdatePresentation(ctx context.Context, id int64, in PresentationInput) (PresentationView, error) {
	var ingredientID int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		cur, err := q.GetPresentationIngredient(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		ingredientID = cur.IngredientID
		plan, err := planPresentation(ctx, q, ingredientID, in)
		if err != nil {
			return err
		}
		// Sin estado, se queda el que tiene: editar el nombre de un empaque que ya no llega no lo
		// vuelve a ofrecer en la captura.
		active := cur.Active
		if in.Active != nil {
			active = *in.Active
		}
		// Sin marca en el cuerpo se queda la que tiene: una pantalla anterior a la 056 no la manda y,
		// al editar el nombre, la borraría.
		brand := cur.Brand
		if in.Brand != nil {
			if brand, err = domain.CleanPresentationBrand(in.Brand); err != nil {
				return err
			}
		}
		before, err := q.GetPresentationForAudit(ctx, id)
		if err != nil {
			return err
		}
		n, err := q.UpdatePresentation(ctx, db.UpdatePresentationParams{
			ID: id, Name: plan.Name, QtyInBase: plan.QtyInBase,
			PurchaseUnitID: approxUnit(plan.Approximate, in.UnitID), IsApproximate: plan.Approximate, Active: active, Brand: brand,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		if in.Tare.Set {
			if err := presentationTare(ctx, q, ingredientID, in.Tare.Value); err != nil {
				return err
			}
			if err := setTare(ctx, q, id, before.TareG, in.Tare.Value); err != nil {
				return err
			}
		}
		// Un cambio de estado (deja de llegar así o vuelve) es «retiró»; uno de nombre o cantidad, «editó».
		if before.Active != active {
			if err := recordAudit(ctx, q, domain.AuditEvent{
				Action: domain.AuditPresentationRetired, EntityType: "presentation", EntityID: id,
				Before: map[string]any{"active": before.Active, "name": before.Name}, After: map[string]any{"active": active},
			}); err != nil {
				return err
			}
		}
		if before.Name != plan.Name || !before.QtyInBase.Equal(plan.QtyInBase) || textoDe(before.Brand) != textoDe(brand) {
			return recordAudit(ctx, q, domain.AuditEvent{
				Action: domain.AuditPresentationUpdated, EntityType: "presentation", EntityID: id,
				Before: map[string]any{"name": before.Name, "qtyInBase": before.QtyInBase, "brand": before.Brand},
				After:  map[string]any{"name": plan.Name, "qtyInBase": plan.QtyInBase, "brand": brand},
			})
		}
		return nil
	})
	if err != nil {
		return PresentationView{}, presentationErr(err)
	}
	return s.presentation(ctx, ingredientID, id)
}

func (s *BackofficeService) presentation(ctx context.Context, ingredientID, id int64) (PresentationView, error) {
	list, err := s.Presentations(ctx, &ingredientID)
	if err != nil {
		return PresentationView{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return PresentationView{}, domain.ErrNotFound
}

// approxUnit: solo un empaque aproximado guarda su unidad de compra; el de siempre es una pieza de
// compra cuyo contenido ya está en la unidad del insumo.
func approxUnit(approximate bool, unitID int16) *int16 {
	if !approximate {
		return nil
	}
	return &unitID
}

// presentationErr traduce lo que la base rechaza: el nombre repetido (índice único) es un 409 que
// se puede corregir; un insumo de otra empresa no existe para esta sesión (la FK compuesta lo
// frena aunque RLS lo dejara pasar).
func presentationErr(err error) error {
	switch {
	case isUniqueViolation(err):
		return fmt.Errorf("%w: ya hay un empaque que se llama así", domain.ErrConflict)
	case isForeignKeyViolation(err):
		return domain.ErrNotFound
	}
	return err
}
