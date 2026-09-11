package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// El arqueo del CAJÓN: una cifra esperada, un conteo, una diferencia (spec 015).
//
// Por qué esto es aritmética de dominio y no una consulta: el cajón es un solo montón de billetes y
// el corte pedía una cifra declarada por cada método que lo toca. Un turno real quedó con «Efectivo»
// en diferencia $0.00 y «Didi efectivo» en −$64.80 — dos diferencias del mismo dinero, sin forma de
// saber si el faltante era real o si ese dinero se contó dentro del otro renglón. Repartir un montón
// de billetes por canal de venta es algo que ninguna persona puede hacer, así que el sistema dejó de
// pedirlo.

// ErrEfectivoFueraDelCajon: se intentó configurar que el efectivo del mostrador no llega al cajón.
//
// Es el único valor del interruptor que no tiene sentido, y la spec 015 lo vuelve editable por
// primera vez. Apagarlo dejaría el fondo de apertura y el neto de movimientos fuera del esperado
// —viven en ese método— y el arqueo se compararía contra una cifra que no incluye el dinero con el
// que abrió el turno.
var ErrEfectivoFueraDelCajon = fmt.Errorf("%w: el efectivo del mostrador siempre está en el cajón", ErrValidation)

// MetodoDelCorte: lo que un método aportó al turno, ya leído de la base.
//
// `Esperado` viene compuesto como lo compone el corte: ventas + propinas, y en el método dueño del
// fondo también el fondo de apertura y el neto de movimientos de caja. Por eso el esperado del cajón
// es una SUMA de estos y no vuelve a agregar el fondo: sumarlo aparte lo contaría dos veces, que es
// la misma familia de defecto que este spec viene a cerrar.
type MetodoDelCorte struct {
	ID          int
	Esperado    decimal.Decimal
	TocaElCajon bool
}

// EsperadoDelCajon suma lo que debería haber en el cajón físico.
//
// Un total NEGATIVO se devuelve tal cual y no es error: una salida de efectivo mayor que lo que
// había lo produce, y rechazarlo obligaría a capturar una mentira. Es el mismo criterio que el neto
// de una liquidación de plataforma, y por eso se valida con `ValidSignedMoney` y no con `ValidMoney`.
func EsperadoDelCajon(metodos []MetodoDelCorte) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, m := range metodos {
		if !m.TocaElCajon {
			continue
		}
		total = total.Add(m.Esperado)
	}
	total = Round2(total)
	if !ValidSignedMoney(total) {
		return decimal.Zero, fmt.Errorf("%w: el esperado del cajón sale de los topes del sistema", ErrValidation)
	}
	return total, nil
}

// MetodosDelCajon dice qué métodos forman el cajón, en el orden en que llegan.
//
// Viaja a la pantalla para que no vuelva a decidirlo por su cuenta: dos derivaciones de la misma
// regla son dos pantallas que pueden diferir, y aquí diferir significa pedirle al operador una cifra
// por un método cuyo dinero ya contó.
//
// Devuelve una lista vacía y no nil: un nil se serializa como `null` y la pantalla no lo puede
// recorrer sin una guarda que alguien va a olvidar.
func MetodosDelCajon(metodos []MetodoDelCorte) []int {
	out := make([]int, 0, len(metodos))
	for _, m := range metodos {
		if m.TocaElCajon {
			out = append(out, m.ID)
		}
	}
	return out
}

// ErrCajonSinEfectivo: un método que no se cobra en billetes no puede entrar al cajón.
var ErrCajonSinEfectivo = fmt.Errorf("%w: ese método no se cobra en efectivo, así que su dinero no está en el cajón", ErrValidation)

// ErrEfectivoAutoDeclarado: el efectivo no se auto-declara, caiga donde caiga.
var ErrEfectivoAutoDeclarado = fmt.Errorf("%w: el efectivo se declara contándolo; auto-declararlo haría que un faltante nunca aparezca", ErrValidation)

