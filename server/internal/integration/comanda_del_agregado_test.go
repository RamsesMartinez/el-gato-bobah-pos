//go:build integration

package integration

import (
	"context"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA COMANDA DEL AGREGADO SALE CON LO AGREGADO, Y CON NADA MÁS.
//
// Es el mecanismo central de la feature y el que decide si cocina prepara dos veces lo mismo. La
// respuesta del agregado dice CUÁLES renglones entraron, y el papel se arma con esos: deducirlos
// comparando contra lo que la pantalla tenía incluiría lo que agregó la otra estación, y cocina
// prepararía de nuevo lo que el compañero ya mandó.
//
// La marca en la base va aparte de la respuesta: es el registro de CUÁNDO salió cada renglón, que
// es lo que se consulta cuando cocina reclama que no le llegó algo.
func TestSoloLosRenglonesAgregadosSalenEnLaComandaDelAgregado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_comanda", "cajero")
	cafe := makeProduct(t, st, "Café comanda", decimal.RequireFromString("100"), false)
	pan := makeProduct(t, st, "Pan comanda", decimal.RequireFromString("50"), false)
	abrirCajaPrincipal(t, st, cajero)

	// Confirmar: los dos renglones salen en la comanda del pedido completo, así que nacen marcados.
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{
			{ProductID: cafe, Qty: decimal.RequireFromString("1")},
			{ProductID: pan, Qty: decimal.RequireFromString("1")},
		},
	})
	if err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if sinEnviar := renglonesSinEnviar(t, st, ord.ID); sinEnviar != 0 {
		t.Fatalf("tras confirmar quedaron %d renglones sin marcar: el primer agregado sacaría el pedido entero y cocina prepararía dos veces", sinEnviar)
	}

	// Agregar: la respuesta dice cuál entró, y solo ese.
	tras, err := svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{
		{ProductID: cafe, Qty: decimal.RequireFromString("2")},
	}, cajero, uuid.New())
	if err != nil {
		t.Fatalf("AddLines: %v", err)
	}
	if len(tras.Agregados) != 1 {
		t.Fatalf("la respuesta trae %d renglones agregados, quiere 1: la pantalla no sabría qué imprimir", len(tras.Agregados))
	}
	if len(tras.Lines) != 3 {
		t.Fatalf("el pedido quedó con %d renglones, quiere 3", len(tras.Lines))
	}

	// Y el id que devuelve es el del renglón NUEVO, no uno de los que ya estaban.
	viejos := map[int64]bool{}
	for _, l := range ord.Lines {
		viejos[l.ID] = true
	}
	if viejos[tras.Agregados[0]] {
		t.Error("la respuesta señaló un renglón que ya estaba: cocina volvería a preparar lo que ya tiene en la plancha")
	}

	// Los tres quedan marcados: el agregado también salió en papel.
	if sinEnviar := renglonesSinEnviar(t, st, ord.ID); sinEnviar != 0 {
		t.Errorf("quedaron %d renglones sin marcar tras agregar", sinEnviar)
	}

	// Un segundo agregado señala solo el suyo, no acumula los anteriores.
	otra, err := svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{
		{ProductID: pan, Qty: decimal.RequireFromString("1")},
	}, cajero, uuid.New())
	if err != nil {
		t.Fatalf("segundo AddLines: %v", err)
	}
	if len(otra.Agregados) != 1 {
		t.Errorf("el segundo agregado señaló %d renglones, quiere 1: el papel saldría con lo del agregado anterior también",
			len(otra.Agregados))
	}
}

func renglonesSinEnviar(t *testing.T, st *store.Store, orderID int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from order_lines
		 where order_id = $1 and enviado_a_cocina_at is null and cancelled_at is null`,
		orderID).Scan(&n); err != nil {
		t.Fatalf("contar renglones sin enviar: %v", err)
	}
	return n
}
