//go:build integration

package integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// Regresión: tras 0023 (business_settings pasó a PK company_id, sin columna id), las queries de
// ajustes seguían con `where id = true` → GET/PUT reventaban con 42703 (500) en prod. Este test
// corre sobre el esquema ya migrado (0023 siembra una fila para la empresa 1) y falla si la query
// vuelve a referenciar una columna inexistente.
func TestBusinessSettingsGetAndUpdate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st)
	admin := makeUser(t, st, "admin_settings", "admin")

	// GET no debe reventar (antes: column "id" does not exist).
	if _, err := settings.Get(ctx); err != nil {
		t.Fatalf("Get business settings: %v", err)
	}

	// PUT actualiza la fila del tenant y hace roundtrip.
	if _, err := settings.SetDeliveryFee(ctx, decimal.RequireFromString("35"), admin); err != nil {
		t.Fatalf("SetDeliveryFee: %v", err)
	}
	got, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get tras update: %v", err)
	}
	if !got.DeliveryFee.Equal(decimal.RequireFromString("35")) {
		t.Fatalf("deliveryFee = %s, want 35", got.DeliveryFee)
	}
}

// El encabezado del ticket sale de business_settings: la 0033 agregó la identidad del negocio y
// sembró business_name desde companies.name para que el papel siga diciendo lo mismo que decía
// cuando el nombre estaba hardcodeado en el front. Este test fija ese contrato y, sobre todo, que
// Get NO devuelva los bytes del logo: esta query corre en cada cobro y no debe mover la imagen.
func TestBusinessSettingsIncludesTicketHeader(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st)

	got, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BusinessName != "El Gato Bobah" {
		t.Errorf("BusinessName = %q, want el nombre sembrado desde companies.name", got.BusinessName)
	}
	if got.HasLogo {
		t.Error("HasLogo = true en una empresa recién migrada; sin logo subido el ticket usa el default del front")
	}
	if got.LogoUpdatedAt != nil {
		t.Errorf("LogoUpdatedAt = %v, want nil cuando no hay logo", got.LogoUpdatedAt)
	}
	if got.Address != "" || got.Phone != "" || got.HeaderNote != "" {
		t.Errorf("campos opcionales sembrados en una empresa nueva: %+v", got)
	}
	// El pie SÍ viene sembrado (0035): un negocio recién dado de alta ya imprime el aviso de que el
	// ticket no tiene valor fiscal, sin que nadie lo configure. Si esto deja de cumplirse, el
	// ticket sale sin la advertencia y nadie se entera hasta que un cliente pide factura.
	if !strings.Contains(got.FooterNote, "TICKET SIN VALOR FISCAL") {
		t.Errorf("el pie por default no se sembró: %q", got.FooterNote)
	}
}

// El logo se sirve por su propio endpoint y no dentro de los ajustes: son 256 KB que no tienen por
// qué viajar en cada lectura del costo de envío. Este test fija las tres cosas que el navegador
// necesita para no reinterpretar el binario ni volver a bajarlo: el mime guardado (no el que dijo
// quien subió), nosniff, y un ETag que permita el 304.
func TestTicketLogoEndpoint(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	h := httpapi.NewHandlers(httpapi.Deps{Settings: app.NewSettingsService(st)})

	// Sin logo subido: 404, no 500. El front cae al logo por default y eso NO es un error.
	w := httptest.NewRecorder()
	h.TicketLogo(w, httptest.NewRequest(http.MethodGet, "/business-settings/ticket-logo", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("sin logo: status = %d, want 404", w.Code)
	}

	// Se siembra el binario directo en la base: la ruta de subida es de otra historia.
	png := []byte("\x89PNG\r\n\x1a\n-bytes-de-prueba")
	if _, err := st.Pool.Exec(ctx,
		"update business_settings set logo_bytes = $1, logo_mime = 'image/png', logo_updated_at = now()", png); err != nil {
		t.Fatalf("sembrar logo: %v", err)
	}

	w = httptest.NewRecorder()
	h.TicketLogo(w, httptest.NewRequest(http.MethodGet, "/business-settings/ticket-logo", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("con logo: status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want el mime guardado", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff: sin esto el navegador puede reinterpretar el binario", got)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("sin ETag: el front vuelve a bajar la imagen en cada ticket")
	}
	if !bytes.Equal(w.Body.Bytes(), png) {
		t.Errorf("cuerpo = %q, want los bytes guardados", w.Body.Bytes())
	}

	// Revalidación: mismo ETag → 304 sin cuerpo.
	req := httptest.NewRequest(http.MethodGet, "/business-settings/ticket-logo", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	h.TicketLogo(w, req)
	if w.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("304 con cuerpo de %d bytes; debe ir vacío", w.Body.Len())
	}
}
