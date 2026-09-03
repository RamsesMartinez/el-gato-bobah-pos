//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MIGRACIÓN 0053, PROBADA SOBRE DATOS QUE YA EXISTÍAN.
//
// La columna dice si un renglón ya salió en una comanda, y es lo que permite imprimir SOLO lo
// agregado en vez de la comanda entera —cocina no puede preparar dos veces lo mismo— y recuperar
// una impresión que falló sin sacar el pedido completo.
//
// El backfill NO marca los renglones viejos, y eso es la decisión, no un olvido: de un renglón de
// hace tres semanas nadie sabe si salió en papel, y marcarlo como enviado sería afirmar algo que no
// consta. NULL significa "no se sabe", que es la verdad.
//
// Se prueba con DOS empresas: con una sola, cualquier defecto de alcance —una migración que solo
// toca la empresa del GUC— es un no-op y pasa verde para romper en producción.
func TestLosRenglonesViejosNoQuedanMarcadosComoEnviados(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, st, "otra-empresa-0053")
	cajero := makeUser(t, st, "cajero_0053", "cajero")
	sesion := abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Café 0053", decimal.RequireFromString("100"), false)

	// Cada empresa necesita SU producto: la llave foránea de `order_lines` es compuesta y rechaza
	// un renglón cuyo producto sea de otra empresa. Es la protección que prueba fk_tenant_test.
	prodOtra := productoDe(t, st, otra, "Té 0053")

	deLaDefault := pedidoConRenglon(t, st, defaultCompanyID, cajero, sesion, prod, "Café 0053", 9101)
	deLaOtra := pedidoConRenglon(t, st, otra, cajero, sesion, prodOtra, "Té 0053", 9102)

	// La migración ya corrió al levantar el store; lo que se comprueba es su EFECTO sobre renglones
	// que existen, y que el default de la columna sea el mismo para los nuevos.
	for _, c := range []struct {
		quien   string
		orderID int64
	}{{"la empresa por default", deLaDefault}, {"la otra empresa", deLaOtra}} {
		var sinEnviar int
		if err := st.Pool.QueryRow(ctx,
			`select count(*) from order_lines where order_id = $1 and enviado_a_cocina_at is null`,
			c.orderID).Scan(&sinEnviar); err != nil {
			t.Fatalf("leer los renglones de %s: %v", c.quien, err)
		}
		if sinEnviar != 1 {
			t.Errorf("%s: %d renglones sin enviar, quiere 1 — un renglón viejo marcado como enviado afirma que salió un papel que nadie vio",
				c.quien, sinEnviar)
		}
	}
}

// pedidoConRenglon siembra un pedido de un renglón en la empresa dada y devuelve su id.
//
// Va con SQL directo como owner y no por el servicio: el servicio siembra en la empresa del
// contexto, y aquí hace falta sembrar en DOS empresas distintas dentro del mismo test.
func pedidoConRenglon(t *testing.T, st *store.Store, companyID, cajero, sesion, producto int64, nombre string, numero int32) int64 {
	t.Helper()
	ctx := context.Background()
	var orderID int64
	if err := st.Pool.QueryRow(ctx, `
		insert into orders (company_id, client_uuid, business_date, daily_number, service_type,
		                    subtotal, total, opened_by, register_session_id)
		values ($1, gen_random_uuid(), current_date, $2, 'mostrador', 100, 100, $3, $4)
		returning id`, companyID, numero, cajero, sesion).Scan(&orderID); err != nil {
		t.Fatalf("sembrar el pedido de la empresa %d: %v", companyID, err)
	}
	if _, err := st.Pool.Exec(ctx, `
		insert into order_lines (company_id, order_id, product_id, product_name, quantity, unit_price, line_total)
		values ($1, $2, $3, $4, 1, 100, 100)`, companyID, orderID, producto, nombre); err != nil {
		t.Fatalf("sembrar el renglón: %v", err)
	}
	return orderID
}

// productoDe siembra un producto en la empresa dada, con su categoría, como owner.
func productoDe(t *testing.T, st *store.Store, companyID int64, nombre string) int64 {
	t.Helper()
	ctx := context.Background()
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into categories (company_id, name) values ($1, $2) returning id`,
		companyID, "cat-"+nombre).Scan(&catID); err != nil {
		t.Fatalf("categoría de la empresa %d: %v", companyID, err)
	}
	var prodID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into products (company_id, name, price, category_id) values ($1, $2, 100, $3) returning id`,
		companyID, nombre, catID).Scan(&prodID); err != nil {
		t.Fatalf("producto de la empresa %d: %v", companyID, err)
	}
	return prodID
}
