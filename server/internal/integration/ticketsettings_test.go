//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// El router REAL, no un handler suelto: lo que hay que probar es que la ruta quedó registrada
// dentro del grupo con RequireRole. Un test contra el handler pasaría igual si alguien la mueve
// fuera del grupo, que es exactamente el error que se quiere atrapar.
func newTicketAPI(t *testing.T) (http.Handler, func(username, role string) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Settings: app.NewSettingsService(st)})
	r := httpapi.Router(config.Config{}, jm, h, st)

	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return tok
	}
	return r, token
}

func do(t *testing.T, r http.Handler, method, path, token string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func logoUpload(t *testing.T, data []byte, filename string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

func validPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	img.Set(0, 0, color.Black)
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// Principio V: la autorización se verifica en el backend, pase lo que pase en la interfaz. El
// cajero no ve la opción, pero puede llamar al endpoint con curl.
func TestTicketSettingsAuthorization(t *testing.T) {
	r, token := newTicketAPI(t)
	cajero := token("cajero_ticket", "cajero")
	png := validPNG(t)
	form, ct := logoUpload(t, png, "logo.png")

	escrituras := []struct {
		method, path, contentType string
		body                      []byte
	}{
		{http.MethodPut, "/api/v1/business-settings", "application/json", []byte(`{"businessName":"Hackeado"}`)},
		{http.MethodPut, "/api/v1/business-settings/ticket-logo", ct, form},
		{http.MethodDelete, "/api/v1/business-settings/ticket-logo", "", nil},
	}
	for _, e := range escrituras {
		if got := do(t, r, e.method, e.path, cajero, e.body, e.contentType).Code; got != http.StatusForbidden {
			t.Errorf("%s %s como cajero → %d, want 403", e.method, e.path, got)
		}
	}

	// Leer sí puede: la caja necesita el encabezado y el logo para imprimir.
	if got := do(t, r, http.MethodGet, "/api/v1/business-settings", cajero, nil, "").Code; got != http.StatusOK {
		t.Errorf("GET business-settings como cajero → %d, want 200", got)
	}
	if got := do(t, r, http.MethodGet, "/api/v1/business-settings/ticket-logo", cajero, nil, "").Code; got != http.StatusNotFound {
		t.Errorf("GET ticket-logo sin logo como cajero → %d, want 404 (no 403)", got)
	}

	// Sin sesión no se lee nada.
	if got := do(t, r, http.MethodGet, "/api/v1/business-settings", "", nil, "").Code; got != http.StatusUnauthorized {
		t.Errorf("GET business-settings sin token → %d, want 401", got)
	}
}

func TestTicketSettingsAdminFlow(t *testing.T) {
	r, token := newTicketAPI(t)
	admin := token("admin_ticket", "admin")
	ctx := context.Background()
	_ = ctx

	body := `{"businessName":"El Gato Bobah","address":"Av. Siempre Viva 742","phone":"55 1234 5678",` +
		`"headerNote":"Wi-Fi: gatobobah","footerNote":"¡Vuelve pronto!","autoPrintOnClose":true}`
	w := do(t, r, http.MethodPut, "/api/v1/business-settings", admin, []byte(body), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("PUT business-settings → %d: %s", w.Code, w.Body.String())
	}

	var got app.BusinessSettings
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if got.HeaderNote != "Wi-Fi: gatobobah" || got.FooterNote != "¡Vuelve pronto!" || !got.AutoPrintOnClose {
		t.Errorf("no hizo roundtrip: %+v", got)
	}

	// Los textos del ticket son bloques (400 caracteres); pasarse de ahí es 400 con motivo, no un check
	// violado de Postgres convertido en 500.
	largo := `{"businessName":"X","headerNote":"` + string(bytes.Repeat([]byte("a"), domain.MaxTicketNote+1)) + `"}`
	if w := do(t, r, http.MethodPut, "/api/v1/business-settings", admin, []byte(largo), "application/json"); w.Code != http.StatusBadRequest {
		t.Errorf("headerNote de %d caracteres → %d, want 400", domain.MaxTicketNote+1, w.Code)
	}
	// Y justo en el tope sí entra: el aviso fiscal completo tiene que caber.
	enTope := `{"businessName":"X","headerNote":"` + string(bytes.Repeat([]byte("a"), domain.MaxTicketNote)) + `"}`
	if w := do(t, r, http.MethodPut, "/api/v1/business-settings", admin, []byte(enTope), "application/json"); w.Code != http.StatusOK {
		t.Errorf("headerNote de %d caracteres → %d, want 200", domain.MaxTicketNote, w.Code)
	}

	// Un .txt renombrado se rechaza por CONTENIDO.
	form, ct := logoUpload(t, []byte("esto no es una imagen"), "logo.png")
	if w := do(t, r, http.MethodPut, "/api/v1/business-settings/ticket-logo", admin, form, ct); w.Code != http.StatusBadRequest {
		t.Errorf(".txt renombrado a .png → %d, want 400", w.Code)
	}

	// Y el válido entra, con su mime detectado.
	form, ct = logoUpload(t, validPNG(t), "logo.png")
	w = do(t, r, http.MethodPut, "/api/v1/business-settings/ticket-logo", admin, form, ct)
	if w.Code != http.StatusOK {
		t.Fatalf("subir PNG válido → %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !got.HasLogo {
		t.Fatalf("tras subir, hasLogo = %v (err %v)", got.HasLogo, err)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/business-settings/ticket-logo", admin, nil, ""); w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", w.Header().Get("Content-Type"))
	}

	// Quitarlo devuelve el ticket al logo por default, y quitarlo dos veces no es un error.
	for i := range 2 {
		w = do(t, r, http.MethodDelete, "/api/v1/business-settings/ticket-logo", admin, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("DELETE ticket-logo (intento %d) → %d", i+1, w.Code)
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.HasLogo {
		t.Errorf("tras borrar, hasLogo = %v (err %v)", got.HasLogo, err)
	}
}

// El interruptor de adicionales sin costo nace ENCENDIDO. Un ticket que de pronto deja de listar
// lo que el cliente pidió sin costo es una regresión silenciosa: cocina lo usa para preparar y el
// cliente para reclamar.
func TestAdicionalesSinCostoSeImprimenPorDefault(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var imprime bool
	if err := st.Pool.QueryRow(ctx,
		`select print_free_modifiers from business_settings limit 1`).Scan(&imprime); err != nil {
		t.Fatalf("leer el interruptor: %v", err)
	}
	if !imprime {
		t.Fatal("los adicionales sin costo deben imprimirse por default")
	}
}

// El interruptor de la comanda de cocina nace APAGADO y se puede encender por empresa.
//
// Apagado por default no es la elección tímida: donde la cocina está pegada al mostrador, la
// comanda sería papel que duplica lo que el cocinero ya ve en la pantalla. Lo enciende el negocio
// que tiene la cocina en otro cuarto, y por eso es un ajuste y no una constante — el mismo binario
// sirve a los dos.
func TestLaComandaDeCocinaNaceApagadaYSeEnciendePorEmpresa(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st)
	admin := makeUser(t, st, "admin_comanda", "admin")

	cur, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cur.PrintKitchenTicket {
		t.Fatal("la comanda debe nacer apagada")
	}

	if _, err := settings.SetBusinessInfo(ctx, domain.BusinessInfo{Name: "El Gato Bobah"},
		domain.PrintSettings{PrintKitchenTicket: true, PrintFreeModifiers: true},
		domain.DefaultTimezone, admin); err != nil {
		t.Fatalf("encender la comanda: %v", err)
	}
	got, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get tras encender: %v", err)
	}
	if !got.PrintKitchenTicket {
		t.Fatal("la comanda debía quedar encendida")
	}
	// Y no arrastró a los otros ajustes de impresión: cada uno se enciende por separado.
	if got.AutoPrintOnClose {
		t.Fatal("encender la comanda no debe encender la impresión automática del ticket")
	}
	if !got.PrintFreeModifiers {
		t.Fatal("encender la comanda no debe apagar los adicionales sin costo")
	}
}
