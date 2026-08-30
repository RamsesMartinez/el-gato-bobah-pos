//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Las tablas nuevas tienen que ser usables POR EL ROL DE APP, no solo por el owner. El grant de
// 0024 fue `on all tables in schema public`, que es puntual: no hay default privileges, así que una
// tabla creada después nace sin permisos para gatobobah_app. Ese fallo es invisible en dev —la API
// local sirve como owner— y en producción devuelve 42501 en el primer request.
func TestPreciosDePlataformaSonUsablesPorElRolDeApp(t *testing.T) {
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	cajero := makeUser(t, owner, "cajero_precios", "cajero")
	prod := makeProduct(t, owner, "Boneless", decimal.RequireFromString("434.98"), false)
	plataforma := platformID(t, owner, defaultCompanyID, "Uber Eats")

	// Todo va por las Queries que entrega WithTenant: son las atadas a la conexión que lleva el
	// GUC. Consultar por st.Pool tomaría OTRA conexión del pool, sin contexto de empresa, y RLS
	// devolvería 0 filas — un falso negativo que parece un fallo de aislamiento.
	if err := appSt.WithTenant(ctx, defaultCompanyID, func(q *db.Queries) error {
		return q.UpsertProductPlatformPrice(ctx, db.UpsertProductPlatformPriceParams{
			ProductID:  prod,
			PlatformID: plataforma,
			Price:      decimal.RequireFromString("587.22"),
			UpdatedBy:  cajero,
		})
	}); err != nil {
		t.Fatalf("insert bajo el rol de app: %v (¿falta el grant en la migración?)", err)
	}

	var n int64
	if err := appSt.WithTenant(ctx, defaultCompanyID, func(q *db.Queries) error {
		var e error
		n, e = q.CountProductPlatformPrices(ctx)
		return e
	}); err != nil {
		t.Fatalf("select bajo el rol de app: %v (¿falta el grant?)", err)
	}
	if n != 1 {
		t.Fatalf("el rol de app debe ver su propia fila, vio %d", n)
	}

	// La otra tabla del par: el grant se otorga tabla por tabla, así que una puede tenerlo y la
	// otra no.
	if err := appSt.WithTenant(ctx, defaultCompanyID, func(q *db.Queries) error {
		_, e := q.GetOptionPlatformPrices(ctx, plataforma)
		return e
	}); err != nil {
		t.Fatalf("select sobre modifier_option_platform_prices: %v (¿falta el grant?)", err)
	}
}

// Un precio de plataforma de una empresa no debe verse ni escribirse desde la otra. Es lo que
// impide que el POS cobre con la lista de precios ajena.
func TestPreciosDePlataformaNoCruzanEntreEmpresas(t *testing.T) {
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-precios")
	cajero := makeUser(t, owner, "cajero_iso", "cajero")
	prod := makeProduct(t, owner, "Alitas", decimal.RequireFromString("398.98"), false)
	plataforma := platformID(t, owner, defaultCompanyID, "Didi")

	if _, err := owner.Pool.Exec(ctx,
		`insert into product_platform_prices (product_id, platform_id, price, updated_by, company_id)
		 values ($1, $2, $3, $4, $5)`, prod, plataforma, "538.62", cajero, defaultCompanyID); err != nil {
		t.Fatalf("sembrar el precio: %v", err)
	}

	// Lectura desde la otra empresa: no debe ver nada.
	var n int64
	if err := appSt.WithTenant(ctx, otra, func(q *db.Queries) error {
		var e error
		n, e = q.CountProductPlatformPrices(ctx)
		return e
	}); err != nil {
		t.Fatalf("select desde la otra empresa: %v", err)
	}
	if n != 0 {
		t.Fatalf("la otra empresa no debe ver el precio, vio %d", n)
	}

	// Y la plataforma tampoco: delivery_platforms es per-tenant, así que resolver el id de una
	// plataforma ajena bajo el tenant equivocado no devuelve fila.
	if err := appSt.WithTenant(ctx, otra, func(q *db.Queries) error {
		_, e := q.GetPlatformByID(ctx, plataforma)
		return e
	}); err == nil {
		t.Fatal("la otra empresa no debe poder resolver una plataforma ajena")
	}
}

// Cada empresa tiene sus PROPIOS métodos de pago, y cada método de plataforma apunta a la
// plataforma de su misma empresa. Es lo que permite saber qué métodos son de Uber sin compararlos
// por nombre, y lo que impide que el corte agrupe dinero de la empresa equivocada.
func TestMetodosDePagoSonPorEmpresaYApuntanASuPlataforma(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()
	otra := makeCompany(t, owner, "otra-metodos")

	// La migración copia los métodos por empresa, pero una empresa creada DESPUÉS los recibe de
	// provisionCompany. Aquí se crea por SQL crudo, así que solo se verifica lo que la migración
	// dejó: que las empresas existentes tengan los suyos y que no se crucen.
	_ = otra

	var cruzados int
	if err := owner.Pool.QueryRow(ctx, `
		select count(*) from payment_methods pm
		join delivery_platforms dp on dp.id = pm.delivery_platform_id
		where dp.company_id <> pm.company_id`).Scan(&cruzados); err != nil {
		t.Fatal(err)
	}
	if cruzados != 0 {
		t.Fatalf("%d métodos apuntan a la plataforma de otra empresa", cruzados)
	}

	// Los seis de plataforma: tres en línea que no tocan el cajón y se autodeclaran, y tres de
	// efectivo que sí lo tocan y exigen conteo. El efectivo del repartidor es dinero real: sin el
	// affects_cash_drawer, aparece como sobrante inexplicable al cerrar el turno.
	rows, err := owner.Pool.Query(ctx, `
		select name, affects_cash_drawer, auto_declare from payment_methods
		where company_id = $1 and kind = 'plataforma' order by sort_key`, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var enLinea, enEfectivo int
	for rows.Next() {
		var name string
		var cajon, auto bool
		if err := rows.Scan(&name, &cajon, &auto); err != nil {
			t.Fatal(err)
		}
		switch {
		case !cajon && auto:
			enLinea++
		case cajon && !auto:
			enEfectivo++
		default:
			t.Fatalf("%q quedó con una combinación imposible: cajón=%v autodeclara=%v", name, cajon, auto)
		}
	}
	if enLinea != 3 || enEfectivo != 3 {
		t.Fatalf("se esperaban 3 métodos en línea y 3 de efectivo, hubo %d y %d", enLinea, enEfectivo)
	}
}

// Un pago no puede quedar apuntando al método de pago de otra empresa. Es el fallo más caro del
// modelo: la llave foránea salta RLS, así que el id ajeno entra; y como el corte hace join con
// payment_methods bajo RLS, ese pago DESAPARECE del corte y de los reportes, dejando un faltante
// por el monto exacto que nadie sabe explicar.
func TestNingunPagoApuntaAlMetodoDeOtraEmpresa(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	var cruzados int
	if err := owner.Pool.QueryRow(ctx, `
		select count(*) from (
			select 1 from order_payments x join payment_methods m on m.id = x.payment_method_id
				where m.company_id <> x.company_id
			union all
			select 1 from expense_payments x join payment_methods m on m.id = x.payment_method_id
				where m.company_id <> x.company_id
			union all
			select 1 from register_session_totals x join payment_methods m on m.id = x.payment_method_id
				where m.company_id <> x.company_id
		) z`).Scan(&cruzados); err != nil {
		t.Fatal(err)
	}
	if cruzados != 0 {
		t.Fatalf("%d filas apuntan al método de pago de otra empresa", cruzados)
	}
}
