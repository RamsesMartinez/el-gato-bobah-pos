package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

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

func TestCanRefund(t *testing.T) {
	if !CanRefund(StatusEntregada) {
		t.Error("una orden entregada debe poder reembolsarse")
	}
	for _, s := range []string{StatusAbierta, StatusLista, StatusCancelada, StatusReembolsada} {
		if CanRefund(s) {
			t.Errorf("no se debe poder reembolsar desde %q", s)
		}
	}
}

func TestBuildOrder(t *testing.T) {
	products := map[int64]PricedProduct{
		1: {ID: 1, Name: "Frappé", Price: d("45"), Cost: d("12"), Active: true},
		2: {ID: 2, Name: "Inactivo", Price: d("10"), Active: false},
	}
	options := map[int64]PricedOption{
		10: {ID: 10, Name: "Perlas", PriceDelta: d("20"), Cost: d("5"), GroupTitle: "Toppings"},
		11: {ID: 11, Name: "Litchi", PriceDelta: d("20"), Cost: d("6"), GroupTitle: "Toppings"},
	}

	// 2 Frappé con Perlas x1 y Litchi x1: unit = 45 + 20 + 20 = 85 ; línea = 170
	got, err := BuildOrder([]OrderLineInput{
		{ProductID: 1, Qty: d("2"), Modifiers: []OrderModInput{{OptionID: 10, Qty: 1}, {OptionID: 11, Qty: 1}}},
	}, products, options)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Total.Equal(d("170")) {
		t.Fatalf("total=%v want 170", got.Total)
	}
	l := got.Lines[0]
	if !l.ModifiersTotal.Equal(d("40")) || !l.UnitCost.Equal(d("23")) || !l.LineTotal.Equal(d("170")) {
		t.Fatalf("línea: modsTotal=%v unitCost=%v lineTotal=%v", l.ModifiersTotal, l.UnitCost, l.LineTotal)
	}

	// producto inactivo → error
	if _, err := BuildOrder([]OrderLineInput{{ProductID: 2, Qty: d("1")}}, products, options); err == nil {
		t.Fatal("producto inactivo debe fallar")
	}
	// opción inexistente → error
	if _, err := BuildOrder([]OrderLineInput{{ProductID: 1, Qty: d("1"), Modifiers: []OrderModInput{{OptionID: 99}}}}, products, options); err == nil {
		t.Fatal("opción inexistente debe fallar")
	}
	// pedido vacío → error
	if _, err := BuildOrder(nil, products, options); err == nil {
		t.Fatal("pedido vacío debe fallar")
	}

	// A04 — cotas: entradas absurdas fallan como validación (400), no como overflow (500).
	// Con decimal ya no hay NaN/±Inf que probar (son imposibles); quedan las cotas reales.
	adversarial := []struct {
		name string
		line OrderLineInput
	}{
		{"qty sobre el tope", OrderLineInput{ProductID: 1, Qty: MaxOrderQty.Add(d("1"))}},
		{"qty sub-centésima que redondea a 0 (numeric(8,2))", OrderLineInput{ProductID: 1, Qty: d("0.001")}},
		{"qty gigante (overflow del total)", OrderLineInput{ProductID: 1, Qty: d("1e50")}},
		{"modificador con qty que haría wrap de int16", OrderLineInput{
			ProductID: 1, Qty: d("1"), Modifiers: []OrderModInput{{OptionID: 10, Qty: 40000}},
		}},
	}
	for _, c := range adversarial {
		t.Run(c.name, func(t *testing.T) {
			if _, err := BuildOrder([]OrderLineInput{c.line}, products, options); err == nil {
				t.Fatalf("%s debe rechazarse", c.name)
			}
		})
	}
}

func TestApplyDeliveryFee(t *testing.T) {
	base := func() BuiltOrder { return BuiltOrder{Subtotal: d("170"), Total: d("170")} }

	// No domicilio: el fee se ignora aunque venga en el body → 0, total intacto.
	if o, err := ApplyDeliveryFee(base(), d("20"), false); err != nil ||
		!o.DeliveryFee.Equal(d("0")) || !o.Total.Equal(d("170")) {
		t.Fatalf("no-domicilio: fee=%v total=%v err=%v", o.DeliveryFee, o.Total, err)
	}
	// Domicilio con fee: se suma al total.
	if o, err := ApplyDeliveryFee(base(), d("20"), true); err != nil ||
		!o.DeliveryFee.Equal(d("20")) || !o.Total.Equal(d("190")) {
		t.Fatalf("domicilio: fee=%v total=%v err=%v", o.DeliveryFee, o.Total, err)
	}
	// Envío gratis (0) es válido a domicilio.
	if o, err := ApplyDeliveryFee(base(), d("0"), true); err != nil ||
		!o.DeliveryFee.Equal(d("0")) || !o.Total.Equal(d("170")) {
		t.Fatalf("envío gratis: fee=%v total=%v err=%v", o.DeliveryFee, o.Total, err)
	}
	// Adversarial: montos absurdos caen como validación (400), no como check de columna violado.
	if _, err := ApplyDeliveryFee(base(), d("-1"), true); err == nil {
		t.Error("fee negativo debe rechazarse")
	}
	if _, err := ApplyDeliveryFee(base(), MaxMoney.Add(d("1")), true); err == nil {
		t.Error("fee > MaxMoney debe rechazarse")
	}
}

