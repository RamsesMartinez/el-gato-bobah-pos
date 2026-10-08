package domain

import (
	"errors"
	"testing"
	"time"
)

// EL INSUMO VUELVE SI NO SE CONSUMIÓ, Y LO DECIDE EL SISTEMA.
//
// No se le pregunta al cajero: el producto y el renglón ya responden. Lo que no se prepara —el
// refresco embotellado— vuelve al almacén aunque su renglón haya «salido a cocina», porque todo
// renglón nace enviado y nada se consumió. Lo que se prepara vuelve solo si la comanda no salió.
//
// Preguntárselo al operador sería pedirle que decida en dos segundos algo que el sistema ya sabe, y
// la respuesta equivocada descuadra el almacén sin que nadie se entere.
func TestReponeInventarioSoloSiNoSeConsumio(t *testing.T) {
	salio := time.Date(2026, 9, 3, 20, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		needsPrep bool
		enviado   *time.Time
		want      bool
	}{
		{"sin preparación y enviado: el refresco sigue en el refri", false, &salio, true},
		{"sin preparación y sin enviar", false, nil, true},
		{"con preparación y enviado: ya está en la plancha", true, &salio, false},
		{"con preparación y sin enviar: la comida no se hizo", true, nil, true},
	}
	for _, c := range cases {
		if got := ReponeInventario(c.needsPrep, c.enviado); got != c.want {
			t.Errorf("%s: ReponeInventario = %v, quiere %v", c.name, got, c.want)
		}
	}
}

// UN RENGLÓN YA ENTREGADO NO SE CANCELA.
//
// Cancelarlo bajaría el total de un pedido del que el cliente ya se llevó la comida. Lo que se hace
// con lo entregado es devolver el dinero, no borrar el renglón — son dos operaciones distintas y
// confundirlas es cómo se pierde el rastro de lo que sí salió.
func TestPuedeCancelarRenglon(t *testing.T) {
	casos := []struct {
		nombre              string
		estado              string
		cantidad, entregado string
		quiere              bool
	}{
		{"pendiente en un pedido abierto", StatusAbierta, "2", "0", true},
		{"pendiente en un pedido listo", StatusLista, "2", "0", true},
		{"ya entregado del todo", StatusAbierta, "2", "2", false},
		{"entregado a medias", StatusAbierta, "2", "1", false},
		// Un pedido que ya terminó no admite que se le muevan renglones: su dinero ya se clasificó.
		{"en un pedido cancelado", StatusCancelada, "2", "0", false},
		{"en un pedido reembolsado", StatusReembolsada, "2", "0", false},
		{"en un pedido entregado", StatusEntregada, "2", "0", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := PuedeCancelarRenglon(c.estado, d(c.cantidad), d(c.entregado))
			if c.quiere && err != nil {
				t.Fatalf("debía poderse cancelar, fue %v", err)
			}
			if !c.quiere && err == nil {
				t.Fatal("debía rechazarse y pasó")
			}
		})
	}
}

// El renglón entregado se rechaza con SU error, para que la pantalla pueda decir qué hacer en vez de
// un "no se pudo" que manda a adivinar.
func TestUnRenglonEntregadoSeRechazaConSuPropioError(t *testing.T) {
	if err := PuedeCancelarRenglon(StatusAbierta, d("2"), d("2")); !errors.Is(err, ErrRenglonYaEntregado) {
		t.Fatalf("err = %v, quiere ErrRenglonYaEntregado", err)
	}
}

// Partir un renglón: el inventario se parte insertando un par de movimientos por cada movimiento
// del original. Si las dos mitades no sumaran exacto a 4 decimales, cada partición dejaría un
// residuo de existencias que nadie movió.
func TestSplitMovementHalvesAddUpExactly(t *testing.T) {
	cases := []struct{ q, n, k string }{
		{"-150", "3", "1"},
		{"-100", "3", "2"},
		{"-33.3333", "3", "1"},
		{"-0.0001", "3", "1"},
		{"-250", "7", "3"},
		{"-1", "2", "1"},
	}
	for _, c := range cases {
		keep, move := SplitMovement(d(c.q), d(c.n), d(c.k))
		if !keep.Add(move).Equal(d(c.q)) {
			t.Errorf("SplitMovement(%s, %s, %s) = %s + %s, no suma %s", c.q, c.n, c.k, keep, move, c.q)
		}
		if keep.Exponent() < -4 || move.Exponent() < -4 {
			t.Errorf("SplitMovement(%s, %s, %s) = %s, %s: más de 4 decimales no caben en la columna", c.q, c.n, c.k, keep, move)
		}
	}
	keep, move := SplitMovement(d("-150"), d("3"), d("1"))
	if !keep.Equal(d("-100")) || !move.Equal(d("-50")) {
		t.Fatalf("SplitMovement(-150, 3, 1) = %s, %s; quiere -100, -50", keep, move)
	}
}

