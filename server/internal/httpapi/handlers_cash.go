package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/shopspring/decimal"
)

// Handlers de caja: abrir, cerrar, el turno vivo, movimientos, propinas e histórico (spec 082, fase
// 2: partidos de handlers_backoffice.go sin cambiar rutas ni respuestas).

// piezasBody: lo que la pantalla manda cuando el operador contó el cajón.
//
// El total NO viaja aquí: si viniera junto a las piezas habría dos cifras del mismo dinero, y el
// servidor tendría que elegir. Los dos caminos van en campos distintos justo para poder rechazar
// que lleguen los dos (FR-015).
type piezasBody struct {
	DenominationID int64 `json:"denominationId"`
	Pieces         int   `json:"pieces"`
}

// piezasDelBody traduce los renglones del cuerpo, sin decidir nada: quién valida es el servicio.
func piezasDelBody(piezas []piezasBody) []app.PiezaCapturada {
	if len(piezas) == 0 {
		return nil
	}
	out := make([]app.PiezaCapturada, 0, len(piezas))
	for _, p := range piezas {
		out = append(out, app.PiezaCapturada{DenominationID: p.DenominationID, Pieces: p.Pieces})
	}
	return out
}

func aperturaDelBody(piezas []piezasBody, total *decimal.Decimal, motivo string) app.AperturaCmd {
	return app.AperturaCmd{Piezas: piezasDelBody(piezas), Total: total, Motivo: motivo}
}

