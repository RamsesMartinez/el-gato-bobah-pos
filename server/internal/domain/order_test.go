package domain

import "testing"

func TestCanTransition(t *testing.T) {
	ok := [][2]string{
		{StatusAbierta, StatusLista},
		{StatusAbierta, StatusEntregada},
		{StatusAbierta, StatusCancelada},
		{StatusLista, StatusEntregada},
		{StatusLista, StatusCancelada},
	}
	for _, tc := range ok {
		if !CanTransition(tc[0], tc[1]) {
			t.Errorf("%s→%s should be allowed", tc[0], tc[1])
		}
	}
	bad := [][2]string{
		{StatusCancelada, StatusCancelada}, // doble cancel (evita doble-restock)
		{StatusEntregada, StatusCancelada}, // void después de entregar
		{StatusEntregada, StatusLista},     // regresión
		{StatusLista, StatusAbierta},       // regresión
		{StatusCancelada, StatusLista},
	}
	for _, tc := range bad {
		if CanTransition(tc[0], tc[1]) {
			t.Errorf("%s→%s should be rejected", tc[0], tc[1])
		}
	}
}

func TestBuildOrder(t *testing.T) {
	products := map[int64]PricedProduct{
		1: {ID: 1, Name: "Frappé", Price: 45, Cost: 12, Active: true},
		2: {ID: 2, Name: "Inactivo", Price: 10, Active: false},
	}
	options := map[int64]PricedOption{
		10: {ID: 10, Name: "Perlas", PriceDelta: 20, Cost: 5, GroupTitle: "Toppings"},
		11: {ID: 11, Name: "Litchi", PriceDelta: 20, Cost: 6, GroupTitle: "Toppings"},
	}

	// 2 Frappé con Perlas x1 y Litchi x1: unit = 45 + 20 + 20 = 85 ; línea = 170
	got, err := BuildOrder([]OrderLineInput{
		{ProductID: 1, Qty: 2, Modifiers: []OrderModInput{{OptionID: 10, Qty: 1}, {OptionID: 11, Qty: 1}}},
	}, products, options)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 170 {
		t.Fatalf("total=%v want 170", got.Total)
	}
	l := got.Lines[0]
	if l.ModifiersTotal != 40 || l.UnitCost != 23 || l.LineTotal != 170 {
		t.Fatalf("línea: modsTotal=%v unitCost=%v lineTotal=%v", l.ModifiersTotal, l.UnitCost, l.LineTotal)
	}

	// producto inactivo → error
	if _, err := BuildOrder([]OrderLineInput{{ProductID: 2, Qty: 1}}, products, options); err == nil {
		t.Fatal("producto inactivo debe fallar")
	}
	// opción inexistente → error
	if _, err := BuildOrder([]OrderLineInput{{ProductID: 1, Qty: 1, Modifiers: []OrderModInput{{OptionID: 99}}}}, products, options); err == nil {
		t.Fatal("opción inexistente debe fallar")
	}
	// pedido vacío → error
	if _, err := BuildOrder(nil, products, options); err == nil {
		t.Fatal("pedido vacío debe fallar")
	}
}
