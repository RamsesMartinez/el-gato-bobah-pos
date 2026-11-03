package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

type originKey struct{}
type actorKey struct{}

// WithOrigin guarda en el contexto de dónde vino el request (red, aparato, clave del aparato).
func WithOrigin(ctx context.Context, o domain.RequestOrigin) context.Context {
	return context.WithValue(ctx, originKey{}, o)
}

// OriginFrom devuelve el origen del request; vacío en un camino sin request (webhook, tarea).
func OriginFrom(ctx context.Context) domain.RequestOrigin {
	o, _ := ctx.Value(originKey{}).(domain.RequestOrigin)
	return o
}

// WithActorUser guarda en el contexto a la persona autenticada que hace la acción.
func WithActorUser(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, actorKey{}, userID)
}

// ActorUserFrom devuelve a la persona autenticada del request, si la hay.
func ActorUserFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(actorKey{}).(int64)
	return id, ok
}

// recordAudit escribe un evento de la bitácora con `q`, que DEBE ser la transacción de la acción:
// si la acción se revierte, el evento también, y si el evento no se puede guardar la acción no se
// aplica (FR-013). Sin autor explícito, el autor es la persona del request, o el sistema.
func recordAudit(ctx context.Context, q *db.Queries, ev domain.AuditEvent) error {
	if ev.ActorKind == "" {
		if id, ok := ActorUserFrom(ctx); ok {
			ev.ActorKind, ev.ActorUserID = domain.ActorUser, &id
		} else {
			ev.ActorKind = domain.ActorSystem
		}
	}
	if err := ev.Validate(); err != nil {
		return err
	}
	before, err := marshalAudit(ev.Before)
	if err != nil {
		return err
	}
	after, err := marshalAudit(ev.After)
	if err != nil {
		return err
	}
	o := OriginFrom(ctx)
	var label *string
	if ev.ActorLabel != "" {
		label = &ev.ActorLabel
	}
	if err := q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		BranchID: ev.BranchID, ActorKind: string(ev.ActorKind), ActorUserID: ev.ActorUserID, ActorLabel: label,
		Action: string(ev.Action), EntityType: ev.EntityType, EntityID: ev.EntityID, OrderID: ev.OrderID,
		SubjectUserID: ev.SubjectUserID, Ip: o.IP, Device: o.Device, DeviceKey: o.DeviceKey,
		Before: before, After: after,
	}); err != nil {
		return fmt.Errorf("registrar %s en la bitácora: %w", ev.Action, err)
	}
	return nil
}

func marshalAudit(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("serializar el evento de auditoría: %w", err)
	}
	return b, nil
}

// auditOrder registra una acción sobre un pedido, con la sucursal del pedido. `actor` es quien la
// hizo cuando el método lo recibe; sin él, el del request (o el sistema).
func auditOrder(ctx context.Context, q *db.Queries, action domain.AuditAction, orderID int64, actor *int64, before, after any) error {
	branch, err := q.GetOrderBranch(ctx, orderID)
	if err != nil {
		return fmt.Errorf("sucursal del pedido %d para la bitácora: %w", orderID, err)
	}
	ev := domain.AuditEvent{Action: action, EntityType: "order", EntityID: orderID, OrderID: &orderID,
		BranchID: &branch, Before: before, After: after}
	if actor != nil && *actor > 0 {
		ev.ActorKind, ev.ActorUserID = domain.ActorUser, actor
	}
	return recordAudit(ctx, q, ev)
}

// auditEntity registra una acción sobre algo que no es un pedido (caja, gasto, precio, asistencia).
func auditEntity(ctx context.Context, q *db.Queries, action domain.AuditAction, entityType string, entityID int64,
	branch *int64, actor *int64, before, after any) error {
	ev := domain.AuditEvent{Action: action, EntityType: entityType, EntityID: entityID, BranchID: branch,
		Before: before, After: after}
	if actor != nil && *actor > 0 {
		ev.ActorKind, ev.ActorUserID = domain.ActorUser, actor
	}
	return recordAudit(ctx, q, ev)
}

// auditCash registra una acción de caja con la sucursal de la caja del turno.
func auditCash(ctx context.Context, q *db.Queries, action domain.AuditAction, sessionID int64, actor *int64, before, after any) error {
	branch, err := q.GetRegisterSessionBranch(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("sucursal del turno %d para la bitácora: %w", sessionID, err)
	}
	return auditEntity(ctx, q, action, "register_session", sessionID, &branch, actor, before, after)
}
