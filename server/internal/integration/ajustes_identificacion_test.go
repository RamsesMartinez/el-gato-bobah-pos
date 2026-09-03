//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Los ajustes de identificación nacen con el comportamiento que el dueño eligió.
//
// pin_only_unlock apagado importa especialmente: encendido, un dedazo que caiga en el PIN de otro
// atribuye la venta a quien no fue, en silencio. No es algo que un negocio deba estrenar sin
// haberlo elegido.
//
// El bloqueo de PANTALLA nace apagado desde 0059, y esa parte sí cambió a propósito: en un local
// donde la tableta vive a la vista del mostrador, bloquearse cada tres minutos son dos toques y un
// PIN a media venta a cambio de nada. La barrera que queda es la caducidad de la SESIÓN, que la
// aplica el servidor y que no se movió — por eso las tres se comprueban juntas: apagar el bloqueo
// de pantalla no puede haber aflojado la sesión de paso.
func TestLosAjustesDeIdentificacionNacenSeguros(t *testing.T) {
	st := newTestStore(t)
	settings := app.NewSettingsService(st, "pepper-de-prueba")

	ajustes, err := settings.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ajustes.PinOnlyUnlock {
		t.Error("pin_only_unlock nació encendido: un negocio estrenaría el modo riesgoso sin elegirlo")
	}
	if ajustes.LockAfterSeconds != 0 {
		t.Errorf("lock_after_seconds = %d, quiere 0: el bloqueo de pantalla nace apagado",
			ajustes.LockAfterSeconds)
	}
	if ajustes.SessionHours != 8 {
		t.Errorf("session_hours = %d, quiere 8", ajustes.SessionHours)
	}
}

// Los tres viven en la MISMA fila que los ajustes del ticket y se escriben con el mismo UPDATE.
// Un interruptor que apaga otro es el fallo clásico de esa tabla y ya mordió una vez.
func TestGuardarLaIdentificacionNoPisaLosAjustesDelTicket(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_ident", "admin")

	antes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Se dejan ENCENDIDOS los del ticket para que el test note si guardar la identificación
	// los apaga.
	print := domain.PrintSettings{
		AutoPrintOnClose:   true,
		PrintFreeModifiers: antes.PrintFreeModifiers,
		PrintKitchenTicket: true,
		KitchenCanCharge:   true,
	}
	info := domain.BusinessInfo{
		Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone,
		HeaderNote: antes.HeaderNote, FooterNote: antes.FooterNote,
	}
	ident := domain.IdentitySettings{PinOnlyUnlock: false, LockAfterSeconds: 60, SessionHours: 12}
	if _, err := settings.SetBusinessInfo(ctx, info, print, ident, antes.Timezone, admin); err != nil {
		t.Fatalf("SetBusinessInfo: %v", err)
	}

	tras, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get tras update: %v", err)
	}
	if tras.LockAfterSeconds != 60 || tras.SessionHours != 12 {
		t.Errorf("los tiempos no se guardaron: bloqueo=%d sesión=%d", tras.LockAfterSeconds, tras.SessionHours)
	}
	if !tras.PrintKitchenTicket || !tras.KitchenCanCharge || !tras.AutoPrintOnClose {
		t.Error("guardar la identificación apagó ajustes del ticket que vivían en la misma fila")
	}
}

// Un tiempo de bloqueo negativo o una sesión de cero horas dejarían la tableta bloqueada siempre o
// nunca autenticada. Se rechazan en la frontera, no se ajustan a un default en silencio.
func TestLosTiemposAbsurdosSeRechazan(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_tiempos", "admin")

	antes, _ := settings.Get(ctx)
	info := domain.BusinessInfo{
		Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone,
		HeaderNote: antes.HeaderNote, FooterNote: antes.FooterNote,
	}
	print := domain.PrintSettings{PrintFreeModifiers: antes.PrintFreeModifiers}

	casos := []struct {
		nombre string
		ident  domain.IdentitySettings
	}{
		{"bloqueo negativo", domain.IdentitySettings{LockAfterSeconds: -1, SessionHours: 8}},
		{"sesión en cero", domain.IdentitySettings{LockAfterSeconds: 180, SessionHours: 0}},
		{"sesión absurda", domain.IdentitySettings{LockAfterSeconds: 180, SessionHours: 100000}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := settings.SetBusinessInfo(ctx, info, print, c.ident, antes.Timezone, admin); err == nil {
				t.Fatal("se aceptó un tiempo absurdo en vez de rechazarlo")
			}
		})
	}
}