// FlagsDelMetodoValidos rechaza las combinaciones imposibles de los tres interruptores.
//
// Las tres reglas miran cosas distintas y por eso son tres parámetros y no dos:
//
//   - `esEfectivo` es una propiedad del método: se cobra en billetes. La resuelve quien llama
//     leyendo `is_cash`, que existe precisamente porque antes esto se deducía de «va al cajón» y
//     sobrecargar un interruptor con dos significados dejaba un bypass de un request: apagar el
//     cajón y encender «automático» en el mismo PATCH pasaba la validación, y el método quedaba
//     indistinguible de uno en línea con sus billetes entrando sin que nadie los contara.
//   - `esEfectivoDelMostrador` es el dueño del fondo de apertura y de los movimientos de caja; su
//     dinero está en el cajón por definición. Esa distinción entre "el dueño del fondo" y "su
//     dinero se cuenta" ya costó una vez: sumar el fondo a los cuatro métodos que tocan el cajón
//     le inventó $4,500 de faltante a un turno.
//   - `tocaElCajon` y `autoDeclara` son lo configurable.
func FlagsDelMetodoValidos(esEfectivo, esEfectivoDelMostrador, tocaElCajon, autoDeclara bool) error {
	if esEfectivoDelMostrador && !tocaElCajon {
		return ErrEfectivoFueraDelCajon
	}
	if tocaElCajon && !esEfectivo {
		return ErrCajonSinEfectivo
	}
	if autoDeclara && esEfectivo {
		return ErrEfectivoAutoDeclarado
	}
	return nil
}

// ErrCajonSinContar: el cierre llegó sin conteo y el cajón esperaba dinero.
//
// No se puede dejar pasar por lo que el cierre hace con los métodos del cajón: cada uno declara SU
// esperado, así que sin conteo los cuatro renglones quedan en diferencia cero y no hay fila de
// conteo de la que sacar la real. El corte reportaría $0 de diferencia con el cajón sin contar —en
// la pantalla y en el histórico—, que es peor que el faltante visible que dejaba el mecanismo
// anterior. Es FR-016: un campo vacío no se trata como cero.
var ErrCajonSinContar = fmt.Errorf("%w: falta contar el cajón antes de cerrar: sin conteo el corte diría que cuadra sin que nadie haya contado", ErrValidation)

// ErrCajonConDineroDelTurno: no se puede cambiar si un método va al cajón mientras ese método ya
// recibió dinero en el turno abierto.
var ErrCajonConDineroDelTurno = fmt.Errorf("%w: el turno abierto ya recibió dinero por ese método; ciérralo antes de cambiar si su efectivo entra al cajón", ErrValidation)

// CambioDeCajonPermitido rechaza mover «va al cajón» a media jornada (spec 015, FR-017).
//
// El interruptor dice si el dinero de ese método ESTÁ en el cajón, y el esperado del turno abierto
// se arma con él. Apagarlo cuando la app ya trajo billetes baja el esperado en dinero que sigue
// físicamente en el cajón —medido: de $835 a $700— y el corte cierra con un sobrante fantasma, que
// nadie audita porque un sobrante no duele. Encenderlo a media jornada hace lo mismo al revés.
//
// Se rechaza en vez de recordar el valor que tenía cada cobro porque los cortes CERRADOS ya
// conservan su forma —cada renglón guarda su propio `affects_cash_drawer`— y la única ventana
// ambigua que quedaba era el turno abierto. Cerrarla cuesta esta validación en vez de una
// migración, y no cierra la puerta a guardar el dato por cobro el día que haga falta.
func CambioDeCajonPermitido(cambiaElFlag bool, cobradoEnElTurnoAbierto decimal.Decimal) error {
	if cambiaElFlag && cobradoEnElTurnoAbierto.IsPositive() {
		return ErrCajonConDineroDelTurno
	}
	return nil
}
