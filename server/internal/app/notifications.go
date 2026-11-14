package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El centro de avisos (spec 084): la campana con lo no leído por persona, la franja y cerrar un aviso a
// mano. Los avisos los abre el trigger de existencias o «Se acabó»; aquí se leen y se cierran.

// NotificationsService lee y cierra avisos, y guarda la configuración de alertas y de destinatarios.
type NotificationsService struct {
	store *store.Store
	now   func() time.Time
}

// NewNotificationsService arma el servicio.
func NewNotificationsService(s *store.Store, now func() time.Time) *NotificationsService {
	if now == nil {
		now = time.Now
	}
	return &NotificationsService{store: s, now: now}
}

// NotificationView es un aviso como lo pinta la campana.
type NotificationView struct {
	ID             int64            `json:"id"`
	Kind           string           `json:"kind"`
	Status         string           `json:"status"`
	ItemType       string           `json:"itemType,omitempty"`
	ItemID         int64            `json:"itemId,omitempty"`
	ItemName       string           `json:"itemName,omitempty"`
	UnitCode       string           `json:"unitCode,omitempty"`
	QueryText      string           `json:"queryText,omitempty"`
	BranchName     string           `json:"branchName"`
	OnHandSeen     *decimal.Decimal `json:"onHandSeen"`
	MinSeen        *decimal.Decimal `json:"minSeen"`
	NeedsCount     bool             `json:"needsCount"`
	OpenedAt       time.Time        `json:"openedAt"`
	OpenedBy       *int64           `json:"openedById"`
	OpenedByName   string           `json:"openedByName,omitempty"`
	ResolvedAt     *time.Time       `json:"resolvedAt"`
	ResolvedByName string           `json:"resolvedByName,omitempty"`
	ResolvedReason string           `json:"resolvedReason,omitempty"`
	Read           bool             `json:"read"`
	Banner         bool             `json:"banner"`
	Confirmations  int              `json:"confirmations"`
}

func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// seesNotFound: «No lo encuentro» solo lo ve quien configura el almacén (decisión del dueño: llega a
// quien puede darlo de alta).
func seesNotFound(perms []domain.Permission) bool {
	return slices.Contains(perms, domain.PermInventoryConfigure)
}

// List devuelve los avisos abiertos (o los cerrados), los más recientes primero. Nunca nil.
func (s *NotificationsService) List(ctx context.Context, user int64, perms []domain.Permission, open bool, limit int) ([]NotificationView, error) {
	if limit <= 0 || limit > 200 {
		return nil, fmt.Errorf("%w: el límite va de 1 a 200", domain.ErrValidation)
	}
	rows, err := s.store.QC(ctx).ListNotifications(ctx, db.ListNotificationsParams{UserID: user, Open: open,
		IncludeNotFound: seesNotFound(perms), Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]NotificationView, 0, len(rows))
	for _, r := range rows {
		v := NotificationView{ID: r.ID, Kind: r.Kind, Status: r.Status, BranchName: r.BranchName, OnHandSeen: r.OnHandSeen,
			MinSeen: r.MinSeen, NeedsCount: r.NeedsCount, OpenedAt: r.OpenedAt, OpenedBy: r.OpenedBy, OpenedByName: r.OpenedByName,
			ResolvedByName: r.ResolvedByName, ResolvedReason: strOf(r.ResolvedReason), Read: r.IsRead, Banner: r.Banner,
			Confirmations: int(r.Confirmations), ItemName: strOf(r.ItemName), UnitCode: strOf(r.UnitCode), QueryText: strOf(r.QueryText)}
		if r.ItemType != nil {
			v.ItemType = string(*r.ItemType)
		}
		if r.IngredientID != nil {
			v.ItemID = *r.IngredientID
		} else if r.ProductID != nil {
			v.ItemID = *r.ProductID
		}
		if r.ResolvedAt.Valid {
			t := r.ResolvedAt.Time
			v.ResolvedAt = &t
		}
		out = append(out, v)
	}
	return out, nil
}

// Unread cuenta lo abierto que la persona no ha leído.
func (s *NotificationsService) Unread(ctx context.Context, user int64, perms []domain.Permission) (int, error) {
	n, err := s.store.QC(ctx).UnreadNotifications(ctx, db.UnreadNotificationsParams{UserID: user, IncludeNotFound: seesNotFound(perms)})
	return int(n), err
}

// MarkRead marca como leídos para la persona. Un id de otra empresa se ignora (RLS).
func (s *NotificationsService) MarkRead(ctx context.Context, user int64, ids []int64) error {
	if len(ids) == 0 || len(ids) > 100 {
		return fmt.Errorf("%w: se marcan de 1 a 100 avisos", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		return q.MarkNotificationsRead(ctx, db.MarkNotificationsReadParams{UserID: user, Ids: ids})
	})
}

// BannerSeen guarda que la persona ya vio la franja de un aviso: no vuelve aunque recargue.
func (s *NotificationsService) BannerSeen(ctx context.Context, user, id int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		return q.MarkBannerSeen(ctx, db.MarkBannerSeenParams{UserID: user, ID: id})
	})
}