// Qué piezas se llevan al partir. Lo pagado se queda en el original (la cobertura es del renglón
// que se cobró); lo pendiente se va primero; y un renglón con piezas entregadas y pendientes no se
// parte por en medio, porque no hay forma de saber cuál de las entregadas se fue.
func TestSplitLine(t *testing.T) {
	cases := []struct {
		name                string
		qty, delivered, cov string
		k                   string
		wantKeepQty, wantKD string
		wantMoveQty, wantMD string
		wantErr             error
		wantMsg             string
	}{
		{name: "pendientes, sin pagos", qty: "3", delivered: "0", cov: "0", k: "1",
			wantKeepQty: "2", wantKD: "0", wantMoveQty: "1", wantMD: "0"},
		{name: "pendientes primero en un renglón mixto", qty: "3", delivered: "1", cov: "0", k: "2",
			wantKeepQty: "1", wantKD: "1", wantMoveQty: "2", wantMD: "0"},
		{name: "todo entregado se parte con su entrega", qty: "3", delivered: "3", cov: "0", k: "1",
			wantKeepQty: "2", wantKD: "2", wantMoveQty: "1", wantMD: "1"},
		{name: "todas juntas en un renglón mixto", qty: "3", delivered: "1", cov: "0", k: "3",
			wantKeepQty: "0", wantKD: "0", wantMoveQty: "3", wantMD: "1"},
		{name: "se lleva solo lo que no está pagado", qty: "3", delivered: "0", cov: "1", k: "2",
			wantKeepQty: "1", wantKD: "0", wantMoveQty: "2", wantMD: "0"},
		{name: "más de las no pagadas", qty: "3", delivered: "0", cov: "2", k: "2",
			wantErr: ErrPieceAlreadyPaid, wantMsg: "Ese producto ya se pagó"},
		{name: "mixto con más piezas que las pendientes", qty: "3", delivered: "1", cov: "0", k: "2.5",
			wantErr: ErrMixedDeliveredPieces, wantMsg: "Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas"},
		{name: "mixto con dos de tres y una pendiente", qty: "3", delivered: "2", cov: "0", k: "2",
			wantErr: ErrMixedDeliveredPieces, wantMsg: "Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas"},
		{name: "más piezas de las que hay", qty: "3", delivered: "0", cov: "0", k: "4",
			wantErr: ErrTooManyPieces, wantMsg: "No hay tantas piezas por quitar"},
		{name: "cero piezas", qty: "3", delivered: "0", cov: "0", k: "0",
			wantErr: ErrEmptySelection, wantMsg: "Elige qué productos paga"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SplitLine(LinePieces{Qty: d(c.qty), Delivered: d(c.delivered), Covered: d(c.cov)}, d(c.k))
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, quiere %v", err, c.wantErr)
				}
				if msg := stripSentinel(err.Error()); msg != c.wantMsg {
					t.Fatalf("texto = %q, quiere %q", msg, c.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("SplitLine: %v", err)
			}
			if !got.Keep.Qty.Equal(d(c.wantKeepQty)) || !got.Keep.Delivered.Equal(d(c.wantKD)) ||
				!got.Move.Qty.Equal(d(c.wantMoveQty)) || !got.Move.Delivered.Equal(d(c.wantMD)) {
				t.Fatalf("SplitLine = %+v; quiere queda %s (%s entregadas), se va %s (%s entregadas)",
					got, c.wantKeepQty, c.wantKD, c.wantMoveQty, c.wantMD)
			}
		})
	}
}

// Cuántas piezas pueden salir de un renglón sin tocar lo pagado: k ≤ n − cubiertas.
func TestMovablePieces(t *testing.T) {
	cases := []struct{ qty, delivered, cov, want string }{
		{"3", "0", "0", "3"},
		{"3", "0", "1", "2"},
		{"3", "3", "3", "0"},
		{"2", "1", "0", "2"},
	}
	for _, c := range cases {
		got := MovablePieces(LinePieces{Qty: d(c.qty), Delivered: d(c.delivered), Covered: d(c.cov)})
		if !got.Equal(d(c.want)) {
			t.Errorf("MovablePieces(%s, %s entregadas, %s cubiertas) = %s, quiere %s", c.qty, c.delivered, c.cov, got, c.want)
		}
	}
}
