//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func decPtr(s string) *decimal.Decimal { v := dec(s); return &v }

// abrirTurnoFF abre la caja en cero, sin pedir motivo.
func abrirTurnoFF(t *testing.T, back *app.BackofficeService, ctx context.Context, reg, user int64) {
	t.Helper()
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: decPtr("0"), Motivo: "prueba", Reason: "float_changed"}, user); err != nil {
		t.Fatalf("abrir: %v", err)
	}
}

// ---- US1: el fondo fijo se configura una vez ----

func TestFixedFloatIsSetPerRegisterAndValidated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	reg := principalRegister(t, st)

	v, err := back.SetFixedFloat(ctx, reg, decPtr("1000"))
	if err != nil || v.FixedFloat == nil || !v.FixedFloat.Equal(dec("1000")) {
		t.Fatalf("fijar $1,000: %+v, %v", v, err)
	}
	todas, err := back.AllCashRegisters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var visto bool
	for _, r := range todas {
		if r.ID == reg && r.FixedFloat != nil && r.FixedFloat.Equal(dec("1000")) {
			visto = true
		}
	}
	if !visto {
		t.Fatalf("Ajustes no ve el fondo fijo: %+v", todas)
	}
	for _, malo := range []string{"-1", "100000000"} {
		if _, err := back.SetFixedFloat(ctx, reg, decPtr(malo)); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("fondo %s: err = %v, quería ErrValidation", malo, err)
		}
	}
	if _, err := back.SetFixedFloat(ctx, 999999, decPtr("10")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("caja inexistente: err = %v", err)
	}
	v, err = back.SetFixedFloat(ctx, reg, nil)
	if err != nil || v.FixedFloat != nil {
		t.Fatalf("quitar el fondo fijo: %+v, %v", v, err)
	}
}

// LA APERTURA SE CUENTA A CIEGAS: la lista de cajas del cajero no trae el fijo, que casi siempre es
// lo que esperará la apertura.
func TestCashierRegisterListDoesNotRevealFixedFloat(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	if _, err := back.SetFixedFloat(ctx, principalRegister(t, st), decPtr("1000")); err != nil {
		t.Fatal(err)
	}
	cajas, err := back.CashRegisters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cajas)
	if strings.Contains(string(raw), "fixedFloat") || strings.Contains(string(raw), "1000") {
		t.Fatalf("la lista del cajero revela el fondo fijo y con él el esperado de la apertura: %s", raw)
	}
}

func TestFixedFloatIsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	reg := principalRegister(t, st)
	if _, err := back.SetFixedFloat(ctx, reg, decPtr("1000")); err != nil {
		t.Fatal(err)
	}
	otra := makeCompany(t, st, "ff-otra")
	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, st *store.Store, ctx context.Context) {
		b := app.NewBackofficeService(st, clock)
		if _, err := b.SetFixedFloat(ctx, reg, decPtr("1")); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("cambiar el fondo de otra empresa: err = %v", err)
		}
		todas, err := b.AllCashRegisters(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range todas {
			if r.ID == reg {
				t.Errorf("se ve la caja de la otra empresa: %+v", r)
			}
		}
	})
	var fijo decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select fixed_float from cash_registers where id = $1`, reg).Scan(&fijo); err != nil || !fijo.Equal(dec("1000")) {
		t.Fatalf("el fondo de la dueña cambió: %s, %v", fijo, err)
	}
}

// ---- US2: el cierre trae el fondo escrito ----

func TestCloseUsesFixedFloatAndSnapshotsWhatWasShown(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_ff_cierre", "cajero")
	reg := principalRegister(t, st)
	if _, err := back.SetFixedFloat(ctx, reg, decPtr("1000")); err != nil {
		t.Fatal(err)
	}
	abrirTurnoFF(t, back, ctx, reg, cajero)

	abierta, err := back.CurrentByRegister(ctx, reg)
	if err != nil || abierta.FixedFloat == nil || !abierta.FixedFloat.Equal(dec("1000")) {
		t.Fatalf("la hoja de cierre no recibe el fijo: %+v, %v", abierta, err)
	}
	// Un cliente sin `floatLeft` (front anterior) deja el fijo.
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("5300"), Motivo: "prueba"})
	if err != nil {
		t.Fatal(err)
	}
	if v.FloatLeft == nil || !v.FloatLeft.Equal(dec("1000")) || v.FixedFloatAtClose == nil || !v.FixedFloatAtClose.Equal(dec("1000")) {
		t.Fatalf("cierre sin cifra: deja %v, snapshot %v", v.FloatLeft, v.FixedFloatAtClose)
	}

	// Si el fijo cambia con la hoja abierta, el snapshot es lo que la hoja mostró: si no, el cierre
	// diría «se dejó distinto del fijo» sin que nadie lo cambiara.
	abrirTurnoFF(t, back, ctx, reg, cajero)
	if _, err := back.SetFixedFloat(ctx, reg, decPtr("1500")); err != nil {
		t.Fatal(err)
	}
	v, err = back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("5300"), Motivo: "prueba",
		FloatLeft: decPtr("1000"), FixedFloatShown: decPtr("1000"), WithdrawalUserID: &cajero})
	if err != nil {
		t.Fatal(err)
	}
	if !v.FixedFloatAtClose.Equal(dec("1000")) {
		t.Fatalf("el snapshot tomó el fijo nuevo y no el que se mostró: %v", v.FixedFloatAtClose)
	}
}

// Una tableta con el front anterior no manda floatLeft: un fijo mayor a lo contado no puede dejarla
// sin cerrar. Una cifra explícita mayor sí se rechaza.
func TestFixedFloatOverCountedNeverBlocksAnOldClient(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_ff_viejo", "cajero")
	reg := principalRegister(t, st)
	if _, err := back.SetFixedFloat(ctx, reg, decPtr("1000")); err != nil {
		t.Fatal(err)
	}
	abrirTurnoFF(t, back, ctx, reg, cajero)
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("800"), Motivo: "prueba", FloatLeft: decPtr("900")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fondo explícito mayor a lo contado: err = %v", err)
	}
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("800"), Motivo: "prueba"})
	if err != nil {
		t.Fatalf("el fijo le rebotó el cierre a un cliente viejo: %v", err)
	}
	if !v.FloatLeft.Equal(dec("800")) || v.Withdrawal != nil {
		t.Fatalf("deja %v, retiro %+v; quería 800 y sin retiro", v.FloatLeft, v.Withdrawal)
	}
}

// ---- US3: el retiro queda registrado ----

func TestWithdrawalIsRecordedOnceAndNeverTouchesTheCount(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_ff_retiro", "cajero")
	luis := makeUser(t, st, "luis_ff_retiro", "gerente")
	reg := principalRegister(t, st)
	if _, err := back.SetFixedFloat(ctx, reg, decPtr("1000")); err != nil {
		t.Fatal(err)
	}
	abrirTurnoFF(t, back, ctx, reg, cajero)
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("5300"), Motivo: "prueba",
		FloatLeft: decPtr("1000"), WithdrawalUserID: &luis})
	if err != nil {
		t.Fatal(err)
	}
	if v.Withdrawal == nil || !v.Withdrawal.Amount.Equal(dec("4300")) || v.Withdrawal.UserName != "Test luis_ff_retiro" {
		t.Fatalf("retiro: %+v", v.Withdrawal)
	}
	// El arqueo se firmó antes del retiro: la diferencia es lo contado contra el esperado, nada más.
	if v.Drawer == nil || v.Drawer.Difference == nil || !v.Drawer.Difference.Equal(dec("5300")) {
		t.Fatalf("el retiro movió la diferencia del cajón: %+v", v.Drawer)
	}

	// El neto del cajón (lo que lee cualquier esperado recalculado) no ve el retiro.
	if net, err := st.Q.NetCashMovements(ctx, v.ID); err != nil || !net.IsZero() {
		t.Fatalf("el retiro entró al neto de movimientos del cajón: %s, %v", net, err)
	}

	det, err := back.SessionDetail(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range det.Breakdown.Ingresos {
		for _, it := range m.Items {
			if it.Concept == "Ventas" && !it.Amount.IsZero() {
				t.Fatalf("el retiro se restó de las ventas: «Ventas» de %s = %s en un turno sin ventas", m.Method, it.Amount)
			}
		}
	}
	var retiroEnEgresos bool
	for _, e := range det.Breakdown.Egresos {
		if e.Concept == "Salidas de efectivo" {
			t.Fatalf("el retiro se contó como salida del negocio: %+v", det.Breakdown.Egresos)
		}
		if e.Concept == "Retiro al cierre" && e.Amount.Equal(dec("4300")) {
			retiroEnEgresos = true
		}
	}
	if !retiroEnEgresos {
		t.Fatalf("el corte no nombra el retiro: %+v", det.Breakdown.Egresos)
	}
	if det.Withdrawal == nil || det.FloatLeft == nil || !det.FloatLeft.Equal(dec("1000")) {
		t.Fatalf("el corte cerrado no trae fondo y retiro: %+v %v", det.Withdrawal, det.FloatLeft)
	}
	if det.Drawer == nil || !det.Drawer.Difference.Equal(dec("5300")) {
		t.Fatalf("el corte cerrado movió la diferencia: %+v", det.Drawer)
	}

	// Un segundo retiro del mismo turno lo frena la base aunque algún camino se saltara el candado.
	var n int
	_, err = st.Pool.Exec(ctx, `insert into register_cash_movements (session_id, kind, amount, concept, user_id, recipient_user_id, recipient_name)
		values ($1, 'retiro', 1, 'x', $2, $2, 'x')`, v.ID, cajero)
	if err == nil {
		t.Fatal("la base aceptó dos retiros del mismo turno")
	}
	_ = st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where session_id = $1 and kind = 'retiro'`, v.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("retiros del turno: %d", n)
	}
}

