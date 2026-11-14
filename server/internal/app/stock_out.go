package app

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// «Se acabó» y «No lo encuentro» (spec 084). Quien vende avisa con un toque que algo se acabó: la
// existencia queda en 0 con un ajuste fijo (no acepta cantidad) y se abre el aviso. Es la vía
// confiable desde el primer día: no depende de que existencias y mínimos estén bien cargados.

// Motivos fijos de los ajustes. Un «se acabó» es ajuste, nunca venta: no cuenta como consumo.
const (
	outReason     = "se acabó"
	outUndoReason = "se acabó (deshecho)"
)

// StockOutService reporta lo que se acabó o no se encuentra.
type StockOutService struct {
	store *store.Store
	now   func() time.Time
}

// NewStockOutService arma el servicio; now es el reloj de la API (la ventana de deshacer usa el de la
// base, que es el que guarda reported_at).
func NewStockOutService(s *store.Store, now func() time.Time) *StockOutService {
	if now == nil {
		now = time.Now
	}
	return &StockOutService{store: s, now: now}
}

// OutItem es un artículo que se puede reportar.
type OutItem struct {
	ItemType string `json:"itemType"`
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	OpenOut  bool   `json:"openOut"`
	Times    int    `json:"times,omitempty"`
}

// Search busca por nombre u otros nombres, sin acentos ni plural.
func (s *StockOutService) Search(ctx context.Context, q string) ([]OutItem, error) {
	keys := domain.SearchKeys(q)
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: escribe parte del nombre", domain.ErrValidation)
	}
	rows, err := s.store.QC(ctx).SearchOutArticles(ctx, keys)
	if err != nil {
		return nil, err
	}
	out := make([]OutItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, OutItem{ItemType: string(r.ItemType), ID: r.ID, Name: r.Name, Unit: r.UnitCode, OpenOut: r.OpenOut})
	}
	return out, nil
}

// OutReportRow es un renglón de «Reportado hoy».
type OutReportRow struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	Name           string     `json:"name"`
	ReportedAt     time.Time  `json:"reportedAt"`
	ReportedBy     int64      `json:"reportedById"`
	ReportedByName string     `json:"reportedBy"`
	UndoneAt       *time.Time `json:"undoneAt"`
	Moved          bool       `json:"moved"`
	Open           bool       `json:"open"`
}

// OutSuggestions son los mosaicos: lo que más se acaba y lo reportado hoy.
type OutSuggestions struct {
	Frequent []OutItem      `json:"frequent"`
	Today    []OutReportRow `json:"today"`
	// UndoSeconds es cuánto dura el deshacer, para que la pantalla no lo adivine.
	UndoSeconds int `json:"undoSeconds"`
}

// Suggestions devuelve lo que más se reportó en 30 días y lo de hoy en la sucursal.
func (s *StockOutService) Suggestions(ctx context.Context) (OutSuggestions, error) {
	q := s.store.QC(ctx)
	now, err := q.DBNow(ctx)
	if err != nil {
		return OutSuggestions{}, err
	}
	loc := businessLocation(ctx, q)
	y, m, d := now.In(loc).Date()
	freq, err := q.FrequentOutArticles(ctx, now.AddDate(0, 0, -30))
	if err != nil {
		return OutSuggestions{}, err
	}
	today, err := q.OutReportsSince(ctx, time.Date(y, m, d, 0, 0, 0, 0, loc))
	if err != nil {
		return OutSuggestions{}, err
	}
	out := OutSuggestions{Frequent: make([]OutItem, 0, len(freq)), Today: make([]OutReportRow, 0, len(today)),
		UndoSeconds: int(domain.OutReportUndoWindow().Seconds())}
	for _, r := range freq {
		out.Frequent = append(out.Frequent, OutItem{ItemType: itemTypeName(r.ItemType), ID: r.ID, Name: r.Name, Times: int(r.Times)})
	}
	for _, r := range today {
		row := OutReportRow{ID: r.ID, Kind: r.Kind, Name: r.Name, ReportedAt: r.ReportedAt, ReportedBy: r.ReportedBy,
			ReportedByName: r.ReportedByName, Moved: r.Moved, Open: r.NotificationStatus == "open"}
		if r.UndoneAt.Valid {
			t := r.UndoneAt.Time
			row.UndoneAt = &t
		}
		out.Today = append(out.Today, row)
	}
	return out, nil
}

