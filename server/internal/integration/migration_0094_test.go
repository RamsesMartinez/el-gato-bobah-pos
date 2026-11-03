//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

// wasteVersion es la migración de lo que se echó a perder (spec 039).
const wasteVersion = 94

// LA 0094 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Crea dos tablas de empresa, siembra los motivos sugeridos de cada empresa que ya existe y agrega
// columnas a tablas vivas (movimientos, renglones de conteo, configuración). Lo que solo se ve sobre
// datos reales: que la siembra es una por empresa y no se repite, que RLS y los grants quedan como
// los de las demás tablas de empresa, y que el Down no se lleva la única copia de una merma.
func TestMigrationWasteOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < wasteVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, wasteVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	var movimientos int64
	if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements`).Scan(&movimientos); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("cada empresa nace con los cinco motivos sugeridos, uno de comida del personal", func(t *testing.T) {
		rows, err := st.Pool.Query(ctx, `
			select c.id, count(r.id), count(r.id) filter (where r.is_staff_meal)
			  from companies c left join waste_reasons r on r.company_id = c.id
			 group by c.id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var co, n, staff int64
			if err := rows.Scan(&co, &n, &staff); err != nil {
				t.Fatal(err)
			}
			if n != 5 || staff != 1 {
				t.Errorf("la empresa %d tiene %d motivos (%d del personal); quiere 5 y 1", co, n, staff)
			}
		}
	})

	t.Run("no reescribe movimientos y las tablas nuevas tienen RLS y grants sin borrar", func(t *testing.T) {
		var n int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements where waste_record_id is not null`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		var total int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements`).Scan(&total); err != nil {
			t.Fatal(err)
		}
		if n != 0 || total != movimientos {
			t.Fatalf("movimientos %d → %d, %d con merma: la migración no rellena nada", movimientos, total, n)
		}
		for _, tbl := range []string{"waste_reasons", "waste_records"} {
			var rls, forced, ins, del bool
			if err := st.Pool.QueryRow(ctx, `
				select c.relrowsecurity, c.relforcerowsecurity,
				       has_table_privilege('gatobobah_app', c.oid, 'INSERT'),
				       has_table_privilege('gatobobah_app', c.oid, 'DELETE')
				  from pg_class c where c.relname = $1`, tbl).Scan(&rls, &forced, &ins, &del); err != nil {
				t.Fatal(err)
			}
			if !rls || !ins || del {
				t.Errorf("%s: RLS %v, insert %v, delete %v; quiere RLS, insert y sin delete (nada se borra)", tbl, rls, ins, del)
			}
		}
	})

	t.Run("el Down se niega en cuanto hay una merma registrada", func(t *testing.T) {
		tx, err := st.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `
			insert into waste_records (company_id, branch_id, reason_id, item_type, ingredient_id, quantity, entered_qty, status, recorded_by, is_staff_meal)
			select i.company_id, b.id, r.id, 'ingrediente', i.id, 1, 1, 'applied', u.id, false
			  from ingredients i
			  join branches b on b.company_id = i.company_id and b.is_headquarters
			  join waste_reasons r on r.company_id = i.company_id
			  join users u on u.company_id = i.company_id
			 limit 1`); err != nil {
			t.Fatalf("sembrar una merma: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = st.Pool.Exec(ctx, `delete from waste_records`) })
		db := prepararGoose(t, st.Pool)
		if err := goose.DownToContext(ctx, db, ".", wasteVersion-1); err == nil {
			t.Fatal("el Down se llevó una merma: es la única copia de qué se tiró y por qué")
		}
		if v := versionDeEsquema(t, st); v < wasteVersion {
			t.Fatalf("el Down negado dejó la base en %d", v)
		}
	})

	t.Run("sin mermas el Down deja todo como antes", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `delete from waste_records`); err != nil {
			t.Fatal(err)
		}
		migrarAbajoHasta(t, st.Pool, wasteVersion-1)
		var tbl, col bool
		if err := st.Pool.QueryRow(ctx, `
			select to_regclass('public.waste_records') is not null,
			       exists (select 1 from information_schema.columns where table_name = 'stock_movements' and column_name = 'waste_record_id')`).Scan(&tbl, &col); err != nil {
			t.Fatal(err)
		}
		if tbl || col {
			t.Fatalf("tras el Down: tabla %v, columna %v; quiere ninguna", tbl, col)
		}
	})
}

// Una empresa nueva nace con los motivos sugeridos en la misma consulta que la crea (CreateCompany),
// no en un trigger: pg_restore cargaría el respaldo y los duplicaría (0076).
func TestANewCompanyIsBornWithItsWasteReasons(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "nace-con-motivos")
	var n, staff int
	if err := st.Pool.QueryRow(context.Background(), `
		select count(*), count(*) filter (where is_staff_meal) from waste_reasons where company_id = $1`, co).Scan(&n, &staff); err != nil {
		t.Fatal(err)
	}
	if n != 5 || staff != 1 {
		t.Fatalf("la empresa nueva nació con %d motivos (%d del personal); quiere 5 y 1", n, staff)
	}
}

// Los checks de la 0094 rechazan lo que el servicio ya no manda, para que un insert directo no
// cuele un estado imposible (revisión de arquitectura del 039).
func TestWasteSchemaRejectsImpossibleRows(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	user := makeUser(t, st, "admin_checks_merma", "admin")
	ing := pieceIngredient(t, st, "Insumo de los checks")
	base := `insert into waste_records (branch_id, reason_id, item_type, ingredient_id, quantity, entered_qty, status, recorded_by, is_staff_meal, unit_cost, total_cost, covered_by_count_id)
		select (select id from branches where company_id = $1 and is_headquarters),
		       (select min(id) from waste_reasons where company_id = $1), 'ingrediente', $2, $3, 1, $4, $5, false, $6, $7, null`
	cases := []struct {
		name                string
		qty, status         string
		unitCost, totalCost any
	}{
		{"un estado que no existe", "1", "lost", nil, nil},
		{"cantidad cero", "0", "applied", nil, nil},
		{"costo a medias", "1", "applied", "10", nil},
		{"cubierta sin conteo que la cubra", "1", "covered", nil, nil},
		{"rechazada sin quién ni cuándo", "1", "rejected", nil, nil},
	}
	for _, c := range cases {
		_, err := st.Pool.Exec(ctx, base, defaultCompanyID, ing, c.qty, c.status, user, c.unitCost, c.totalCost)
		exigeViolacionDeRestriccion(t, err, c.name)
	}
	_, err := st.Pool.Exec(ctx, `insert into waste_reasons (company_id, name, description) values ($1, '  CADUCÓ ', 'repetido')`, defaultCompanyID)
	exigeViolacionDeRestriccion(t, err, "un motivo con el nombre de otro, cambiando mayúsculas y acentos")
}
