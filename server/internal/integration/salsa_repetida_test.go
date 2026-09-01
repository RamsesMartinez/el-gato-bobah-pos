//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Dos salsas del mismo sabor en una línea, de punta a punta.
//
// El pedido real que lo destapó: unas alitas con salsa de mango habanero y nada más. El grupo pide
// dos salsas, y hasta ahora no había forma de dejar dos veces la misma — el operador terminaba
// eligiendo una que el cliente no pidió.
//
// El tope por opción (`max_per_line`) ya estaba en 2 para las 64 salsas de producción; lo que
// faltaba era ejercerlo. Este test cubre que la cantidad llegue a la base y que el tope se respete.
func TestDosSalsasDelMismoSaborEnUnaLinea(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_salsas", "cajero")
	prod := makeProduct(t, st, "Alitas", decimal.RequireFromString("200"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	// Una salsa que admite dos por línea y una que no, como están hoy en producción.
	mango := opcionConTope(t, st, "Salsas alitas", "Mango habanero", decimal.RequireFromString("15"), 2)
	sinSalsa := opcionConTope(t, st, "Salsas alitas", "Sin salsa", decimal.Zero, 1)

	pedir := func(opt int64, veces int, monto string) error {
		_, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID:  uuid.New(),
			ServiceType: "mostrador",
			OpenedBy:    cajero,
			Lines: []domain.OrderLineInput{{
				ProductID: prod, Qty: decimal.RequireFromString("1"),
				Modifiers: []domain.OrderModInput{{OptionID: opt, Qty: veces}},
			}},
			Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString(monto)}},
		})
		return err
	}

	// 200 + 15×2 = 230: el cargo del extra se cobra las DOS veces, no una.
	if err := pedir(mango, 2, "230"); err != nil {
		t.Fatalf("dos del mismo sabor deben poder venderse: %v", err)
	}

	var cant int16
	var delta decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select olm.quantity, olm.price_delta from order_line_modifiers olm
		 where olm.modifier_option_id = $1 order by olm.id desc limit 1`, mango).Scan(&cant, &delta); err != nil {
		t.Fatalf("leer el extra persistido: %v", err)
	}
	if cant != 2 {
		t.Fatalf("cantidad guardada = %d, quiere 2: sin esto el ticket de cocina pide una sola", cant)
	}
	// El delta guardado es POR UNA, no por las dos: el total ya multiplicó. Si se guardara
	// multiplicado, el ticket impreso mostraría "x2 @$30" y cobraría $60.
	if !delta.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("price_delta guardado = %s, quiere 15 (por unidad, no por las dos)", delta)
	}

	// Tres ya no: el negocio dijo dos.
	if err := pedir(mango, 3, "245"); !errors.Is(err, domain.ErrOptionOverMax) {
		t.Fatalf("tres del mismo sabor debe rechazarse, fue %v", err)
	}

	// Y una que no se repite se rechaza en la segunda.
	if err := pedir(sinSalsa, 2, "200"); !errors.Is(err, domain.ErrOptionOverMax) {
		t.Fatalf("repetir una opción con tope 1 debe rechazarse, fue %v", err)
	}
}

// opcionConTope siembra una opción de modificador con su `max_per_line`. El harness general crea
// opciones con el default, y este test necesita justo las dos variantes.
func opcionConTope(t *testing.T, st *store.Store, grupo, nombre string, delta decimal.Decimal, tope int16) int64 {
	t.Helper()
	ctx := context.Background()
	var groupID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into modifier_groups (company_id, name) values ($1, $2)
		 on conflict (company_id, name) do update set name = excluded.name returning id`,
		defaultCompanyID, grupo).Scan(&groupID); err != nil {
		t.Fatalf("grupo %s: %v", grupo, err)
	}
	var id int64
	if err := st.Pool.QueryRow(ctx,
		`insert into modifier_options (company_id, group_id, name, price_delta, max_per_line)
		 values ($1, $2, $3, $4, $5) returning id`,
		defaultCompanyID, groupID, nombre, delta, tope).Scan(&id); err != nil {
		t.Fatalf("opción %s: %v", nombre, err)
	}
	return id
}
