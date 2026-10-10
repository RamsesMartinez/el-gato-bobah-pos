//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// SUCURSALES (spec 025, migración 0076).
//
// Lo que vigila este archivo y un unitario no puede ver: que la sucursal se resuelva bajo RLS en
// los tres casos donde RLS ya falló, que la llave compuesta impida ligar algo a una sucursal de
// otra empresa (los chequeos de FK saltan RLS), y que con dos sucursales nada escoja una en
// silencio.

func headquartersOf(t *testing.T, st *store.Store, company int64) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from branches where company_id = $1 and is_headquarters`, company).Scan(&id); err != nil {
		t.Fatalf("matriz de la empresa %d: %v", company, err)
	}
	return id
}

// addBranch da de alta una segunda sucursal como owner, con su caja principal.
func addBranch(t *testing.T, st *store.Store, company int64, code string) (branch, register int64) {
	t.Helper()
	ctx := context.Background()
	if err := st.Pool.QueryRow(ctx,
		`insert into branches (company_id, code, name) values ($1, $2::text, $2::text) returning id`, company, code).Scan(&branch); err != nil {
		t.Fatalf("alta de la sucursal %s: %v", code, err)
	}
	if err := st.Pool.QueryRow(ctx,
		`insert into cash_registers (company_id, branch_id, name, is_primary) values ($1, $2, $3, true) returning id`,
		company, branch, "Caja "+code).Scan(&register); err != nil {
		t.Fatalf("caja principal de %s: %v", code, err)
	}
	return branch, register
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestANewCompanyIsBornWithItsHeadquarters(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	company := makeCompany(t, st, "nueva-sucursal")

	var n, number int
	var code string
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*), min(branch_number), min(code::text) from branches where company_id = $1 and is_headquarters`,
		company).Scan(&n, &number, &code); err != nil {
		t.Fatal(err)
	}
	if n != 1 || number != 1 || code != "NUEVASUCUR" {
		t.Fatalf("la empresa nueva debe nacer con una matriz número 1 y código NUEVASUCUR; tiene %d matriz(ces), número %d, código %q", n, number, code)
	}
}

func TestBranchRulesHoldForTheAppRole(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	hq := headquartersOf(t, owner, defaultCompanyID)
	appConn := conexionDeEmpresa(t, appRoleStore(t), defaultCompanyID)

	if _, err := appConn.Exec(context.Background(), `update branches set code = 'OTRO' where id = $1`, hq); pgCode(err) != "23514" {
		t.Fatalf("el código de una sucursal no debe poder cambiar (va impreso en tickets), fue: %v", err)
	}
	if _, err := appConn.Exec(context.Background(), `update branches set is_active = false where id = $1`, hq); pgCode(err) != "23514" {
		t.Fatalf("no se debe poder desactivar la única sucursal activa, fue: %v", err)
	}
	if _, err := appConn.Exec(context.Background(), `delete from branches where id = $1`, hq); pgCode(err) != "42501" {
		t.Fatalf("una sucursal con historia se desactiva, no se borra: el rol de la app no debe tener delete, fue: %v", err)
	}
}

// current_branch_id() en los tres casos. Nunca debe devolver la sucursal de la dueña desde otra
// sesión, y sin empresa debe dar el error de negocio (EGB01), no un 22P02 por el cast de ”.
func TestCurrentBranchInTheThreeCases(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	other := makeCompany(t, owner, "otra-sucursal")
	ownerHQ := headquartersOf(t, owner, defaultCompanyID)
	otherHQ := headquartersOf(t, owner, other)

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		got, err := st.QC(ctx).CurrentBranch(ctx)
		if err != nil {
			// store traduce EGB01 a ErrBranchAmbiguous; un 22P02 por el cast de '' no se traduce y
			// cae aquí.
			if !errors.Is(err, domain.ErrBranchAmbiguous) {
				t.Fatalf("sin empresa la sucursal debe fallar con el error de negocio, fue: %v", err)
			}
			return
		}
		if got == ownerHQ {
			t.Fatalf("otra sesión obtuvo la sucursal de la empresa dueña (%d)", ownerHQ)
		}
		if got != otherHQ {
			t.Fatalf("la sesión de la otra empresa debe obtener su propia matriz %d, obtuvo %d", otherHQ, got)
		}
	})

}

