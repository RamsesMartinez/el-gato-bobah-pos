//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func encenderSoloPin(t *testing.T, st *store.Store, admin int64) error {
	t.Helper()
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	antes, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	ident := domain.DefaultIdentity()
	ident.PinOnlyUnlock = true
	_, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, ident, antes.Timezone, admin)
	return err
}

// La compuerta: encender el modo BORRA los PINs y obliga a recapturarlos.
//
// Es lo único que se puede hacer, y se descubrió al implementar: bcrypt saliniza, así que de lo
// guardado no se puede leer el largo de un PIN ni saber si dos personas comparten uno. Recapturar
// es el único momento en que el PIN está en claro y se puede validar las dos cosas.
func TestEncenderSoloPinObligaARecapturarLosPins(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_recap", "admin")

	ana := makeUser(t, st, "ana_recap", "cajero")
	if err := users.SetPIN(ctx, ana, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	if err := encenderSoloPin(t, st, admin); err != nil {
		t.Fatalf("encender: %v", err)
	}

	// El PIN de antes dejó de servir: era de 4 dígitos y nadie puede saber si lo compartía.
	var tienePin bool
	if err := st.Pool.QueryRow(ctx,
		`select pin_hash is not null from users where id = $1`, ana).Scan(&tienePin); err != nil {
		t.Fatalf("leer el PIN: %v", err)
	}
	if tienePin {
		t.Error("el PIN viejo sobrevivió: nadie puede garantizar que tenga 6 dígitos ni que sea único")
	}

	// Y el nuevo tiene que cumplir la regla del modo.
	if err := users.SetPIN(ctx, ana, "4827"); err == nil {
		t.Error("con el modo encendido se aceptó un PIN de 4 dígitos")
	}
	if err := users.SetPIN(ctx, ana, "482715"); err != nil {
		t.Errorf("un PIN de 6 dígitos debe aceptarse: %v", err)
	}
}

// Sin el secreto del servidor no hay forma de comparar dos PINs por igualdad, así que el modo NO se
// enciende. Fail-closed: nunca se activa un modo cuya única protección no se puede aplicar.
func TestSinSecretoElModoNoSeEnciende(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "") // sin pepper
	admin := makeUser(t, st, "admin_nopepper", "admin")

	antes, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	ident := domain.DefaultIdentity()
	ident.PinOnlyUnlock = true

	_, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, ident, antes.Timezone, admin)
	if !errors.Is(err, domain.ErrSinPepper) {
		t.Fatalf("sin secreto = %v, quiere ErrSinPepper", err)
	}
}

// Con los PINs en regla sí se enciende. Y apagarlo nunca tiene compuerta: volver al modo seguro
// siempre se puede.
func TestConPinsEnReglaSeEnciendeYSePuedeApagar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_ok", "admin")

	ana := makeUser(t, st, "ana_ok", "cajero")
	luis := makeUser(t, st, "luis_ok", "cajero")
	if err := encenderSoloPin(t, st, admin); err != nil {
		t.Fatalf("con el secreto configurado debe encenderse: %v", err)
	}
	// Los PINs se capturan DESPUÉS de encender: es cuando el sistema puede exigir 6 dígitos y
	// comprobar que no se repitan.
	if err := users.SetPIN(ctx, ana, "482715"); err != nil {
		t.Fatalf("SetPIN ana: %v", err)
	}
	if err := users.SetPIN(ctx, luis, "913572"); err != nil {
		t.Fatalf("SetPIN luis: %v", err)
	}
	tras, _ := settings.Get(ctx)
	if !tras.PinOnlyUnlock {
		t.Fatal("el modo no quedó encendido")
	}

	info := domain.BusinessInfo{Name: tras.BusinessName, Address: tras.Address, Phone: tras.Phone}
	if _, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, domain.DefaultIdentity(), tras.Timezone, admin); err != nil {
		t.Fatalf("apagarlo siempre debe poderse: %v", err)
	}
}

// Con el modo encendido, un PIN nuevo que coincida con el de otra persona se rechaza — y el
// mensaje NO dice de quién: si lo dijera, el formulario sería un oráculo para averiguar el PIN de
// un compañero probando números.
func TestConSoloPinNoSePuedeRepetirElPinDeOtro(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_orac", "admin")

	ana := makeUser(t, st, "ana_orac", "cajero")
	luis := makeUser(t, st, "luis_orac", "cajero")
	if err := encenderSoloPin(t, st, admin); err != nil {
		t.Fatalf("encender: %v", err)
	}
	if err := users.SetPIN(ctx, ana, "482715"); err != nil {
		t.Fatalf("SetPIN ana: %v", err)
	}
	if err := users.SetPIN(ctx, luis, "913572"); err != nil {
		t.Fatalf("SetPIN luis: %v", err)
	}

	err := users.SetPIN(ctx, luis, "482715")
	if !errors.Is(err, domain.ErrPinRepetido) {
		t.Fatalf("repetir el PIN de otro = %v, quiere ErrPinRepetido", err)
	}
	if strings.Contains(err.Error(), "ana_orac") {
		t.Errorf("el error dice DE QUIÉN es el PIN: el formulario sería un oráculo. %v", err)
	}
}

// FR-010 en la frontera HTTP. Con el modo de solo-PIN APAGADO, un `userId` ausente se rechaza.
//
// Es el control que la constitución nombra explícitamente: un parámetro de frontera inválido no cae
// a un default en silencio. Aquí el default silencioso sería aceptar cualquier PIN sin saber de
// quién es, y con él la atribución del arqueo dejaría de significar nada.
func TestSinIndicarQuienYSinSoloPinSeRechaza(t *testing.T) {
	st := newTestStore(t)
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	h := httpapi.NewHandlers(httpapi.Deps{
		Auth: app.NewAuthServiceConPepper(st, jm, clock, "pepper-de-prueba"),
	})

	cuerpo, _ := json.Marshal(map[string]any{"pin": "482715"})
	req := httptest.NewRequest(http.MethodPost, "/auth/pin-switch", bytes.NewReader(cuerpo))
	req = req.WithContext(httpapi.ConUsuarioDePrueba(req.Context(), 7, defaultCompanyID))
	w := httptest.NewRecorder()
	h.PinSwitch(w, req)

	// 4xx, no 5xx y no 200: es una petición mal formada, no una falla del servidor.
	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("status = %d, quiere un 4xx: sin saber de quién es el PIN no se puede desbloquear", w.Code)
	}
}