// El error de producto no vendible debe decir QUÉ producto es, no solo un número: el operador
// tiene el carrito enfrente y "producto no disponible (id 510)" no le dice cuál quitar.
func TestProductoNoDisponibleNombraElProducto(t *testing.T) {
	prods := map[int64]PricedProduct{
		7: {ID: 7, Name: "Chococino", Price: decimal.NewFromInt(50), Active: false},
	}
	lines := []OrderLineInput{{ProductID: 7, Qty: decimal.NewFromInt(1)}}

	_, err := BuildOrder(lines, prods, map[int64]PricedOption{})
	if !errors.Is(err, ErrProductNotSell) {
		t.Fatalf("debe seguir siendo ErrProductNotSell, fue %v", err)
	}
	var pu ProductUnavailable
	if !errors.As(err, &pu) {
		t.Fatalf("debe poder desempaquetarse como ProductUnavailable, fue %T", err)
	}
	if pu.ProductID != 7 || pu.Name != "Chococino" {
		t.Fatalf("id/nombre incorrectos: %+v", pu)
	}
	if !strings.Contains(err.Error(), "Chococino") {
		t.Fatalf("el mensaje debe nombrar el producto: %q", err.Error())
	}
}

// Un id que el catálogo del tenant no conoce no se puede nombrar sin leer el catálogo de otra
// empresa — la RLS lo impide a propósito. El error lleva el id y un mensaje que sí le dice al
// operador qué hacer, en vez de un "no disponible" que suena a que el producto se acabó.
func TestProductoFueraDelMenuLlevaElIdYUnMensajeAccionable(t *testing.T) {
	lines := []OrderLineInput{{ProductID: 510, Qty: decimal.NewFromInt(1)}}

	_, err := BuildOrder(lines, map[int64]PricedProduct{}, map[int64]PricedOption{})
	if !errors.Is(err, ErrProductNotSell) {
		t.Fatalf("debe seguir siendo ErrProductNotSell, fue %v", err)
	}
	var pu ProductUnavailable
	if !errors.As(err, &pu) {
		t.Fatalf("debe poder desempaquetarse como ProductUnavailable, fue %T", err)
	}
	if pu.ProductID != 510 || pu.Name != "" {
		t.Fatalf("sin nombre y con el id: %+v", pu)
	}
	if !strings.Contains(err.Error(), "510") {
		t.Fatalf("el mensaje debe llevar el id: %q", err.Error())
	}
	// No debe decir solo "no disponible": ese texto hace pensar que se agotó, cuando lo que pasa
	// es que el menú de la pantalla no es el de la empresa en la que está la sesión.
	if !strings.Contains(err.Error(), "menú") {
		t.Fatalf("el mensaje debe explicar que no está en este menú: %q", err.Error())
	}
}

func TestMetodoCorrespondeALaPlataforma(t *testing.T) {
	uber := int16(5)
	didi := int16(8)
	casos := []struct {
		nombre         string
		metodo, pedido *int16
		quiere         bool
	}{
		{"mostrador con efectivo: el caso de todos los días", nil, nil, true},
		{"Uber con el método de Uber", &uber, &uber, true},
		{"Uber con el método de Didi", &didi, &uber, false},
		{"Uber con el efectivo de mostrador deja faltante", nil, &uber, false},
		{"mostrador con método de Uber deja sobrante", &uber, nil, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := MetodoCorrespondeALaPlataforma(c.metodo, c.pedido); got != c.quiere {
				t.Fatalf("quiere %v, obtuvo %v", c.quiere, got)
			}
		})
	}
}

