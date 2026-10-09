//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// draftsAPI es el router real con los servicios de la cuenta en captura sobre el rol de la app.
func draftsAPI(t *testing.T, st *store.Store) (http.Handler, *realtime.Broker, func(username, role string) (int64, string)) {
	t.Helper()
	appSt := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	broker := realtime.NewBroker()
	orders := app.NewOrdersService(appSt, clock)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Orders: orders, Drafts: app.NewDraftsService(appSt, orders), Broker: broker,
		Backoffice: app.NewBackofficeService(appSt, clock),
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
	return r, broker, token
}

func jsonBody(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rawJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("respuesta ilegible: %v: %s", err, body)
	}
	return m
}

// mustArray falla si el campo no es un arreglo en el JSON CRUDO: `null` tumba la pantalla al primer
// `.map()` con el servidor respondiendo 200 (AGENTS.md §1).
func mustArray(t *testing.T, m map[string]any, k string) []any {
	t.Helper()
	v, ok := m[k].([]any)
	if !ok {
		t.Fatalf("%s = %#v, quería un arreglo (nunca null ni ausente)", k, m[k])
	}
	return v
}

// nextEvent espera el siguiente evento del broker o falla.
func nextEvent(t *testing.T, ch <-chan realtime.Event) realtime.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó ningún evento: las otras tabletas no se enterarían")
	}
	return realtime.Event{}
}

func TestDraftsHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	r, broker, token := draftsAPI(t, st)
	_, tok := token("cajero_http_cuentas", "cajero")
	cafe := makeProduct(t, st, "Café HTTP", pesos("30"), false)
	events, unsubscribe := broker.Subscribe(defaultCompanyID)
	defer unsubscribe()

	expectDraftEvent := func(t *testing.T, id string) {
		t.Helper()
		ev := nextEvent(t, events)
		data, _ := json.Marshal(ev.Data)
		m := rawJSON(t, data)
		if ev.Type != "draft.updated" || m["id"] != id {
			t.Fatalf("evento = %s %s, quería draft.updated de %s", ev.Type, data, id)
		}
		for _, k := range []string{"orderId", "status", "updatedAt"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("el evento no trae %s: %s", k, data)
			}
		}
	}

	id := uuid.New().String()
	op := uuid.New().String()
	create := jsonBody(t, map[string]any{
		"id": id, "folioName": "Persa",
		"lines": []map[string]any{{"opId": op, "productId": cafe, "qty": "1", "modifiers": []any{}, "notes": ""}},
	})

	t.Run("crear: 201 y luego 200 en el reintento", func(t *testing.T) {
		w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, create, "application/json")
		if w.Code != http.StatusCreated {
			t.Fatalf("POST = %d: %s", w.Code, w.Body)
		}
		m := rawJSON(t, w.Body.Bytes())
		lines := mustArray(t, m, "lines")
		mustArray(t, m, "unavailable")
		mustArray(t, lines[0].(map[string]any), "modifiers")
		if m["folioName"] != "Persa" || m["status"] != "capturando" {
			t.Fatalf("cuerpo = %s", w.Body)
		}
		expectDraftEvent(t, id)
		w = do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, create, "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("reintento = %d: %s", w.Code, w.Body)
		}
		// El reintento también avisa: si fue la «Nuevo» de otra tableta, sí cambió.
		expectDraftEvent(t, id)
	})

	var lineID string
	var lineVersion float64
	t.Run("leer", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/pos/drafts/"+id, tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET = %d: %s", w.Code, w.Body)
		}
		l := mustArray(t, rawJSON(t, w.Body.Bytes()), "lines")[0].(map[string]any)
		lineID, lineVersion = l["id"].(string), l["version"].(float64)
		if w := do(t, r, http.MethodGet, "/api/v1/pos/drafts/"+uuid.New().String(), tok, nil, ""); w.Code != http.StatusNotFound {
			t.Fatalf("GET de una cuenta que no existe = %d", w.Code)
		}
	})

	t.Run("agregar, cambiar, quitar", func(t *testing.T) {
		add := jsonBody(t, map[string]any{"opId": uuid.New().String(), "productId": cafe, "qty": "1"})
		if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/"+id+"/lines", tok, add, "application/json"); w.Code != http.StatusOK {
			t.Fatalf("agregar = %d: %s", w.Code, w.Body)
		}
		expectDraftEvent(t, id)
		stale := jsonBody(t, map[string]any{"expectedVersion": lineVersion, "qty": "5"})
		w := do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id+"/lines/"+lineID, tok, stale, "application/json")
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "DRAFT_CHANGED") {
			t.Fatalf("cambiar con versión vieja = %d: %s", w.Code, w.Body)
		}
		cur := mustArray(t, rawJSON(t, do(t, r, http.MethodGet, "/api/v1/pos/drafts/"+id, tok, nil, "").Body.Bytes()), "lines")[0].(map[string]any)
		v := cur["version"].(float64)
		change := jsonBody(t, map[string]any{"expectedVersion": v, "notes": "sin hielo"})
		if w := do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id+"/lines/"+lineID, tok, change, "application/json"); w.Code != http.StatusOK {
			t.Fatalf("cambiar = %d: %s", w.Code, w.Body)
		}
		expectDraftEvent(t, id)
		w = do(t, r, http.MethodDelete, "/api/v1/pos/drafts/"+id+"/lines/"+lineID+"?expectedVersion="+itoa(int(v)+1), tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("quitar = %d: %s", w.Code, w.Body)
		}
		expectDraftEvent(t, id)
	})

	t.Run("parámetros inválidos son 400", func(t *testing.T) {
		cases := []struct{ method, path, body string }{
			{http.MethodGet, "/api/v1/pos/drafts/no-es-uuid", ""},
			{http.MethodPost, "/api/v1/pos/drafts/no-es-uuid/lines", `{"opId":"` + uuid.New().String() + `","productId":1,"qty":"1"}`},
			{http.MethodDelete, "/api/v1/pos/drafts/" + id + "/lines/" + uuid.New().String() + "?expectedVersion=dos", ""},
			{http.MethodDelete, "/api/v1/pos/drafts/" + id + "/lines/" + uuid.New().String(), ""},
			{http.MethodPatch, "/api/v1/pos/drafts/" + id + "/lines/no-es-uuid", `{"expectedVersion":1,"qty":"1"}`},
			{http.MethodPost, "/api/v1/pos/drafts", `{"id":"` + uuid.New().String() + `","lines":[{"opId":"x","productId":1,"qty":"1"}]}`},
			{http.MethodPost, "/api/v1/pos/drafts", `{"id":"` + uuid.New().String() + `","lines":[{"opId":"` + uuid.New().String() + `","productId":1,"qty":"NaN"}]}`},
			{http.MethodPost, "/api/v1/pos/drafts", `{"id":"` + uuid.New().String() + `","lines":[]}`},
			{http.MethodPatch, "/api/v1/pos/drafts/" + id, `{"customerName":"x"}`},
		}
		for _, c := range cases {
			w := do(t, r, c.method, c.path, tok, []byte(c.body), "application/json")
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %s %s = %d, quería 400: %s", c.method, c.path, c.body, w.Code, w.Body)
			}
		}
	})

	t.Run("cabecera: campos ausentes no cambian, null borra", func(t *testing.T) {
		cur := rawJSON(t, do(t, r, http.MethodGet, "/api/v1/pos/drafts/"+id, tok, nil, "").Body.Bytes())
		hv := cur["headerVersion"].(float64)
		w := do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok,
			jsonBody(t, map[string]any{"expectedHeaderVersion": hv, "customerName": "Mesa 4"}), "application/json")
		if w.Code != http.StatusOK || rawJSON(t, w.Body.Bytes())["customerName"] != "Mesa 4" {
			t.Fatalf("PATCH = %d: %s", w.Code, w.Body)
		}
		expectDraftEvent(t, id)
		w = do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok,
			jsonBody(t, map[string]any{"expectedHeaderVersion": hv + 1, "serviceType": "para_llevar"}), "application/json")
		if m := rawJSON(t, w.Body.Bytes()); w.Code != http.StatusOK || m["customerName"] != "Mesa 4" || m["serviceType"] != "para_llevar" {
			t.Fatalf("un PATCH sin customerName lo borró: %d %s", w.Code, w.Body)
		}
		expectDraftEvent(t, id)
		w = do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok,
			[]byte(`{"expectedHeaderVersion":`+itoa(int(hv)+2)+`,"customerName":null}`), "application/json")
		if m := rawJSON(t, w.Body.Bytes()); w.Code != http.StatusOK || m["customerName"] != nil {
			t.Fatalf("customerName:null no lo borró: %d %s", w.Code, w.Body)
		}
		expectDraftEvent(t, id)
	})

	t.Run("importar", func(t *testing.T) {
		body := jsonBody(t, map[string]any{"accounts": []map[string]any{{
			"id": uuid.New().String(), "folioName": "Bombay",
			"lines": []map[string]any{{"opId": uuid.New().String(), "productId": cafe, "qty": "2"}},
		}}})
		w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/import", tok, body, "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("import = %d: %s", w.Code, w.Body)
		}
		res := mustArray(t, rawJSON(t, w.Body.Bytes()), "results")
		if res[0].(map[string]any)["outcome"] != "created" {
			t.Fatalf("import = %s", w.Body)
		}
		expectDraftEvent(t, res[0].(map[string]any)["id"].(string))
		empty := do(t, r, http.MethodPost, "/api/v1/pos/drafts/import", tok, []byte(`{"accounts":[]}`), "application/json")
		if empty.Code != http.StatusOK {
			t.Fatalf("import vacío = %d", empty.Code)
		}
		mustArray(t, rawJSON(t, empty.Body.Bytes()), "results")
	})
}

