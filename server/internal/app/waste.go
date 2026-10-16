package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Lo que se echó a perder (spec 039). Registrar resta del almacén en ese momento, o queda por
// aprobar si su costo pasa el tope de la empresa; aprobar, rechazar y deshacer cambian su estado y
// nunca lo borran. Toda escritura toma los insumos sin esperar antes de escribir (como las compras):
// una venta en curso del mismo insumo responde «vuelve a intentar» y no se forma un ciclo.

const wasteMovementReason = "se echó a perder"

// RecordWasteCmd es una merma tal como se capturó: un insumo (en una unidad o en un empaque) o un
// producto (en piezas).
type RecordWasteCmd struct {
	ReasonID       int64
	IngredientID   *int64
	ProductID      *int64
	Qty            decimal.Decimal
	UnitID         *int16
	PresentationID *int64
	Note           *string
	Actor          int64
}

// WasteResult dice cómo quedó: «applied» si ya restó, «pending» si espera aprobación.
type WasteResult struct {
	ID        int64            `json:"id"`
	Status    string           `json:"status"`
	TotalCost *decimal.Decimal `json:"totalCost"`
}

// RecordWaste registra lo que se echó a perder.
func (s *BackofficeService) RecordWaste(ctx context.Context, cmd RecordWasteCmd) (WasteResult, error) {
	entered := domain.Round4(cmd.Qty)
	if !domain.ValidQty(entered, domain.MaxStockQty, false) {
		return WasteResult{}, fmt.Errorf("%w: la cantidad va de más de 0 a %s", domain.ErrValidation, domain.MaxStockQty)
	}
	if (cmd.IngredientID == nil) == (cmd.ProductID == nil) {
		return WasteResult{}, fmt.Errorf("%w: se registra un artículo o un producto", domain.ErrValidation)
	}
	if cmd.ProductID != nil && (cmd.UnitID != nil || cmd.PresentationID != nil) {
		return WasteResult{}, fmt.Errorf("%w: un producto se registra en piezas", domain.ErrValidation)
	}
	note, err := wasteNote(cmd.Note)
	if err != nil {
		return WasteResult{}, err
	}
	var out WasteResult
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		branch, err := q.CurrentBranchID(ctx)
		if err != nil {
			return err
		}
		reason, err := activeWasteReason(ctx, q, cmd.ReasonID)
		if err != nil {
			return err
		}
		plan, err := planWaste(ctx, q, cmd, entered)
		if err != nil {
			return err
		}
		threshold, err := wasteThreshold(ctx, q)
		if err != nil {
			return err
		}
		status := domain.WasteApplied
		if domain.WasteNeedsApproval(threshold, plan.total) {
			status = domain.WastePending
		}
		if status == domain.WasteApplied {
			if err := lockIngredients(ctx, q, plan.ingredientIDs()); err != nil {
				return err
			}
		}
		itemType := db.StockItemType("ingrediente")
		if cmd.ProductID != nil {
			itemType = "producto"
		}
		rec, err := q.InsertWasteRecord(ctx, db.InsertWasteRecordParams{
			BranchID: branch, ReasonID: reason.ID, ItemType: itemType, IngredientID: cmd.IngredientID,
			ProductID: cmd.ProductID, Quantity: plan.quantity, EnteredQty: entered, PresentationID: cmd.PresentationID,
			UnitCost: plan.unitCost, TotalCost: plan.total, IsStaffMeal: reason.IsStaffMeal, Note: note,
			Status: string(status), RecordedBy: cmd.Actor,
		})
		if err != nil {
			return err
		}
		if status == domain.WasteApplied {
			if err := insertWasteMovements(ctx, q, rec.ID, branch, cmd.Actor, plan.deltas, movementCost(cmd.ProductID, plan.unitCost)); err != nil {
				return err
			}
		}
		out = WasteResult{ID: rec.ID, Status: string(status), TotalCost: plan.total}
		return auditEntity(ctx, q, domain.AuditWasteRecorded, "waste_record", rec.ID, &branch, &cmd.Actor, nil,
			map[string]any{"status": status, "quantity": plan.quantity, "totalCost": plan.total, "reason": reason.Name})
	})
	return out, err
}

