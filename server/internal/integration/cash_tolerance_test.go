//go:build integration

package integration

import (
	"context"
	"errors"
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
	if err := back.SetCashTolerance(ctx, dp("40")); err != nil {
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
}

// AISLAMIENTO (los tres casos): la tolerancia de una empresa no se lee ni se cambia desde otra.
func TestCashToleranceIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	if err := app.NewBackofficeService(st, clock).SetCashTolerance(ctx, dp("75")); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, st, "otra-tolerancia")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, s *store.Store, c context.Context) {
		b := app.NewBackofficeService(s, clock)
		if tol, err := b.CashTolerance(c); err == nil && tol.Equal(decimal.RequireFromString("75")) {
			t.Fatal("otra empresa leyó la tolerancia ajena")
		}
		_ = b.SetCashTolerance(c, dp("1"))
	})
	tol, err := app.NewBackofficeService(st, clock).CashTolerance(ctx)
	if err != nil || !tol.Equal(decimal.RequireFromString("75")) {
		t.Fatalf("otra empresa cambió la tolerancia: %v %v", tol, err)
	}
}
