package httpapi

import (
	"encoding/json"
	"testing"
)

// Un renglón del lote sin `minStock` deja el mínimo como está; con `minStock: null` lo quita. Antes
// las dos cosas eran lo mismo y la tabla regresaba el mínimo que otra persona acababa de capturar.
func TestABatchRowWithoutMinStockKeepsIt(t *testing.T) {
	var body batchBody
	raw := `{"items":[{"itemType":"ingrediente","id":1,"enabled":true},{"itemType":"ingrediente","id":2,"enabled":true,"minStock":null},{"itemType":"ingrediente","id":3,"enabled":true,"minStock":"5"}]}`
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	items, err := body.items()
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].KeepMin || items[1].KeepMin || items[2].KeepMin || items[2].MinStock == nil || items[2].MinStock.String() != "5" {
		t.Fatalf("%+v", items)
	}
}

// Un mínimo que no es número es 400, no «sin mínimo»: caer a un valor en silencio programaría una
// alerta distinta de la que se escribió (constitución V).
func TestABatchRowWithABadMinStockIsRejected(t *testing.T) {
	var body batchBody
	if err := json.Unmarshal([]byte(`{"items":[{"itemType":"ingrediente","id":1,"enabled":true,"minStock":"abc"}]}`), &body); err != nil {
		t.Fatal(err)
	}
	if _, err := body.items(); err == nil {
		t.Fatal("un mínimo «abc» se aceptó")
	}
}