// El tope de veces que una opción puede ir en la MISMA línea lo pone el negocio en `max_per_line`
// (las 64 salsas están en 2; 818 opciones donde repetir no tiene sentido están en 1). Hasta que la
// pantalla dejó pedir dos iguales, ese tope nunca se ejercía: todo llegaba en 1.
//
// Se valida en el servidor y no solo en la pantalla porque la pantalla es espejo, no barrera. Un
// cliente que mande 40 salsas en una línea no es un ataque —se le cobrarían— pero manda a cocina un
// ticket que el negocio nunca aceptó, y lo hace sin que nada avise.
func TestBuildOrderRespetaMaxPerLine(t *testing.T) {
	products := map[int64]PricedProduct{
		1: {ID: 1, Name: "Boneless", Price: d("200"), Cost: d("80"), Active: true},
	}
	options := map[int64]PricedOption{
		10: {ID: 10, Name: "Mango habanero", PriceDelta: d("0"), GroupTitle: "Salsas", MaxPerLine: 2},
		12: {ID: 12, Name: "Sin salsa", PriceDelta: d("0"), GroupTitle: "Salsas", MaxPerLine: 1},
	}
	linea := func(optID int64, q int) []OrderLineInput {
		return []OrderLineInput{{ProductID: 1, Qty: d("1"), Modifiers: []OrderModInput{{OptionID: optID, Qty: q}}}}
	}

	// Dos del mismo sabor: el caso que la feature vino a permitir.
	got, err := BuildOrder(linea(10, 2), products, options)
	if err != nil {
		t.Fatalf("dos salsas iguales deben pasar: %v", err)
	}
	if n := got.Lines[0].Modifiers[0].Qty; n != 2 {
		t.Fatalf("cantidad persistida = %d, quiere 2", n)
	}

	// Una tercera ya no: el negocio dijo dos.
	if _, err := BuildOrder(linea(10, 3), products, options); !errors.Is(err, ErrOptionOverMax) {
		t.Fatalf("tres del mismo sabor debe rechazarse, fue %v", err)
	}

	// Y una opción que no se repite se rechaza en la segunda.
	if _, err := BuildOrder(linea(12, 2), products, options); !errors.Is(err, ErrOptionOverMax) {
		t.Fatalf("repetir una opción que no lo admite debe rechazarse, fue %v", err)
	}

	// max_per_line en 0 significa "sin configurar", no "ninguna": el default de la columna es 1 y
	// un 0 solo puede venir de datos viejos. Tratarlo como tope haría irrepetible TODO.
	sinConfigurar := map[int64]PricedOption{
		10: {ID: 10, Name: "Mango habanero", PriceDelta: d("0"), GroupTitle: "Salsas", MaxPerLine: 0},
	}
	if _, err := BuildOrder(linea(10, 1), products, sinConfigurar); err != nil {
		t.Fatalf("una opción sin tope configurado debe aceptar la primera: %v", err)
	}
}