// wastePlan es lo que una merma restaría: los movimientos y su costo.
type wastePlan struct {
	quantity decimal.Decimal // en la unidad base del insumo, o piezas del producto
	deltas   []domain.StockDelta
	unitCost *decimal.Decimal
	total    *decimal.Decimal
}

func (p wastePlan) ingredientIDs() []int64 {
	var ids []int64
	for _, d := range p.deltas {
		if d.IngredientID != nil && !slices.Contains(ids, *d.IngredientID) {
			ids = append(ids, *d.IngredientID)
		}
	}
	slices.Sort(ids)
	return ids
}

// planWaste calcula lo que resta una merma con el costo de este momento. Un producto resta lo mismo
// que venderlo, por la misma función que la venta (`ExpandSale`): una regla propia aquí divergiría.
func planWaste(ctx context.Context, q *db.Queries, cmd RecordWasteCmd, entered decimal.Decimal) (wastePlan, error) {
	if cmd.ProductID != nil {
		prod, err := q.ProductCostForWaste(ctx, *cmd.ProductID)
		if err != nil {
			return wastePlan{}, notFound(err)
		}
		graph, err := loadStockGraph(ctx, q)
		if err != nil {
			return wastePlan{}, err
		}
		deltas, _ := graph.ExpandSale(domain.SaleLine{ProductID: prod.ID, Qty: entered})
		if len(deltas) == 0 {
			return wastePlan{}, fmt.Errorf("%w: este producto no tiene qué restar; captura su receta o registra los artículos", domain.ErrValidation)
		}
		return withCost(wastePlan{quantity: entered, deltas: deltas}, prod.CurrentCost)
	}
	base := entered
	ing, err := q.IngredientForWaste(ctx, db.IngredientForWasteParams{ID: *cmd.IngredientID, UnitID: cmd.UnitID})
	if err != nil {
		return wastePlan{}, notFound(err)
	}
	if cmd.PresentationID != nil {
		p, err := q.GetPresentationForLine(ctx, *cmd.PresentationID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.IngredientID != ing.ID) {
			return wastePlan{}, fmt.Errorf("%w: ese empaque no es de este artículo", domain.ErrValidation)
		}
		if err != nil {
			return wastePlan{}, err
		}
		base = base.Mul(p.QtyInBase)
	} else {
		if !ing.SameKind {
			return wastePlan{}, fmt.Errorf("%w: esa unidad no es del tipo del artículo", domain.ErrValidation)
		}
		base = base.Mul(ing.Factor)
	}
	base = domain.Round4(base)
	if !domain.ValidQty(base, domain.MaxStockQty, false) {
		return wastePlan{}, fmt.Errorf("%w: la cantidad es demasiado grande", domain.ErrValidation)
	}
	id := ing.ID
	return withCost(wastePlan{quantity: base, deltas: []domain.StockDelta{{ItemType: "ingrediente", IngredientID: &id, Qty: base}}}, ing.CurrentCost)
}

// withCost pone el costo de este momento. Un costo en cero es «sin costo»: guardarlo como $0
// sumaría cero en el reporte y no avisaría que falta.
func withCost(p wastePlan, unitCost decimal.Decimal) (wastePlan, error) {
	if !unitCost.IsPositive() {
		return p, nil
	}
	total := domain.Round2(p.quantity.Mul(unitCost))
	if !domain.ValidMoney(total, true) {
		return wastePlan{}, fmt.Errorf("%w: el costo de esta merma es demasiado grande", domain.ErrValidation)
	}
	uc := unitCost
	p.unitCost, p.total = &uc, &total
	return p, nil
}

// movementCost es el costo por unidad que se copia al movimiento: el del registro solo si es de un
// insumo. El de un producto es por pieza, y en una receta de un solo insumo acababa valuando el
// gramo como el producto entero (revisión de código).
func movementCost(productID *int64, unitCost *decimal.Decimal) *decimal.Decimal {
	if productID != nil {
		return nil
	}
	return unitCost
}

