//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"uuid"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Defectos que encontró la revisión de código de la 030 (go-backend-reviewer, security-auditor,
// db-architect). Cada test nombra el fallo que atrapa.

// El «+» y la fusión suman sin tope: 100 toques de 10 000 desbordan numeric(8,2) y la cuenta responde
// 500; antes de eso guarda una cantidad que el pedido rechazaría al enviar.
func TestAccumulatedQtyHasACeiling(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café del tope acumulado", pesos("1"), false)
	v := k.newDraft(t, addOf(cafe, "10000"))
	line := v.Lines[0].ID
	_, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), IntoLineID: &line, Qty: pesos("1")})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("«+» por encima de MaxOrderQty = %v, quería 400", err)
	}
	_, err = k.drafts.AddLine(k.ctx, v.ID, addOf(cafe, "1"))
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fusión por encima de MaxOrderQty = %v, quería 400", err)
	}
	_, _, err = k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Actor: k.user,
		Lines: []app.DraftLineCmd{addOf(cafe, "10000"), addOf(cafe, "10000")}})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("dos renglones pelados de 10 000 al crear = %v, quería 400", err)
	}
}

// Una petición con miles de renglones hace miles de consultas antes de chocar con el tope de la
// cuenta (y el import reintentaba Create por cada producto que no existe).
func TestTooManyLinesInOneRequest(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café de la ráfaga", pesos("1"), false)
	many := make([]app.DraftLineCmd, domain.MaxDraftLines+1)
	for i := range many {
		many[i] = app.DraftLineCmd{OpID: uuid.New(), ProductID: cafe, Qty: pesos("1"), Notes: "n" + itoa(i)}
	}
	if _, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Lines: many, Actor: k.user}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Create con %d renglones = %v, quería 400", len(many), err)
	}
	if _, err := k.drafts.Import(k.ctx, []app.ImportAccount{{ID: uuid.New(), Lines: many}}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Import con %d renglones = %v, quería 400", len(many), err)
	}
}

// Una pestaña con una opción borrada tumbaba la subida de TODAS: la tableta no borraba su copia y
// reintentaba para siempre.
func TestImportRejectsOnlyTheBadTab(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café de la pestaña buena", pesos("30"), false)
	opt := optionID(t, k.st, defaultCompanyID)
	if _, err := k.st.Pool.Exec(context.Background(), `delete from modifier_options where id = $1`, opt); err != nil {
		t.Fatal(err)
	}
	ghost := int64(987654321)
	tabs := []app.ImportAccount{
		{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "1")}},
		{ID: uuid.New(), Lines: []app.DraftLineCmd{{OpID: uuid.New(), ProductID: cafe, Qty: pesos("1"),
			Modifiers: []domain.DraftModifier{{OptionID: opt, Qty: 1}}}}},
		{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "2"), addOf(ghost, "1")}},
	}
	res, err := k.drafts.Import(k.ctx, tabs, k.user)
	if err != nil {
		t.Fatalf("una pestaña mala tumbó la subida: %v", err)
	}
	if res[0].Outcome != "created" || res[1].Outcome != "rejected" || res[2].Outcome != "created" {
		t.Fatalf("= %+v; quería created, rejected, created", res)
	}
	got, err := k.drafts.Get(k.ctx, tabs[2].ID)
	if err != nil || len(got.Lines) != 1 {
		t.Fatalf("la tercera subió con %v renglones (%v): el producto que no existe se queda fuera y lo demás sube", got, err)
	}
}

// Con la bolsa agotada por cuentas vivas, la siguiente respondía 409 y ninguna tableta podía vender
// hasta que alguien descartara (o hasta 12 horas). Un nombre numerado es mejor que no vender.
func TestExhaustedBagStillOpensAccounts(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café de la bolsa vacía", pesos("1"), false)
	names := domain.NombresDelEsquema(domain.EsquemaPorDefecto)
	for _, n := range names {
		k.newDraftNamed(t, n, addOf(cafe, "1"))
	}
	v, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user})
	if err != nil {
		t.Fatalf("con %d cuentas vivas la siguiente no abrió: %v", len(names), err)
	}
	if contieneNombre(names, *v.FolioName) {
		t.Fatalf("salió %q, que ya es de una cuenta viva", *v.FolioName)
	}
}

