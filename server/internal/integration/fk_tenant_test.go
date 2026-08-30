//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// esViolacionDeLlave exige que el rechazo venga de la LLAVE FORÁNEA (23503) y no de cualquier otra
// cosa. No es rigor de más: dos de estos tests pasaban en verde sin la migración aplicada, uno
// porque el insert chocaba con un check de `orders` y otro porque tenía mal el nombre de una
// columna. Un test que solo pide "que falle" no distingue la protección que busca del typo que
// escribió quien lo puso.
func esViolacionDeLlave(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// Las tablas que mueven dinero no deben poder apuntar al catálogo de otra empresa, y el que lo
// impide tiene que ser el ESQUEMA, no una validación de servicio que el próximo endpoint puede
// olvidar.
//
// Todo se inserta como OWNER a propósito: el owner salta RLS, igual que lo saltan los chequeos de
// integridad referencial de Postgres. Ese es justo el hueco — una escritura administrativa (un
// data-fix, un backfill de migración) cruzaba las empresas sin una sola protesta, y el síntoma
// aparecía en el corte de caja y no en el insert.
func TestElEsquemaRechazaRenglonesDeVentaQueCruzanEmpresas(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-fk-venta")
	cajero := makeUser(t, owner, "cajero_fk_venta", "cajero")
	sess := abrirCajaPrincipal(t, owner, cajero)
	// El producto es de la empresa por default; el pedido, de `otra`.
	prod := makeProduct(t, owner, "Hamburguesa", decimal.RequireFromString("150.00"), false)

	var orderID int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into orders (company_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id)
		 values ($1, gen_random_uuid(), current_date, 9001, 'mostrador', 150, 150, $2, $3) returning id`,
		otra, cajero, sess).Scan(&orderID); err != nil {
		t.Fatalf("sembrar el pedido de la otra empresa: %v", err)
	}

	if _, err := owner.Pool.Exec(ctx,
		`insert into order_lines (company_id, order_id, product_id, product_name, quantity, unit_price, line_total)
		 values ($1, $2, $3, 'Hamburguesa', 1, 150, 150)`, otra, orderID, prod); !esViolacionDeLlave(err) {
		t.Fatalf("el esquema debe rechazar un renglón de venta cuyo producto es de otra empresa, fue: %v", err)
	}
}

// La otra mitad del total de un ticket: los extras. Un delta cobrado con la opción de otra empresa
// suma dinero que ningún reporte de esa empresa puede explicar.
func TestElEsquemaRechazaExtrasQueCruzanEmpresas(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-fk-extra")
	cajero := makeUser(t, owner, "cajero_fk_extra", "cajero")
	sess := abrirCajaPrincipal(t, owner, cajero)
	prod := productoDeEmpresa(t, owner, otra, "Papas otra")
	opcion := optionID(t, owner, defaultCompanyID) // opción de la empresa por default

	var orderID int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into orders (company_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id)
		 values ($1, gen_random_uuid(), current_date, 9002, 'mostrador', 100, 100, $2, $3) returning id`,
		otra, cajero, sess).Scan(&orderID); err != nil {
		t.Fatalf("sembrar el pedido: %v", err)
	}
	var lineID int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into order_lines (company_id, order_id, product_id, product_name, quantity, unit_price, line_total)
		 values ($1, $2, $3, 'Papas otra', 1, 100, 100) returning id`,
		otra, orderID, prod).Scan(&lineID); err != nil {
		t.Fatalf("sembrar el renglón: %v", err)
	}

	if _, err := owner.Pool.Exec(ctx,
		`insert into order_line_modifiers (company_id, order_line_id, modifier_option_id, group_title, option_name, quantity, price_delta)
		 values ($1, $2, $3, 'Extras', 'Extra', 1, 20)`, otra, lineID, opcion); !esViolacionDeLlave(err) {
		t.Fatalf("el esquema debe rechazar un extra cuya opción es de otra empresa, fue: %v", err)
	}
}

// Y la plataforma del pedido: es la columna con la que el corte separa el dinero de Uber del de
// mostrador. Un pedido apuntando a la plataforma de otra empresa lo manda al subtotal equivocado.
func TestElEsquemaRechazaUnPedidoConPlataformaAjena(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-fk-plataforma")
	cajero := makeUser(t, owner, "cajero_fk_plat", "cajero")
	sess := abrirCajaPrincipal(t, owner, cajero)
	plataforma := platformID(t, owner, defaultCompanyID, "Uber Eats")

	if _, err := owner.Pool.Exec(ctx,
		// service_type domicilio: `orders_check` exige que una plataforma solo aparezca en un
		// pedido a domicilio. Con 'mostrador' el insert rebotaba por ESE check y el test pasaba sin
		// probar nada de la llave.
		`insert into orders (company_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id, delivery_platform_id)
		 values ($1, gen_random_uuid(), current_date, 9003, 'domicilio', 100, 100, $2, $3, $4)`,
		otra, cajero, sess, plataforma); !esViolacionDeLlave(err) {
		t.Fatalf("el esquema debe rechazar un pedido con la plataforma de otra empresa, fue: %v", err)
	}
}

// Un pedido SIN plataforma (mostrador) sigue entrando: la llave compuesta usa MATCH SIMPLE, así que
// una columna nula no se valida. Si esto se rompiera, dejaría de poderse vender en mostrador — el
// caso más común de todos.
func TestUnPedidoDeMostradorSigueEntrandoSinPlataforma(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, owner, "cajero_mostrador", "cajero")
	sess := abrirCajaPrincipal(t, owner, cajero)

	if _, err := owner.Pool.Exec(ctx,
		`insert into orders (company_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id)
		 values ($1, gen_random_uuid(), current_date, 9004, 'mostrador', 100, 100, $2, $3)`,
		defaultCompanyID, cajero, sess); err != nil {
		t.Fatalf("un pedido de mostrador no debe verse afectado: %v", err)
	}
}

// El movimiento de inventario es la otra cara del mismo dinero: alimenta el costo de venta.
func TestElEsquemaRechazaMovimientosDeStockQueCruzanEmpresas(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-fk-stock")
	usuario := makeUser(t, owner, "gerente_fk_stock", "gerente")
	prod := makeProduct(t, owner, "Refresco", decimal.RequireFromString("30.00"), true)

	if _, err := owner.Pool.Exec(ctx,
		`insert into stock_movements (company_id, item_type, product_id, movement_type, quantity, user_id)
		 values ($1, 'producto', $2, 'ajuste', 5, $3)`, otra, prod, usuario); !esViolacionDeLlave(err) {
		t.Fatalf("el esquema debe rechazar un movimiento de stock sobre el producto de otra empresa, fue: %v", err)
	}
}