func insertWasteMovements(ctx context.Context, q *db.Queries, recordID, branch, actor int64, deltas []domain.StockDelta, unitCost *decimal.Decimal) error {
	reason := wasteMovementReason
	for _, d := range deltas {
		qty := domain.Round4(d.Qty.Neg())
		if qty.IsZero() {
			continue
		}
		p := db.InsertWasteMovementParams{
			ItemType: db.StockItemType(d.ItemType), IngredientID: d.IngredientID, ProductID: d.ProductID, Quantity: qty,
			WasteRecordID: &recordID, UserID: &actor, Reason: &reason, BranchID: branch,
			ModifierOptionID: d.OptionID, ComponentOfProductID: d.ComponentOf,
		}
		if d.ProductID == nil && len(deltas) == 1 {
			p.UnitCost = unitCost
		}
		if err := q.InsertWasteMovement(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func activeWasteReason(ctx context.Context, q *db.Queries, id int64) (db.GetWasteReasonRow, error) {
	r, err := q.GetWasteReason(ctx, id)
	if err != nil {
		return r, notFound(err)
	}
	if r.ArchivedAt.Valid || r.MergedIntoID != nil {
		return r, fmt.Errorf("%w: ese motivo ya no se usa; elige otro", domain.ErrValidation)
	}
	return r, nil
}

func wasteThreshold(ctx context.Context, q *db.Queries) (*decimal.Decimal, error) {
	t, err := q.WasteApprovalThreshold(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func wasteNote(n *string) (*string, error) {
	if n == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*n)
	if v == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(v) > 200 {
		return nil, fmt.Errorf("%w: la nota lleva hasta 200 letras", domain.ErrValidation)
	}
	return &v, nil
}

// lockWaste toma el registro para cambiarlo y valida la transición en el dominio.
func lockWaste(ctx context.Context, q *db.Queries, id int64, act domain.WasteAction) (db.LockWasteRecordRow, domain.WasteStatus, error) {
	r, err := q.LockWasteRecord(ctx, id)
	if err != nil {
		return r, "", notFound(err)
	}
	next, err := domain.NextWasteStatus(domain.WasteStatus(r.Status), act)
	return r, next, err
}

// ApproveWaste aprueba una merma por aprobar: resta en ese momento y, si no tenía costo, toma el de
// hoy (si no, quedaría sin costo para siempre).
func (s *BackofficeService) ApproveWaste(ctx context.Context, id, actor int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		r, next, err := lockWaste(ctx, q, id, domain.WasteApprove)
		if err != nil {
			return err
		}
		cmd := RecordWasteCmd{IngredientID: r.IngredientID, ProductID: r.ProductID}
		plan, err := planWaste(ctx, q, cmd, r.Quantity)
		if err != nil {
			return err
		}
		if err := lockIngredients(ctx, q, plan.ingredientIDs()); err != nil {
			return err
		}
		unitCost, total := r.UnitCost, r.TotalCost
		if unitCost == nil {
			unitCost, total = plan.unitCost, plan.total
		}
		// Un conteo abierto que ya capturó el artículo ajustará esta pérdida al cerrar: aprobarla
		// también la restaría dos veces. Queda cubierta por ese conteo, como al cerrarlo.
		seenBy, err := q.OpenCountThatSaw(ctx, db.OpenCountThatSawParams{BranchID: r.BranchID, Ids: plan.ingredientIDs(), Since: r.RecordedAt})
		if err != nil {
			return err
		}
		if seenBy != 0 {
			n, err := q.ResolveWasteRecord(ctx, db.ResolveWasteRecordParams{Status: string(domain.WasteCovered), ResolvedBy: &actor,
				UnitCost: unitCost, TotalCost: total, CoveredByCountID: &seenBy, ID: id, FromStatus: r.Status})
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("%w: esta merma ya se resolvió", domain.ErrConflict)
			}
			branch := r.BranchID
			return auditEntity(ctx, q, domain.AuditWasteApproved, "waste_record", id, &branch, &actor, nil,
				map[string]any{"totalCost": total, "coveredByCount": seenBy})
		}
		n, err := q.ResolveWasteRecord(ctx, db.ResolveWasteRecordParams{Status: string(next), ResolvedBy: &actor,
			UnitCost: unitCost, TotalCost: total, ID: id, FromStatus: r.Status})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: esta merma ya se resolvió", domain.ErrConflict)
		}
		if err := insertWasteMovements(ctx, q, id, r.BranchID, actor, plan.deltas, movementCost(r.ProductID, unitCost)); err != nil {
			return err
		}
		branch := r.BranchID
		return auditEntity(ctx, q, domain.AuditWasteApproved, "waste_record", id, &branch, &actor, nil,
			map[string]any{"totalCost": total})
	})
}

