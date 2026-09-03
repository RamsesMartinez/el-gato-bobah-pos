package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Devolver dinero: la operación que el sistema no tenía.
//
// Tenía dos que decían que una venta no ocurrió —cancelar y reembolsar— y ninguna miraba lo que el
// cliente ya había pagado. Cancelar un pedido cobrado lo sacaba de los reportes y dejaba los cobros
// en la base, con el arqueo esperando ese dinero en el cajón; reembolsar uno entregado sin cobrar
// anotaba como pérdida un ingreso que nunca ocurrió.

// DevolucionCmd: qué se devuelve y de qué pedido.
//
// `LineID` nulo = contra la cuenta entera. Con renglón, el tope es lo cobrado de ESE renglón: sin esa
// cota, devolver tres veces un platillo de $60 en un pedido de $500 pasa sin que nada lo frene.
type DevolucionCmd struct {
	OrderID int64
	LineID  *int64
	Monto   decimal.Decimal
	Motivo  string
	ActorID int64
}

// CancelacionCmd: cancelar un pedido, resolviendo su dinero si lo tiene.
type CancelacionCmd struct {
	OrderID int64
	Motivo  string
	ActorID int64
	// Devolver: el cajero confirma que el dinero se le regresa al cliente. Sin esto, un pedido con
	// cobros NO se cancela — es el agujero que esta feature cierra.
	Devolver bool
}

// Devolver registra una devolución y, si el dinero salió del cajón, su movimiento de caja.
//
// Todo en UNA transacción: un pedido marcado como devuelto sin su salida de caja, o al revés, es
// justo el descuadre que esto viene a eliminar.
func (s *OrdersService) Devolver(ctx context.Context, cmd DevolucionCmd) error {
	motivo, err := domain.MotivoValido(cmd.Motivo)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		return s.devolverEnTx(ctx, q, cmd, motivo)
	})
}

// devolverEnTx es el cuerpo compartido por Devolver y por CancelarConDevolucion: cancelar con
// devolución tiene que ir en la MISMA transacción que la cancelación, y dos copias de esta lógica
// serían dos formas distintas de mover el mismo dinero.
func (s *OrdersService) devolverEnTx(ctx context.Context, q *db.Queries, cmd DevolucionCmd, motivo string) error {
	entradas, err := s.cobradoPorMetodo(ctx, q, cmd.OrderID)
	if err != nil {
		return err
	}
	cobrado := decimal.Zero
	for _, e := range entradas {
		cobrado = cobrado.Add(e.Monto)
	}

	devuelto, err := q.SumOrderRefunds(ctx, db.SumOrderRefundsParams{OrderID: cmd.OrderID, LineID: cmd.LineID})
	if err != nil {
		return err
	}
	yaDevuelto := devuelto.DevueltoTotal
	if cmd.LineID != nil {
		// Contra un renglón el tope es lo suyo, no lo del pedido.
		yaDevuelto = devuelto.DevueltoDelRenglon
	}
	if err := domain.ValidarDevolucion(cmd.Monto, cobrado, yaDevuelto); err != nil {
		return err
	}

	// El dinero sale por donde entró. Devolver en efectivo lo que entró por tarjeta saca del cajón
	// dinero que nunca estuvo ahí, y el arqueo cierra con un faltante inventado.
	for _, parte := range domain.RepartirDevolucion(entradas, cmd.Monto) {
		var movimiento *int64
		if parte.SaleDelCajon {
			id, err := s.salidaDeCaja(ctx, q, parte.Monto, motivo, cmd.ActorID)
			if err != nil {
				return err
			}
			movimiento = id
		}
		if _, err := q.InsertOrderRefund(ctx, db.InsertOrderRefundParams{
			OrderID:         cmd.OrderID,
			OrderLineID:     cmd.LineID,
			PaymentMethodID: parte.MetodoID,
			Amount:          parte.Monto,
			Reason:          motivo,
			RefundedBy:      cmd.ActorID,
			CashMovementID:  movimiento,
		}); err != nil {
			return err
		}
	}

	// `orders.refund_amount` pasa a ser la SUMA del libro, no un número que se escribe aparte:
	// `RefundsByDay` ya lo lee y dos verdades sobre el mismo dinero es lo que el principio III
	// prohíbe.
	return q.RecalcOrderRefundAmount(ctx, cmd.OrderID)
}

