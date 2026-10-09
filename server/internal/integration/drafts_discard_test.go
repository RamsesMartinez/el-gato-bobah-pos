//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// DESCARTAR DEJA RASTRO Y DEVUELVE EL NOMBRE A LA BOLSA (D-7, FR-012, US5).
func TestDiscardReturnsTheName(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ctx := context.Background()
	cafe := makeProduct(t, k.st, "Café descartado", pesos("30"), false)
	v := k.newDraftNamed(t, "Persa", addOf(cafe, "1"))
	names, _ := k.orders.NombresDisponibles(k.ctx)
	if contieneNombre(names, "Persa") {
		t.Fatal("«Persa» se ofrece mientras la cuenta vive")
	}

	if err := k.drafts.Discard(k.ctx, v.ID, k.user); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	names, _ = k.orders.NombresDisponibles(k.ctx)
	if !contieneNombre(names, "Persa") {
		t.Fatal("el nombre de la cuenta descartada no volvió a la bolsa")
	}
	var by *int64
	var reason string
	if err := k.st.Pool.QueryRow(ctx, `select discarded_by, discard_reason from order_drafts where id = $1`, v.ID).Scan(&by, &reason); err != nil {
		t.Fatal(err)
	}
	if by == nil || *by != k.user || reason != domain.DiscardManual {
		t.Fatalf("discarded_by=%v reason=%s: el rastro dice quién y por qué", by, reason)
	}
	var orders int
	if err := k.st.Pool.QueryRow(ctx, `select count(*) from orders`).Scan(&orders); err != nil || orders != 0 {
		t.Fatalf("descartar creó %d pedidos: no gasta folio", orders)
	}

	t.Run("dos veces no es error", func(t *testing.T) {
		if err := k.drafts.Discard(k.ctx, v.ID, k.user); err != nil {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("una cuenta vacía se descarta como vacía", func(t *testing.T) {
		e := k.newDraft(t, addOf(cafe, "1"))
		if _, err := k.drafts.RemoveLine(k.ctx, e.ID, e.Lines[0].ID, e.Lines[0].Version); err != nil {
			t.Fatal(err)
		}
		if err := k.drafts.Discard(k.ctx, e.ID, k.user); err != nil {
			t.Fatal(err)
		}
		var reason string
		if err := k.st.Pool.QueryRow(ctx, `select discard_reason from order_drafts where id = $1`, e.ID).Scan(&reason); err != nil || reason != domain.DiscardEmpty {
			t.Fatalf("reason = %s (%v)", reason, err)
		}
	})

	t.Run("una cuenta ya enviada no se descarta: ya es pedido", func(t *testing.T) {
		s := k.newDraft(t, addOf(cafe, "1"))
		if _, err := k.drafts.Send(k.ctx, s.ID, k.user); err != nil {
			t.Fatal(err)
		}
		if err := k.drafts.Discard(k.ctx, s.ID, k.user); !errors.Is(err, domain.ErrDraftAlreadySent) {
			t.Fatalf("= %v, quería DRAFT_SENT", err)
		}
		if got := draftStatus(t, k.st, s.ID); got != domain.DraftSent {
			t.Fatalf("quedó %s", got)
		}
	})

	t.Run("una que no existe", func(t *testing.T) {
		if err := k.drafts.Discard(k.ctx, uuid.New(), k.user); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("= %v", err)
		}
	})
}

// La bolsa se vació y otro se llevó el nombre DESPUÉS de que nació la cuenta: descartarla no se lo
// quita (la guarda de `taken_at`).
func TestDiscardAfterBagRefillKeepsTheNewOwner(t *testing.T) {
	k := newDraftsKit(t)
	ctx := context.Background()
	cafe := makeProduct(t, k.st, "Café de la vuelta", pesos("30"), false)
	v := k.newDraftNamed(t, "Persa", addOf(cafe, "1"))
	if _, err := k.st.Pool.Exec(ctx, `delete from folio_consumido`); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into folio_consumido (scheme, name, taken_at) values ('razas', 'Persa', now() + interval '1 minute')`); err != nil {
		t.Fatal(err)
	}
	if err := k.drafts.Discard(k.ctx, v.ID, k.user); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := k.st.Pool.QueryRow(ctx, `select count(*) from folio_consumido where name = 'Persa'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("filas de «Persa» = %d: se soltó el nombre del dueño nuevo y se repetiría antes de acabar la vuelta", n)
	}
}

func TestDiscardHTTP(t *testing.T) {
	st := newTestStore(t)
	r, broker, token := draftsAPI(t, st)
	cajero, tok := token("cajero_descarta_http", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	cafe := makeProduct(t, st, "Café descartado por HTTP", pesos("30"), false)
	create := func() string {
		id := uuid.New().String()
		if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, jsonBody(t, map[string]any{
			"id": id, "lines": []map[string]any{{"opId": uuid.New().String(), "productId": cafe, "qty": "1"}},
		}), "application/json"); w.Code != http.StatusCreated {
			t.Fatalf("crear = %d: %s", w.Code, w.Body)
		}
		return id
	}
	events, unsubscribe := broker.Subscribe(defaultCompanyID)
	defer unsubscribe()
	id := create()
	nextEvent(t, events)
	w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/"+id+"/discard", tok, []byte(`{}`), "application/json")
	if w.Code != http.StatusNoContent {
		t.Fatalf("discard = %d: %s", w.Code, w.Body)
	}
	if ev := nextEvent(t, events); ev.Type != "draft.updated" {
		t.Fatalf("evento = %s", ev.Type)
	}
	sent := create()
	if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/"+sent+"/send", tok, []byte(`{}`), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("send = %d", w.Code)
	}
	w = do(t, r, http.MethodPost, "/api/v1/pos/drafts/"+sent+"/discard", tok, []byte(`{}`), "application/json")
	m := rawJSON(t, w.Body.Bytes())
	body := m["error"].(map[string]any)
	if w.Code != http.StatusConflict || body["code"] != "DRAFT_SENT" || !strings.Contains(body["message"].(string), "cancelar el pedido") {
		t.Fatalf("discard de una enviada = %d: %s", w.Code, w.Body)
	}
}

func TestDiscardInTheThreeCases(t *testing.T) {
	k := newDraftsKit(t)
	other := makeCompany(t, k.st, "otra-descarte")
	otherUser := makeUserIn(t, k.st, other, "cajero_otra_descarte", "cajero")
	cafe := makeProduct(t, k.st, "Café ajeno al descarte", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewDraftsService(st, app.NewOrdersService(st, clock))
		_ = svc.Discard(ctx, v.ID, otherUser)
	})
	if got := draftStatus(t, k.st, v.ID); got != domain.DraftCapturing {
		t.Fatalf("otra empresa descartó la cuenta de la dueña: %s", got)
	}
}
