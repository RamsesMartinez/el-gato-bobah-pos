//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL PIN CORTO DE QUIEN ESTABA DADO DE BAJA SOBREVIVÍA AL ENCENDIDO DEL MODO.
//
// Encender solo-PIN borra los PINs para obligar a recapturarlos con seis dígitos y sin repetidos:
// es el único momento en que están en claro. Pero el borrado decía `where is_active`, así que a
// quien estaba dado de baja se le quedaba su PIN de cuatro dígitos —y su huella de búsqueda—
// intactos. Basta reactivarlo para que en un negocio de seis dígitos exista un PIN de cuatro que
// abre la caja: 10,000 combinaciones en vez de un millón, y nadie lo ve porque el ajuste dice que
// el modo está bien encendido.
func TestEncenderSoloPinBorraTambienElPinDeQuienEstaDadoDeBaja(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	dueno := makeUser(t, st, "dueno_baja", "admin")
	quienSeFue := makeUser(t, st, "exempleado_baja", "cajero")
	if err := users.SetPIN(ctx, quienSeFue, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set is_active = false where id = $1`, quienSeFue); err != nil {
		t.Fatalf("dar de baja: %v", err)
	}

	if err := encenderSoloPin(t, st, dueno); err != nil {
		t.Fatalf("encender solo-PIN: %v", err)
	}

	var pinHash, pinLookup *string
	if err := st.Pool.QueryRow(ctx,
		`select pin_hash, pin_lookup from users where id = $1`, quienSeFue).Scan(&pinHash, &pinLookup); err != nil {
		t.Fatalf("leer el PIN: %v", err)
	}
	if pinHash != nil || pinLookup != nil {
		t.Errorf("a quien está dado de baja le quedó su PIN de cuatro dígitos: reactivarlo abre la caja con 10,000 combinaciones")
	}
}

// CON SOLO-PIN ENCENDIDO, LA PUERTA DE ELEGIR PERSONA SIGUE ABIERTA.
//
// Son dos caminos al mismo desbloqueo con lockouts SEPARADOS —`pin:<objetivo>` y
// `pinsolo:<quien pide>`—, así que quien agota uno pasa al otro y duplica su presupuesto de
// intentos. Y el modo existe justamente para que la plantilla no se muestre: dejar el camino que
// nombra a la persona lo contradice.
func TestConSoloPinNoSePuedeDesbloquearEligiendoPersona(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthServiceConPepper(st, jm, clock, "pepper-de-prueba")

	dueno := makeUser(t, st, "dueno_puerta", "admin")
	ana := makeUser(t, st, "ana_puerta", "cajero")
	luis := makeUser(t, st, "luis_puerta", "cajero")
	// Ana entra de verdad: sin sesión de estación el relevo se niega antes de llegar a la puerta
	// que este test viene a cerrar, y el test pasaría por la razón equivocada.
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := encenderSoloPin(t, st, dueno); err != nil {
		t.Fatalf("encender solo-PIN: %v", err)
	}
	if err := users.SetPIN(ctx, luis, "482715"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}
	estacion, err := svc.Login(ctx, "ana_puerta", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	_, err = svc.PinSwitchEnEstacion(ctx, luis, "482715", ana, estacion.RefreshToken)
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("el camino de elegir persona sigue abierto con solo-PIN (err=%v): son dos lockouts para el mismo desbloqueo", err)
	}
}

// SI NO SE PUEDE SABER EN QUÉ MODO ESTÁ EL NEGOCIO, NO SE ACEPTA UN PIN CORTO.
//
// `politicaDePin` devolvía "no es solo-PIN" ante CUALQUIER error de lectura, así que un hipo de la
// consulta —un timeout de sentencia, una conexión que se cae— hacía que el negocio en modo de seis
// dígitos aceptara uno de cuatro y le calculara su huella de búsqueda, que en ese modo es
// directamente desbloqueable. El modo de fallo de un control tiene que ser proteger.
func TestSiNoSePuedeLeerElModoNoSeAceptaUnPinCorto(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	dueno := makeUser(t, st, "dueno_hipo", "admin")
	ana := makeUser(t, st, "ana_hipo", "cajero")
	if err := encenderSoloPin(t, st, dueno); err != nil {
		t.Fatalf("encender solo-PIN: %v", err)
	}

	// El hipo: leer los ajustes falla. No es "no hay fila" —eso es una empresa nueva, tiene su
	// default seguro y no debe rechazar nada—, es que la lectura no se pudo hacer.
	if _, err := st.Pool.Exec(ctx, `drop table business_settings cascade`); err != nil {
		t.Fatalf("provocar el fallo de lectura: %v", err)
	}

	if err := users.SetPIN(ctx, ana, "4827"); err == nil {
		t.Error("se aceptó un PIN de cuatro dígitos sin poder leer el modo del negocio: en solo-PIN ese PIN abre la caja")
	}
}

// EL CAMINO DE SOLO-PIN NO PUEDE FUNCIONAR CON EL MODO APAGADO.
//
// `PinSwitchSoloPin` es exportada y solo comprobaba que hubiera secreto; el gate del modo vivía en
// el handler. Con el modo apagado, ese camino identifica a la persona POR SU PIN, que es justo lo
// que el modo por default no hace: ahí el PIN solo prueba, y por eso basta con cuatro dígitos.
// Deducir de quién es un PIN de cuatro dígitos con el modo apagado abre lo que el mínimo de seis
// existe para cerrar. Hoy lo tapa el handler; un segundo llamador lo destapa.
func TestConElModoApagadoElCaminoDeSoloPinSeNiega(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthServiceConPepper(st, jm, clock, "pepper-de-prueba")

	ana := makeUser(t, st, "ana_apagado", "cajero")
	luis := makeUser(t, st, "luis_apagado", "cajero")
	if err := users.SetPIN(ctx, luis, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	// El modo está apagado: es el default y no se tocó.
	if _, err := svc.PinSwitchSoloPin(ctx, "4827", ana, "lo-que-sea"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("el camino de solo-PIN funcionó con el modo apagado (err=%v): deduce de quién es un PIN de cuatro dígitos", err)
	}
}