// RejectWaste rechaza una merma por aprobar: nunca restó y no resta.
func (s *BackofficeService) RejectWaste(ctx context.Context, id, actor int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		r, next, err := lockWaste(ctx, q, id, domain.WasteReject)
		if err != nil {
			return err
		}
		if _, err := q.ResolveWasteRecord(ctx, db.ResolveWasteRecordParams{Status: string(next), ResolvedBy: &actor,
			ID: id, FromStatus: r.Status}); err != nil {
			return err
		}
		branch := r.BranchID
		return auditEntity(ctx, q, domain.AuditWasteRejected, "waste_record", id, &branch, &actor, nil, nil)
	})
}

// UndoWaste deshace una merma: devuelve exactamente lo que restó, con su movimiento inverso. Lo
// deshace quien la registró, o quien administra el almacén (`canConfigure`). Solo el mismo día y si
// ese artículo no se contó después: el conteo ya corrigió lo guardado.
func (s *BackofficeService) UndoWaste(ctx context.Context, id, actor int64, canConfigure bool) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		r, next, err := lockWaste(ctx, q, id, domain.WasteUndo)
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if r.RecordedBy != actor && !canConfigure {
			return fmt.Errorf("%w: solo quien la registró la deshace", domain.ErrForbidden)
		}
		if err != nil {
			return err
		}
		var moves []db.WasteMovementsRow
		if r.Status == string(domain.WasteApplied) {
			moves, err = q.WasteMovements(ctx, &id)
			if err != nil {
				return err
			}
			var ids []int64
			for _, m := range moves {
				if m.IngredientID != nil && !slices.Contains(ids, *m.IngredientID) {
					ids = append(ids, *m.IngredientID)
				}
			}
			slices.Sort(ids)
			counted, err := q.IngredientsCountedSince(ctx, db.IngredientsCountedSinceParams{BranchID: r.BranchID, Ids: ids, Since: r.RecordedAt})
			if err != nil {
				return err
			}
			now, err := q.DBNow(ctx)
			if err != nil {
				return err
			}
			if err := domain.WasteCanUndo(r.RecordedAt, now, s.location(ctx), counted); err != nil {
				return err
			}
			if err := lockIngredients(ctx, q, ids); err != nil {
				return err
			}
		}
		if _, err := q.ResolveWasteRecord(ctx, db.ResolveWasteRecordParams{Status: string(next), ResolvedBy: &actor,
			ID: id, FromStatus: r.Status}); err != nil {
			return err
		}
		reason := wasteMovementReason
		for _, m := range moves {
			if err := q.InsertWasteMovement(ctx, db.InsertWasteMovementParams{
				ItemType: m.ItemType, IngredientID: m.IngredientID, ProductID: m.ProductID, Quantity: m.Quantity.Neg(),
				UnitCost: m.UnitCost, WasteRecordID: &id, UserID: &actor, Reason: &reason, BranchID: r.BranchID,
				ModifierOptionID: m.ModifierOptionID, ComponentOfProductID: m.ComponentOfProductID,
			}); err != nil {
				return err
			}
		}
		branch := r.BranchID
		return auditEntity(ctx, q, domain.AuditWasteUndone, "waste_record", id, &branch, &actor,
			map[string]any{"status": r.Status}, nil)
	})
}

// location es la zona del local, para decidir qué es «el mismo día».
func (s *BackofficeService) location(ctx context.Context) *time.Location {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.LoadBusinessLocation(tz)
}

