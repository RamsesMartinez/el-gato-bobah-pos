package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// El esperado del CAJÓN: una sola cifra contra la que se compara un conteo.
//
// Es la aritmética de la spec 015. Lo que arregla, medido: el corte pedía una cifra declarada por
// cada método que toca el cajón —el efectivo del mostrador y los tres de plataforma en efectivo— y
// un turno real quedó con «Efectivo» en diferencia 0.00 y «Didi efectivo» en −64.80, sin forma de
// saber si eran $64.80 que no llegaron o $64.80 que sí estaban y se contaron dentro de los otros.

func metodo(esperado string, toca bool) MetodoDelCorte {
	return MetodoDelCorte{ID: 1, Esperado: mnt(esperado), TocaElCajon: toca}
}

func TestEsperadoDelCajon(t *testing.T) {
	casos := []struct {
		nombre  string
		metodos []MetodoDelCorte
		quiere  string
		err     error
		porQue  string
	}{
		{
			nombre: "suma solo lo que está en el cajón",
			metodos: []MetodoDelCorte{
				metodo("700", true),  // mostrador: ventas + fondo + neto de movimientos
				metodo("100", true),  // una app que cobra en efectivo y el dinero regresa al cajón
				metodo("300", false), // tarjeta: su dinero no está en el cajón
			},
			quiere: "800",
			porQue: "meter la tarjeta al cajón haría contar billetes que están en una terminal",
		},
		{
			nombre:  "una caja que no maneja efectivo espera cero, y no es error",
			metodos: []MetodoDelCorte{metodo("300", false)},
			quiere:  "0",
			porQue:  "una caja secundaria cierra sin nada que contar; tratarlo como error la deja sin poder cerrar",
		},
		{
			nombre:  "sin métodos, cero",
			metodos: nil,
			quiere:  "0",
		},
		{
			nombre: "un esperado negativo se devuelve tal cual y NO es error",
			metodos: []MetodoDelCorte{
				metodo("-150", true),
			},
			quiere: "-150",
			porQue: "una salida de efectivo mayor que lo que había lo produce, y rechazarlo obligaría a capturar una mentira — el mismo criterio que el neto de una liquidación",
		},
		{
			nombre: "el redondeo no arrastra centavos",
			metodos: []MetodoDelCorte{
				metodo("0.005", true),
				metodo("0.005", true),
			},
			quiere: "0.01",
			porQue: "dos medios centavos son un centavo, no cero: el arqueo se compara contra esto",
		},
		{
			nombre: "un esperado absurdo se rechaza como validación, no revienta",
			metodos: []MetodoDelCorte{
				metodo("9999999", true),
				metodo("9999999", true),
			},
			err:    ErrValidation,
			porQue: "desbordar el numeric(10,2) tiene que salir como 400 accionable y no como 500",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := EsperadoDelCajon(c.metodos)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("quiere %v y dio %v — %s", c.err, err, c.porQue)
				}
				return
			}
			if err != nil {
				t.Fatalf("no debía fallar: %v — %s", err, c.porQue)
			}
			if !got.Equal(d(c.quiere)) {
				t.Fatalf("esperó %s y quiere %s — %s", got, c.quiere, c.porQue)
			}
		})
	}
}

// QUIÉNES FORMAN EL CAJÓN, para que la pantalla no lo vuelva a decidir por su cuenta.
//
// Dos derivaciones de la misma regla son dos pantallas que pueden diferir, y aquí eso significaría
// pedirle al operador una cifra por un método cuyo dinero ya contó.
func TestMetodosDelCajon(t *testing.T) {
	metodos := []MetodoDelCorte{
		{ID: 1, Esperado: mnt("700"), TocaElCajon: true},
		{ID: 2, Esperado: mnt("300"), TocaElCajon: false},
		{ID: 8, Esperado: mnt("100"), TocaElCajon: true},
	}
	got := MetodosDelCajon(metodos)
	if len(got) != 2 || got[0] != 1 || got[1] != 8 {
		t.Fatalf("los métodos del cajón salieron %v y son el 1 y el 8, en ese orden", got)
	}
	if MetodosDelCajon(nil) == nil {
		t.Fatal("sin métodos devuelve nil y tiene que ser una lista vacía: un nil se serializa como null y la pantalla no puede recorrerlo")
	}
}