func (h *Handlers) OpenCashSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID int64        `json:"registerId"`
		Counts     []piezasBody `json:"counts"`
		// OpeningCash sigue llamándose igual para no romper el nombre que la pantalla ya usa, pero
		// ahora es el camino MANUAL: exige `manualReason` y es excluyente con `counts`.
		OpeningCash  *decimal.Decimal `json:"openingCash"`
		ManualReason string           `json:"manualReason"`
		// Motivo de una apertura distinta del cierre anterior (spec 032, punto 6).
		OpeningReason     string `json:"openingReason"`
		OpeningReasonNote string `json:"openingReasonNote"`
		// OpenedAt: abrir desde una hora anterior (spec 048), RFC3339. Exige cash.adjust_count.
		OpenedAt *time.Time `json:"openedAt"`
		// ExcludePending: abrir dejando sin incluir los gastos sin corte (R13). Exige cash.adjust_count.
		ExcludePending bool `json:"excludePending"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	// Mover la hora o excluir los gastos sin corte mueven el esperado del corte: por permiso, aquí y
	// no en el router, porque abrir sin tocar nada lo sigue pudiendo hacer el cajero (R13).
	if (body.OpenedAt != nil || body.ExcludePending) && !h.requirePermission(w, r, domain.PermCashAdjustCount) {
		return
	}
	apertura := aperturaDelBody(body.Counts, body.OpeningCash, body.ManualReason)
	apertura.Reason, apertura.ReasonNote = body.OpeningReason, body.OpeningReasonNote
	apertura.OpenedAt, apertura.ExcludePending = body.OpenedAt, body.ExcludePending
	sess, err := h.backoffice.OpenSession(r.Context(), body.RegisterID, apertura, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, sess)
}

// GET /cash-sessions/pending-expenses?registerId= — los gastos sin corte de la caja desde su último
// cierre, que la apertura ofrece incluir (spec 048). registerId ausente o malformado = 400.
func (h *Handlers) PendingExpenses(w http.ResponseWriter, r *http.Request) {
	reg, err := strconv.ParseInt(r.URL.Query().Get("registerId"), 10, 64)
	if err != nil || reg <= 0 {
		Error(w, fmt.Errorf("%w: registerId", domain.ErrValidation))
		return
	}
	items, err := h.backoffice.PendingExpenses(r.Context(), reg)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /cash-sessions/attendance?registerId= — qué pide la asistencia para abrir o cerrar esa caja
// (spec 046). Un registerId ausente o malformado se rechaza: no hay caja «por omisión».
func (h *Handlers) CashSessionAttendance(w http.ResponseWriter, r *http.Request) {
	registerID, err := strconv.ParseInt(r.URL.Query().Get("registerId"), 10, 64)
	if err != nil || registerID <= 0 {
		Error(w, fmt.Errorf("%w: falta la caja", domain.ErrValidation))
		return
	}
	u, _ := userFrom(r.Context())
	state, err := h.backoffice.CashAttendanceState(r.Context(), registerID, u.ID, h.permissionsOf(r.Context(), u))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, state)
}

// GET /cash-sessions/current?registerId= — sesión abierta de una caja (null si está cerrada).
func (h *Handlers) CurrentCashSession(w http.ResponseWriter, r *http.Request) {
	registerID, err := strconv.ParseInt(r.URL.Query().Get("registerId"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	sess, err := h.backoffice.CurrentByRegister(r.Context(), registerID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sess) // null si esa caja no está abierta
}

func (h *Handlers) CloseCashSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID int64                      `json:"registerId"`
		Declared   map[string]decimal.Decimal `json:"declared"` // methodId(string) → contado
		Counts     []piezasBody               `json:"counts"`   // el cajón, contado por denominaciones
		// CountedCash + ManualReason son el camino MANUAL del cajón, igual que `openingCash` en la
		// apertura: se usa cuando hay algo que el catálogo de denominaciones no puede expresar. El
		// motivo es obligatorio (FR-014); sin él quedaría un arqueo con una cifra que nadie puede
		// reconstruir. Va aparte de `declared` a propósito: el dinero del cajón NO se declara por
		// método, y un método de cajón que llegue en `declared` se rechaza.
		CountedCash  *decimal.Decimal `json:"countedCash"`
		ManualReason string           `json:"manualReason"`
		Notes        string           `json:"notes"`
		TipsDecision string           `json:"tipsDecision"`
		// terminalId(string) → total del corte de la terminal (spec 032, arqueo por terminal).
		TerminalCounts map[string]decimal.Decimal `json:"terminalCounts"`
		// FloatLeft: el fondo que se queda para el siguiente turno (2026-10-10).
		FloatLeft *decimal.Decimal `json:"floatLeft"`
		// FixedFloatShown y WithdrawalUserID (spec 035): el fijo que la hoja mostró y quién se lleva
		// el retiro. Ausentes en una tableta con el front anterior: el servidor usa el fijo vigente y
		// a quien cierra.
		FixedFloatShown  *decimal.Decimal `json:"fixedFloatShown"`
		WithdrawalUserID *int64           `json:"withdrawalUserId"`
		// AuthorizedClose (spec 046): cerrar sin la salida de quien abrió. Una tableta con el front
		// anterior no lo manda y recibe el 409 de siempre.
		AuthorizedClose *struct {
			Reason string `json:"reason"`
		} `json:"authorizedClose"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	// Una llave que no parsea se RECHAZA, no se descarta. Descartarla cerraba el turno como si
	// nadie hubiera declarado ese método: el corte comparaba lo esperado contra cero y le inventaba
	// al cajero un faltante por todo lo que sí había contado.
	declared := map[int]decimal.Decimal{}
	for k, v := range body.Declared {
		id, err := strconv.Atoi(k)
		if err != nil {
			Error(w, fmt.Errorf("%w: %q no es un método de pago", domain.ErrValidation, k))
			return
		}
		declared[id] = v
	}
	terminales := map[int64]decimal.Decimal{}
	for k, v := range body.TerminalCounts {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			Error(w, fmt.Errorf("%w: %q no es una terminal", domain.ErrValidation, k))
			return
		}
		terminales[id] = v
	}
	u, _ := userFrom(r.Context())
	var authorized *string
	if body.AuthorizedClose != nil {
		authorized = &body.AuthorizedClose.Reason
	}
	sess, err := h.backoffice.CloseSession(r.Context(), body.RegisterID, u.ID, app.CierreCmd{
		Perms: h.permissionsOf(r.Context(), u), AuthorizedReason: authorized,
		TerminalCounts: terminales, FloatLeft: body.FloatLeft,
		FixedFloatShown: body.FixedFloatShown, WithdrawalUserID: body.WithdrawalUserID,
		Declarado: declared, Piezas: piezasDelBody(body.Counts),
		Total: body.CountedCash, Motivo: body.ManualReason, Notas: body.Notes, Propinas: body.TipsDecision,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sess)
}

