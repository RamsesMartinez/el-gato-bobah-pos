package httpapi

import (
	"testing"
)

// low_stock_update con warehouse_id manda el almacén: sin él, el mínimo se guardaría como el del
// artículo y no como el de ese almacén.
func TestLowStockUpdateSendsTheWarehouse(t *testing.T) {
	wh := int64(7)
	body, err := lowStockUpdateArgs{Items: []lowStockItemArg{{ItemType: "ingrediente", ID: 1, Enabled: true}}, WarehouseID: &wh}.body()
	if err != nil {
		t.Fatal(err)
	}
	if body["warehouseId"] != int64(7) {
		t.Fatalf("el cuerpo = %v; quiere warehouseId 7", body)
	}
	body, err = lowStockUpdateArgs{Items: []lowStockItemArg{{ItemType: "ingrediente", ID: 1}}}.body()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["warehouseId"]; ok {
		t.Fatalf("sin almacén el cuerpo trae warehouseId: %v", body)
	}
}

// El conteo de un almacén se lee con ?warehouseId=; sin él, ninguno (el de venta).
func TestWarehouseQuery(t *testing.T) {
	if q := warehouseQuery(nil); q != nil {
		t.Fatalf("sin almacén = %v", q)
	}
	wh := int64(9)
	if q := warehouseQuery(&wh); q.Get("warehouseId") != "9" {
		t.Fatalf("con almacén 9 = %v", q)
	}
}
