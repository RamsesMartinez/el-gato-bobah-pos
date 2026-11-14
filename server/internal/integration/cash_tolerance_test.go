//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// «POR EXPLICAR» (#290, fase 4; decisión P1). Un corte que pasa la tolerancia y nadie explicó sale
// marcado para quien configura la caja; una nota al cerrar o el motivo de la apertura siguiente lo
// explican. No bloquea el cierre.
func TestACutOverToleranceNeedsExplanation(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	gerente := makeUser(t, st, "gerente_tolerancia", "gerente")
	if err := back.SetCashTolerance(ctx, gerente, dp("40")); err != nil {
		t.Fatal(err)
	}
	if tol, err := back.CashTolerance(ctx); err != nil || !tol.Equal(decimal.RequireFromString("40")) {
		t.Fatalf("tolerancia guardada: %v %v", tol, err)
	}
	id := corteConFaltante(t, ctx, st, gerente) // −50, pasa la tolerancia de 40
	fila := filaDelHistorico(t, ctx, back, id, true)
	if fila.NeedsExplanation == nil || !*fila.NeedsExplanation {
		t.Fatalf("un faltante de 50 con tolerancia de 40 no salió «por explicar»: %v", fila.NeedsExplanation)
	}
	if oculta := filaDelHistorico(t, ctx, back, id, false); oculta.NeedsExplanation != nil {
		t.Fatal("la marca viajó a quien no configura la caja")
	}
	if _, err := back.AdjustSession(ctx, id, gerente, domain.AdjustmentInput{Kind: "note", Reason: "other", Note: "El hielo se pagó del cajón"}, true); err != nil {
		t.Fatal(err)
	}
	if f := filaDelHistorico(t, ctx, back, id, true); f.NeedsExplanation == nil || *f.NeedsExplanation {
		t.Fatalf("con una nota el corte siguió «por explicar»: %v", f.NeedsExplanation)
	}
	det, _ := back.SessionDetail(ctx, id)
	if det.NeedsExplanation == nil || *det.NeedsExplanation {
		t.Fatalf("el detalle no coincide con la lista: %v", det.NeedsExplanation)
	}
}

// La explicación al cerrar se guarda como nota en la misma transacción; con arqueo ciego, quien no
// configura la caja no la puede mandar (sería una sonda de si hubo diferencia).
func TestTheExplanationAtCloseIsANoteAndNotAProbe(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_explica_cierre", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	prenderCiego(t, ctx, app.NewSettingsService(st, "pepper-de-prueba"))
	exp := &app.CloseExplanation{Reason: "unrecorded_movement", Note: "Se pagó el hielo sin registrarlo"}
	_, err := back.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 2, "20", 4, "5", 1), Explanation: exp, CanSeeArqueo: false})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("una explicación en un cierre ciego: %v; quería 403", err)
	}
	c, err := back.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 2, "20", 4, "5", 1), Explanation: exp, CanSeeArqueo: true})
	if err != nil {
		t.Fatal(err)
	}
	det, _ := back.SessionDetail(ctx, c.ID)
	if len(det.Adjustments) != 1 || det.Adjustments[0].Kind != "note" || det.Adjustments[0].ReasonNote != exp.Note {
		t.Fatalf("la explicación no quedó como nota del corte: %+v", det.Adjustments)
	}
}

