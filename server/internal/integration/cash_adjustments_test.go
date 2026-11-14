//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
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

// corteConFaltante cierra un turno con el cajón $50 corto ($785 contra $835) y devuelve su id.
func corteConFaltante(t *testing.T, ctx context.Context, st *store.Store, user int64) int64 {
	t.Helper()
	back := app.NewBackofficeService(st, clock)
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, user)
	c, err := back.CloseSession(ctx, principal, user, app.CierreCmd{Piezas: piezasDe(t, st, "500", 1, "100", 2, "20", 4, "5", 1)})
	if err != nil {
		t.Fatalf("cerrar con faltante: %v", err)
	}
	return c.ID
}

// CORREGIR UN CORTE CERRADO SIN REESCRIBIRLO (#290, fase 3). Antes se hacía con SQL y sin rastro.
func TestACorrectionChangesTheDifferenceButNotWhatWasSigned(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	gerente := makeUser(t, st, "gerente_correccion", "gerente")
	id := corteConFaltante(t, ctx, st, gerente)

	adj, err := back.AdjustSession(ctx, id, gerente, domain.AdjustmentInput{
		Kind: "count_correction", After: dp("835"), Reason: "miscount", Note: "Un billete de $50 estaba en el sobre"})
	if err != nil {
		t.Fatalf("corregir el conteo: %v", err)
	}
	if adj.Before == nil || !adj.Before.Equal(decimal.RequireFromString("785")) {
		t.Fatalf("el antes de la corrección es %v; lo firmado era 785", adj.Before)
	}

	det, err := back.SessionDetail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if det.Difference == nil || !det.Difference.Total.IsZero() {
		t.Fatalf("con el conteo corregido a lo esperado, la diferencia debía cuadrar: %+v", det.Difference)
	}
	if det.Drawer == nil || !det.Drawer.Counted.Equal(decimal.RequireFromString("785")) {
		t.Fatalf("lo firmado al cerrar cambió: %v — una corrección no reescribe el conteo", det.Drawer.Counted)
	}
	var firmado string
	_ = st.Pool.QueryRow(ctx, `select total::text from session_cash_counts where session_id = $1 and moment = 'cierre'`, id).Scan(&firmado)
	if firmado != "785.00" {
		t.Fatalf("el conteo guardado pasó a %s", firmado)
	}
	if len(det.Adjustments) != 1 || det.Effective == nil || det.Effective.DrawerCounted == nil || !det.Effective.DrawerCounted.Equal(decimal.RequireFromString("835")) {
		t.Fatalf("el detalle no muestra la corrección: %+v / %+v", det.Adjustments, det.Effective)
	}
	if fila := filaDelHistorico(t, ctx, back, id, true); fila.Difference == nil || !fila.Difference.Total.IsZero() || !fila.Adjusted {
		t.Fatalf("la lista no usa la diferencia corregida: %+v (corregido %v)", fila.Difference, fila.Adjusted)
	}

	// REVERTIR la deja como estaba, y las dos quedan a la vista.
	if _, err := back.ReverseAdjustment(ctx, id, adj.ID, gerente, "Me equivoqué de corte"); err != nil {
		t.Fatalf("revertir: %v", err)
	}
	det, _ = back.SessionDetail(ctx, id)
	if !det.Difference.Total.Equal(decimal.RequireFromString("-50")) || len(det.Adjustments) != 2 || det.Adjustments[0].ReversedBy == nil {
		t.Fatalf("tras revertir: diferencia %s, %d correcciones, revertida por %v", det.Difference.Total, len(det.Adjustments), det.Adjustments[0].ReversedBy)
	}
	if _, err := back.ReverseAdjustment(ctx, id, adj.ID, gerente, "otra vez"); !errors.Is(err, domain.ErrAdjustmentReversed) {
		t.Fatalf("revertir dos veces: %v", err)
	}

	// LA BITÁCORA lo registra en la misma transacción.
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from audit_events where action in ('cash.session_adjusted', 'cash.session_adjustment_reversed') and entity_id = $1`, id).Scan(&n)
	if n != 2 {
		t.Fatalf("eventos de bitácora: %d; quería 2", n)
	}
}

// Dos tabletas revierten la misma corrección a la vez: una gana, la otra recibe «ya se revirtió».
func TestTwoReversalsAtOnceOnlyOneWins(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	gerente := makeUser(t, st, "gerente_reverso_doble", "gerente")
	id := corteConFaltante(t, ctx, st, gerente)
	adj, err := back.AdjustSession(ctx, id, gerente, domain.AdjustmentInput{Kind: "note", Reason: "other", Note: "nota"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); _, errs[i] = back.ReverseAdjustment(ctx, id, adj.ID, gerente, "x") }()
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		} else if !errors.Is(e, domain.ErrAdjustmentReversed) {
			t.Fatalf("el perdedor recibió %v; quería «ya se revirtió»", e)
		}
	}
	if ok != 1 {
		t.Fatalf("ganaron %d reversos", ok)
	}
}

// Los bordes que el servicio cuida: corte abierto, corrección de otro corte, y que corregir un corte
// no mueva lo que espera el turno abierto de esa caja (lo fijó su conteo de apertura).
func TestCorrectionEdges(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	gerente := makeUser(t, st, "gerente_bordes_corr", "gerente")
	id := corteConFaltante(t, ctx, st, gerente)
	principal := registerID(t, st, "Caja principal")

	if _, err := back.OpenSession(ctx, principal, app.AperturaCmd{Total: dp("785"), Motivo: "prueba"}, gerente); err != nil {
		t.Fatal(err)
	}
	abierta, _ := back.CurrentByRegister(ctx, principal)
	if _, err := back.AdjustSession(ctx, abierta.ID, gerente, domain.AdjustmentInput{Kind: "note", Reason: "other", Note: "x"}); !errors.Is(err, domain.ErrSessionNotClosed) {
		t.Fatalf("corregir un turno abierto: %v", err)
	}
	antes := *abierta.Drawer.Expected
	adj, err := back.AdjustSession(ctx, id, gerente, domain.AdjustmentInput{Kind: "float_correction", After: dp("700"), Reason: "float_error", Note: "Se dejaron 700"})
	if err != nil {
		t.Fatal(err)
	}
	despues, _ := back.CurrentByRegister(ctx, principal)
	if !despues.Drawer.Expected.Equal(antes) {
		t.Fatalf("corregir el corte anterior movió lo que espera el turno abierto: %s → %s", antes, *despues.Drawer.Expected)
	}
	if _, err := back.ReverseAdjustment(ctx, abierta.ID, adj.ID, gerente, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revertir desde otro corte: %v", err)
	}
	if _, err := back.AdjustSession(ctx, 999999, gerente, domain.AdjustmentInput{Kind: "note", Reason: "other", Note: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("corte inexistente: %v", err)
	}
}

// AISLAMIENTO (constitución IV, los tres casos): otra empresa ni lee ni corrige ni revierte.
func TestCorrectionsAreIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	gerente := makeUser(t, st, "gerente_aislado_corr", "gerente")
	id := corteConFaltante(t, ctx, st, gerente)
	adj, err := app.NewBackofficeService(st, clock).AdjustSession(ctx, id, gerente, domain.AdjustmentInput{Kind: "note", Reason: "other", Note: "x"})
	if err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, st, "otra-correcciones")
	otroUser := makeUserIn(t, st, other, "gerente_otra_corr", "gerente")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, s *store.Store, c context.Context) {
		b := app.NewBackofficeService(s, clock)
		if _, err := b.AdjustSession(c, id, otroUser, domain.AdjustmentInput{Kind: "note", Reason: "other", Note: "x"}); err == nil {
			t.Fatal("otra empresa corrigió un corte ajeno")
		}
		if _, err := b.ReverseAdjustment(c, id, adj.ID, otroUser, "x"); err == nil {
			t.Fatal("otra empresa revirtió una corrección ajena")
		}
		var n int
		if err := s.Pool.QueryRow(c, `select count(*) from cash_session_adjustments`).Scan(&n); err == nil && n != 0 {
			t.Fatalf("otra empresa leyó %d correcciones", n)
		}
	})
}

// LA SUMA DEL PERIODO (P4): suma la diferencia corregida de TODOS los cortes del filtro, no solo los
// de la página. Excluye ventas, propinas y fondo: es solo sobrantes y faltantes.
func TestThePeriodDifferenceAddsEveryCutOfTheFilter(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	gerente := makeUser(t, st, "gerente_suma_periodo", "gerente")
	corteConFaltante(t, ctx, st, gerente) // −50 en la caja principal
	// −50 en la Caja fuerte: abre con 500 y cierra contando 450.
	fuerte := registerID(t, st, "Caja fuerte")
	if _, err := back.OpenSession(ctx, fuerte, aperturaAMano(dec("500")), gerente); err != nil {
		t.Fatal(err)
	}
	if _, err := back.CloseSession(ctx, fuerte, gerente, app.CierreCmd{Total: dp("450"), Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}
	suma, err := back.SessionPeriodDifference(ctx, domain.SessionHistoryFilter{Limit: 1}, true)
	if err != nil {
		t.Fatal(err)
	}
	if suma == nil || !suma.Equal(decimal.RequireFromString("-100")) {
		t.Fatalf("la suma del periodo es %v; con dos cortes de −50 en dos páginas de uno es −100", suma)
	}
	// Con arqueo ciego, quien no configura la caja no recibe la suma: incluye el último cierre.
	prenderCiego(t, ctx, app.NewSettingsService(st, "pepper-de-prueba"))
	if oculta, err := back.SessionPeriodDifference(ctx, domain.SessionHistoryFilter{Limit: 1}, false); err != nil || oculta != nil {
		t.Fatalf("con arqueo ciego la suma viajó: %v %v", oculta, err)
	}
}

// Por el router: corregir y revertir piden ajustar el arqueo (#290, fase 3). Quien solo opera la caja
// recibe 403, y el gerente corrige; un monto vacío es 400, no una corrección a $0.
func TestCorrectionsHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, nil), Settings: app.NewSettingsService(st, "pepper-de-prueba"),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) (string, int64) {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok, id
	}
	gerente, gerenteID := token("gerente_corr_http", "gerente")
	cajero, _ := token("cajero_corr_http", "cajero")
	id := corteConFaltante(t, ctx, st, gerenteID)
	path := "/api/v1/cash-sessions/" + strconv.FormatInt(id, 10) + "/adjustments"
	cuerpo := `{"kind":"count_correction","after":"835","reason":"miscount","note":"Billete en el sobre"}`
	if w := do(t, r, http.MethodPost, path, cajero, []byte(cuerpo), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("el cajero corrigió un corte: %d", w.Code)
	}
	if w := do(t, r, http.MethodPost, path, gerente, []byte(`{"kind":"count_correction","after":null,"reason":"miscount","note":"x"}`), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("un monto vacío = %d; quería 400", w.Code)
	}
	w := do(t, r, http.MethodPost, path, gerente, []byte(cuerpo), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("el gerente corrige = %d %s", w.Code, w.Body.String())
	}
	var adj struct{ ID int64 }
	_ = json.Unmarshal(w.Body.Bytes(), &adj)
	rev := path + "/" + strconv.FormatInt(adj.ID, 10) + "/reverse"
	if w := do(t, r, http.MethodPost, rev, cajero, []byte(`{"note":"x"}`), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("el cajero revirtió: %d", w.Code)
	}
	if w := do(t, r, http.MethodPost, rev, gerente, []byte(`{"note":"Me equivoqué"}`), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("revertir = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, rev, gerente, []byte(`{"note":"otra vez"}`), "application/json"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "ALREADY_REVERSED") {
		t.Fatalf("revertir dos veces = %d %s", w.Code, w.Body.String())
	}
	// El detalle trae las correcciones como arreglo, nunca null.
	det := do(t, r, http.MethodGet, "/api/v1/cash-sessions/"+strconv.FormatInt(id, 10), gerente, nil, "")
	if !strings.Contains(det.Body.String(), `"adjustments":[{`) {
		t.Fatalf("el detalle no trae las correcciones: %s", det.Body.String()[:200])
	}
}
