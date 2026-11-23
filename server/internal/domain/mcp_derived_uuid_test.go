package domain

import "testing"

// Un reintento de una escritura con la misma clave tiene que mandar el MISMO id a la ruta: la red de
// la ruta (crear una cuenta con un id que ya existe, cobrar con un clientUuid ya usado) es la que
// impide el doble cobro si el registro de idempotencia falla.
func TestDerivedUUIDIsStablePerKeyAndDistinctPerPart(t *testing.T) {
	a := DerivedUUID("2", "15", "cobro-0001", "order_pay")
	if a != DerivedUUID("2", "15", "cobro-0001", "order_pay") {
		t.Fatal("la misma clave dio dos ids: un reintento cobraría dos veces")
	}
	for _, other := range [][]string{
		{"3", "15", "cobro-0001", "order_pay"},
		{"2", "16", "cobro-0001", "order_pay"},
		{"2", "15", "cobro-0002", "order_pay"},
		{"2", "15", "cobro-0001", "order_draft_create"},
		{"2", "151", "cobro-0001", "order_pay"}, // sin separador, "15"+"1…" chocaría con "151"+"…"
	} {
		if DerivedUUID(other...) == a {
			t.Fatalf("%v dio el mismo id que otra clave", other)
		}
	}
	if DerivedUUID("2", "15c", "x") == DerivedUUID("2", "15", "cx") {
		t.Fatal("las partes se concatenan sin separar: dos claves distintas comparten id")
	}
	if v := a[6] >> 4; v != 8 {
		t.Fatalf("versión %d, esperaba 8 (derivado, RFC 9562)", v)
	}
	if a[8]&0xC0 != 0x80 {
		t.Fatalf("variante %08b, esperaba la de RFC 9562", a[8])
	}
}
