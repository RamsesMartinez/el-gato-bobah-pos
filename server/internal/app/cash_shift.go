package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
	"github.com/shopspring/decimal"
)

// CashShiftService abre y cierra los turnos de caja y arma la vista del turno vivo (spec 082, fase 2:
// partido de BackofficeService sin cambiar ninguna respuesta). El conteo vive aquí y no en un
// servicio aparte: solo existe dentro de la transacción de abrir y de cerrar.
type CashShiftService struct{ cashDeps }

func (s *CashShiftService) OpenSession(ctx context.Context, registerID int64, cmd AperturaCmd, userID int64) (*SessionView, error) {
	reg, err := s.activeRegister(ctx, registerID)
	if err != nil {
		return nil, err
	}
	// Antes que el conteo: a quien abre una caja que no se cuenta no se le pide motivo ni piezas.
	// La transacción lo vuelve a revisar con la caja tomada.
	if !reg.CountsCash {
		return nil, domain.ErrRegisterNotCounted
	}
	if _, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID); err == nil {
		return nil, domain.ErrConflict // esa caja ya está abierta
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// El total sale de las PIEZAS y del catálogo, nunca de lo que mande el cliente: es la misma
	// regla que `BuildOrder` con los precios. Se resuelve antes de abrir la transacción porque leer
	// el catálogo no necesita estar dentro de ella.
	// La moneda del turno es la de la columna, que hoy siempre toma su default: no hay forma de
	// elegir otra al abrir. Cuando la haya, ESTA línea es la que cambia — y el test de FR-011 ya
	// cubre que una denominación de otra moneda se rechaza.
	piezas, err := s.piezasConSuValor(ctx, cmd.Piezas, string(domain.DefaultCurrency))
	if err != nil {
		return nil, err
	}
	total, err := domain.TotalDeclarado(piezas, cmd.Total, cmd.Motivo)
	if err != nil {
		return nil, err
	}
	// A CIEGAS (punto 6): el cierre anterior se compara aquí y no se devuelve. Una diferencia exige
	// motivo de la lista; sin cierre anterior contado no hay contra qué comparar.
	var prev *decimal.Decimal
	if p, err := s.store.QC(ctx).LastClosingCountOfRegister(ctx, registerID); err == nil {
		prev = &p
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if cmd.OpenedAt != nil && cmd.ExcludePending {
		return nil, fmt.Errorf("%w: una apertura con hora no excluye los gastos sin corte", domain.ErrValidation)
	}
	var reason, note *string
	if domain.OpeningNeedsReason(prev, total) {
		if err := domain.ValidOpeningReason(cmd.Reason, cmd.ReasonNote); err != nil {
			return nil, err
		}
		reason = &cmd.Reason
		if n := strings.TrimSpace(cmd.ReasonNote); n != "" {
			note = &n
		}
	}

	var sess db.RegisterSession
	// UNA SOLA TRANSACCIÓN. Son tres escrituras —la sesión, el conteo y sus renglones— y si la
	// segunda o la tercera fallan después de que la primera comprometió, queda una sesión ABIERTA
	// SIN CONTEO Y SIN MOTIVO, que es justo lo que FR-016 prohíbe. Peor: la caja queda bloqueada,
	// porque `one_open_session_per_register` no deja abrir otra hasta resolver la huérfana a mano.
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// LA ASISTENCIA DE QUIEN ABRE (spec 046), con su fila tomada y antes de escribir el turno: una
		// salida que confirma a la vez en otra tableta espera, y se decide con lo que de verdad quedó.
		p, err := cashAttendanceOf(ctx, q, userID)
		if err != nil {
			return err
		}
		if err := domain.RequireCashOpenAttendance(p.Attendance, s.now()); err != nil {
			return err
		}
		// Una caja que no lleva arqueo no abre turno (spec 047). Con su fila tomada: quitarle el
		// arqueo a la vez espera a que este turno exista, y entonces se rechaza.
		cr, err := q.GetCashRegisterForUpdate(ctx, registerID)
		if err != nil {
			return err
		}
		if !cr.CountsCash {
			return domain.ErrRegisterNotCounted
		}
		// LA HORA DE APERTURA (spec 048, R13), con la caja tomada: lo que se valida es lo que queda.
		// Sin hora explicita, la apertura incluye sola los gastos sin corte desde el último cierre
		// tomando la hora del primero; excluirlos o mover la hora a mano exige el permiso, que revisa
		// el handler. Un solo reloj, el del servicio: la hora de los pagos sale de él.
		now := s.now()
		last, err := lastCloseOf(ctx, q, registerID)
		if err != nil {
			return err
		}
		openedAt, backdate := now, ""
		switch {
		case cmd.OpenedAt != nil:
			if err := domain.ValidBackdatedOpening(*cmd.OpenedAt, now, last); err != nil {
				return err
			}
			openedAt, backdate = *cmd.OpenedAt, "manual"
		case !cmd.ExcludePending:
			since := pgtype.Timestamptz{}
			if last != nil {
				since = pgtype.Timestamptz{Time: *last, Valid: true}
			}
			pend, err := q.ListPendingExpensePayments(ctx, db.ListPendingExpensePaymentsParams{RegisterID: &registerID, Since: since})
			if err != nil {
				return err
			}
			if len(pend) > 0 && pend[0].PaidAt.Before(now) {
				openedAt, backdate = pend[0].PaidAt, "automatic"
			}
		}
		abierta, err := q.OpenSession(ctx, db.OpenSessionParams{
			BusinessDate: pgtype.Date{Time: domain.BusinessDate(openedAt, s.businessLocation(ctx)), Valid: true},
			OpeningCash:  total,
			OpenedBy:     userID,
			RegisterID:   registerID,
			OpenedAt:     pgtype.Timestamptz{Time: openedAt, Valid: true},
		})
		if err != nil {
			return err
		}
		sess = abierta
		// LOS GASTOS SIN CORTE CON HORA DENTRO DE ESTE CORTE ENTRAN A ÉL. La caja está tomada FOR
		// UPDATE, así que un pago que se registra a la vez espera y se decide contra este turno.
		if backdate != "" {
			swept, err := sweepPendingExpenses(ctx, q, abierta.ID, registerID, openedAt, now)
			if err != nil {
				return err
			}
			if err := auditCash(ctx, q, domain.AuditCashOpeningBackdated, abierta.ID, &userID, nil,
				map[string]any{"openedAt": openedAt, "reason": backdate, "sweptPayments": swept}); err != nil {
				return err
			}
		}
		// LO QUE QUEDÓ ANTES DE ESTA APERTURA YA NO ENTRA A NINGÚN CORTE: abrir sin incluirlo es
		// elegir «sin incluir», y así se guarda. Si no, quedaría pendiente para siempre.
		dropped, err := q.ExcludePendingBefore(ctx, db.ExcludePendingBeforeParams{RegisterID: &registerID, Before: abierta.OpenedAt})
		if err != nil {
			return err
		}
		for _, d := range dropped {
			if err := auditEntity(ctx, q, domain.AuditExpenseCountChanged, "expense", d.ExpenseID, nil, &userID,
				map[string]any{"includeInCount": true, "sessionId": nil},
				map[string]any{"paymentId": d.ID, "includeInCount": false, "reason": "opened_without_it", "sessionId": abierta.ID}); err != nil {
				return err
			}
		}
		// El modo de tarjeta lo copia este update; la fila de OpenSession todavía trae el default.
		if sess.CardCountMode, err = q.SetOpeningExtras(ctx, db.SetOpeningExtrasParams{ID: abierta.ID, Reason: reason, Note: note}); err != nil {
			return err
		}
		// Solo la caja PRINCIPAL reclama lo huérfano: es la única que vende, y el esperado de una
		// secundaria ignora ventas y devoluciones. Si la barra abría primero se quedaba con ellas y
		// no restaban ni sumaban en ningún corte (spec 031, revisión de D7).
		// Con la fila tomada y no con la lectura de antes de la transacción: pasar la venta a otra caja
		// a la vez (spec 049) espera a este candado, y esta caja pudo dejar de vender mientras tanto.
		if cr.IsPrimary {
			// LOS PEDIDOS DE PLATAFORMA QUE SE ACEPTARON SIN TURNO ENTRAN A ÉSTE (spec 021).
			//
			// Aceptar no exige turno abierto: la cocina no puede esperar a que alguien abra caja. Lo
			// que paga esa decisión es este renglón — sin él, esos pedidos quedan fuera de todo corte
			// para siempre, y el dinero que el negocio sí recibió no aparece en ninguna parte.
			//
			// Va DENTRO de esta transacción a propósito: si la apertura se deshace, los pedidos tienen
			// que quedar huérfanos otra vez y no colgando de un turno que no existe.
			if err := ReclamarPedidosDePlataformaHuerfanos(ctx, q, abierta.ID,
				pgtype.Date{Time: s.businessDate(ctx), Valid: true}); err != nil {
				return err
			}
			// Y LAS DEVOLUCIONES QUE SE HICIERON SIN TURNO (spec 031, D7): una devolución por tarjeta o
			// por plataforma no espera a que alguien abra caja, y sin esto no restaría de ningún corte.
			if err := q.ClaimOrphanRefunds(ctx, abierta.ID); err != nil {
				return err
			}
		}
		// Sin esperado: al abrir no hay nada que esperar — el fondo ES lo que se contó. El check del
		// esquema lo exige nulo justo aquí.
		if err := s.guardarConteo(ctx, q, abierta.ID, db.CashCountMomentApertura, total, nil,
			cmd.Piezas, motivoDelCamino(cmd.Total, cmd.Motivo), userID); err != nil {
			return err
		}
		return auditCash(ctx, q, "cash.opened", abierta.ID, &userID, nil,
			map[string]any{"amount": total, "reason": cmd.Motivo})
	})
	if err != nil {
		// El pre-check de arriba NO es atómico: dos tabletas pueden pasarlo las dos y la segunda
		// choca aquí con `one_open_session_per_register`. La fila secuencial ya devuelve
		// ErrConflict; esto hace que la carrera termine igual, y no en un 500.
		return nil, traduceConflictoDeCaja(err, db.CashCountMomentApertura)
	}
	return s.vistaDelTurnoAbierto(ctx, sess, reg)
}