// Una tableta que reusa como opId el id de un renglón de OTRA empresa chocaba con la llave global:
// 23505 → 500 en agregar, y un oráculo de existencia entre empresas.
func TestLineIDsAreScopedByCompany(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	other := makeCompany(t, k.st, "otra-renglon")
	otherUser := makeUserIn(t, k.st, other, "cajero_otra_renglon", "cajero")
	var cafeB int64
	if err := k.st.Pool.QueryRow(context.Background(), `
		with c as (insert into categories (company_id, name) values ($1, 'b') returning id)
		insert into products (company_id, name, category_id, price) select $1, 'Café B renglón', id, 10 from c returning id`,
		other).Scan(&cafeB); err != nil {
		t.Fatal(err)
	}
	theirs, _, err := k.drafts.Create(k.tenant(t, other), app.CreateDraftCmd{ID: uuid.New(), Actor: otherUser,
		Lines: []app.DraftLineCmd{addOf(cafeB, "1")}})
	if err != nil {
		t.Fatal(err)
	}
	cafe := makeProduct(t, k.st, "Café A renglón", pesos("1"), false)
	mine := k.newDraft(t, addOf(cafe, "1"))
	_, err = k.drafts.AddLine(k.ctx, mine.ID, app.DraftLineCmd{OpID: theirs.Lines[0].ID, ProductID: cafe, Qty: pesos("1"), Notes: "x"})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		t.Fatalf("un uuid de otra empresa chocó en la base (%s): %v", pgErr.ConstraintName, err)
	}
	if err != nil {
		t.Fatalf("= %v", err)
	}
}

// Al mandar la cuenta, sus renglones ya viven en el pedido; quedarse con ellos bloqueaba para
// siempre el borrado de cualquier producto que alguna vez se capturó (FK no action).
func TestSendDeletesTheDraftLines(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café borrable", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	if _, err := k.drafts.Send(k.ctx, v.ID, k.user); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from order_draft_lines where draft_id = $1`, v.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("quedaron %d renglones de una cuenta enviada", n)
	}
	again, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil || again.Order.ID == 0 {
		t.Fatalf("el reintento tras borrar los renglones: %v", err)
	}
}

// Una cuenta cuyo pedido ya existía (pestaña importada que sí se había enviado) respondía
// `created: true`: la tableta diría que nació un pedido que ya estaba.
func TestSendOfAnAlreadyExistingOrderIsNotCreated(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café ya pedido", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	if _, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: v.ID, ServiceType: "mostrador", OpenedBy: k.user,
		Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}}); err != nil {
		t.Fatal(err)
	}
	res, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || len(res.PrintLineIDs) != 0 {
		t.Fatalf("created=%v printLineIds=%v", res.Created, res.PrintLineIDs)
	}
}

// Una «Nuevo» que otra tableta descarta mientras ésta le agrega: lo agregado no puede caer en una
// cuenta muerta y responder 200.
func TestNewOfOrderNeverLandsInADiscardedDraft(t *testing.T) {
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	a, b := k.tenant(t, k.company), k.tenant(t, k.company)
	for round := range 10 {
		ord := k.order(t, "1", "0", false)
		first, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
		if err != nil {
			t.Fatal(err)
		}
		var landed *app.DraftView
		var addErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = k.drafts.Discard(a, first.ID, k.user) }()
		go func() { defer wg.Done(); landed, addErr = k.newOf(t, b, ord.ID, addOf(k.product, "1")) }()
		wg.Wait()
		if addErr != nil {
			t.Fatalf("vuelta %d: %v", round, addErr)
		}
		if got := draftStatus(t, k.st, landed.ID); got != domain.DraftCapturing {
			t.Fatalf("vuelta %d: lo agregado cayó en una cuenta %s y se respondió como si nada", round, got)
		}
	}
}