// Por el router: leer la tolerancia es de quien opera la caja; cambiarla, de quien la configura. Un
// valor vacío, negativo o absurdo es 400.
func TestCashToleranceHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, nil), Settings: app.NewSettingsService(st, "pepper-de-prueba"),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	gerente, cajero := token("gerente_tol_http", "gerente"), token("cajero_tol_http", "cajero")
	if w := do(t, r, http.MethodGet, "/api/v1/settings/cash-close", cajero, nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"tolerance":"0`) {
		t.Fatalf("el cajero lee la tolerancia = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPut, "/api/v1/settings/cash-close", cajero, []byte(`{"tolerance":"50"}`), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("el cajero cambió la tolerancia = %d", w.Code)
	}
	for _, malo := range []string{`{}`, `{"tolerance":null}`, `{"tolerance":"-1"}`, `{"tolerance":"1e100000000"}`} {
		if w := do(t, r, http.MethodPut, "/api/v1/settings/cash-close", gerente, []byte(malo), "application/json"); w.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d; quería 400", malo, w.Code)
		}
	}
	if w := do(t, r, http.MethodPut, "/api/v1/settings/cash-close", gerente, []byte(`{"tolerance":"50"}`), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("el gerente cambia la tolerancia = %d %s", w.Code, w.Body.String())
	}

	// Un rol propio que configura la caja y no la opera (roles híbridos) lee la tolerancia que él mismo
	// edita: con «operar» en el GET, la pantalla de Administrar le daba 403 al cargar (revisión fase 4).
	ctx := context.Background()
	var propio int64
	if err := st.Pool.QueryRow(ctx, `insert into roles (company_id, name) values ($1, 'Contador de cajas') returning id`, defaultCompanyID).Scan(&propio); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
		select $1, $2, id, true from permissions where code = 'cash.configure'`, defaultCompanyID, propio); err != nil {
		t.Fatal(err)
	}
	contador := makeUser(t, st, "contador_tol_http", "cajero")
	if _, err := st.Pool.Exec(ctx, `update users set role_id = $2 where id = $1`, contador, propio); err != nil {
		t.Fatal(err)
	}
	tokContador, err := jm.Issue(domain.User{ID: contador, CompanyID: defaultCompanyID, Name: "contador", Role: domain.RoleCajero})
	if err != nil {
		t.Fatal(err)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/settings/cash-close", tokContador, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("quien solo configura la caja lee la tolerancia = %d; quería 200", w.Code)
	}
}

// Cambiar la tolerancia reclasifica todos los cortes pasados: queda en la bitácora quién, cuándo, de
// cuánto a cuánto (revisión de la fase 4: subirla de $0 a $500 borraba las marcas sin rastro).
func TestChangingTheToleranceIsAudited(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	gerente := makeUser(t, st, "gerente_tol_bitacora", "gerente")
	if err := back.SetCashTolerance(ctx, gerente, dp("500")); err != nil {
		t.Fatal(err)
	}
	var antes, despues string
	var actor int64
	if err := st.Pool.QueryRow(ctx, `select before->>'tolerance', after->>'tolerance', actor_user_id from audit_events
		where action = 'settings.cash_tolerance' order by id desc limit 1`).Scan(&antes, &despues, &actor); err != nil {
		t.Fatalf("el cambio de tolerancia no quedó en la bitácora: %v", err)
	}
	if antes != "0" || despues != "500" || actor != gerente {
		t.Fatalf("bitácora = %s → %s por %d; quería 0 → 500 por %d", antes, despues, actor, gerente)
	}
}

// AISLAMIENTO (los tres casos): la tolerancia de una empresa no se lee ni se cambia desde otra.
func TestCashToleranceIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	gerenteDeLaPrueba := makeUser(t, st, "gerente_tol_aislada", "gerente")
	if err := app.NewBackofficeService(st, clock).SetCashTolerance(ctx, gerenteDeLaPrueba, dp("75")); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, st, "otra-tolerancia")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, s *store.Store, c context.Context) {
		b := app.NewBackofficeService(s, clock)
		// Sin la empresa de la sesión rige la más estricta: $0, nunca la ajena.
		if tol, err := b.CashTolerance(c); err != nil || !tol.IsZero() {
			t.Fatalf("leyó %v (%v); quería $0, no la tolerancia ajena", tol, err)
		}
		// Escribir desde otra empresa o sin empresa no encuentra fila (otra empresa tiene la suya, que no
		// es la que se mira abajo).
		_ = b.SetCashTolerance(c, gerenteDeLaPrueba, dp("1"))
	})
	tol, err := app.NewBackofficeService(st, clock).CashTolerance(ctx)
	if err != nil || !tol.Equal(decimal.RequireFromString("75")) {
		t.Fatalf("otra empresa cambió la tolerancia: %v %v", tol, err)
	}
}

// Por el router: con arqueo ciego, un cajero que manda la explicación recibe 403 y el turno sigue
// abierto; el permiso sale del handler, no de quien llama al servicio (revisión de seguridad de la
// fase 4: la prueba de servicio pasaba CanSeeArqueo a mano y un cambio en el handler no la ponía en
// rojo). Una nota así quitaría la marca «por explicar» de un faltante que el cajero no vio.
func TestABlindCashierCannotExplainThroughTheRouter(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, clock), Settings: settings,
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	cajero := makeUser(t, st, "cajero_ciego_router", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	prenderCiego(t, ctx, settings)
	tok, err := jm.Issue(domain.User{ID: cajero, CompanyID: defaultCompanyID, Name: "cajero", Role: domain.RoleCajero})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(`{"registerId":%d,"countedCash":"700","manualReason":"prueba","tipsDecision":"quedan_en_caja",
		"explanation":{"reason":"unrecorded_movement","note":"Se pagó el hielo"}}`, principal))
	if w := do(t, r, http.MethodPost, "/api/v1/cash-sessions/close", tok, body, "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("un cajero con arqueo ciego explicó la diferencia = %d %s; quería 403", w.Code, w.Body.String())
	}
	var abierta bool
	if err := st.Pool.QueryRow(ctx, `select exists(select 1 from register_sessions where register_id = $1 and status = 'abierta')`, principal).Scan(&abierta); err != nil || !abierta {
		t.Fatalf("el turno se cerró con el 403: abierta=%v %v", abierta, err)
	}
}