func TestWithdrawalRecipientRules(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_ff_quien", "cajero")
	reg := principalRegister(t, st)

	// Sin fijo y con fondo tecleado, el retiro se registra igual; sin persona, es de quien cierra.
	abrirTurnoFF(t, back, ctx, reg, cajero)
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("500"), Motivo: "prueba", FloatLeft: decPtr("200")})
	if err != nil {
		t.Fatal(err)
	}
	if v.Withdrawal == nil || !v.Withdrawal.Amount.Equal(dec("300")) || v.Withdrawal.UserName != "Test cajero_ff_quien" {
		t.Fatalf("retiro sin fijo ni persona: %+v", v.Withdrawal)
	}

	// Todo se queda: sin retiro ni fila.
	abrirTurnoFF(t, back, ctx, reg, cajero)
	v, err = back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("500"), Motivo: "prueba", FloatLeft: decPtr("500")})
	if err != nil || v.Withdrawal != nil {
		t.Fatalf("todo se queda: %+v, %v", v.Withdrawal, err)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where session_id = $1`, v.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("un cierre sin retiro dejó %d movimientos", n)
	}

	// Alguien de otra empresa no puede llevarse el retiro (bajo el rol de la app, como en producción).
	otra := makeCompany(t, st, "ff-quien-otra")
	ajeno := makeUserIn(t, st, otra, "ajeno_ff", "gerente")
	app2 := appRoleStore(t)
	ctxA, release, err := app2.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	b2 := app.NewBackofficeService(app2, clock)
	abrirTurnoFF(t, b2, ctxA, reg, cajero)
	if _, err := b2.CloseSession(ctxA, reg, cajero, app.CierreCmd{Total: decPtr("500"), Motivo: "prueba", FloatLeft: decPtr("0"), WithdrawalUserID: &ajeno}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("retiro a alguien de otra empresa: err = %v", err)
	}
}

// El fondo fijo lo configura quien tiene el permiso, no quien opera la caja; y una cifra malformada
// es 400, nunca un fondo en cero.
func TestFixedFloatEndpointNeedsPermission(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	appSt := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(appSt, clock), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, appSt)
	token := func(username, role string) (int64, string) {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return id, tok
	}
	_, cajeroTok := token("http_cajero_fondo", "cajero")
	_, gerenteTok := token("http_gerente_fondo", "gerente")
	reg := principalRegister(t, st)
	path := "/api/v1/cash-registers/" + strconv.FormatInt(reg, 10) + "/fixed-float"

	if w := do(t, r, http.MethodPut, path, cajeroTok, []byte(`{"fixedFloat": 1000}`), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("un cajero cambió el fondo fijo: %d %s", w.Code, w.Body)
	}
	if w := do(t, r, http.MethodPut, path, gerenteTok, []byte(`{"fixedFloat": "mil"}`), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("fondo malformado: %d %s", w.Code, w.Body)
	}
	w := do(t, r, http.MethodPut, path, gerenteTok, []byte(`{"fixedFloat": 1000}`), "application/json")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"fixedFloat":"1000"`) {
		t.Fatalf("el gerente no pudo fijar el fondo: %d %s", w.Code, w.Body)
	}
	w = do(t, r, http.MethodPut, path, gerenteTok, []byte(`{"fixedFloat": null}`), "application/json")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"fixedFloat":null`) {
		t.Fatalf("quitar el fondo fijo: %d %s", w.Code, w.Body)
	}
}

// ---- Revisión de código (T025–T027) ----

// Con arqueo ciego, el cierre anterior no puede decirle al cajero la cifra contra la que se va a
// comparar su apertura: el fondo que se dejó, o lo contado menos el retiro, ES ese esperado.
func TestClosedShiftDetailDoesNotRevealNextOpeningToCashier(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Backoffice: app.NewBackofficeService(appSt, clock), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, appSt)
	token := func(username, role string) (int64, string) {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return id, tok
	}
	cajero, cajeroTok := token("cajero_ff_ciego", "cajero")
	_, gerenteTok := token("gerente_ff_ciego", "gerente")
	prenderCiego(t, ctx, app.NewSettingsService(st, "pepper-de-prueba"))
	reg := principalRegister(t, st)
	abrirTurnoFF(t, back, ctx, reg, cajero)
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("5300"), Motivo: "prueba", FloatLeft: decPtr("1000")})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/cash-sessions/" + strconv.FormatInt(v.ID, 10)
	type detalle struct {
		FloatLeft         *string `json:"floatLeft"`
		FixedFloatAtClose *string `json:"fixedFloatAtClose"`
		Withdrawal        *struct {
			Amount string `json:"amount"`
		} `json:"withdrawal"`
		Movements []struct {
			Kind string `json:"kind"`
		} `json:"movements"`
		Breakdown struct {
			Egresos []struct {
				Concept string `json:"concept"`
			} `json:"egresos"`
		} `json:"breakdown"`
	}
	leer := func(tok string) detalle {
		w := do(t, r, http.MethodGet, path, tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("detalle: %d %s", w.Code, w.Body)
		}
		var d detalle
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	c := leer(cajeroTok)
	if c.FloatLeft != nil || c.FixedFloatAtClose != nil || c.Withdrawal != nil {
		t.Fatalf("el cajero leyó el esperado de su próxima apertura en el cierre anterior: fondo %v, fijo %v, retiro %v",
			c.FloatLeft, c.FixedFloatAtClose, c.Withdrawal)
	}
	for _, m := range c.Movements {
		if m.Kind == domain.CashRetiro {
			t.Fatal("el cajero leyó el retiro en los movimientos del cierre anterior (contado − retiro = esperado de apertura)")
		}
	}
	for _, e := range c.Breakdown.Egresos {
		if e.Concept == "Retiro al cierre" {
			t.Fatal("el cajero leyó el retiro en los egresos del cierre anterior")
		}
	}
	g := leer(gerenteTok)
	if g.FloatLeft == nil || g.Withdrawal == nil || g.Withdrawal.Amount != "4300" {
		t.Fatalf("quien configura la caja dejó de ver el fondo y el retiro: %+v", g)
	}
}

// La respuesta del cierre y el corte reabierto describen el mismo turno: el retiro aparece en los
// dos, en movimientos y en egresos.
func TestCloseResponseShowsWithdrawalLikeTheReopenedShift(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_ff_resp", "cajero")
	reg := principalRegister(t, st)
	abrirTurnoFF(t, back, ctx, reg, cajero)
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: decPtr("5300"), Motivo: "prueba", FloatLeft: decPtr("1000")})
	if err != nil {
		t.Fatal(err)
	}
	det, err := back.SessionDetail(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	retiroEn := func(movs []app.CashMovementView) int {
		n := 0
		for _, m := range movs {
			if m.Kind == domain.CashRetiro {
				n++
			}
		}
		return n
	}
	if retiroEn(v.Movements) != 1 || retiroEn(det.Movements) != 1 {
		t.Fatalf("retiro en movimientos: cierre %d, corte reabierto %d", retiroEn(v.Movements), retiroEn(det.Movements))
	}
	if !v.Breakdown.EgresosTotal.Equal(det.Breakdown.EgresosTotal) || len(v.Breakdown.Egresos) != len(det.Breakdown.Egresos) {
		t.Fatalf("egresos: cierre %+v, corte reabierto %+v", v.Breakdown.Egresos, det.Breakdown.Egresos)
	}
}

// Un retiro rechazado no deja un cierre a medias: el turno sigue abierto, sin fondo, sin snapshot
// del fijo y sin conteo de cierre firmado.
func TestRejectedWithdrawalLeavesTheShiftOpenAndUntouched(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_ff_atom", "cajero")
	reg := principalRegister(t, st)
	if _, err := app.NewBackofficeService(st, clock).SetFixedFloat(ctx, reg, decPtr("200")); err != nil {
		t.Fatal(err)
	}
	otra := makeCompany(t, st, "ff-atom-otra")
	ajeno := makeUserIn(t, st, otra, "ajeno_ff_atom", "gerente")
	appSt := appRoleStore(t)
	ctxA, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	b := app.NewBackofficeService(appSt, clock)
	abrirTurnoFF(t, b, ctxA, reg, cajero)
	if _, err := b.CloseSession(ctxA, reg, cajero, app.CierreCmd{Total: decPtr("500"), Motivo: "prueba",
		FloatLeft: decPtr("200"), WithdrawalUserID: &ajeno}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("retiro a alguien de otra empresa: err = %v", err)
	}
	var status string
	var floatLeft, fijo *decimal.Decimal
	var conteos int
	if err := st.Pool.QueryRow(ctx, `select s.status::text, s.float_left, s.fixed_float_at_close,
		(select count(*) from session_cash_counts c where c.session_id = s.id and c.moment = 'cierre')
		from register_sessions s where s.register_id = $1 order by s.id desc limit 1`, reg).
		Scan(&status, &floatLeft, &fijo, &conteos); err != nil {
		t.Fatal(err)
	}
	if status != "abierta" || floatLeft != nil || fijo != nil || conteos != 0 {
		t.Fatalf("el retiro rechazado dejó un cierre a medias: status %s, fondo %v, fijo %v, conteos de cierre %d",
			status, floatLeft, fijo, conteos)
	}
}
