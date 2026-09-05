//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// AGREGAR RENGLONES DOS VECES MUEVE DOS COSAS: LO QUE SE COBRA Y LO QUE SE DESCUENTA.
//
// Crear el pedido ya era idempotente y cobrarlo también; agregar quedó fuera. Y es el peor de los
// tres para quedarse sin llave, porque un reintento no solo le cobra de más al cliente: descuenta la
// materia prima otra vez, y eso no se descubre hasta que el conteo físico no cuadra semanas después.
//
// Nada más lo atrapa: dos renglones idénticos en el mismo pedido son legítimos —el cliente pidió
// otro café— así que ninguna validación puede distinguir el reintento de la segunda orden.
func TestAgregarElMismoLoteDosVecesNoDuplicaNiCobraDeMas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_idem_lineas", "cajero")
	prod := makeProduct(t, st, "Café idempotente", decimal.RequireFromString("50"), true)
	abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)

	pedido, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido: %v", err)
	}
	totalAntes := pedido.Total
	movimientosAntes := countOrderMovements(t, st, pedido.ID)

	// El MISMO lote, dos veces. Es el doble tap sobre una tableta que no alcanzó a pintar la
	// respuesta, o el reintento tras un corte de red al confirmar.
	lote := uuid.New()
	agregado := []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("2")}}

	uno, err := svc.AddLines(ctx, pedido.ID, agregado, cajero, lote)
	if err != nil {
		t.Fatalf("primer agregado: %v", err)
	}
	dos, err := svc.AddLines(ctx, pedido.ID, agregado, cajero, lote)
	if err != nil {
		t.Fatalf("el reenvío del mismo lote debe ser inocuo, no un error: %v", err)
	}

	esperado := totalAntes.Add(decimal.RequireFromString("100")) // 2 × $50, una sola vez
	if !uno.Total.Equal(esperado) {
		t.Fatalf("el primer agregado dejó el total en %s y debía dejarlo en %s", uno.Total, esperado)
	}
	if !dos.Total.Equal(esperado) {
		t.Errorf("el reenvío subió el total de %s a %s: se le está cobrando dos veces al cliente",
			esperado, dos.Total)
	}

	var renglones int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from order_lines where order_id = $1 and cancelled_at is null`,
		pedido.ID).Scan(&renglones); err != nil {
		t.Fatalf("contar renglones: %v", err)
	}
	if renglones != 2 { // el original + el agregado, una sola vez
		t.Errorf("el pedido quedó con %d renglones y debía quedar con 2: el lote se aplicó dos veces", renglones)
	}

	// Y lo que no se ve hasta el conteo físico: el inventario.
	movimientosUno := countOrderMovements(t, st, pedido.ID)
	if movimientosUno <= movimientosAntes {
		t.Fatalf("el primer agregado no descontó inventario (%d → %d)", movimientosAntes, movimientosUno)
	}
	if len(dos.Agregados) != 0 {
		t.Errorf("el reenvío devolvió %d renglones como recién agregados: cocina volvería a "+
			"prepararlos", len(dos.Agregados))
	}
}

// Un lote mal dirigido REBOTA, no se aplica.
//
// El caso real: la pantalla equivocada, o un pedido que cambió bajo los pies del operador. Si la
// llave valiera solo dentro de un pedido, el mismo lote entraría también en el otro y la comida se
// le cargaría a una cuenta ajena — que en una mesa compartida es una discusión con el cliente.
func TestUnLoteDeRenglonesNoSeAplicaAOtroPedido(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_lote_ajeno", "cajero")
	prod := makeProduct(t, st, "Café ajeno", decimal.RequireFromString("30"), false)
	abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)

	nuevo := func() *app.OrderView {
		t.Helper()
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		})
		if err != nil {
			t.Fatalf("crear: %v", err)
		}
		return o
	}
	a, b := nuevo(), nuevo()

	lote := uuid.New()
	linea := []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}}
	if _, err := svc.AddLines(ctx, a.ID, linea, cajero, lote); err != nil {
		t.Fatalf("agregar al primero: %v", err)
	}

	_, err := svc.AddLines(ctx, b.ID, linea, cajero, lote)
	if err == nil {
		t.Fatal("el mismo lote entró en dos pedidos distintos: la comida se le carga a una cuenta ajena")
	}
	if !strings.Contains(err.Error(), "ya se agregaron al pedido") {
		t.Errorf("el rechazo tiene que decir a qué pedido se aplicó el lote, y dijo: %v", err)
	}

	var renglonesB int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from order_lines where order_id = $1`, b.ID).Scan(&renglonesB); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if renglonesB != 1 {
		t.Errorf("el segundo pedido quedó con %d renglones: el lote ajeno sí se aplicó", renglonesB)
	}
}

// Sin llave se sigue pudiendo agregar: un cliente viejo no se queda sin poder trabajar.
//
// Es un techo consciente — sin llave no hay protección — y por eso el front SIEMPRE la manda. Se
// prueba para que quede claro que la ausencia es un camino soportado y no un descuido.
func TestAgregarSinLlaveSigueFuncionando(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_sin_llave", "cajero")
	prod := makeProduct(t, st, "Café sin llave", decimal.RequireFromString("25"), false)
	abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)

	pedido, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	vista, err := svc.AddLines(ctx, pedido.ID,
		[]domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		cajero, uuid.Nil())
	if err != nil {
		t.Fatalf("agregar sin llave: %v", err)
	}
	if len(vista.Agregados) != 1 {
		t.Errorf("sin llave se agregaron %d renglones y debía agregarse 1", len(vista.Agregados))
	}
}
