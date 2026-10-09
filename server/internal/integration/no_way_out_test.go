//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Un pedido al que se le quitaron todos los productos no se entrega: «Entregar todo» lo cerraba
// como una venta de $0 que el corte y Ventas contaban como venta. Ese pedido se cierra con
// «Cerrar pedido», que es una cancelación.
func TestDeliverAllRejectsAnOrderWithoutProducts(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ord, cajero, _ := pedidoDeVarios(t, st, app.NewOrdersService(st, clock), "sin_productos", 2)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)

	for _, l := range ord.Lines {
		if _, err := svc.CancelarRenglon(tctx, ord.ID, l.ID, cajero, "se equivocó"); err != nil {
			t.Fatalf("cancelar renglón %d: %v", l.ID, err)
		}
	}
	err = svc.DeliverAll(tctx, ord.ID)
	if !errors.Is(err, domain.ErrNoProducts) {
		t.Fatalf("DeliverAll sin productos vivos = %v, quiere ErrNoProducts", err)
	}
	if estado, _ := estadoYCierre(t, st, ord.ID); estado == domain.StatusEntregada {
		t.Fatalf("el pedido sin productos quedó %q: el corte lo contaría como venta de $0", estado)
	}
}

// Quitar lo que falta deja el pedido con lo que sí se entregó, y lo cierra.
//
// El renglón de 3 piezas con 1 entregada se PARTE: las 2 pendientes se van a un renglón nuevo que se
// quita, y la entregada se queda. Partirlo inserta pares de movimientos «renglón partido» que se
// anulan: si moviera existencias, dividir una cuenta descuadraría el almacén.
func TestCancelPendingClosesWithWhatWasDelivered(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_quitar_lo_que_falta", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	frappe := makeProduct(t, st, "Frappé partido", pesos("50"), true)
	refresco := makeProduct(t, st, "Refresco pendiente", pesos("30"), false)
	extra := optionID(t, st, defaultCompanyID, frappe)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)

	ord, err := svc.Create(tctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{
			{ProductID: frappe, Qty: pesos("3"), Notes: "sin crema",
				Modifiers: []domain.OrderModInput{{OptionID: extra, Qty: 1}}},
			{ProductID: refresco, Qty: pesos("1")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	partido, otro := ord.Lines[0].ID, ord.Lines[1].ID
	if err := svc.DeliverLine(tctx, ord.ID, partido, pesos("1")); err != nil {
		t.Fatalf("entregar una pieza: %v", err)
	}
	antes := existencias(t, st, frappe)

	if _, err := svc.CancelPending(tctx, ord.ID, cajero, "   "); !errors.Is(err, domain.ErrValidation) ||
		!strings.Contains(err.Error(), "Elige por qué se quitan") {
		t.Fatalf("sin motivo = %v, quiere «Elige por qué se quitan»", err)
	}

	res, err := svc.CancelPending(tctx, ord.ID, cajero, "Ya no lo quiere")
	if err != nil {
		t.Fatalf("CancelPending: %v", err)
	}
	if res.Removed != 2 || res.Restocked != 0 {
		t.Fatalf("respuesta = %+v, quiere 2 quitados y 0 repuestos (los dos salieron a cocina)", res)
	}

	type renglon struct {
		id                     int64
		product                int64
		qty, delivered, total  decimal.Decimal
		unitPrice, mods, cost  decimal.Decimal
		cancelled, sentKitchen bool
		notes                  *string
		reason                 *string
	}
	rows, err := st.Pool.Query(ctx, `
		select id, product_id, quantity, delivered_qty, line_total, unit_price, modifiers_total, unit_cost,
		       cancelled_at is not null, enviado_a_cocina_at is not null, notes, cancel_reason
		from order_lines where order_id = $1 and product_id = $2 order by id`, ord.ID, frappe)
	if err != nil {
		t.Fatalf("leer renglones: %v", err)
	}
	var del []renglon
	for rows.Next() {
		var r renglon
		if err := rows.Scan(&r.id, &r.product, &r.qty, &r.delivered, &r.total, &r.unitPrice, &r.mods, &r.cost,
			&r.cancelled, &r.sentKitchen, &r.notes, &r.reason); err != nil {
			t.Fatalf("scan: %v", err)
		}
		del = append(del, r)
	}
	rows.Close()
	if len(del) != 2 {
		t.Fatalf("el frappé quedó en %d renglones, quiere 2 (el entregado y el quitado)", len(del))
	}
	queda, quitado := del[0], del[1]
	if queda.id != partido || queda.cancelled || !queda.qty.Equal(pesos("1")) || !queda.delivered.Equal(pesos("1")) ||
		!queda.total.Equal(pesos("70")) {
		t.Fatalf("lo entregado = %+v; quiere 1 pieza entregada, viva, de $70", queda)
	}
	if !quitado.cancelled || !quitado.qty.Equal(pesos("2")) || !quitado.delivered.IsZero() ||
		!quitado.unitPrice.Equal(pesos("50")) || !quitado.mods.Equal(pesos("20")) || !quitado.cost.Equal(queda.cost) ||
		!quitado.total.Equal(pesos("140")) || !quitado.sentKitchen || quitado.notes == nil || *quitado.notes != "sin crema" ||
		quitado.reason == nil || *quitado.reason != "Ya no lo quiere" {
		t.Fatalf("lo quitado = %+v; quiere 2 piezas canceladas con el precio, el costo, la nota y la cocina del original", quitado)
	}
	var mods int
	if err := st.Pool.QueryRow(ctx, `select count(*) from order_line_modifiers where order_line_id = $1 and modifier_option_id = $2`,
		quitado.id, extra).Scan(&mods); err != nil || mods != 1 {
		t.Fatalf("modificadores del renglón nuevo = %d (%v), quiere 1 copiado", mods, err)
	}
	var vivo bool
	if err := st.Pool.QueryRow(ctx, `select cancelled_at is null from order_lines where id = $1`, otro).Scan(&vivo); err != nil || vivo {
		t.Fatalf("el refresco pendiente sigue vivo (%v)", err)
	}

	// [M1] Los pares «renglón partido» se anulan: el almacén queda igual que antes de quitar.
	var pares int
	var neto decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `
		select count(*), coalesce(sum(quantity), 0) from stock_movements
		where order_id = $1 and movement_type = 'venta' and reason = 'renglón partido'`, ord.ID).Scan(&pares, &neto); err != nil {
		t.Fatalf("movimientos del partido: %v", err)
	}
	if pares != 2 || !neto.IsZero() {
		t.Fatalf("movimientos «renglón partido» = %d con neto %s; quiere un par que se anule", pares, neto)
	}
	var enNuevo decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `
		select coalesce(sum(quantity), 0) from stock_movements
		where order_line_id = $1 and movement_type = 'venta'`, quitado.id).Scan(&enNuevo); err != nil {
		t.Fatalf("movimientos del renglón nuevo: %v", err)
	}
	if !enNuevo.Equal(pesos("-2")) {
		t.Fatalf("el renglón nuevo carga %s de existencias, quiere -2 (sus dos piezas)", enNuevo)
	}
	if despues := existencias(t, st, frappe); !despues.Equal(antes) {
		t.Fatalf("existencias %s → %s: partir el renglón movió el almacén", antes, despues)
	}

	if estado, _ := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada {
		t.Fatalf("el pedido quedó %q, quiere entregada: ya no le falta nada", estado)
	}
	var total decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select total from orders where id = $1`, ord.ID).Scan(&total); err != nil || !total.Equal(pesos("70")) {
		t.Fatalf("total = %s (%v), quiere 70", total, err)
	}

	if _, err := svc.CancelPending(tctx, ord.ID, cajero, "Ya no lo quiere"); !errors.Is(err, domain.ErrConflict) ||
		!strings.Contains(err.Error(), "Ya no falta nada por entregar") {
		t.Fatalf("segunda vez = %v, quiere «Ya no falta nada por entregar»", err)
	}
}

// ordersAPI arma el router real sobre el rol de la aplicación. resolve nil = los permisos de hoy.
func ordersAPI(t *testing.T, st *store.Store, resolve httpapi.PermissionResolver) (http.Handler, func(username, role string) (int64, string)) {
	t.Helper()
	appSt := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Orders: app.NewOrdersService(appSt, clock), Broker: realtime.NewBroker(), Permissions: resolve,
	})
	r := httpapi.Router(config.Config{}, jm, h, appSt)
	token := func(username, role string) (int64, string) {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return id, tok
	}
	return r, token
}

// emptiedOrder deja un pedido abierto al que ya se le quitaron todos los productos, con un cobro
// previo de `paid` (cero = sin pagos).
func emptiedOrder(t *testing.T, st *store.Store, cajero int64, suffix string, paid string) int64 {
	t.Helper()
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord := crearPedidoDeVarios(t, st, svc, cajero, suffix, 1)
	if !pesos(paid).IsZero() {
		if _, err := svc.Charge(ctx, app.ChargeCmd{
			OrderID: ord.ID, MethodID: paymentMethodID(t, st, "Efectivo"), Amount: pesos(paid), ActorID: cajero,
		}); err != nil {
			t.Fatalf("Charge: %v", err)
		}
	}
	if !pesos(paid).IsZero() {
		// Con pagos, quitar el producto ya se rechaza (spec 027, FR-023). Este estado solo existe en
		// datos de antes de esa regla —el pedido del 2026-10-04 es uno—, así que se arma directo.
		if _, err := st.Pool.Exec(ctx, `update order_lines set cancelled_at = now(), cancelled_by = $2, cancel_reason = 'se equivocó'
			where order_id = $1`, ord.ID, cajero); err != nil {
			t.Fatalf("quitar el producto: %v", err)
		}
		if err := st.Q.RecalcOrderTotals(ctx, ord.ID); err != nil {
			t.Fatalf("recalcular: %v", err)
		}
		return ord.ID
	}
	if _, err := svc.CancelarRenglon(ctx, ord.ID, ord.Lines[0].ID, cajero, "se equivocó"); err != nil {
		t.Fatalf("quitar el producto: %v", err)
	}
	return ord.ID
}

// [U2] Un pedido sin productos y sin pagos lo cierra CUALQUIER rol, sin motivo: «Cerrar pedido» no
// pregunta nada. Si solo admin y gerente pudieran, el pedido vacío se quedaría en el tablero de un
// cajero sin salida, que es el incidente. Es una cancelación de verdad y cuenta en los reportes.
func TestAnOrderWithoutProductsCanBeClosedByAnyRole(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	r, token := ordersAPI(t, st, nil)
	abridor := makeUser(t, st, "cajero_abre_vacios", "cajero")
	abrirCajaPrincipal(t, st, abridor)

	for _, role := range []string{"admin", "gerente", "cajero", "mesero"} {
		t.Run(role, func(t *testing.T) {
			_, tok := token(role+"_cierra_vacio", role)
			id := emptiedOrder(t, st, abridor, "vacio_"+role, "0")
			movs := countOrderMovements(t, st, id)

			w := do(t, r, http.MethodPost, "/api/v1/orders/"+itoa(int(id))+"/lines/cancel-pending", tok, nil, "")
			if w.Code != http.StatusOK {
				t.Fatalf("cancel-pending sin cuerpo como %s = %d: %s", role, w.Code, w.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["removed"] != float64(0) || body["restocked"] != float64(0) {
				t.Fatalf("respuesta = %s (%v), quiere {removed: 0, restocked: 0}", w.Body.String(), err)
			}
			var estado, motivo string
			if err := st.Pool.QueryRow(ctx, `select status::text, coalesce(cancel_reason, '') from orders where id = $1`, id).
				Scan(&estado, &motivo); err != nil {
				t.Fatalf("leer el pedido: %v", err)
			}
			if estado != domain.StatusCancelada || motivo != "Sin productos" {
				t.Fatalf("el pedido quedó %q con motivo %q, quiere cancelada con «Sin productos»", estado, motivo)
			}
			if n := countOrderMovements(t, st, id); n != movs {
				t.Fatalf("movimientos de inventario %d → %d: cada producto ya se resolvió al quitarlo", movs, n)
			}
		})
	}

	// Cuenta como cancelación en el resumen de Ventas.
	var fecha pgtype.Date
	if err := st.Pool.QueryRow(ctx, `select business_date from orders order by id desc limit 1`).Scan(&fecha); err != nil {
		t.Fatalf("fecha: %v", err)
	}
	rows, err := db.New(st.Pool).SalesTotalsByStatus(ctx, db.SalesTotalsByStatusParams{Desde: fecha, Hasta: fecha})
	if err != nil {
		t.Fatalf("SalesTotalsByStatus: %v", err)
	}
	canceladas := int32(0)
	for _, row := range rows {
		if row.Status == db.OrderStatusCancelada {
			canceladas = row.Ventas
		}
	}
	if canceladas != 4 {
		t.Fatalf("Ventas cuenta %d cancelaciones, quiere las 4", canceladas)
	}

	t.Run("con pagos se rechaza", func(t *testing.T) {
		appSt := appRoleStore(t)
		tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
		if err != nil {
			t.Fatalf("AcquireTenant: %v", err)
		}
		defer release()
		id := emptiedOrder(t, st, abridor, "vacio_con_pago", "20")
		_, err = app.NewOrdersService(appSt, clock).CancelPending(tctx, id, abridor, "")
		if !errors.Is(err, domain.ErrOrderHasPayments) {
			t.Fatalf("sin productos y con pagos = %v, quiere ErrOrderHasPayments", err)
		}
		if estado, _ := estadoYCierre(t, st, id); estado == domain.StatusCancelada {
			t.Fatal("se canceló un pedido con pagos sin devolverlos: ese dinero saldría del corte sin rastro")
		}
	})
}

// [U3] Las rutas preguntan por permiso. Hoy todos los roles tienen `orders.cancel_pending`, así que
// el 403 se ve con un resolutor que no da ninguno; sin esta prueba, una ruta que perdiera su gate
// pasaría todo lo demás.
func TestCancelRoutesAskForTheirPermission(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	abridor := makeUser(t, st, "cajero_permisos_rutas", "cajero")
	abrirCajaPrincipal(t, st, abridor)

	t.Run("quitar lo que falta sin el permiso", func(t *testing.T) {
		none := func(domain.Role) []domain.Permission { return []domain.Permission{} }
		r, token := ordersAPI(t, st, none)
		_, tok := token("admin_sin_permisos", "admin")
		id := emptiedOrder(t, st, abridor, "sin_permiso", "0")
		w := do(t, r, http.MethodPost, "/api/v1/orders/"+itoa(int(id))+"/lines/cancel-pending", tok, nil, "")
		assertForbidden(t, w.Code, w.Body.Bytes(), "Tu usuario no puede quitar productos")
		if estado, _ := estadoYCierre(t, st, id); estado == domain.StatusCancelada {
			t.Fatal("el 403 no detuvo la cancelación")
		}
	})

	// Cajero y mesero quitan lo que falta, pero cancelar el pedido entero saca dinero del corte:
	// los dos tienen que topar con el 403, no solo el primero que alguien se acordó de probar.
	for _, role := range []string{"cajero", "mesero"} {
		t.Run("cancelar el pedido como "+role, func(t *testing.T) {
			r, token := ordersAPI(t, st, nil)
			_, tok := token(role+"_no_cancela", role)
			ord := crearPedidoDeVarios(t, st, app.NewOrdersService(st, clock), abridor, "no_cancela_"+role, 1)
			w := do(t, r, http.MethodPost, "/api/v1/orders/"+itoa(int(ord.ID))+"/cancel", tok,
				[]byte(`{"reason":"Ya no lo quiere"}`), "application/json")
			assertForbidden(t, w.Code, w.Body.Bytes(), "Tu usuario no puede cancelar pedidos")
			if estado, _ := estadoYCierre(t, st, ord.ID); estado == domain.StatusCancelada {
				t.Fatal("el 403 no detuvo la cancelación")
			}
		})
	}
}

// Los rechazos de «Cerrar pedido» llegan a la tarjeta como texto. Un «conflicto» pelón —el nombre
// del sentinel, sin frase— no le dice a quien opera qué pasó ni qué hacer.
//
// El caso de fondo: un pedido ya entregado que se queda sin productos vivos (se le quitó todo
// después de entregarlo). Sin pagos, el plan es «cerrarlo vacío», pero una entregada no pasa a
// cancelada.
func TestCancelPendingRejectionsSpeakToTheOperator(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	r, token := ordersAPI(t, st, nil)
	abridor := makeUser(t, st, "cajero_rechazos_con_texto", "cajero")
	abrirCajaPrincipal(t, st, abridor)
	_, tok := token("gerente_rechazos_con_texto", "gerente")

	svc := app.NewOrdersService(st, clock)
	entregado := crearPedidoDeVarios(t, st, svc, abridor, "entregado_vacio", 1)
	if err := svc.DeliverAll(ctx, entregado.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update order_lines set cancelled_at = now() where order_id = $1`, entregado.ID); err != nil {
		t.Fatalf("vaciar el pedido entregado: %v", err)
	}

	cases := []struct {
		name   string
		path   string
		body   []byte
		status int
		text   string
	}{
		{"entregado sin productos vivos", "/api/v1/orders/" + itoa(int(entregado.ID)) + "/lines/cancel-pending", nil,
			http.StatusConflict, "Ese pedido ya se cerró"},
		{"pedido que no existe", "/api/v1/orders/999999999/lines/cancel-pending", nil,
			http.StatusNotFound, "Ese pedido no existe"},
		{"número de pedido ilegible", "/api/v1/orders/abc/lines/cancel-pending", nil,
			http.StatusBadRequest, "Ese número de pedido no es válido"},
		{"cuerpo ilegible", "/api/v1/orders/" + itoa(int(entregado.ID)) + "/lines/cancel-pending", []byte(`{"reason":`),
			http.StatusBadRequest, "No se pudo leer el motivo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := do(t, r, http.MethodPost, c.path, tok, c.body, "application/json")
			var env struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatalf("respuesta ilegible: %v (%s)", err, w.Body.String())
			}
			if w.Code != c.status || env.Error.Message != c.text {
				t.Fatalf("= %d %q, quiere %d %q", w.Code, env.Error.Message, c.status, c.text)
			}
		})
	}
	if estado, _ := estadoYCierre(t, st, entregado.ID); estado != domain.StatusEntregada {
		t.Fatalf("el pedido entregado quedó %q", estado)
	}
}

