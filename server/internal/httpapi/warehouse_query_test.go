package httpapi

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// ?warehouseId= vacío viene de una pantalla que interpoló algo sin valor: se rechaza, no se lee como
// «el de venta» (constitución V). Ausente sí es el de venta.
func TestWarehouseFromQueryRejectsAnEmptyValue(t *testing.T) {
	cases := []struct {
		url  string
		want domain.WarehouseSel
		ok   bool
	}{
		{"/x", domain.WarehouseSel{}, true},
		{"/x?warehouseId=4", domain.WarehouseSel{ID: 4}, true},
		{"/x?warehouseId=all", domain.WarehouseSel{All: true}, true},
		{"/x?warehouseId=", domain.WarehouseSel{}, false},
		{"/x?warehouseId", domain.WarehouseSel{}, false},
	}
	for _, c := range cases {
		got, err := warehouseFromQuery(httptest.NewRequest("GET", c.url, nil), true)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, domain.ErrValidation)) || got != c.want {
			t.Fatalf("%s → %+v, %v; quiere %+v ok=%v", c.url, got, err, c.want, c.ok)
		}
	}
}