// WasteRow es un renglón de las listas de merma.
type WasteRow struct {
	ID           int64            `json:"id"`
	ItemType     string           `json:"itemType"`
	ItemName     string           `json:"itemName"`
	Quantity     decimal.Decimal  `json:"quantity"`
	UnitCode     string           `json:"unitCode"`
	TotalCost    *decimal.Decimal `json:"totalCost"`
	Status       string           `json:"status"`
	Reason       string           `json:"reason"`
	RecordedBy   string           `json:"recordedBy"`
	RecordedAt   time.Time        `json:"recordedAt"`
	CanUndo      bool             `json:"canUndo"`
	RecordedByID int64            `json:"-"`
	ReportReason int64            `json:"reportReasonId,omitempty"`
	IsStaffMeal  bool             `json:"isStaffMeal,omitempty"`
	Note         *string          `json:"note,omitempty"`
}

// WasteToday lista lo registrado hoy en la sucursal.
func (s *BackofficeService) WasteToday(ctx context.Context) ([]WasteRow, error) {
	loc := s.location(ctx)
	now, err := s.store.QC(ctx).DBNow(ctx)
	if err != nil {
		return nil, err
	}
	y, m, d := now.In(loc).Date()
	rows, err := s.store.QC(ctx).WasteOfDay(ctx, time.Date(y, m, d, 0, 0, 0, 0, loc))
	if err != nil {
		return nil, err
	}
	out := make([]WasteRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, WasteRow{ID: r.ID, ItemType: string(r.ItemType), ItemName: r.ItemName, Quantity: r.Quantity,
			UnitCode: r.UnitCode, TotalCost: r.TotalCost, Status: r.Status, Reason: r.ReasonName,
			RecordedBy: r.RecordedByName, RecordedAt: r.RecordedAt, RecordedByID: r.RecordedBy, Note: r.Note,
			CanUndo: r.Status == string(domain.WasteApplied) || r.Status == string(domain.WastePending)})
	}
	return out, nil
}

// PendingWaste lista las mermas por aprobar de la sucursal.
func (s *BackofficeService) PendingWaste(ctx context.Context) ([]WasteRow, error) {
	rows, err := s.store.QC(ctx).PendingWaste(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WasteRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, WasteRow{ID: r.ID, ItemType: string(r.ItemType), ItemName: r.ItemName, Quantity: r.Quantity,
			UnitCode: r.UnitCode, TotalCost: r.TotalCost, Status: string(domain.WastePending), Reason: r.ReasonName,
			RecordedBy: r.RecordedByName, RecordedAt: r.RecordedAt})
	}
	return out, nil
}

// WasteReason es un motivo de la empresa.
type WasteReason struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsStaffMeal bool   `json:"isStaffMeal"`
	Uses        int32  `json:"uses"`
}

// WasteReasons lista los motivos que se ofrecen, los más usados primero.
func (s *BackofficeService) WasteReasons(ctx context.Context) ([]WasteReason, error) {
	rows, err := s.store.QC(ctx).ListActiveWasteReasons(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WasteReason, 0, len(rows))
	for _, r := range rows {
		out = append(out, WasteReason{ID: r.ID, Name: r.Name, Description: r.Description, IsStaffMeal: r.IsStaffMeal, Uses: r.Uses})
	}
	return out, nil
}

// CreateWasteReason crea un motivo. Si ya existe uno con ese nombre (sin distinguir mayúsculas,
// acentos ni espacios), responde con él como conflicto para que la pantalla lo ofrezca.
func (s *BackofficeService) CreateWasteReason(ctx context.Context, name, description string, actor int64) (int64, error) {
	if err := domain.ValidWasteReason(name, description); err != nil {
		return 0, err
	}
	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		existing, err := q.FindWasteReasonByName(ctx, name)
		if err == nil {
			return domain.WasteReasonExists{ID: existing}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		id, err = q.InsertWasteReason(ctx, db.InsertWasteReasonParams{
			Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), CreatedBy: &actor})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Otra tableta lo creó entre la búsqueda y el insert: el índice único de la 0094 manda.
			return fmt.Errorf("%w: ese motivo ya existe", domain.ErrConflict)
		}
		return err
	})
	return id, err
}