// cobradoPorMetodo traduce lo que la base sabe al tipo del dominio, que es donde vive la regla.
func (s *OrdersService) cobradoPorMetodo(ctx context.Context, q *db.Queries, orderID int64) ([]domain.CobradoPorMetodo, error) {
	filas, err := q.SumOrderPaymentsByMethod(ctx, orderID)
	if err != nil {
		return nil, err
	}
	entradas := make([]domain.CobradoPorMetodo, 0, len(filas))
	for _, f := range filas {
		entradas = append(entradas, domain.CobradoPorMetodo{
			MetodoID:   f.MethodID,
			Nombre:     f.Name,
			EsEfectivo: f.EsEfectivo,
			Activo:     f.IsActive,
			Monto:      f.Cobrado,
		})
	}
	return entradas, nil
}

// salidaDeCaja registra que el efectivo salió del cajón, para que el arqueo lo descuente solo.
//
// Sin turno abierto NO se rechaza la devolución: el dinero ya se le regresó al cliente y negarse a
// registrarlo no lo devuelve a la caja — solo lo deja sin rastro. Se anota la devolución sin
// movimiento, y el corte siguiente muestra la diferencia con su explicación en el libro.
func (s *OrdersService) salidaDeCaja(ctx context.Context, q *db.Queries, monto decimal.Decimal, motivo string, actor int64) (*int64, error) {
	sess, err := q.LockOpenPrimarySession(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	mov, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
		SessionID: sess.ID,
		Kind:      "salida",
		Amount:    domain.Round2(monto),
		Concept:   fmt.Sprintf("Devolución: %s", motivo),
		UserID:    actor,
	})
	if err != nil {
		return nil, err
	}
	return &mov.ID, nil
}

// CancelarConDevolucion cancela un pedido resolviendo su dinero en la MISMA transacción.
//
// `Cancel` ignoraba los cobros: respondía 204, la venta salía de los reportes, los renglones de
// `order_payments` se quedaban intactos y el arqueo seguía esperando ese dinero en el cajón. Si se
// le devolvía al cliente, el corte cerraba con un faltante que ningún renglón explicaba; si no, el
// negocio se quedaba con dinero que no aparecía en ninguna venta.
//
// Una transacción y no dos llamadas: en el hueco entre "cancelado" y "devuelto" vive exactamente el
// descuadre que esto elimina.
func (s *OrdersService) CancelarConDevolucion(ctx context.Context, cmd CancelacionCmd) error {
	motivo, err := domain.MotivoValido(cmd.Motivo)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrder(ctx, cmd.OrderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if !domain.CanTransition(string(o.Status), domain.StatusCancelada) {
			return domain.ErrConflict
		}
		lineas, err := lineasDeEntrega(ctx, q, cmd.OrderID)
		if err != nil {
			return err
		}
		if domain.HayEntregaParcial(lineas) {
			return domain.ErrCancelarConEntregas
		}

		entradas, err := s.cobradoPorMetodo(ctx, q, cmd.OrderID)
		if err != nil {
			return err
		}
		cobrado := decimal.Zero
		for _, e := range entradas {
			cobrado = cobrado.Add(e.Monto)
		}
		if cobrado.GreaterThan(decimal.Zero) {
			if !cmd.Devolver {
				return domain.ErrCancelarSinDevolver
			}
			// Lo que queda por devolver, no lo cobrado: un pedido al que ya se le devolvió una
			// parte no puede devolver esa parte otra vez al cancelarse.
			yaDevuelto, err := q.SumOrderRefunds(ctx, db.SumOrderRefundsParams{OrderID: cmd.OrderID})
			if err != nil {
				return err
			}
			porDevolver := domain.MontoDevolvible(cobrado, yaDevuelto.DevueltoTotal)
			if porDevolver.GreaterThan(decimal.Zero) {
				if err := s.devolverEnTx(ctx, q, DevolucionCmd{
					OrderID: cmd.OrderID, Monto: porDevolver, Motivo: motivo, ActorID: cmd.ActorID,
				}, motivo); err != nil {
					return err
				}
			}
		}

		if err := q.CancelOrder(ctx, db.CancelOrderParams{
			ID: cmd.OrderID, CancelledBy: &cmd.ActorID, CancelReason: &motivo,
		}); err != nil {
			return err
		}
		return q.RestockCancelledOrder(ctx, db.RestockCancelledOrderParams{Oid: &cmd.OrderID, ActorID: &cmd.ActorID})
	})
}

