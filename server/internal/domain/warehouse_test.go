package domain

import (
	"errors"
	"testing"
)

func TestValidWarehouseName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"normal", "Bodega", true},
		{"con espacios de más se cuenta limpio", "  Cuarto   frío ", true},
		{"vacío no es un nombre", "", false},
		{"solo espacios no es un nombre", "   ", false},
		{"cuarenta letras caben", "abcdefghijabcdefghijabcdefghijabcdefghij", true},
		{"cuarenta y una no caben", "abcdefghijabcdefghijabcdefghijabcdefghijk", false},
		{"cuenta letras, no bytes", "ñññññññññññññññññññññññññññññññññññññññ", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidWarehouseName(c.in)
			if (err == nil) != c.ok {
				t.Fatalf("ValidWarehouseName(%q) = %v, esperaba ok=%v", c.in, err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("un nombre inválido es 400 (ErrValidation), llegó %v", err)
			}
		})
	}
	if got := CleanWarehouseName("  Cuarto   frío "); got != "Cuarto frío" {
		t.Fatalf("se guarda limpio: %q", got)
	}
}

// Las tres razones por las que no se archiva un almacén, cada una con su sentinel: la pantalla dice
// algo distinto en cada caso (no es el de venta / traspásalo primero / termina el conteo).
func TestCanArchiveWarehouse(t *testing.T) {
	cases := []struct {
		name      string
		isSales   bool
		withStock int64
		countOpen bool
		want      error
	}{
		{"vacío y sin conteo se archiva", false, 0, false, nil},
		{"el de venta no se archiva aunque esté vacío", true, 0, false, ErrWarehouseIsSales},
		{"con existencias no se archiva", false, 3, false, ErrWarehouseHasStock},
		{"con un conteo abierto no se archiva", false, 0, true, ErrWarehouseCountOpen},
		{"el de venta gana sobre lo demás", true, 3, true, ErrWarehouseIsSales},
	}
	// Con compras por recibir tampoco: lo que llegue después quedaría en un almacén que no se ve.
	if err := CanArchiveWarehouse(false, 0, false, true); !errors.Is(err, ErrWarehousePendingPurchases) {
		t.Fatalf("con compras por recibir = %v; quiere ErrWarehousePendingPurchases", err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CanArchiveWarehouse(c.isSales, c.withStock, c.countOpen, false)
			if c.want == nil {
				if err != nil {
					t.Fatalf("esperaba poder archivar, llegó %v", err)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("esperaba %v, llegó %v", c.want, err)
			}
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("no poder archivar es estado del negocio (409), llegó %v", err)
			}
		})
	}
	var has WarehouseHasStock
	if !errors.As(CanArchiveWarehouse(false, 7, false, false), &has) || has.Items != 7 {
		t.Fatal("el rechazo por existencias dice cuántos artículos tiene")
	}
}

// Un parámetro de almacén presente y malformado se rechaza; nunca cae al de venta (constitución V).
func TestParseWarehouseParam(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		allowAll bool
		want     WarehouseSel
		ok       bool
	}{
		{"ausente es el de venta", "", true, WarehouseSel{}, true},
		{"un id", "12", true, WarehouseSel{ID: 12}, true},
		{"todos donde se admite", "all", true, WarehouseSel{All: true}, true},
		{"todos donde no se admite", "all", false, WarehouseSel{}, false},
		{"cero no es un almacén", "0", true, WarehouseSel{}, false},
		{"negativo no es un almacén", "-3", true, WarehouseSel{}, false},
		{"texto no es un almacén", "bodega", true, WarehouseSel{}, false},
		{"espacios no se recortan en silencio", " 12", true, WarehouseSel{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseWarehouseParam(c.in, c.allowAll)
			if (err == nil) != c.ok {
				t.Fatalf("ParseWarehouseParam(%q) = %v, esperaba ok=%v", c.in, err, c.ok)
			}
			if err != nil {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("malformado es 400, llegó %v", err)
				}
				return
			}
			if got != c.want {
				t.Fatalf("ParseWarehouseParam(%q) = %+v, esperaba %+v", c.in, got, c.want)
			}
		})
	}
}