// SetWasteThreshold cambia el tope a partir del cual una merma espera aprobación. Nulo o vacío:
// ninguna espera.
func (s *BackofficeService) SetWasteThreshold(ctx context.Context, threshold *string, actor int64) error {
	var value *decimal.Decimal
	if threshold != nil && strings.TrimSpace(*threshold) != "" {
		v, err := decimal.NewFromString(strings.TrimSpace(*threshold))
		if err != nil || !v.IsPositive() || !domain.ValidMoney(v, false) {
			return fmt.Errorf("%w: el tope es un monto mayor que 0", domain.ErrValidation)
		}
		v = domain.Round2(v)
		value = &v
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		before, err := wasteThreshold(ctx, q)
		if err != nil {
			return err
		}
		n, err := q.SetWasteApprovalThreshold(ctx, value)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: el negocio no tiene configuración", domain.ErrNotFound)
		}
		return auditEntity(ctx, q, domain.AuditSettingsWasteThreshold, "business_settings", 0, nil, &actor,
			map[string]any{"threshold": before}, map[string]any{"threshold": value})
	})
}

// WasteItemUnit es una unidad en la que se puede capturar un artículo (del mismo tipo que su base).
type WasteItemUnit struct {
	ID   int16  `json:"id"`
	Code string `json:"code"`
}

// WastePack es un empaque del artículo, con lo que trae en su unidad base.
type WastePack struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	QtyInBase decimal.Decimal `json:"qtyInBase"`
}

// WasteIngredient es un artículo que se puede registrar.
type WasteIngredient struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	UnitCode string          `json:"unitCode"`
	Units    []WasteItemUnit `json:"units"`
	Packs    []WastePack     `json:"packs"`
}

// WasteProduct es un producto que se puede registrar (en piezas).
type WasteProduct struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// WasteRecent es algo registrado hace poco en la sucursal.
type WasteRecent struct {
	ItemType string `json:"itemType"`
	ID       int64  `json:"id"`
}

// WasteItems es todo lo que la captura ofrece. Ruta propia: la usan cajero y cocina, que no entran
// al catálogo de insumos ni a los empaques de compra.
type WasteItems struct {
	Ingredients []WasteIngredient `json:"ingredients"`
	Products    []WasteProduct    `json:"products"`
	Recent      []WasteRecent     `json:"recent"`
}

// WasteItemsForCapture arma la lista de la captura. Toda lista va como [] aunque esté vacía.
func (s *BackofficeService) WasteItemsForCapture(ctx context.Context) (WasteItems, error) {
	q := s.store.QC(ctx)
	ings, err := q.WasteIngredients(ctx)
	if err != nil {
		return WasteItems{}, err
	}
	packs, err := q.WastePresentations(ctx)
	if err != nil {
		return WasteItems{}, err
	}
	units, err := q.WasteUnits(ctx)
	if err != nil {
		return WasteItems{}, err
	}
	prods, err := q.WasteProducts(ctx)
	if err != nil {
		return WasteItems{}, err
	}
	recent, err := q.RecentWasteItems(ctx)
	if err != nil {
		return WasteItems{}, err
	}
	byIng := map[int64][]WastePack{}
	for _, p := range packs {
		byIng[p.IngredientID] = append(byIng[p.IngredientID], WastePack{ID: p.ID, Name: p.Name, QtyInBase: p.QtyInBase})
	}
	out := WasteItems{Ingredients: make([]WasteIngredient, 0, len(ings)), Products: make([]WasteProduct, 0, len(prods)),
		Recent: make([]WasteRecent, 0, len(recent))}
	for _, i := range ings {
		us := []WasteItemUnit{}
		for _, u := range units {
			if u.Kind == i.UnitKind {
				us = append(us, WasteItemUnit{ID: u.ID, Code: u.Code})
			}
		}
		ps := byIng[i.ID]
		if ps == nil {
			ps = []WastePack{}
		}
		out.Ingredients = append(out.Ingredients, WasteIngredient{ID: i.ID, Name: i.Name, UnitCode: i.UnitCode, Units: us, Packs: ps})
	}
	for _, p := range prods {
		out.Products = append(out.Products, WasteProduct{ID: p.ID, Name: p.Name})
	}
	for _, r := range recent {
		out.Recent = append(out.Recent, WasteRecent{ItemType: r.ItemType, ID: r.ItemID})
	}
	return out, nil
}
