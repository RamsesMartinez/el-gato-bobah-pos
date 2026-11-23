//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

const costReasonVersion = 116

// LA 0116 (motivo de una corrección de costo, spec 090) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Lo
// que se prueba: la corrección de la API anterior (sin motivo) sigue entrando, la nueva guarda su
// motivo, el check rechaza un motivo vacío o sin recortar, RLS sigue separando empresas, y el Down
// quita la columna sin negarse.
func TestMigration0116OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, costReasonVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	type art struct{ company, ingredient, user int64 }
	var arts []art
	rows, err := st.Pool.Query(ctx, `
		select distinct on (i.company_id) i.company_id, i.id,
		       (select min(u.id) from users u where u.company_id = i.company_id)
		  from ingredients i
		 where i.is_active and exists (select 1 from users u where u.company_id = i.company_id)
		 order by i.company_id, i.id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a art
		if err := rows.Scan(&a.company, &a.ingredient, &a.user); err != nil {
			t.Fatal(err)
		}
		arts = append(arts, a)
	}
	rows.Close()
	if len(arts) < 2 {
		t.Fatalf("el respaldo trae insumos de %d empresas; la prueba necesita dos", len(arts))
	}
	before := func() int64 {
		var n int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from ingredient_cost_corrections`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	n0 := before()

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", costReasonVersion); err != nil {
		t.Fatal(err)
	}
	if before() != n0 {
		t.Fatal("migrar cambió cuántas correcciones hay")
	}
	var nulls int64
	if err := st.Pool.QueryRow(ctx, `select count(*) from ingredient_cost_corrections where reason is not null`).Scan(&nulls); err != nil || nulls != 0 {
		t.Fatalf("correcciones viejas con motivo inventado: %d %v", nulls, err)
	}

	a, b := arts[0], arts[1]
	app := restoredAppRoleStore(t)
	insert := func(t *testing.T, x art, reason any) error {
		t.Helper()
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, itoa(int(x.company))); err != nil {
			t.Fatal(err)
		}
		if reason == nil {
			// Así inserta la API anterior: sin la columna.
			_, err = tx.Exec(ctx, `insert into ingredient_cost_corrections (ingredient_id, previous_cost, new_cost, user_id)
				values ($1, 1, 2, $2)`, x.ingredient, x.user)
		} else {
			_, err = tx.Exec(ctx, `insert into ingredient_cost_corrections (ingredient_id, previous_cost, new_cost, user_id, reason)
				values ($1, 1, 2, $2, $3)`, x.ingredient, x.user, reason)
		}
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	t.Run("la API anterior sigue corrigiendo sin motivo", func(t *testing.T) {
		if err := insert(t, a, nil); err != nil {
			t.Fatalf("insert de la versión anterior: %v", err)
		}
	})
	t.Run("la nueva guarda su motivo", func(t *testing.T) {
		if err := insert(t, b, "difiere de la última compra"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("motivo vacío, de solo espacios, sin recortar o de más de 200: rechazado", func(t *testing.T) {
		for _, bad := range []string{"", "   ", " con espacios ", strings.Repeat("x", 201)} {
			if err := insert(t, a, bad); err == nil {
				t.Errorf("%q entró", bad)
			}
		}
	})
	t.Run("una empresa no ve los motivos de la otra", func(t *testing.T) {
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, itoa(int(a.company))); err != nil {
			t.Fatal(err)
		}
		var n int64
		if err := tx.QueryRow(ctx, `select count(*) from ingredient_cost_corrections where reason = 'difiere de la última compra'`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("la empresa A ve %d motivos de la B (%v)", n, err)
		}
	})
	t.Run("el Down quita la columna sin negarse", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, costReasonVersion-1)
		var exists bool
		if err := st.Pool.QueryRow(ctx, `select exists (select 1 from information_schema.columns
			where table_name = 'ingredient_cost_corrections' and column_name = 'reason')`).Scan(&exists); err != nil || exists {
			t.Fatalf("la columna sigue tras el Down (%v)", err)
		}
	})
}