// ReportOutCmd es un «se acabó». No lleva cantidad: deja la existencia en 0.
type ReportOutCmd struct {
	ItemType  string
	ID        int64
	ClientKey uuid.UUID
	Actor     int64
}

// OutReportResult es lo que la pantalla necesita para el aviso verde y el deshacer.
type OutReportResult struct {
	ID             int64            `json:"id"`
	Kind           string           `json:"kind"`
	NotificationID int64            `json:"notificationId"`
	Delta          *decimal.Decimal `json:"delta"`
	UndoUntil      time.Time        `json:"undoUntil"`
	Confirmations  int              `json:"confirmations"`
}

func itemTypeOf(s string) (db.StockItemType, error) {
	switch s {
	case "ingrediente", "producto":
		return db.StockItemType(s), nil
	}
	return "", fmt.Errorf("%w: tipo de artículo desconocido", domain.ErrValidation)
}

func itemTypeName(t *db.StockItemType) string {
	if t == nil {
		return ""
	}
	return string(*t)
}

func itemIDs(t db.StockItemType, id int64) (ing, prod *int64) {
	if t == "ingrediente" {
		return &id, nil
	}
	return nil, &id
}

// ReportOut deja el artículo en 0, abre (o confirma) el aviso y lo audita, en una transacción.
// created es falso cuando la clave ya existía: se devuelve el reporte guardado.
//
// Orden de candados: primero el insumo y su existencia (como una venta), después el aviso. Al revés,
// una venta del mismo artículo cerraría un ciclo con este reporte (research R12).
func (s *StockOutService) ReportOut(ctx context.Context, cmd ReportOutCmd) (OutReportResult, bool, error) {
	itemType, err := itemTypeOf(cmd.ItemType)
	if err != nil {
		return OutReportResult{}, false, err
	}
	ing, prod := itemIDs(itemType, cmd.ID)
	var out OutReportResult
	created := false
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if prev, ok, err := previousReport(ctx, q, cmd.ClientKey); err != nil || ok {
			out = prev
			return err
		}
		if err := reportLimit(ctx, q, cmd.Actor); err != nil {
			return err
		}
		art, err := q.OutArticle(ctx, db.OutArticleParams{ItemType: itemType, ID: cmd.ID})
		if err != nil {
			return notFound(err)
		}
		if !art.IsActive {
			return domain.ErrNotFound
		}
		branch, err := q.CurrentBranchID(ctx)
		if err != nil {
			return err
		}
		if ing != nil {
			if err := lockIngredients(ctx, q, []int64{*ing}); err != nil {
				return err
			}
		}
		existing, err := q.OpenOutNotification(ctx, db.OpenOutNotificationParams{BranchID: branch, IngredientID: ing, ProductID: prod})
		alreadyOpen := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		kind := "confirmation"
		var movement *int64
		var delta *decimal.Decimal
		notification := existing
		if !alreadyOpen {
			kind = "report"
			levels, err := q.StockOnHandForUpdate(ctx, db.StockOnHandForUpdateParams{BranchID: branch, IngredientID: ing, ProductID: prod})
			if err != nil {
				return err
			}
			onHand := decimal.Zero
			if len(levels) > 0 {
				onHand = levels[0]
			}
			if !onHand.IsZero() {
				d := onHand.Neg()
				id, err := q.InsertOutAdjustment(ctx, db.InsertOutAdjustmentParams{ItemType: itemType, IngredientID: ing,
					ProductID: prod, Quantity: d, UserID: &cmd.Actor, Reason: ptr(outReason), BranchID: branch})
				if err != nil {
					return err
				}
				movement, delta = &id, &d
			}
			ids, err := q.OpenManualOut(ctx, db.OpenManualOutParams{BranchID: branch, ItemType: &itemType, IngredientID: ing,
				ProductID: prod, ItemName: &art.Name, UnitCode: &art.UnitCode, OnHandSeen: &onHand, OpenedBy: &cmd.Actor})
			if err != nil {
				return err
			}
			if len(ids) == 1 {
				notification = ids[0]
			} else {
				// El ajuste a 0 lo acaba de abrir el trigger (un artículo con alerta «al acabarse»): el
				// reporte se cuelga de ese aviso y lo deja a su nombre, o la franja le saldría a quien
				// reportó y el panel diría que lo reportó el sistema.
				if notification, err = q.ClaimTriggerOut(ctx, db.ClaimTriggerOutParams{OpenedBy: &cmd.Actor, BranchID: branch, IngredientID: ing, ProductID: prod}); err != nil {
					return err
				}
			}
		}
		rep, err := q.InsertOutReport(ctx, db.InsertOutReportParams{BranchID: branch, Kind: kind, ItemType: &itemType,
			IngredientID: ing, ProductID: prod, NotificationID: notification, MovementID: movement, Delta: delta,
			ClientKey: cmd.ClientKey, ReportedBy: cmd.Actor})
		if err != nil {
			return err
		}
		live, err := q.LiveReportsOnNotification(ctx, notification)
		if err != nil {
			return err
		}
		if err := q.NotifyNotificationsChanged(ctx); err != nil {
			return err
		}
		out = OutReportResult{ID: rep.ID, Kind: kind, NotificationID: notification, Delta: delta,
			UndoUntil: rep.ReportedAt.Add(domain.OutReportUndoWindow()), Confirmations: int(live)}
		created = true
		return auditEntity(ctx, q, domain.AuditStockOutReported, "stock_out_report", rep.ID, &branch, &cmd.Actor, nil,
			map[string]any{"item": art.Name, "itemType": itemType, "kind": kind, "delta": delta})
	})
	if err != nil {
		return OutReportResult{}, false, lockErr(err)
	}
	return out, created, nil
}

