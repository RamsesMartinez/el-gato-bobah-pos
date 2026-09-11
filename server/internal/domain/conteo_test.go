package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// mnt: atajo local. `d` ya lo usa limits_test.go en este mismo paquete.
func mnt(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// La suma del conteo: piezas × valor, redondeada en la frontera.
//
// Es la única razón de ser de la feature. Si el servidor no recalcula desde las piezas, el operador
// sigue sumando y el faltante que esto viene a quitar sigue naciendo en la libreta.
func TestTotalDelConteo(t *testing.T) {
	casos := []struct {
		nombre string
		piezas []PiezaContada
		quiere string
		err    error
		porQue string
	}{
		{
			nombre: "el cajón del spec: 6 monedas de $10 y 3 billetes de $50",
			piezas: []PiezaContada{{Valor: mnt("10"), Piezas: 6}, {Valor: mnt("50"), Piezas: 3}},
			quiere: "210",
		},
		{
			nombre: "las monedas de 50 centavos no pierden el centavo",
			piezas: []PiezaContada{{Valor: mnt("0.50"), Piezas: 7}},
			quiere: "3.5",
			porQue: "7 monedas de 50¢ son $3.50; truncar el medio peso es exactamente el error que se viene a quitar",
		},
		{
			nombre: "un cajón vacío suma cero y NO es error",
			piezas: nil,
			quiere: "0",
			porQue: "una caja puede arrancar vacía; tratarlo como error dejaría al operador sin poder abrir",
		},
		{
			nombre: "cero piezas de una denominación no aporta ni estorba",
			piezas: []PiezaContada{{Valor: mnt("100"), Piezas: 0}, {Valor: mnt("50"), Piezas: 2}},
			quiere: "100",
			porQue: "FR-009: el cero es válido de entrada y simplemente no suma",
		},
		{
			nombre: "piezas negativas se rechazan",
			piezas: []PiezaContada{{Valor: mnt("100"), Piezas: -1}},
			err:    ErrValidation,
			porQue: "restarían del cajón y el arqueo cuadraría contra una cifra imposible",
		},
		{
			nombre: "una denominación sin valor se rechaza",
			piezas: []PiezaContada{{Valor: mnt("0"), Piezas: 5}},
			err:    ErrValidation,
			porQue: "una pieza que no vale dinero ocupa un botón y no suma: si llega aquí, el catálogo está roto",
		},
		{
			nombre: "un conteo absurdo se rechaza como validación, no revienta",
			piezas: []PiezaContada{{Valor: mnt("1000"), Piezas: 999999999}},
			err:    ErrValidation,
			porQue: "desbordar el numeric(10,2) tiene que salir como 400 accionable y no como 500",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := TotalDelConteo(c.piezas)
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
				t.Fatalf("sumó %s y quiere %s — %s", got, c.quiere, c.porQue)
			}
		})
	}
}

// Los dos caminos son EXCLUYENTES, y ninguno de los dos también es un error.
//
// FR-014 a FR-016. Sin esto vuelve a haber cifras de efectivo sin explicación, que es el problema
// original: un arqueo con un número que nadie puede reconstruir ni justificar.
func TestComoSeDeclaraElEfectivo(t *testing.T) {
	piezas := []PiezaContada{{Valor: mnt("100"), Piezas: 2}}

	casos := []struct {
		nombre string
		piezas []PiezaContada
		total  *decimal.Decimal
		motivo string
		quiere string
		err    error
		porQue string
	}{
		{
			nombre: "contando: el total sale de las piezas",
			piezas: piezas,
			quiere: "200",
		},
		{
			nombre: "a mano con motivo: se respeta el total capturado",
			total:  ptr(mnt("2350")),
			motivo: "había un billete que no está en la lista",
			quiere: "2350",
		},
		{
			nombre: "los dos caminos a la vez se rechazan",
			piezas: piezas,
			total:  ptr(mnt("999")),
			motivo: "porque sí",
			err:    ErrConteoAmbiguo,
			porQue: "son dos cifras del mismo dinero y no hay forma de saber cuál manda",
		},
		{
			nombre: "a mano sin motivo se rechaza",
			total:  ptr(mnt("2350")),
			err:    ErrConteoSinExplicar,
			porQue: "FR-016: un arqueo sin desglose y sin motivo es una cifra que nadie puede auditar",
		},
		{
			nombre: "a mano con un motivo de puros espacios se rechaza",
			total:  ptr(mnt("2350")),
			motivo: "   ",
			err:    ErrConteoSinExplicar,
			porQue: "un motivo en blanco es no haber contestado, y pasa el chequeo de \"no vacío\" ingenuo",
		},
		{
			nombre: "a mano con un motivo de caracteres invisibles se rechaza",
			total:  ptr(mnt("5000")),
			motivo: "\u200b",
			err:    ErrConteoSinExplicar,
			porQue: "TrimSpace no toca el ancho cero y btrim solo quita el espacio ASCII: se colaba un fondo de $5,000 con un motivo que en pantalla se ve vacío",
		},
		{
			nombre: "a mano con un motivo más largo que el tope se rechaza aquí, no en la base",
			total:  ptr(mnt("2350")),
			motivo: strings.Repeat("a", MaxMotivo+1),
			err:    ErrValidation,
			porQue: "el check del esquema lo rechaza con un 23514 que sube como 500, y el cuerpo del 500 se registra en el log con el texto del operador dentro",
		},
		{
			nombre: "a mano con un motivo justo en el tope pasa",
			total:  ptr(mnt("2350")),
			motivo: strings.Repeat("a", MaxMotivo),
			quiere: "2350",
			porQue: "el tope es inclusivo en el esquema (between 1 and 200); rechazarlo aquí partiría en dos la misma regla",
		},
		{
			nombre: "sin piezas y sin total: se abre en cero, que es un cajón vacío",
			quiere: "0",
			porQue: "una caja puede arrancar vacía; el camino manual es para OTRA cosa",
		},
		{
			nombre: "a mano con un total negativo se rechaza",
			total:  ptr(mnt("-5")),
			motivo: "un motivo cualquiera",
			err:    ErrValidation,
			porQue: "no existe un cajón con menos que nada",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := TotalDeclarado(c.piezas, c.total, c.motivo)
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
				t.Fatalf("declaró %s y quiere %s — %s", got, c.quiere, c.porQue)
			}
		})
	}
}

// Los dos sentinels caen en ErrValidation, que es lo que los vuelve 400 y no 500.
//
// El mapeo a HTTP vive solo en httpapi.Error vía errors.Is; si un sentinel nuevo no envuelve a
// ErrValidation, el handler lo deja caer al default y una captura mal hecha sale como "el servidor
// se rompió".
func TestLosErroresDelConteoSonDeValidacion(t *testing.T) {
	for _, err := range []error{ErrConteoAmbiguo, ErrConteoSinExplicar} {
		if !errors.Is(err, ErrValidation) {
			t.Errorf("%v no es un ErrValidation: saldría como 500 y no como 400", err)
		}
	}
}

func ptr(v decimal.Decimal) *decimal.Decimal { return &v }
