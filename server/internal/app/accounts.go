package app

import (
	"context"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// AccountsService es la fila de cuentas vivas del POS (spec 030).
type AccountsService struct {
	store  *store.Store
	orders *OrdersService
}

func NewAccountsService(s *store.Store, orders *OrdersService) *AccountsService {
	return &AccountsService{store: s, orders: orders}
}

// AccountItem es una ficha de la fila: una cuenta en captura o un pedido no cerrado.
type AccountItem struct {
	Key     string     `json:"key"`
	Kind    string     `json:"kind"`
	DraftID *uuid.UUID `json:"draftId"`
	// DraftVersion es la versión de la cuenta en captura (kind=draft): la que exige descartarla.
	DraftVersion      *int32          `json:"draftVersion"`
	OrderID           *int64          `json:"orderId"`
	Number            *int            `json:"number"`
	FolioName         *string         `json:"folioName"`
	State             string          `json:"state"`
	Group             string          `json:"group"`
	KitchenReady      bool            `json:"kitchenReady"`
	PlatformID        *int16          `json:"platformId"`
	ServiceType       string          `json:"serviceType"`
	CustomerName      *string         `json:"customerName"`
	OpenedAt          time.Time       `json:"openedAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	BusinessDate      *string         `json:"businessDate"`
	Total             decimal.Decimal `json:"total"`
	Paid              decimal.Decimal `json:"paid"`
	Outstanding       decimal.Decimal `json:"outstanding"`
	LineCount         int             `json:"lineCount"`
	PendingDraftID    *uuid.UUID      `json:"pendingDraftId"`
	PendingCount      int             `json:"pendingCount"`
	ClosedWithPending bool            `json:"closedWithPending"`
}

// LiveAccounts es la respuesta de GET /pos/accounts.
type LiveAccounts struct {
	Items       []AccountItem   `json:"items"`
	Outstanding decimal.Decimal `json:"outstanding"`
	ServerTime  time.Time       `json:"serverTime"`
}

// debtWindowDays es cuánto hacia atrás mira la fila las entregadas que deben (research R-7). La hoja
// «+N» y el cierre de caja piden `olderDebts` y miran todo.
const debtWindowDays = 90

// Live lista las cuentas vivas de la empresa: las cuentas en captura y los pedidos no cerrados, con el
// estado que ve quien atiende (domain.AccountState). Barre primero (12 horas y «Nuevo» de pedidos
// cancelados), para que la fila no muestre lo que ya venció.
//
// `outstanding` suma SOLO los pedidos, con el mismo predicado de cada ficha (constitución III): lo
// que se está capturando no es dinero que alguien deba todavía.
func (s *AccountsService) Live(ctx context.Context, olderDebts bool) (*LiveAccounts, error) {
	if err := s.store.WithTx(ctx, func(q *db.Queries) error { return sweepDrafts(ctx, q) }); err != nil {
		return nil, err
	}
	q := s.store.QC(ctx)
	now := s.orders.now()
	today := domain.BusinessDate(now, s.orders.location(ctx))
	since := today.AddDate(0, 0, -debtWindowDays)
	if olderDebts {
		since = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	orders, err := q.ListLiveOrders(ctx, pgtype.Date{Time: since, Valid: true})
	if err != nil {
		return nil, err
	}
	drafts, err := q.ListLiveDrafts(ctx)
	if err != nil {
		return nil, err
	}

	// Lo «Nuevo» se pega a su pedido; las cuentas nuevas son fichas propias.
	pending := map[int64]db.ListLiveDraftsRow{}
	var fresh []db.ListLiveDraftsRow
	for _, d := range drafts {
		if d.OrderID != nil {
			pending[*d.OrderID] = d
			continue
		}
		fresh = append(fresh, d)
	}

	out := &LiveAccounts{Items: make([]AccountItem, 0, len(orders)+len(fresh)), Outstanding: decimal.Zero, ServerTime: now}
	seen := map[int64]bool{}
	addOrder := func(r db.ListLiveOrdersRow, closedWithPending bool) {
		state, listed := domain.AccountState(false, string(r.Status), r.Paid, r.Total)
		if !listed && !closedWithPending {
			return
		}
		if !listed {
			// Cerrada con lo «Nuevo» todavía capturándose: la ficha existe para que eso no se pierda de
			// vista; lo que la pantalla ofrece ahí lo decide `closedWithPending`.
			state = domain.AccountPaidInKitchen
		}
		id, number := r.ID, int(r.DailyNumber)
		date := r.BusinessDate.Time.Format("2006-01-02")
		it := AccountItem{
			Key: "o:" + itoa64(r.ID), Kind: "order", OrderID: &id, Number: &number, FolioName: r.FolioName,
			State: state, Group: domain.AccountGroup(state, r.BusinessDate.Time, today),
			KitchenReady: string(r.Status) == domain.StatusLista, PlatformID: r.DeliveryPlatformID,
			ServiceType: string(r.ServiceType), CustomerName: r.CustomerName, OpenedAt: r.OpenedAt,
			UpdatedAt: r.UpdatedAt, BusinessDate: &date, Total: r.Total, Paid: r.Paid,
			Outstanding: domain.PorCobrar(r.Total, r.Paid), LineCount: int(r.Renglones),
			ClosedWithPending: closedWithPending,
		}
		if d, ok := pending[r.ID]; ok {
			did := d.ID
			it.PendingDraftID, it.PendingCount = &did, int(d.LineCount)
		}
		out.Items = append(out.Items, it)
		out.Outstanding = out.Outstanding.Add(it.Outstanding)
		seen[r.ID] = true
	}
	for _, r := range orders {
		addOrder(r, false)
	}
	var orphans []int64
	for orderID := range pending {
		if !seen[orderID] {
			orphans = append(orphans, orderID)
		}
	}
	if len(orphans) > 0 {
		rows, err := q.GetOrdersForAccounts(ctx, orphans)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			addOrder(db.ListLiveOrdersRow(r), true)
		}
	}
	for _, d := range fresh {
		v, err := buildDraftView(ctx, q, draftRowOf(d))
		if err != nil {
			return nil, err
		}
		did, version := d.ID, d.Version
		out.Items = append(out.Items, AccountItem{
			Key: "d:" + d.ID.String(), Kind: "draft", DraftID: &did, DraftVersion: &version, FolioName: d.FolioName,
			State: domain.AccountCapturing, Group: domain.GroupCapturing, PlatformID: d.DeliveryPlatformID,
			ServiceType: string(d.ServiceType), CustomerName: d.CustomerName, OpenedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt, Total: v.Total, Paid: decimal.Zero, Outstanding: v.Total,
			LineCount: int(d.LineCount),
		})
	}
	sortByOpenedAt(out.Items)
	out.Outstanding = domain.Round2(out.Outstanding)
	return out, nil
}

func sortByOpenedAt(items []AccountItem) {
	slices.SortStableFunc(items, func(a, b AccountItem) int { return a.OpenedAt.Compare(b.OpenedAt) })
}

func draftRowOf(d db.ListLiveDraftsRow) db.GetDraftRow {
	return db.GetDraftRow{
		ID: d.ID, CompanyID: d.CompanyID, OrderID: d.OrderID, Status: d.Status, FolioName: d.FolioName,
		FolioScheme: d.FolioScheme, ServiceType: d.ServiceType, CustomerName: d.CustomerName,
		DeliveryPlatformID: d.DeliveryPlatformID, PlatformOrderRef: d.PlatformOrderRef, DeliveryFee: d.DeliveryFee,
		DiscountAmount: d.DiscountAmount, DiscountPercent: d.DiscountPercent, DiscountSetBy: d.DiscountSetBy,
		PlatformRefSetBy: d.PlatformRefSetBy, OpenedBy: d.OpenedBy, HeaderVersion: d.HeaderVersion,
		Version: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, SentAt: d.SentAt, DiscardedAt: d.DiscardedAt,
		DiscardedBy: d.DiscardedBy, DiscardReason: d.DiscardReason, OpenedByName: d.OpenedByName,
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
