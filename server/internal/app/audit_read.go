package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// AuditService lee la bitácora (spec 033). Escribirla no pasa por aquí: cada acción la escribe en
// su propia transacción con recordAudit.
type AuditService struct {
	store *store.Store
}

func NewAuditService(s *store.Store) *AuditService { return &AuditService{store: s} }

// AuditEventView es un evento como lo ve quien tiene permiso: incluye IP y aparato.
type AuditEventView struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurredAt"`
	Action     string          `json:"action"`
	ActorKind  string          `json:"actorKind"`
	ActorName  string          `json:"actorName"`
	IP         string          `json:"ip"`
	Device     json.RawMessage `json:"device,omitempty"`
	DeviceKey  string          `json:"deviceKey"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	OrderID    *int64          `json:"orderId,omitempty"`
}

// actorName es el nombre que se muestra: la persona, o la plataforma, o «Sistema».
func actorName(kind string, name, label *string) string {
	switch {
	case name != nil:
		return *name
	case label != nil && *label != "":
		return *label
	case kind == "system":
		return "Sistema"
	}
	return ""
}

// OrderHistory devuelve los eventos de un pedido en orden. Siempre un arreglo, aunque esté vacío.
func (s *AuditService) OrderHistory(ctx context.Context, orderID int64) ([]AuditEventView, error) {
	rows, err := s.store.QC(ctx).ListOrderHistory(ctx, &orderID)
	if err != nil {
		return nil, err
	}
	out := make([]AuditEventView, 0, len(rows))
	for _, r := range rows {
		out = append(out, viewOf(r.ID, r.OccurredAt, r.Action, r.ActorKind, actorName(r.ActorKind, r.ActorName, r.ActorLabel),
			r.Ip, r.Device, r.DeviceKey, r.Before, r.After, &orderID))
	}
	return out, nil
}

func viewOf(id int64, at time.Time, action, kind, name, ip string, device []byte, key *string, before, after []byte, order *int64) AuditEventView {
	return AuditEventView{ID: id, OccurredAt: at, Action: action, ActorKind: kind, ActorName: name, IP: ip,
		Device: device, DeviceKey: derefStr(key), Before: before, After: after, OrderID: order}
}

// AuditPage es una página de la bitácora con su total y cuántas acciones hubo de cada grupo.
type AuditPage struct {
	Items         []AuditEventView `json:"items"`
	Total         int64            `json:"total"`
	CountsByGroup map[string]int64 `json:"countsByGroup"`
	DeviceKeys    []string         `json:"deviceKeys"`
}

// List devuelve una página de la bitácora general. Los días se resuelven en la zona del negocio.
func (s *AuditService) List(ctx context.Context, f domain.AuditFilter) (*AuditPage, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	q := s.store.QC(ctx)
	tz, err := q.GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	loc := domain.LoadBusinessLocation(tz)
	from := time.Date(f.From.Year(), f.From.Month(), f.From.Day(), 0, 0, 0, 0, loc)
	to := time.Date(f.To.Year(), f.To.Month(), f.To.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	var key *string
	if f.DeviceKey != "" {
		key = &f.DeviceKey
	}
	var actions []string
	if as := f.Actions(); as != nil {
		actions = make([]string, len(as))
		for i, a := range as {
			actions[i] = string(a)
		}
	}

	page := &AuditPage{Items: []AuditEventView{}, CountsByGroup: map[string]int64{}, DeviceKeys: []string{}}
	for _, g := range domain.AllAuditGroups() {
		page.CountsByGroup[string(g)] = 0
	}
	byAction, err := q.CountAuditEventsByAction(ctx, db.CountAuditEventsByActionParams{
		FromAt: from, ToAt: to, ActorUserID: f.ActorUserID, DeviceKey: key})
	if err != nil {
		return nil, err
	}
	for _, r := range byAction {
		if g, ok := domain.AuditGroupOf(domain.AuditAction(r.Action)); ok {
			page.CountsByGroup[string(g)] += r.N
		}
	}
	if page.Total, err = q.CountAuditEvents(ctx, db.CountAuditEventsParams{
		FromAt: from, ToAt: to, ActorUserID: f.ActorUserID, Actions: actions, DeviceKey: key}); err != nil {
		return nil, err
	}
	keys, err := q.ListAuditDeviceKeys(ctx, db.ListAuditDeviceKeysParams{
		FromAt: from, ToAt: to, ActorUserID: f.ActorUserID, Actions: actions})
	if err != nil {
		return nil, err
	}
	page.DeviceKeys = append(page.DeviceKeys, keys...)
	rows, err := q.ListAuditEvents(ctx, db.ListAuditEventsParams{
		FromAt: from, ToAt: to, ActorUserID: f.ActorUserID, Actions: actions, DeviceKey: key,
		Lim: int32(f.Size), Off: int32(f.Page * f.Size)})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		page.Items = append(page.Items, viewOf(r.ID, r.OccurredAt, r.Action, r.ActorKind,
			actorName(r.ActorKind, r.ActorName, r.ActorLabel), r.Ip, r.Device, r.DeviceKey, r.Before, r.After, r.OrderID))
	}
	return page, nil
}

// AuditActor es una persona por la que se filtra la bitácora.
type AuditActor struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Actors devuelve las personas del negocio para el filtro de la bitácora.
func (s *AuditService) Actors(ctx context.Context) ([]AuditActor, error) {
	rows, err := s.store.QC(ctx).ListAuditActors(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AuditActor, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuditActor{ID: r.ID, Name: r.Name})
	}
	return out, nil
}