// Barrido contra enviar y contra abrir «Nuevo» a la vez: los candados en orden distinto
// interbloqueaban (40P01 → 500).
func TestSweepNeverDeadlocks(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	a, b, c := k.tenant(t, k.company), k.tenant(t, k.company), k.tenant(t, k.company)
	for round := range 8 {
		ord := k.order(t, "1", "0", false)
		nuevo, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
		if err != nil {
			t.Fatal(err)
		}
		other := k.newDraft(t, addOf(k.product, "1"))
		if _, err := k.st.Pool.Exec(context.Background(),
			`update order_drafts set updated_at = now() - interval '13 hours' where id = any($1)`, []uuid.UUID{nuevo.ID, other.ID}); err != nil {
			t.Fatal(err)
		}
		errs := make([]error, 3)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); _, errs[0] = k.drafts.Send(a, nuevo.ID, k.user) }()
		go func() { defer wg.Done(); _, errs[1] = k.newOf(t, b, ord.ID, addOf(k.product, "1")) }()
		go func() { defer wg.Done(); _, errs[2] = k.accounts.Live(c, false) }()
		wg.Wait()
		for i, err := range errs {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
				t.Fatalf("vuelta %d, operación %d: interbloqueo: %v", round, i, err)
			}
		}
	}
}

// El descuento entraba por POST /pos/drafts e import sin el evento de seguridad ni el tope del
// PATCH: una cuenta nacía al 100 % sin una línea en la bitácora.
func TestDiscountThroughCreateKeepsTheControls(t *testing.T) {
	st := newTestStore(t)
	r, _, token := draftsAPI(t, st)
	_, tok := token("cajero_desc_al_crear", "cajero")
	crepa := makeProduct(t, st, "Crepa con descuento al crear", pesos("100"), false)
	var bitacora bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bitacora, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(anterior)

	body := func() []byte {
		return jsonBody(t, map[string]any{"id": uuid.New().String(), "header": map[string]any{"discount": map[string]any{"percent": "100"}},
			"lines": []map[string]any{{"opId": uuid.New().String(), "productId": crepa, "qty": "1"}}})
	}
	if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, body(), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("crear = %d: %s", w.Code, w.Body)
	}
	if log := bitacora.String(); !strings.Contains(log, "draft_discount_set") || !strings.Contains(log, `"descuento_nuevo":"100.00%"`) {
		t.Fatalf("la cuenta nació con descuento sin evento: %s", log)
	}
	bitacora.Reset()
	imp := jsonBody(t, map[string]any{"accounts": []map[string]any{{"id": uuid.New().String(),
		"header": map[string]any{"discount": map[string]any{"amount": "10"}},
		"lines":  []map[string]any{{"opId": uuid.New().String(), "productId": crepa, "qty": "1"}}}}})
	if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/import", tok, imp, "application/json"); w.Code != http.StatusOK {
		t.Fatalf("import = %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(bitacora.String(), "draft_discount_set") {
		t.Fatalf("el import puso un descuento sin evento: %s", bitacora.String())
	}
	saw429 := false
	for range 200 {
		if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, body(), "application/json"); w.Code == http.StatusTooManyRequests {
			saw429 = true
			break
		}
	}
	if !saw429 {
		t.Fatal("200 cuentas con descuento seguidas no toparon con el limitador del descuento")
	}
}

// El folio de plataforma se cambiaba por PATCH sin el evento que deja su gemelo
// (PATCH /orders/{id}/platform-ref): sin él, dos cambios borran la evidencia del primero.
func TestPlatformRefChangeLeavesTheEvent(t *testing.T) {
	st := newTestStore(t)
	r, _, token := draftsAPI(t, st)
	_, tok := token("cajero_folio_cuenta", "cajero")
	cafe := makeProduct(t, st, "Café con folio", pesos("30"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	id := uuid.New().String()
	w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, jsonBody(t, map[string]any{"id": id,
		"lines": []map[string]any{{"opId": uuid.New().String(), "productId": cafe, "qty": "1"}}}), "application/json")
	hv := rawJSON(t, w.Body.Bytes())["headerVersion"].(float64)
	var bitacora bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bitacora, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(anterior)
	for i, ref := range []string{"UB-1", "UB-2"} {
		body := map[string]any{"expectedHeaderVersion": hv + float64(i), "platformOrderRef": ref}
		if i == 0 {
			body["platformId"] = uber
		}
		if w := do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok, jsonBody(t, body), "application/json"); w.Code != http.StatusOK {
			t.Fatalf("PATCH %s = %d: %s", ref, w.Code, w.Body)
		}
	}
	if log := bitacora.String(); strings.Count(log, "draft_platform_ref_set") != 2 || !strings.Contains(log, `"folio_anterior":"UB-1"`) {
		t.Fatalf("cambiar el folio no dejó el evento con el anterior: %s", log)
	}
}