// QUIÉN RECIBE PRODUCTOS (D-9, D-11).
//
// Antes bastaba el estado: el entregado recibía siempre, porque el cliente que sigue en la mesa pide
// una más. Con la 030 «cerrada» es pagada Y entregada: esa cuenta ya no recibe —lo que pidan después
// es otra cuenta—, y la entregada que todavía debe sí, y vuelve a cocina. La pagada que sigue en
// cocina también recibe: era el caso 2 del lienzo, al que no había forma de agregarle nada.
//
// Las dos mitades (recibe y reabre) van juntas a propósito: permitir el agregado SIN reabrir es peor
// que rechazarlo — el renglón entra, el tablero solo lista abierta y lista, y nadie prepara la comida.
func TestCanReceiveLines(t *testing.T) {
	d := decimal.RequireFromString
	uber := int16(2)
	casos := []struct {
		nombre string
		o      OrderForAdd
		want   error
		reabre bool
	}{
		{"abierta", OrderForAdd{Status: StatusAbierta, Paid: d("0"), Total: d("100")}, nil, false},
		{"pagada en cocina recibe (caso 2)", OrderForAdd{Status: StatusAbierta, Paid: d("100"), Total: d("100")}, nil, false},
		{"lista", OrderForAdd{Status: StatusLista, Paid: d("0"), Total: d("100")}, nil, false},
		{"entregada que debe $5", OrderForAdd{Status: StatusEntregada, Paid: d("95"), Total: d("100")}, nil, true},
		{"entregada y saldada: cerrada", OrderForAdd{Status: StatusEntregada, Paid: d("100"), Total: d("100")}, ErrOrderClosed, false},
		// Desde la 031 no hay tolerancia: un centavo de diferencia es deuda y la cuenta sigue viva.
		{"entregada con $0.01 de diferencia: debe y recibe (031 quitó la tolerancia)", OrderForAdd{Status: StatusEntregada, Paid: d("99.99"), Total: d("100")}, nil, true},
		{"entregada de $0", OrderForAdd{Status: StatusEntregada, Paid: d("0"), Total: d("0")}, ErrOrderClosed, false},
		{"de plataforma abierta", OrderForAdd{Status: StatusAbierta, Paid: d("0"), Total: d("100"), PlatformID: &uber}, ErrPlatformOrderNoLines, false},
		{"de plataforma entregada", OrderForAdd{Status: StatusEntregada, Paid: d("0"), Total: d("100"), PlatformID: &uber}, ErrPlatformOrderNoLines, false},
		{"de plataforma cancelada", OrderForAdd{Status: StatusCancelada, Paid: d("0"), Total: d("100"), PlatformID: &uber}, ErrPlatformOrderNoLines, false},
		{"cancelada: su dinero ya se decidió", OrderForAdd{Status: StatusCancelada, Paid: d("0"), Total: d("100")}, ErrConflict, false},
		{"reembolsada: un arqueo firmado ya la contó", OrderForAdd{Status: StatusReembolsada, Paid: d("100"), Total: d("100")}, ErrConflict, false},
		// La tableta suspendida vuelve con un estado viejo; uno que no existe no abre la puerta.
		{"estado desconocido", OrderForAdd{Status: "", Paid: d("0"), Total: d("100")}, ErrConflict, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := CanReceiveLines(c.o)
			if c.want == nil && err != nil {
				t.Fatalf("CanReceiveLines = %v, quería nil", err)
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("CanReceiveLines = %v, quería %v", err, c.want)
			}
			if err == nil {
				if got := ReabreAlAgregar(c.o.Status); got != c.reabre {
					t.Fatalf("ReabreAlAgregar(%s) = %v, quería %v", c.o.Status, got, c.reabre)
				}
			}
		})
	}
	// ErrOrderClosed no es el ErrConflict genérico: la pantalla ofrece «empezar cuenta nueva».
	if errors.Is(ErrOrderClosed, ErrPlatformOrderNoLines) || errors.Is(ErrPlatformOrderNoLines, ErrConflict) {
		t.Fatal("los dos rechazos tienen que distinguirse: uno ofrece cuenta nueva y el otro no")
	}
}

// OrderNotVoided es lo que queda de PuedeRecibirLineas para los caminos que NO agregan: entregar un
// renglón, cancelarlo, devolver un pago, pasar productos desde un pedido. Ahí la pregunta sigue
// siendo solo «¿el dinero de este pedido ya se decidió?».
func TestOrderNotVoided(t *testing.T) {
	casos := map[string]bool{
		StatusAbierta: true, StatusLista: true, StatusEntregada: true,
		StatusCancelada: false, StatusReembolsada: false, "": false,
	}
	for estado, want := range casos {
		if got := OrderNotVoided(estado); got != want {
			t.Errorf("OrderNotVoided(%q) = %v, quería %v", estado, got, want)
		}
	}
}

// Reabrir es consecuencia de agregar, NO una transición que alguien pueda pedir. Si se colara a
// CanTransition, la tableta podría des-entregar un pedido a mano y ese camino no tiene dueño: nada
// valida por qué, ni deja rastro de quién lo hizo.
func TestDesEntregarSigueSinSerUnaTransicionPedible(t *testing.T) {
	for _, destino := range []string{StatusAbierta, StatusLista} {
		if CanTransition(StatusEntregada, destino) {
			t.Errorf("CanTransition(entregada, %s) = true: des-entregar a mano no es una acción de esta app", destino)
		}
	}
}

// UN PEDIDO DE PLATAFORMA NO ES DE MOSTRADOR (spec 029). La base lo rechaza con un check, y ese
// rechazo llegaba como 500: «el servidor se rompió» por una combinación que quien opera puede
// corregir.
func TestUnPedidoDePlataformaNoEsDeMostrador(t *testing.T) {
	uber := int16(1)
	casos := []struct {
		servicio   string
		plataforma *int16
		ok         bool
	}{
		{"mostrador", &uber, false},
		{"domicilio", &uber, true},
		{"para_llevar", &uber, true},
		{"mostrador", nil, true},
	}
	for _, c := range casos {
		err := ValidPlatformServiceType(c.servicio, c.plataforma)
		if c.ok && err != nil {
			t.Errorf("%s con plataforma=%v debía aceptarse: %v", c.servicio, c.plataforma != nil, err)
		}
		if !c.ok && !errors.Is(err, ErrValidation) {
			t.Errorf("%s con plataforma debía rechazarse como validación, fue %v", c.servicio, err)
		}
	}
}