func TestTwoBranchesWithoutSelectorAreRejectedNotGuessed(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	addBranch(t, owner, defaultCompanyID, "NORTE")

	appConn := conexionDeEmpresa(t, appRoleStore(t), defaultCompanyID)
	_, err := appConn.Exec(context.Background(), `select current_branch_id()`)
	if pgCode(err) != "EGB01" {
		t.Fatalf("con dos sucursales y sin selector, la sucursal no se adivina: quería EGB01, fue %v", err)
	}
}

// Las FK compuestas: un branch_id de OTRA empresa se rechaza en las seis tablas aunque lo escriba
// el owner, que salta RLS igual que los chequeos de integridad de Postgres.
func TestABranchOfAnotherCompanyIsRejected(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	other := makeCompany(t, owner, "ajena-sucursal")
	foreign := headquartersOf(t, owner, other)
	user := makeUser(t, owner, "gerente_sucursal", "gerente")
	product := makeProduct(t, owner, "Té", decimal.RequireFromString("50.00"), true)
	platform := platformID(t, owner, defaultCompanyID, "Uber Eats")

	cases := []struct {
		table string
		sql   string
		args  []any
	}{
		{"cash_registers", `insert into cash_registers (company_id, branch_id, name) values ($1, $2, 'Ajena')`,
			[]any{defaultCompanyID, foreign}},
		{"platform_connections", `insert into platform_connections (company_id, branch_id, delivery_platform_id, external_store_id, label)
			values ($1, $2, $3, 'tienda-ajena', 'Ajena')`, []any{defaultCompanyID, foreign, platform}},
		{"orders", `insert into orders (company_id, branch_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by)
			values ($1, $2, gen_random_uuid(), current_date, 9100, 'mostrador', 10, 10, $3)`, []any{defaultCompanyID, foreign, user}},
		{"stock_movements", `insert into stock_movements (company_id, branch_id, item_type, product_id, movement_type, quantity, user_id)
			values ($1, $2, 'producto', $3, 'ajuste', 1, $4)`, []any{defaultCompanyID, foreign, product, user}},
		{"expenses", `insert into expenses (company_id, branch_id, category_id, amount, expense_date, created_by)
			values ($1, $2, (select id from expense_categories where company_id = $1 limit 1), 10, current_date, $3)`,
			[]any{defaultCompanyID, foreign, user}},
	}
	for _, c := range cases {
		t.Run(c.table, func(t *testing.T) {
			if _, err := owner.Pool.Exec(ctx, c.sql, c.args...); !esViolacionDeLlave(err) {
				t.Fatalf("%s aceptó la sucursal de otra empresa: %v", c.table, err)
			}
		})
	}

	t.Run("un gasto sin sucursal es de toda la empresa", func(t *testing.T) {
		if _, err := owner.Pool.Exec(ctx,
			`insert into expenses (company_id, category_id, amount, expense_date, created_by)
			 values ($1, (select id from expense_categories where company_id = $1 limit 1), 10, current_date, $2)`,
			defaultCompanyID, user); err != nil {
			t.Fatalf("un gasto sin sucursal debe aceptarse: %v", err)
		}
	})
}

// Una escritura como owner para OTRA empresa (pruebas, scripts de datos) sin branch_id cae en la
// sucursal de ESA empresa, no en la del ajuste de sesión: si no, la llave compuesta la rechazaría.
func TestAnOwnerInsertForAnotherCompanyFallsInItsOwnBranch(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	other := makeCompany(t, owner, "otra-owner")
	var branch int64
	if err := owner.Pool.QueryRow(context.Background(),
		`insert into cash_registers (company_id, name) values ($1, 'Caja secundaria') returning branch_id`, other).Scan(&branch); err != nil {
		t.Fatalf("insert como owner para otra empresa: %v", err)
	}
	if want := headquartersOf(t, owner, other); branch != want {
		t.Fatalf("la caja de la otra empresa debe caer en su matriz %d, cayó en %d", want, branch)
	}
}