// CurrentByRegister devuelve la sesión abierta de una caja con sus esperados en vivo, o nil si
// esa caja está cerrada.
func (s *CashShiftService) CurrentByRegister(ctx context.Context, registerID int64) (*SessionView, error) {
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.vistaDelTurnoAbierto(ctx, sess, reg)
}

func (s *CashShiftService) CloseSession(ctx context.Context, registerID int64, userID int64, cmd CierreCmd) (*SessionView, error) {
	// Copia: el mapa es de quien llama y el efectivo se sustituye por el total del conteo. Mutar el
	// del handler dejaría el cuerpo del request diciendo algo que ya no es.
	declared := make(map[int]decimal.Decimal, len(cmd.Declarado))
	for k, v := range cmd.Declarado {
		declared[k] = v
	}
	// allowZero: un método puede cerrar en 0 (sin ventas). Rechaza negativos y absurdos.
	for _, d := range declared {
		if !domain.ValidMoney(domain.Round2(d), true) {
			return nil, domain.ErrValidation
		}
	}
	decision, err := domain.ParseTipsDecision(cmd.Propinas)
	if err != nil {
		return nil, err
	}
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	// TODO EL CIERRE VA EN UNA TRANSACCIÓN QUE EMPIEZA BLOQUEANDO EL TURNO (spec 031, D8).
	//
	// El esperado se calculaba antes de abrirla: un cobro, un pago devuelto o un movimiento que
	// confirmaba entre esa lectura y el cierre quedaba en el turno cerrado sin entrar al esperado
	// firmado —sobrante en el arqueo y un pago que ya no se podía devolver—. Con el turno bloqueado,
	// lo que ya lo tenía termina antes de que se lea nada, y lo que llega después espera y lo ve
	// cerrado. Las lecturas de abajo van después del candado aunque usen otra conexión: lo que
	// confirmó ya está, y lo que no, no puede escribir en este turno.
	var view *SessionView
	var conteo *conteoResuelto
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockSessionForClose(ctx, sess.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		// LA SALIDA DE QUIEN ABRIÓ (spec 046), después del candado del turno y antes de todo lo demás:
		// sin resolverla no hay nada que revisar. Turno → persona; checar solo toma la persona, así
		// que no hay ciclo.
		if err := s.closeAttendance(ctx, q, sess, userID, cmd); err != nil {
			return err
		}
		// Ningún pedido sin terminar sobrevive al cierre. Va ANTES de calcular nada: si el turno se
		// cierra con pendientes, esos pedidos quedan colgados de un arqueo ya firmado y su venta cae
		// en un corte que nadie puede volver a cuadrar. Cerrar es además el momento en que el
		// operador SÍ puede resolverlos: está frente a la caja y el local está vacío.
		if err := s.sinPedidosPendientes(ctx, sess.ID); err != nil {
			return err
		}
		// Ni un entregado que debe (no hay fiados, decisión del dueño 2026-10-09): se cobra o se
		// cancela con su motivo antes de cerrar.
		debe, errDebe := s.owingOrders(ctx, reg.IsPrimary)
		if errDebe != nil {
			return errDebe
		}
		if errDebe := domain.NoOwingOrders(debe); errDebe != nil {
			return errDebe
		}
		// LA PROPINA PENDIENTE NO SE QUEDA SIN DUEÑO (spec 032). Con un peso o más hay que decidir;
		// lo que quede —incluido un sobrante de centavos, que no se puede entregar— lo hereda el
		// siguiente turno de esta misma caja.
		fuentes, errTips := tipSources(ctx, q, sess.ID)
		if errTips != nil {
			return errTips
		}
		pendiente := domain.PendingTips(fuentes)
		if domain.TipsDecisionRequired(pendiente) && decision != domain.TipsDecisionKeepInBox {
			return domain.ErrTipsDecisionNeeded
		}
		if errTips := q.SetTipsCarriedOver(ctx, db.SetTipsCarriedOverParams{ID: sess.ID, TipsCarriedOver: pendiente}); errTips != nil {
			return errTips
		}
		if errTips := carryOver(ctx, q, sess.ID, fuentes); errTips != nil {
			return errTips
		}
		if err := s.saveTerminalCounts(ctx, q, sess, userID, cmd.TerminalCounts); err != nil {
			return err
		}
		var err error
		// Sin las cuentas vivas: su barrido abre su propia transacción, y con la conexión del turno
		// bloqueado eso confirmaba el cierre a medias y soltaba el candado de la 031 (D8).
		view, err = s.sessionWithExpected(ctx, sess, reg, false)
		if err != nil {
			return err
		}

		// EL CAJÓN SE DECLARA UNA SOLA VEZ, contándolo. Un método cuyo dinero está ahí no acepta una
		// cifra propia: son dos declaraciones del mismo dinero, y de ahí salía el turno con
		// «Efectivo» en diferencia $0.00 y «Didi efectivo» en −$64.80 sin forma de saber cuál era el
		// faltante real.
		//
		// Se RECHAZA en vez de ignorarse. El caso realista no es un atacante sino una tableta con el
		// front viejo en caché —la aplicación es una PWA con service worker— mandando el cuerpo de
		// antes, y descartar su cifra en silencio deja al operador creyendo que declaró algo que no
		// se guardó.
		delCajon := map[int]bool{}
		if view.Drawer != nil {
			for _, id := range view.Drawer.MethodIDs {
				delCajon[id] = true
			}
		}
		for _, t := range view.Totals {
			if delCajon[t.MethodID] {
				if _, vino := declared[t.MethodID]; vino {
					// La acción primero y sin explicar el mecanismo: quien lee esto está cerrando
					// una caja, no entendiendo el modelo. El nombre del método va porque es lo
					// único que hace el mensaje diagnosticable cuando alguien lo reporta.
					return fmt.Errorf("%w: recarga la pantalla para cerrar: «%s» ahora se cuenta con el cajón",
						domain.ErrValidation, t.Name)
				}
			}
		}

		conteo, err = s.conteoDelCajon(ctx, view.Drawer != nil, cmd)
		if err != nil {
			return err
		}
		// Y SIN CONTEO NO SE CIERRA UN CAJÓN QUE ESPERA DINERO. La pantalla ya bloquea el botón, pero
		// la guardia tiene que estar aquí: abajo cada método del cajón declara su esperado, así que
		// un cierre que llega sin conteo deja los cuatro renglones en diferencia cero y ninguna fila
		// de la cual sacar la real — el corte reportaría $0 con el cajón sin contar.
		if view.Drawer != nil && view.Drawer.RequiresCount && conteo == nil {
			return domain.ErrCajonSinContar
		}
		// Cada método del cajón declara SU esperado, con conteo o sin él. No es un truco: es la única
		// forma de escribir "a este método nadie le declaró una cifra propia" en una columna
		// `not null`, y es lo que hace que ninguno reporte una diferencia que cancele la de otro. La
		// diferencia del cajón —la única que existe— vive en la fila del conteo.
		for _, t := range view.Totals {
			if delCajon[t.MethodID] {
				declared[t.MethodID] = esperadoDe(t)
			}
		}

		for i := range view.Totals {
			t := &view.Totals[i]
			esperado := esperadoDe(*t)
			t.Declared = domain.Round2(domain.ResolveDeclared(t.AutoDeclare, esperado, declared[t.MethodID]))
			t.Difference = domain.Round2(t.Declared.Sub(esperado))
			if err := q.SaveSessionTotal(ctx, db.SaveSessionTotalParams{
				SessionID: sess.ID, PaymentMethodID: int16(t.MethodID),
				// La propina BRUTA: el snapshot alimenta el desglose del corte cerrado, que resta la
				// devuelta al leerlo. Guardar la neta la restaría dos veces.
				Expected: esperado, Declared: t.Declared, Tips: t.grossTips,
				// El snapshot: con qué configuración se cerró este renglón. Leerlo del catálogo al
				// consultar el corte lo reagruparía según el interruptor de hoy.
				AffectsCashDrawer: delCajon[t.MethodID],
			}); err != nil {
				return err
			}
		}
		// DENTRO de la misma transacción que escribe los totales y cierra el turno. Si el conteo
		// fallara después de comprometerse el cierre, quedaría un arqueo firmado cuyo efectivo no se
		// puede reconstruir — y el turno ya cerrado, o sea sin forma de volver a intentarlo.
		if conteo != nil {
			// El esperado del cajón se GUARDA con el conteo: sale de `order_payments`, y una venta
			// cancelada o reembolsada mañana movería la cifra contra la que el operador firmó hoy.
			if err := s.guardarConteo(ctx, q, sess.ID, db.CashCountMomentCierre, conteo.total,
				view.Drawer.Expected, cmd.Piezas, conteo.motivo, userID); err != nil {
				return err
			}
		}
		var n *string
		if cmd.Notas != "" {
			n = &cmd.Notas
		}
		if conteo != nil {
			deja, errFondo := domain.ResolveFloatLeft(cmd.FloatLeft, reg.FixedFloat, conteo.total)
			if errFondo != nil {
				return errFondo
			}
			mostrado := reg.FixedFloat
			if cmd.FixedFloatShown != nil {
				if v := domain.Round2(*cmd.FixedFloatShown); domain.ValidMoney(v, true) {
					mostrado = &v
				} else {
					return fmt.Errorf("%w: el fondo fijo mostrado no es una cantidad válida", domain.ErrValidation)
				}
			}
			if err := q.SetFloatLeft(ctx, db.SetFloatLeftParams{ID: sess.ID, FloatLeft: &deja, FixedFloatAtClose: mostrado}); err != nil {
				return err
			}
			view.FloatLeft, view.FixedFloatAtClose = &deja, mostrado
			// EL RETIRO VA EN LA MISMA TRANSACCIÓN QUE EL CIERRE: o quedan los dos o ninguno. Va
			// después del conteo a propósito: no es parte del esperado que se acaba de firmar.
			retiro, errRetiro := domain.Withdrawal(conteo.total, deja)
			if errRetiro != nil {
				return errRetiro
			}
			if retiro.IsPositive() {
				quien := userID
				if cmd.WithdrawalUserID != nil {
					quien = *cmd.WithdrawalUserID
				}
				fila, err := q.InsertWithdrawal(ctx, db.InsertWithdrawalParams{
					SessionID: sess.ID, Amount: retiro, UserID: userID, RecipientUserID: quien,
				})
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return fmt.Errorf("%w: elige a una persona activa del personal para el retiro", domain.ErrValidation)
					}
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "23503" {
						return fmt.Errorf("%w: elige a una persona activa del personal para el retiro", domain.ErrValidation)
					}
					return err
				}
				view.Withdrawal = &WithdrawalView{Amount: retiro}
				if fila.RecipientName != nil {
					view.Withdrawal.UserName = *fila.RecipientName
				}
			}
		}
		if err := q.CloseSession(ctx, db.CloseSessionParams{ID: sess.ID, ClosedBy: &userID, Notes: n}); err != nil {
			return err
		}
		return auditCash(ctx, q, "cash.closed", sess.ID, &userID, nil,
			map[string]any{"declared": cmd.Declarado, "total": cmd.Total, "reason": cmd.Motivo, "tips": cmd.Propinas})
	})
	if err != nil {
		return nil, traduceConflictoDeCaja(err, db.CashCountMomentCierre)
	}
	view.Status = "cerrada"
	view.FixedFloat = nil // solo viaja en el turno abierto
	// El retiro nace dentro del cierre, después de armar la vista: se releen los movimientos para que
	// esta respuesta diga lo mismo que el corte reabierto (movimientos y egresos).
	moves, err := s.store.QC(ctx).ListCashMovements(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	view.Movements = movementViews(moves)
	view.Breakdown = corteBreakdown(sess.OpeningCash, view.corteMethods, moves)
	if view.TerminalCounts, err = s.terminalCounts(ctx, sess.ID); err != nil {
		return nil, err
	}
	if conteo != nil {
		// Se vuelve a leer el conteo ya guardado para que la diferencia que devuelve el cierre sea la
		// de la columna GENERADA y no una resta que este método haga aparte: dos restas del mismo
		// dinero son dos cifras que tarde o temprano difieren.
		conteos, err := s.conteosDelTurno(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		view.Counts = conteos
		if conteos != nil && conteos.Cierre != nil && view.Drawer != nil {
			view.Drawer.Counted = &conteos.Cierre.Total
			view.Drawer.Difference = conteos.Cierre.Difference
			view.Drawer.RequiresCount = false
		}
	}
	view.Difference = cutDifferenceOf(view.Totals, view.Drawer)
	return view, nil
}

// SellingRegisterOpen: ¿se puede cobrar ahora mismo? Contesta EXACTAMENTE la misma pregunta que
// hace OrdersService.Create antes de aceptar una venta (la caja PRINCIPAL con turno abierto), y no
// "¿hay alguna caja abierta?" como antes: con la caja fuerte abierta el POS decía que sí y el cobro
// tronaba hasta el final, con el ticket ya armado.
//
// Chequeo ligero, sin calcular esperados. Disponible a cualquier rol autenticado: saber si el
// negocio está operando no es dato sensible.
func (s *CashShiftService) SellingRegisterOpen(ctx context.Context) (bool, error) {
	estado, err := s.EstadoDeCaja(ctx)
	if err != nil {
		return false, err
	}
	return estado.Open, nil
}

// EstadoDeCaja contesta lo mismo que SellingRegisterOpen y además si el turno abierto ya no es de
// hoy.
//
// Va colgado del endpoint que el POS YA consulta para saber si puede cobrar: el aviso no cuesta un
// viaje más ni un estado nuevo que sincronizar. Y es un AVISO, nunca un bloqueo: un negocio en
// operación prefiere una fecha corrida a una caja parada, y convertir un descuido administrativo en
// un cobro imposible es peor que el descuido.
func (s *CashShiftService) EstadoDeCaja(ctx context.Context) (EstadoDeCajaView, error) {
	sess, err := s.store.QC(ctx).GetOpenPrimarySession(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return EstadoDeCajaView{}, nil
	}
	if err != nil {
		return EstadoDeCajaView{}, err
	}
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	zona := domain.LoadBusinessLocation(tz)
	fecha := sess.BusinessDate.Time.Format("2006-01-02")
	return EstadoDeCajaView{
		Open: true, OpenedAt: &sess.OpenedAt, BusinessDate: &fecha,
		DeOtroDia: domain.TurnoDeOtroDia(sess.OpenedAt, s.now(), zona),
	}, nil
}
