package domain

import (
	"errors"
	"testing"
)

func TestIdentitySettingsValidate(t *testing.T) {
	ok := func(lock, horas int) IdentitySettings {
		return IdentitySettings{LockAfterSeconds: lock, SessionHours: horas}
	}
	casos := []struct {
		nombre string
		in     IdentitySettings
		quiere error
	}{
		{"los defaults", ok(180, 8), nil},
		// Cero es una elección válida: una caja en una oficina cerrada no necesita bloquearse.
		{"sin bloqueo", ok(0, 8), nil},
		{"una hora de bloqueo", ok(3600, 8), nil},
		{"treinta días de sesión", ok(180, 720), nil},

		{"bloqueo negativo", ok(-1, 8), ErrValidation},
		// Un cero de más deja la tableta bloqueada para siempre y nadie buscaría la causa aquí.
		{"bloqueo absurdo", ok(36000, 8), ErrValidation},
		// Cero horas dejaría la tableta pidiendo credenciales a cada instante.
		{"sesión en cero", ok(180, 0), ErrValidation},
		{"sesión negativa", ok(180, -8), ErrValidation},
		{"sesión absurda", ok(180, 100000), ErrValidation},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if err := c.in.Validate(); !errors.Is(err, c.quiere) {
				t.Fatalf("Validate(%+v) = %v, quiere %v", c.in, err, c.quiere)
			}
		})
	}
}

// EL BLOQUEO NACE APAGADO.
//
// Nacía en 180 segundos y el dueño lo pidió al revés: un local donde la tableta está siempre a la
// vista del mostrador no gana nada bloqueándose cada tres minutos, y sí pierde — cada bloqueo son
// dos toques y un PIN en medio de una venta.
//
// Encenderlo es un interruptor en Ajustes, y lo que lo enciende es poner un tiempo mayor que cero:
// no hay una segunda columna que pueda contradecir a la primera.
func TestElBloqueoDePantallaNaceApagado(t *testing.T) {
	d := DefaultIdentity()
	if d.LockAfterSeconds != 0 {
		t.Errorf("LockAfterSeconds nace en %d, quiere 0: el negocio nuevo no debe bloquearse solo",
			d.LockAfterSeconds)
	}
	// Y sigue siendo un ajuste válido: cero no es un error de captura que haya que corregir.
	if err := d.Validate(); err != nil {
		t.Errorf("los defaults no validan: %v", err)
	}
	// Lo que SÍ cambia con esto es la sesión, que no se toca: sin ella no habría ninguna barrera.
	if d.SessionHours != 8 {
		t.Errorf("SessionHours = %d, quiere 8: apagar el bloqueo de pantalla no puede alargar la sesión",
			d.SessionHours)
	}
}