// EL EFECTIVO DEL MOSTRADOR NO PUEDE SALIR DEL CAJÓN.
//
// La spec 015 vuelve editable el interruptor por primera vez, y este es el único valor que no tiene
// sentido: los billetes que el cliente pone en el mostrador están en el cajón por definición.
// Apagarlo dejaría el fondo de apertura y los movimientos de caja fuera del esperado, y el arqueo
// se compararía contra una cifra que no incluye el dinero con el que abrió el turno.
func TestFlagsDelMetodoValidos(t *testing.T) {
	casos := []struct {
		nombre                                           string
		efectivo, delMostrador, tocaElCajon, autoDeclara bool
		quiere                                           error
	}{
		{"el efectivo del mostrador dentro del cajón es lo normal", true, true, true, false, nil},
		{"sacar del cajón el efectivo del mostrador", true, true, false, false, ErrEfectivoFueraDelCajon},
		// El reparto de una app en efectivo lo hace a veces gente del local y a veces el repartidor
		// de la plataforma, que se lo lleva: los dos valores son configuración legítima.
		{"efectivo de app que entra al cajón", true, false, true, false, nil},
		{"efectivo de app que se lleva el repartidor", true, false, false, false, nil},
		{"la tarjeta no entra al cajón", false, false, true, false, ErrCajonSinEfectivo},
		{"la tarjeta se auto-declara sin problema", false, false, false, true, nil},
		{"auto-declarar el efectivo del cajón", true, true, true, true, ErrEfectivoAutoDeclarado},
		// EL BYPASS DE UN REQUEST: sacarlo del cajón no lo deja de hacer efectivo, y el efectivo
		// que alguien contó con la mano no se declara solo.
		{"sacarlo del cajón y auto-declararlo a la vez", true, false, false, true, ErrEfectivoAutoDeclarado},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := FlagsDelMetodoValidos(c.efectivo, c.delMostrador, c.tocaElCajon, c.autoDeclara)
			if !errors.Is(err, c.quiere) {
				t.Fatalf("FlagsDelMetodoValidos(%v, %v, %v, %v) = %v, quiere %v",
					c.efectivo, c.delMostrador, c.tocaElCajon, c.autoDeclara, err, c.quiere)
			}
		})
	}
}

// MOVER «VA AL CAJÓN» A MEDIA JORNADA (spec 015, FR-017).
//
// Los bordes son tres y los tres importan: sin cambio no se estorba nunca, con cambio y sin dinero
// se permite (configurar antes de vender es el caso normal), y con cambio y con dinero se rechaza.
// El tercero es el que protege: el dinero ya cobrado sigue en el cajón y el esperado no puede
// dejar de pedirlo.
func TestCambioDeCajonPermitido(t *testing.T) {
	casos := []struct {
		nombre  string
		cambia  bool
		cobrado string
		quiere  error
	}{
		{"no se toca el interruptor, aunque el turno tenga dinero", false, "835", nil},
		{"se cambia antes de que el método venda nada", true, "0", nil},
		{"se cambia con dinero de ese método ya en el turno", true, "135", ErrCajonConDineroDelTurno},
		{"un centavo ya es dinero", true, "0.01", ErrCajonConDineroDelTurno},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := CambioDeCajonPermitido(c.cambia, decimal.RequireFromString(c.cobrado))
			if !errors.Is(err, c.quiere) {
				t.Fatalf("CambioDeCajonPermitido(%v, %s) = %v, quiere %v", c.cambia, c.cobrado, err, c.quiere)
			}
		})
	}
}