// EL DESCUENTO POR EL CAMINO NUEVO TIENE LOS CONTROLES DEL VIEJO: el mismo tope por usuario y el
// mismo evento de seguridad con el anterior y el nuevo (constitución IV, «el camino nuevo que se
// salta el control viejo»).
func TestDraftDiscountKeepsTheOldControls(t *testing.T) {
	st := newTestStore(t)
	r, _, token := draftsAPI(t, st)
	_, tok := token("cajero_desc_cuenta", "cajero")
	crepa := makeProduct(t, st, "Crepa con descuento", pesos("100"), false)
	id := uuid.New().String()
	w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, jsonBody(t, map[string]any{
		"id": id, "lines": []map[string]any{{"opId": uuid.New().String(), "productId": crepa, "qty": "1"}},
	}), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("crear = %d: %s", w.Code, w.Body)
	}
	hv := rawJSON(t, w.Body.Bytes())["headerVersion"].(float64)

	var bitacora bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bitacora, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(anterior)

	w = do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok,
		jsonBody(t, map[string]any{"expectedHeaderVersion": hv, "discount": map[string]any{"amount": "20"}}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("descuento = %d: %s", w.Code, w.Body)
	}
	log := bitacora.String()
	if !strings.Contains(log, "draft_discount_set") || !strings.Contains(log, `"descuento_anterior":"0.00"`) ||
		!strings.Contains(log, `"descuento_nuevo":"20.00"`) {
		t.Fatalf("el evento de seguridad no quedó con el anterior y el nuevo: %s", log)
	}
	bitacora.Reset()
	w = do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok,
		jsonBody(t, map[string]any{"expectedHeaderVersion": hv + 1, "customerName": "Sin descuento"}), "application/json")
	if w.Code != http.StatusOK || strings.Contains(bitacora.String(), "draft_discount_set") {
		t.Fatalf("un cambio sin descuento dejó evento de descuento (%d): %s", w.Code, bitacora.String())
	}

	saw429 := false
	for i := 0; i < 200; i++ {
		w := do(t, r, http.MethodPatch, "/api/v1/pos/drafts/"+id, tok,
			jsonBody(t, map[string]any{"expectedHeaderVersion": 0, "discount": nil}), "application/json")
		if w.Code == http.StatusTooManyRequests {
			saw429 = true
			break
		}
	}
	if !saw429 {
		t.Fatal("200 cambios de cabecera seguidos no toparon con el limitador del descuento")
	}
}

// POST /pos/drafts/{id}/send en JSON crudo: printLineIds siempre arreglo, y avisa el pedido Y la cuenta.
func TestSendDraftHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	r, broker, token := draftsAPI(t, st)
	cajero, tok := token("cajero_envia_http", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	cafe := makeProduct(t, st, "Café enviado por HTTP", pesos("30"), false)
	id := uuid.New().String()
	if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts", tok, jsonBody(t, map[string]any{
		"id": id, "lines": []map[string]any{{"opId": uuid.New().String(), "productId": cafe, "qty": "1"}},
	}), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("crear = %d: %s", w.Code, w.Body)
	}
	events, unsubscribe := broker.Subscribe(defaultCompanyID)
	defer unsubscribe()

	w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/"+id+"/send", tok, []byte(`{}`), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("send = %d: %s", w.Code, w.Body)
	}
	m := rawJSON(t, w.Body.Bytes())
	if len(mustArray(t, m, "printLineIds")) != 1 || m["created"] != true {
		t.Fatalf("cuerpo = %s", w.Body)
	}
	order := m["order"].(map[string]any)
	mustArray(t, order, "lines")
	got := map[string]bool{}
	for range 2 {
		got[nextEvent(t, events).Type] = true
	}
	if !got["order.created"] || !got["draft.updated"] {
		t.Fatalf("eventos = %v: el tablero y las otras tabletas tienen que enterarse", got)
	}

	again := do(t, r, http.MethodPost, "/api/v1/pos/drafts/"+id+"/send", tok, []byte(`{}`), "application/json")
	if again.Code != http.StatusOK || len(mustArray(t, rawJSON(t, again.Body.Bytes()), "printLineIds")) != 0 {
		t.Fatalf("reintento = %d: %s", again.Code, again.Body)
	}
	if w := do(t, r, http.MethodPost, "/api/v1/pos/drafts/no-es-uuid/send", tok, []byte(`{}`), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("id inválido = %d", w.Code)
	}
}