// previousReport devuelve el reporte de una clave ya usada: repetir la petición devuelve lo mismo.
func previousReport(ctx context.Context, q *db.Queries, key uuid.UUID) (OutReportResult, bool, error) {
	r, err := q.GetOutReportByClientKey(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutReportResult{}, false, nil
	}
	if err != nil {
		return OutReportResult{}, false, err
	}
	live, err := q.LiveReportsOnNotification(ctx, r.NotificationID)
	if err != nil {
		return OutReportResult{}, false, err
	}
	return OutReportResult{ID: r.ID, Kind: r.Kind, NotificationID: r.NotificationID, Delta: r.Delta,
		UndoUntil: r.ReportedAt.Add(domain.OutReportUndoWindow()), Confirmations: int(live)}, true, nil
}

func reportLimit(ctx context.Context, q *db.Queries, actor int64) error {
	now, err := q.DBNow(ctx)
	if err != nil {
		return err
	}
	n, err := q.CountUserReportsSince(ctx, db.CountUserReportsSinceParams{UserID: actor, Since: now.Add(-time.Hour)})
	if err != nil {
		return err
	}
	if domain.ReportLimitReached(int(n)) {
		return domain.ErrReportLimit
	}
	return nil
}

// UndoOut repone lo que restó un «se acabó» si no pasaron los 90 segundos (reloj de la base) y nada
// movió el artículo después. Lo deshace quien lo reportó, o quien configura el almacén.
func (s *StockOutService) UndoOut(ctx context.Context, id, actor int64, canConfigure bool) error {
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		r, err := q.GetOutReportForUndo(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if r.Kind == "not_found" || r.UndoneAt.Valid {
			return domain.ErrNotFound
		}
		if r.ReportedBy != actor && !canConfigure {
			return fmt.Errorf("%w: solo quien lo reportó lo puede deshacer", domain.ErrForbidden)
		}
		now, err := q.DBNow(ctx)
		if err != nil {
			return err
		}
		if !domain.CanUndoOutReport(r.ReportedAt, now) {
			return domain.ErrUndoExpired
		}
		// Si otros confirmaron que se acabó, deshacer el reporte propio no repone nada: sigue acabado.
		others, err := q.LiveReportsOnNotification(ctx, r.NotificationID)
		if err != nil {
			return err
		}
		others--
		var undoMovement int64
		if r.MovementID != nil && others == 0 {
			if r.IngredientID != nil {
				if err := lockIngredients(ctx, q, []int64{*r.IngredientID}); err != nil {
					return err
				}
			}
			moved, err := q.ArticleMovedSince(ctx, db.ArticleMovedSinceParams{BranchID: r.BranchID, MovementID: *r.MovementID,
				IngredientID: r.IngredientID, ProductID: r.ProductID, ReportedAt: r.ReportedAt})
			if err != nil {
				return err
			}
			if moved {
				return domain.ErrUndoAfterMovement
			}
			if undoMovement, err = q.InsertOutAdjustment(ctx, db.InsertOutAdjustmentParams{ItemType: *r.ItemType,
				IngredientID: r.IngredientID, ProductID: r.ProductID, Quantity: r.Delta.Neg(), UserID: &actor,
				Reason: ptr(outUndoReason), BranchID: r.BranchID}); err != nil {
				return err
			}
		}
		if err := q.MarkOutReportUndone(ctx, db.MarkOutReportUndoneParams{ID: id, UndoMovementID: nullID(undoMovement)}); err != nil {
			return err
		}
		// Después del ajuste de vuelta: la existencia antes que el aviso, como una venta (R12).
		if others == 0 {
			if err := q.DismissAsUndone(ctx, db.DismissAsUndoneParams{ID: r.NotificationID, ResolvedBy: &actor}); err != nil {
				return err
			}
		}
		if err := q.NotifyNotificationsChanged(ctx); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditStockOutUndone, "stock_out_report", id, &r.BranchID, &actor, nil,
			map[string]any{"delta": r.Delta})
	})
	return lockErr(err)
}