func TestEachBranchOpensItsOwnPrimaryRegister(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, owner, "cajero_sucursales", "cajero")
	abrirCajaPrincipal(t, owner, cashier)
	_, northRegister := addBranch(t, owner, defaultCompanyID, "NORTE")

	if _, err := owner.Pool.Exec(ctx,
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id) values ($1, 0, $2, $3)`,
		fixedNow, cashier, northRegister); err != nil {
		t.Fatalf("la caja principal de la segunda sucursal debe poder abrir con la de la matriz abierta: %v", err)
	}

	var northBranch int64
	if err := owner.Pool.QueryRow(ctx, `select branch_id from cash_registers where id = $1`, northRegister).Scan(&northBranch); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx,
		`insert into cash_registers (company_id, branch_id, name, is_primary) values ($1, $2, 'Otra principal', true)`,
		defaultCompanyID, northBranch); pgCode(err) != "23505" {
		t.Fatalf("dos cajas principales en la misma sucursal deben rechazarse, fue: %v", err)
	}
}

// El pedido queda en la sucursal de la caja de su turno, y el inventario que descuenta es el de
// esa sucursal.
func TestAnOrderAndItsStockStayInTheBranchOfItsRegister(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	cashier := makeUser(t, owner, "cajero_norte", "cajero")
	product := makeProduct(t, owner, "Galleta", decimal.RequireFromString("20.00"), true)
	northBranch, northRegister := addBranch(t, owner, defaultCompanyID, "NORTE")

	var session int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id) values ($1, 0, $2, $3) returning id`,
		fixedNow, cashier, northRegister).Scan(&session); err != nil {
		t.Fatal(err)
	}
	var orderBranch int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into orders (client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by, register_session_id)
		 values (gen_random_uuid(), current_date, 1, 'mostrador', 20, 20, $1, $2) returning branch_id`,
		cashier, session).Scan(&orderBranch); err != nil {
		t.Fatalf("pedido en la segunda sucursal: %v", err)
	}
	if orderBranch != northBranch {
		t.Fatalf("el pedido debe quedar en la sucursal de la caja de su turno (%d), quedó en %d", northBranch, orderBranch)
	}

	hq := headquartersOf(t, owner, defaultCompanyID)
	for _, branch := range []int64{hq, northBranch} {
		if _, err := owner.Pool.Exec(ctx,
			`insert into stock_movements (branch_id, item_type, product_id, movement_type, quantity, user_id)
			 values ($1, 'producto', $2, 'ajuste', 5, $3)`, branch, product, cashier); err != nil {
			t.Fatalf("movimiento en la sucursal %d: %v", branch, err)
		}
	}
	var levels int
	if err := owner.Pool.QueryRow(ctx,
		`select count(*) from stock_levels where product_id = $1`, product).Scan(&levels); err != nil {
		t.Fatal(err)
	}
	if levels != 2 {
		t.Fatalf("cada sucursal lleva sus propias existencias: quería 2 filas, hay %d", levels)
	}
}

// Las consultas que cambiaron de forma con la 0076, bajo el rol de la app y en los tres casos:
// ninguna alcanza lo de la empresa dueña desde otra sesión.
func TestBranchScopedQueriesInTheThreeCases(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	other := makeCompany(t, owner, "otra-inventario")
	cashier := makeUser(t, owner, "cajero_inv", "cajero")
	product := makeProduct(t, owner, "Pan", decimal.RequireFromString("15.00"), true)
	if _, err := owner.Pool.Exec(context.Background(),
		`insert into stock_movements (item_type, product_id, movement_type, quantity, user_id) values ('producto', $1, 'ajuste', 7, $2)`,
		product, cashier); err != nil {
		t.Fatal(err)
	}
	abrirCajaPrincipal(t, owner, cashier)

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		q := st.QC(ctx)
		if levels, err := q.ListStockLevels(ctx); err == nil {
			for _, l := range levels {
				if l.ItemName == "Pan" {
					t.Fatalf("otra sesión ve las existencias de la empresa dueña")
				}
			}
		} else if !errors.Is(err, domain.ErrBranchAmbiguous) {
			t.Fatalf("ListStockLevels: %v", err)
		}
		if _, err := q.GetOpenPrimarySession(ctx); err == nil {
			t.Fatalf("otra sesión encontró el turno abierto de la empresa dueña")
		}
	})
}