func assertForbidden(t *testing.T, code int, body []byte, text string) {
	t.Helper()
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, quiere 403: %s", code, body)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Error.Message != text {
		t.Fatalf("texto = %q (%v), quiere %q", env.Error.Message, err, text)
	}
}

// Quitar lo que falta no puede dejar el total por debajo de lo cobrado: el pedido de $100 cobrado
// completo, con la mitad entregada, quedaría en $50 con $100 en caja y ninguna venta que explique
// los otros $50. Es el mismo rechazo que quitar un producto.
func TestCancelPendingCannotLeaveTheOrderOverpaid(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ord, cajero, efectivo := pedidoDeVarios(t, st, app.NewOrdersService(st, clock), "sobrepagado", 2)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)
	if _, err := svc.Charge(tctx, app.ChargeCmd{OrderID: ord.ID, MethodID: efectivo, Amount: pesos("100"), ActorID: cajero}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if err := svc.DeliverLine(tctx, ord.ID, ord.Lines[0].ID, pesos("1")); err != nil {
		t.Fatalf("DeliverLine: %v", err)
	}

	_, err = svc.CancelPending(tctx, ord.ID, cajero, "Ya no lo quiere")
	if !errors.Is(err, domain.ErrOrderWouldBeOverpaid) {
		t.Fatalf("quitar lo pagado = %v, quiere ErrOrderWouldBeOverpaid", err)
	}
	var vivos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from order_lines where order_id = $1 and cancelled_at is null`, ord.ID).
		Scan(&vivos); err != nil || vivos != 2 {
		t.Fatalf("renglones vivos = %d (%v): el rechazo tiene que deshacer todo lo quitado", vivos, err)
	}
}

// NINGUNA SECUENCIA DEJA UN PEDIDO SIN SALIDA (SC-003).
//
// El incidente del 2026-10-04 fue exactamente eso: un pedido abierto sin una acción que lo cerrara,
// que bloqueó el corte de caja. Se recorren secuencias de cobrar, quitar (un producto y lo que
// falta, con sus rechazos), pasar, entregar y devolver en varios órdenes, y al final a cada pedido
// abierto se le APLICA su salida —entregar lo pendiente, cobrar lo que falta, o quitar lo que falta
// si se quedó vacío— y tiene que terminar cerrado. No basta con que exista un botón: tiene que
// funcionar.
func TestNoSequenceLeavesAnOrderWithoutAWayOut(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	type action struct {
		name string
		run  func(s *splitTable, touched map[int64]bool)
	}
	lastPayment := func(s *splitTable) int64 {
		var id int64
		_ = st.Pool.QueryRow(context.Background(), `select coalesce(max(id), 0) from order_payments where order_id = $1`, s.order.ID).Scan(&id)
		return id
	}
	actions := []action{
		{"cobra el 1", func(s *splitTable, _ map[int64]bool) {
			_, _ = s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, ClientUUID: uuid.New(),
				Lines: []domain.SelectedPieces{{LineID: s.order.Lines[0].ID, Qty: pesos("1")}}})
		}},
		{"cobra 30 por monto", func(s *splitTable, _ map[int64]bool) {
			_, _ = s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, Amount: pesos("30")})
		}},
		{"cobra lo que falta", func(s *splitTable, _ map[int64]bool) {
			_, _ = s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, ActorID: s.cashier, ClientUUID: uuid.New(), AllRemaining: true})
		}},
		{"quita el 2", func(s *splitTable, _ map[int64]bool) {
			_, _ = s.svc.CancelarRenglon(s.ctx, s.order.ID, s.order.Lines[1].ID, s.cashier, "Ya no lo quiere")
		}},
		{"quita todos, uno por uno", func(s *splitTable, _ map[int64]bool) {
			for _, l := range s.order.Lines {
				_, _ = s.svc.CancelarRenglon(s.ctx, s.order.ID, l.ID, s.cashier, "Ya no lo quiere")
			}
		}},
		{"quita lo que falta", func(s *splitTable, _ map[int64]bool) {
			_, _ = s.svc.CancelPending(s.ctx, s.order.ID, s.cashier, "Ya no lo quiere")
		}},
		{"entrega el 1", func(s *splitTable, _ map[int64]bool) {
			_ = s.svc.DeliverLine(s.ctx, s.order.ID, s.order.Lines[0].ID, pesos("1"))
		}},
		{"pasa el 3 a uno nuevo", func(s *splitTable, touched map[int64]bool) {
			if r, err := s.svc.MoveLines(s.ctx, app.MoveLinesCmd{ClientUUID: uuid.New(), FromOrderID: s.order.ID, ActorID: s.cashier,
				Lines: []domain.SelectedPieces{{LineID: s.order.Lines[2].ID, Qty: pesos("1")}}}); err == nil {
				touched[r.To.ID] = true
			}
		}},
		{"devuelve el último pago", func(s *splitTable, _ map[int64]bool) {
			if id := lastPayment(s); id != 0 {
				_, _ = s.svc.VoidPayment(s.ctx, s.order.ID, id, s.cashier, "Se le cobró a otra persona")
			}
		}},
	}
	// Todas las parejas y tríos en orden, con un recorrido fijo: reproducible, y suficiente para
	// cruzar cada acción con cada otra antes y después.
	var sequences [][]int
	for a := range actions {
		for b := range actions {
			sequences = append(sequences, []int{a, b})
			for c := range actions {
				if (a+b+c)%3 == 0 {
					sequences = append(sequences, []int{a, b, c})
				}
			}
		}
	}
	for n, seq := range sequences {
		s := newSplitTable(t, st, "salida_"+strconv.Itoa(n), "50", "60", "70")
		touched := map[int64]bool{s.order.ID: true}
		names := []string{}
		for _, i := range seq {
			actions[i].run(s, touched)
			names = append(names, actions[i].name)
		}
		for id := range touched {
			if err := closeWithItsWayOut(s, id); err != nil {
				t.Fatalf("tras %v el pedido %d no tiene salida: %v", names, id, err)
			}
		}
		s.done()
	}
}

// closeWithItsWayOut aplica a un pedido la salida que el tablero le ofrece y exige que quede cerrado.
func closeWithItsWayOut(s *splitTable, id int64) error {
	ctx := context.Background()
	var status string
	var live, pending, payments int
	var outstanding decimal.Decimal
	if err := s.st.Pool.QueryRow(ctx, `
		select o.status::text,
		       (select count(*) from order_lines where order_id = o.id and cancelled_at is null),
		       (select count(*) from order_lines where order_id = o.id and cancelled_at is null and delivered_qty < quantity),
		       (select count(*) from order_payments where order_id = o.id),
		       greatest(o.total - (select coalesce(sum(amount), 0) from order_payments where order_id = o.id), 0)
		  from orders o where o.id = $1`, id).Scan(&status, &live, &pending, &payments, &outstanding); err != nil {
		return err
	}
	if status == domain.StatusCancelada || status == domain.StatusReembolsada {
		return nil
	}
	if live == 0 {
		if payments > 0 {
			return fmt.Errorf("sin productos y con %d pagos: solo sale devolviéndolos", payments)
		}
		if _, err := s.svc.CancelPending(s.ctx, id, s.cashier, ""); err != nil {
			return fmt.Errorf("«Cerrar pedido» sin productos: %w", err)
		}
		return nil
	}
	if pending > 0 {
		if err := s.svc.DeliverAll(s.ctx, id); err != nil {
			return fmt.Errorf("«Entregar todo»: %w", err)
		}
	}
	if outstanding.IsPositive() {
		if _, err := s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: id, MethodID: s.cash, ActorID: s.cashier, Amount: outstanding}); err != nil {
			return fmt.Errorf("«Cobrar %s»: %w", outstanding, err)
		}
	}
	if err := s.st.Pool.QueryRow(ctx, `select status::text from orders where id = $1`, id).Scan(&status); err != nil {
		return err
	}
	if status != domain.StatusEntregada {
		return fmt.Errorf("tras su salida quedó %s", status)
	}
	return nil
}