// NotFoundResult es el aviso de «No lo encuentro».
type NotFoundResult struct {
	ID             int64 `json:"id"`
	NotificationID int64 `json:"notificationId"`
}

// ReportNotFound guarda lo que se escribió y avisa a quien configura el almacén. No toca existencias.
func (s *StockOutService) ReportNotFound(ctx context.Context, text string, key uuid.UUID, actor int64) (NotFoundResult, bool, error) {
	clean, err := domain.CleanNotFoundText(text)
	if err != nil {
		return NotFoundResult{}, false, err
	}
	var out NotFoundResult
	created := false
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if prev, ok, err := previousReport(ctx, q, key); err != nil || ok {
			out = NotFoundResult{ID: prev.ID, NotificationID: prev.NotificationID}
			return err
		}
		if err := reportLimit(ctx, q, actor); err != nil {
			return err
		}
		branch, err := q.CurrentBranchID(ctx)
		if err != nil {
			return err
		}
		n, err := q.OpenNotFound(ctx, db.OpenNotFoundParams{BranchID: branch, QueryText: &clean, OpenedBy: &actor})
		if err != nil {
			return err
		}
		rep, err := q.InsertOutReport(ctx, db.InsertOutReportParams{BranchID: branch, Kind: "not_found",
			NotificationID: n, ClientKey: key, ReportedBy: actor})
		if err != nil {
			return err
		}
		out, created = NotFoundResult{ID: rep.ID, NotificationID: n}, true
		return q.NotifyNotificationsChanged(ctx)
	})
	return out, created, err
}

func nullID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

// businessLocation es la zona del negocio (la configurada, o la del centro de México).
func businessLocation(ctx context.Context, q *db.Queries) *time.Location {
	tz, err := q.GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.LoadBusinessLocation(tz)
}