// ResolveCmd es cerrar un aviso a mano.
type ResolveCmd struct {
	// Action: arrived («ya llegó»), dismiss, created (se dio de alta), alias (era otro nombre).
	Action       string
	IngredientID *int64
}

// Resolve cierra un aviso a mano con su motivo, quién y bitácora. Cada acción pide su permiso y su
// tipo de aviso; una acción desconocida es 400.
func (s *NotificationsService) Resolve(ctx context.Context, user int64, perms []domain.Permission, id int64, cmd ResolveCmd) error {
	need := domain.PermInventoryConfigure
	status, reason := "resolved", domain.ReasonDismissed
	switch cmd.Action {
	case "arrived":
		need, reason = domain.PermInventoryReceive, domain.ReasonArrived
	case "dismiss":
		status = "dismissed"
	case "created":
		reason = domain.ReasonCreated
	case "alias":
		reason = domain.ReasonAlias
	default:
		return fmt.Errorf("%w: acción desconocida", domain.ErrValidation)
	}
	if !slices.Contains(perms, need) {
		return fmt.Errorf("%w: %s", domain.ErrForbidden, domain.PermissionDeniedMessage(need))
	}
	if (cmd.Action == "created" || cmd.Action == "alias") && cmd.IngredientID == nil {
		return fmt.Errorf("%w: falta el insumo", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		n, err := q.GetNotificationForUpdate(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if n.Status != "open" {
			return domain.ErrNotificationClosed
		}
		switch cmd.Action {
		case "arrived":
			if n.Kind != string(domain.KindStockOut) {
				return fmt.Errorf("%w: «ya llegó» es para lo que se acabó", domain.ErrValidation)
			}
		case "created", "alias":
			if n.Kind != string(domain.KindNotFound) {
				return fmt.Errorf("%w: solo un «No lo encuentro» se da de alta o se liga a otro nombre", domain.ErrValidation)
			}
			if cmd.Action == "alias" {
				text := strOf(n.QueryText)
				if _, err := setArticleAltNamesIn(ctx, q, *cmd.IngredientID, func(cur []string) []string {
					return append(slices.Clone(cur), text)
				}, nil); err != nil {
					return err
				}
			} else if _, err := q.OutArticle(ctx, db.OutArticleParams{ItemType: "ingrediente", ID: *cmd.IngredientID}); err != nil {
				return notFound(err)
			}
		}
		if _, err := q.ResolveNotification(ctx, db.ResolveNotificationParams{ID: id, Status: status, ResolvedBy: &user,
			ResolvedReason: ptr(string(reason))}); err != nil {
			return err
		}
		if err := q.NotifyNotificationsChanged(ctx); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditNotificationResolved, "notification", id, &n.BranchID, &user, nil,
			map[string]any{"action": cmd.Action, "kind": n.Kind, "ingredientId": cmd.IngredientID})
	})
}

// evaluateEverywhere evalúa un artículo en cada sucursal activa: para lo que cambia sin movimiento
// (el mínimo, archivar, unir, cambiar de unidad), que el trigger no ve. Va al final de la transacción
// del cambio, cuando la existencia y la configuración ya son las nuevas.
func evaluateEverywhere(ctx context.Context, q *db.Queries, itemType db.StockItemType, ing, prod *int64) error {
	branches, err := q.CompanyBranches(ctx)
	if err != nil {
		return err
	}
	for _, b := range branches {
		if err := q.EvaluateLowStock(ctx, db.EvaluateLowStockParams{BranchID: b, ItemType: itemType, IngredientID: ing, ProductID: prod}); err != nil {
			return err
		}
	}
	return nil
}

// evaluateCounted cierra como «contado» el «se acabó» de lo que un conteo incluyó (FR-019) y lo vuelve
// a evaluar con la cifra ya contada: un conteo que confirma la cifra no inserta movimiento y el trigger
// no lo ve. Va después de marcar el conteo cerrado, así lo abierto ya no pide conteo.
func evaluateCounted(ctx context.Context, q *db.Queries, countID, branch, user int64) error {
	items, err := q.CountedArticlesOf(ctx, countID)
	if err != nil {
		return err
	}
	for _, it := range items {
		ing, prod := itemIDs(it.ItemType, it.ID)
		if _, err := q.ResolveOpenOfArticle(ctx, db.ResolveOpenOfArticleParams{ResolvedBy: &user,
			ResolvedReason: ptr(string(domain.ReasonCounted)), IngredientID: ing, ProductID: prod,
			Kind: string(domain.KindStockOut), BranchID: &branch}); err != nil {
			return err
		}
		if err := q.EvaluateLowStock(ctx, db.EvaluateLowStockParams{BranchID: branch, ItemType: it.ItemType, IngredientID: ing, ProductID: prod}); err != nil {
			return err
		}
	}
	return nil
}
