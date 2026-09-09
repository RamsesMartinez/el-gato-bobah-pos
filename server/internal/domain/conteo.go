package domain

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// El conteo de efectivo por denominaciones (spec 003).
//
// Por qué vive aquí y no en el servicio: es aritmética pura y es la razón de ser de la feature. El
// operador cuenta el cajón y hoy suma en una libreta; ese paso es de donde nacen los faltantes que
// después nadie puede explicar. Probar la suma sin base de datos es lo que permite cubrir los bordes
// —el medio peso, el cajón vacío, el desbordamiento— sin montar nada.

// ErrConteoAmbiguo: se mandaron piezas Y un total capturado a mano.
//
// No es un caso raro que valga la pena tolerar eligiendo uno: son dos cifras del mismo dinero y
// quedarse con cualquiera de las dos es inventar cuál era la buena.
var ErrConteoAmbiguo = fmt.Errorf("%w: se contaron piezas y además se capturó un total a mano", ErrValidation)

// ErrConteoSinExplicar: se capturó un total a mano sin decir por qué.
//
// FR-016. Sin esto vuelve a haber arqueos con una cifra de efectivo que nadie puede reconstruir ni
// justificar, que es el problema que esta feature viene a cerrar.
var ErrConteoSinExplicar = fmt.Errorf("%w: capturar el total a mano exige un motivo", ErrValidation)

// PiezaContada: cuántas piezas de una denominación se contaron.
//
// Lleva el VALOR y no el id: el dominio no consulta el catálogo, y el servicio ya tiene que leerlo
// para validar que la denominación sea de la moneda del turno. Pasarle el valor resuelto mantiene
// esto sin I/O, que es lo que lo hace probable sin base.
type PiezaContada struct {
	Valor  decimal.Decimal
	Piezas int
}

// TotalDelConteo suma piezas × valor y redondea en la frontera.
//
// Un conteo VACÍO vale cero y no es error: una caja puede arrancar con el cajón vacío, y tratarlo
// como error dejaría al operador sin poder abrir el turno.
func TotalDelConteo(piezas []PiezaContada) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, p := range piezas {
		if p.Piezas < 0 {
			return decimal.Zero, fmt.Errorf("%w: %d piezas de $%s", ErrValidation, p.Piezas, p.Valor)
		}
		// Un valor no positivo aquí significa que el catálogo está roto: el esquema tiene
		// `check (value > 0)`. Se rechaza igual en vez de sumar cero en silencio, porque una
		// denominación fantasma en la pantalla es un botón que el operador toca y no hace nada.
		if !p.Valor.IsPositive() {
			return decimal.Zero, fmt.Errorf("%w: una denominación de $%s no vale dinero", ErrValidation, p.Valor)
		}
		if p.Piezas == 0 {
			continue
		}
		total = total.Add(p.Valor.Mul(decimal.NewFromInt(int64(p.Piezas))))
	}
	total = Round2(total)
	// El tope se comprueba sobre el TOTAL y no pieza por pieza: mil billetes de $1000 pasan uno a
	// uno y desbordan el numeric(10,2) al sumarse. Sale como validación para que el handler lo
	// traduzca a 400 y no a un 500 con el error crudo de Postgres.
	if !ValidMoney(total, true) {
		return decimal.Zero, fmt.Errorf("%w: el conteo suma más de lo que el sistema admite", ErrValidation)
	}
	return total, nil
}

// TotalDeclarado resuelve los DOS CAMINOS de FR-014: contar por denominaciones, o capturar el total
// a mano con un motivo. Nunca los dos, y el motivo no es opcional en el segundo.
//
// Sin piezas y sin total es un cajón vacío, no un error: el camino manual existe para el billete que
// no está en la lista, no para abrir en cero.
func TotalDeclarado(piezas []PiezaContada, aMano *decimal.Decimal, motivo string) (decimal.Decimal, error) {
	conteo := len(piezas) > 0
	switch {
	case conteo && aMano != nil:
		return decimal.Zero, ErrConteoAmbiguo
	case aMano != nil:
		// `TrimSpace` y no `!= ""`: un motivo de puros espacios es no haber contestado, y pasa el
		// chequeo ingenuo de "no vacío".
		if strings.TrimSpace(motivo) == "" {
			return decimal.Zero, ErrConteoSinExplicar
		}
		total := Round2(*aMano)
		if !ValidMoney(total, true) {
			return decimal.Zero, fmt.Errorf("%w: el total capturado no es un importe válido", ErrValidation)
		}
		return total, nil
	default:
		return TotalDelConteo(piezas)
	}
}