// CancelarRenglon cancela UN renglón de un pedido vivo.
//
// Existía la columna y no la operación: ninguna consulta escribía `order_lines.cancelled_at`,
// mientras el error de cancelar un pedido con entregas parciales mandaba al operador a "cancela los
// que falten" — que no se podía hacer desde ningún lado. La única salida practicable era marcar como
// entregado lo que seguía en la plancha.
//
// Devuelve si repuso el inventario, porque la pantalla tiene que poder decirlo: cancelar algo que ya
// salió a cocina baja el total pero NO devuelve el insumo, y callarlo descuadra el almacén sin que
// nadie sepa por qué.
func (s *OrdersService) CancelarRenglon(ctx context.Context, orderID, lineID, actor int64, motivo string) (repuso bool, err error) {
	razon, err := domain.MotivoValido(motivo)
	if err != nil {
		return false, err
	}
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		l, err := q.GetOrderLineForCancel(ctx, db.GetOrderLineForCancelParams{ID: lineID, OrderID: orderID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if l.CancelledAt.Valid {
			// Ya estaba cancelado: no-op idempotente. Un doble tap no puede reponer dos veces el
			// mismo insumo, que es inventar existencias.
			return nil
		}
		if err := domain.PuedeCancelarRenglon(string(l.OrderStatus), l.Quantity, l.DeliveredQty); err != nil {
			return err
		}
		if err := q.CancelOrderLine(ctx, db.CancelOrderLineParams{
			ID: lineID, CancelledBy: &actor, CancelReason: &razon,
		}); err != nil {
			return err
		}
		// La regla la decide el dominio con lo que la base ya sabe, no el cajero.
		repuso = domain.ReponeInventario(nullTime(l.EnviadoACocinaAt))
		if repuso {
			if err := q.RestockCancelledLine(ctx, db.RestockCancelledLineParams{
				LineID: &lineID, ActorID: &actor,
			}); err != nil {
				return err
			}
		}
		// El total baja igual, haya repuesto o no: el cliente no paga lo que se canceló.
		return q.RecalcOrderTotals(ctx, orderID)
	})
	return repuso, err
}

// nullTime traduce el timestamptz opcional de pgx a lo que el dominio entiende.
func nullTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// PorDevolver dice cuánto queda por devolver de un pedido, o de uno de sus renglones.
//
// Lo usa la frontera cuando no se manda monto, que es el caso de todos los días: "devuélvele todo".
// Se calcula aquí y no en la pantalla porque el tope es lo COBRADO menos lo ya devuelto, y la
// pantalla no tiene esas dos cifras sin pedirlas — y si las pidiera, quedarían viejas entre la
// consulta y el toque.
func (s *OrdersService) PorDevolver(ctx context.Context, orderID int64, lineID *int64) (decimal.Decimal, error) {
	q := s.store.QC(ctx)
	entradas, err := s.cobradoPorMetodo(ctx, q, orderID)
	if err != nil {
		return decimal.Zero, err
	}
	cobrado := decimal.Zero
	for _, e := range entradas {
		cobrado = cobrado.Add(e.Monto)
	}
	devuelto, err := q.SumOrderRefunds(ctx, db.SumOrderRefundsParams{OrderID: orderID, LineID: lineID})
	if err != nil {
		return decimal.Zero, err
	}
	ya := devuelto.DevueltoTotal
	if lineID != nil {
		ya = devuelto.DevueltoDelRenglon
	}
	return domain.MontoDevolvible(cobrado, ya), nil
}