// POST /cash-sessions/transfer — traspaso de efectivo entre dos cajas abiertas.
func (h *Handlers) CashTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromRegisterID int64           `json:"fromRegisterId"`
		ToRegisterID   int64           `json:"toRegisterId"`
		Amount         decimal.Decimal `json:"amount"`
		Note           string          `json:"note"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.backoffice.Transfer(r.Context(), body.FromRegisterID, body.ToRegisterID, body.Amount, body.Note, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// GET /cash-status — ¿se puede cobrar? (la caja principal con turno abierto). El POS lo usa para
// mostrar o bloquear la pantalla de venta, así que responde la misma regla que el cobro.
func (h *Handlers) CashStatus(w http.ResponseWriter, r *http.Request) {
	estado, err := h.backoffice.EstadoDeCaja(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, estado)
}

// GET /cash-sessions/{id}/sales — las ventas de un corte, paginadas.
//
// El detalle del corte ya trae la primera página; esto existe para poder llegar al resto. Un arqueo
// cuyas ventas no se pueden recorrer completas no se puede auditar, y la pantalla decir "hay 340 y
// te muestro 200" resuelve la honestidad pero no el problema.
func (h *Handlers) CashSessionSales(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	// La misma paginación del resto de las listas, con sus mismos rechazos: un tamaño absurdo se
	// devuelve como 400 y nunca cae a un default en silencio.
	limit, offset, err := paginaDeQuery(r.URL.Query(), 20)
	if err != nil {
		Error(w, err)
		return
	}
	ventas, total, ingreso, err := h.backoffice.SessionSalesPage(r.Context(), id, limit, offset)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{
		"items": ventas, "total": total, "salesTotal": ingreso,
	})
}

// GET /cash-sessions?page=&pageSize=&from=&to= — histórico de cortes, paginado y opcionalmente
// acotado por el día del turno. Sin páginas solo existían los 50 más recientes: un corte más viejo
// no se podía abrir desde la pantalla donde se audita.
func (h *Handlers) CashHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset, err := paginaDeQuery(q, 20)
	if err != nil {
		Error(w, err)
		return
	}
	f := domain.SessionHistoryFilter{Limit: limit, Offset: offset}
	for _, p := range []struct {
		name string
		dst  **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if v := q.Get(p.name); v != "" {
			d, err := parseDate(v, time.Time{})
			if err != nil {
				Error(w, err)
				return
			}
			*p.dst = &d
		}
	}
	u, _ := userFrom(r.Context())
	canSee := slices.Contains(h.permissionsOf(r.Context(), u), domain.PermCashConfigure)
	rows, total, err := h.backoffice.SessionHistoryFor(r.Context(), f, canSee)
	if err != nil {
		Error(w, err)
		return
	}
	// La suma de sobrantes y faltantes del filtro, no de la página (#290, P4).
	periodo, err := h.backoffice.SessionPeriodDifference(r.Context(), f, canSee)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows, "total": total, "page": offset / limit, "pageSize": limit,
		"periodDifference": periodo})
}

// GET /cash-sessions/{id} — detalle de un corte (totales guardados + movimientos).
func (h *Handlers) CashSessionDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	u, _ := userFrom(r.Context())
	view, err := h.backoffice.SessionDetailFor(r.Context(), id, slices.Contains(h.permissionsOf(r.Context(), u), domain.PermCashConfigure))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, view)
}

// POST /cash-sessions/movements — registra entrada/salida de efectivo en la sesión abierta de una caja.
func (h *Handlers) CreateCashMovement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID int64 `json:"registerId"`
		app.CashMovementCmd
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	body.UserID = u.ID
	sess, err := h.backoffice.RecordCashMovement(r.Context(), body.RegisterID, body.CashMovementCmd)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, sess)
}

// PendingTips: la propina por entregar del turno abierto de una caja.
func (h *Handlers) PendingTips(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("registerId"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, fmt.Errorf("%w: caja inválida", domain.ErrValidation))
		return
	}
	v, err := h.tips.Pending(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// PayoutTips: reparte la propina pendiente entre una o varias personas.
func (h *Handlers) PayoutTips(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RegisterID int64 `json:"registerId"`
		app.TipPayoutCmd
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	body.ActorID = u.ID
	items, err := h.tips.Payout(r.Context(), body.RegisterID, body.TipPayoutCmd)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"items": items})
}

// CorrectCashOut: reverso de una salida + la salida bien capturada (spec 032, punto 4).
func (h *Handlers) CorrectCashOut(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, domain.ErrValidation)
		return
	}
	var body app.CashMovementCmd
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	body.UserID = u.ID
	sess, err := h.backoffice.CorrectCashOut(r.Context(), id, body)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, sess)
}

// CashAlerts: propina sin entregar y salidas sin concepto de las cajas abiertas (spec 032).
func (h *Handlers) CashAlerts(w http.ResponseWriter, r *http.Request) {
	v, err := h.backoffice.CashAlerts(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /cash-sessions/{id}/adjustments — corrige un corte cerrado sin reescribirlo (#290, fase 3).
func (h *Handlers) AdjustCashSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Kind   string           `json:"kind"`
		After  *decimal.Decimal `json:"after"`
		Reason string           `json:"reason"`
		Note   string           `json:"note"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	canSee := slices.Contains(h.permissionsOf(r.Context(), u), domain.PermCashConfigure)
	adj, err := h.backoffice.AdjustSession(r.Context(), id, u.ID, domain.AdjustmentInput{
		Kind: body.Kind, After: body.After, Reason: body.Reason, Note: body.Note}, canSee)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, adj)
}

// POST /cash-sessions/{id}/adjustments/{adjId}/reverse — deshace una corrección con otra.
func (h *Handlers) ReverseCashSessionAdjustment(w http.ResponseWriter, r *http.Request) {
	id, err1 := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	adjID, err2 := strconv.ParseInt(chi.URLParam(r, "adjId"), 10, 64)
	if err1 != nil || err2 != nil || id <= 0 || adjID <= 0 {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	canSee := slices.Contains(h.permissionsOf(r.Context(), u), domain.PermCashConfigure)
	adj, err := h.backoffice.ReverseAdjustment(r.Context(), id, adjID, u.ID, body.Note, canSee)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, adj)
}
