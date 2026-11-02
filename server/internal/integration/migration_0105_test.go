//go:build integration

package integration

import (
	"context"
	"testing"
)

// densityVersion es la migración de densidad y tara (spec 058).
const densityVersion = 105

// LA 0105 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Solo agrega columnas nulas a tablas vivas (insumos, empaques, renglones de compra y de conteo) y un
// trigger en el cambio de unidad. Lo que solo se ve sobre datos reales: que ninguna fila existente
// viola los checks nuevos, que nada se reescribe al subir, y que el trigger que limpia la densidad al
// pasar a piezas no toca los insumos de la otra empresa.
func TestMigrationDensityAndTareOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < densityVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, densityVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	var ingredients, presentations, items, lines int64
	if err := st.Pool.QueryRow(ctx, `select (select count(*) from ingredients), (select count(*) from ingredient_presentations),
		(select count(*) from expense_items), (select count(*) from stock_count_lines)`).Scan(&ingredients, &presentations, &items, &lines); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("nada existente cambia y todas las columnas nuevas nacen vacías", func(t *testing.T) {
		var ing, pres, it, ln, withData int64
		if err := st.Pool.QueryRow(ctx, `select (select count(*) from ingredients), (select count(*) from ingredient_presentations),
			(select count(*) from expense_items), (select count(*) from stock_count_lines),
			(select count(*) from ingredients where density_g_per_ml is not null)
			+ (select count(*) from ingredient_presentations where tare_g is not null)
			+ (select count(*) from expense_items where density_g_per_ml is not null)
			+ (select count(*) from stock_count_lines where gross_g is not null)`).Scan(&ing, &pres, &it, &ln, &withData); err != nil {
			t.Fatal(err)
		}
		if ing != ingredients || pres != presentations || it != items || ln != lines || withData != 0 {
			t.Fatalf("insumos %d→%d, empaques %d→%d, renglones %d→%d, conteos %d→%d, con datos nuevos %d",
				ingredients, ing, presentations, pres, items, it, lines, ln, withData)
		}
	})

	gatobobah := companyBySlug(t, st, "gatobobah")
	other := companyBySlug(t, st, "bobah-pruebas")
	var user int64
	if err := st.Pool.QueryRow(ctx, `select id from users where company_id = $1 order by id limit 1`, gatobobah).Scan(&user); err != nil {
		t.Fatal(err)
	}
	syrup := ingredientIn(t, st, gatobobah, "Jarabe mig 58", "ml")
	otherSyrup := ingredientIn(t, st, other, "Jarabe mig 58", "ml")
	setDensity := func(id, company int64) error {
		_, err := st.Pool.Exec(ctx, `update ingredients set density_g_per_ml = 1.1, density_method = 'weighing',
			density_set_by = (select id from users where company_id = $2 order by id limit 1), density_set_at = now()
			where id = $1`, id, company)
		return err
	}

	t.Run("la densidad va completa o no va, y dentro de lo posible", func(t *testing.T) {
		_, err := st.Pool.Exec(ctx, `update ingredients set density_g_per_ml = 1.1 where id = $1`, syrup)
		exigeViolacionDeRestriccion(t, err, "una densidad sin quién ni cómo")
		_, err = st.Pool.Exec(ctx, `update ingredients set density_g_per_ml = 3.5, density_method = 'manual', density_set_by = $2,
			density_set_at = now() where id = $1`, syrup, user)
		exigeViolacionDeRestriccion(t, err, "una densidad de 3.5 g/ml")
		_, err = st.Pool.Exec(ctx, `update ingredients set density_g_per_ml = 1.1, density_method = 'adivinada', density_set_by = $2,
			density_set_at = now() where id = $1`, syrup, user)
		exigeViolacionDeRestriccion(t, err, "un método que no existe")
		if err := setDensity(syrup, gatobobah); err != nil {
			t.Fatalf("una densidad completa: %v", err)
		}
	})

	t.Run("quien puso la densidad es de la misma empresa", func(t *testing.T) {
		var foreign int64
		if err := st.Pool.QueryRow(ctx, `select id from users where company_id = $1 order by id limit 1`, other).Scan(&foreign); err != nil {
			t.Fatal(err)
		}
		_, err := st.Pool.Exec(ctx, `update ingredients set density_set_by = $2 where id = $1`, syrup, foreign)
		exigeViolacionDeRestriccion(t, err, "un usuario de otra empresa como autor de la densidad")
	})

	var bottle int64
	t.Run("lo que pesa vacío un empaque va mayor que cero y con tope", func(t *testing.T) {
		if err := st.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
			values ($1, $2, 'Botella', 1000, 120) returning id`, gatobobah, syrup).Scan(&bottle); err != nil {
			t.Fatalf("empaque con tara: %v", err)
		}
		_, err := st.Pool.Exec(ctx, `update ingredient_presentations set tare_g = 0 where id = $1`, bottle)
		exigeViolacionDeRestriccion(t, err, "una tara de cero")
		_, err = st.Pool.Exec(ctx, `update ingredient_presentations set tare_g = 100001 where id = $1`, bottle)
		exigeViolacionDeRestriccion(t, err, "una tara de más de 100 kg")
	})

	t.Run("un empaque de un insumo en piezas no guarda tara aunque se la manden", func(t *testing.T) {
		cups := ingredientIn(t, st, gatobobah, "Vasos mig 58", "pieza")
		var tare *string
		if err := st.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
			values ($1, $2, 'Paquete', 50, 10) returning tare_g::text`, gatobobah, cups).Scan(&tare); err != nil {
			t.Fatalf("el empaque debió guardarse sin tara, no tronar (una unión hacia piezas lo haría): %v", err)
		}
		if tare != nil {
			t.Fatalf("un empaque en piezas guardó tara %s", *tare)
		}
	})

	t.Run("bajo el rol de la app, pasar de ml a g conserva todo y a piezas lo limpia", func(t *testing.T) {
		app := restoredAppRoleStore(t)
		ice := ingredientIn(t, st, gatobobah, "Hielo mig 58", "ml")
		if err := setDensity(ice, gatobobah); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
			values ($1, $2, 'Bolsa', 5000, 30)`, gatobobah, ice); err != nil {
			t.Fatal(err)
		}
		conn := conexionDeEmpresa(t, app, gatobobah)
		var density, tare *string
		if _, err := conn.Exec(ctx, `update ingredients set base_unit_id = (select id from units where code = 'g') where id = $1`, ice); err != nil {
			t.Fatalf("ml → g como gatobobah_app: %v", err)
		}
		if err := conn.QueryRow(ctx, `select i.density_g_per_ml::text, p.tare_g::text from ingredients i
			join ingredient_presentations p on p.ingredient_id = i.id where i.id = $1`, ice).Scan(&density, &tare); err != nil {
			t.Fatal(err)
		}
		if density == nil || tare == nil {
			t.Fatalf("entre peso y líquido se perdió la densidad (%v) o la tara (%v)", density, tare)
		}
		if _, err := conn.Exec(ctx, `update ingredients set base_unit_id = (select id from units where code = 'pieza') where id = $1`, ice); err != nil {
			t.Fatalf("g → pieza como gatobobah_app (el update de los empaques dentro del trigger): %v", err)
		}
		if err := conn.QueryRow(ctx, `select i.density_g_per_ml::text, p.tare_g::text from ingredients i
			join ingredient_presentations p on p.ingredient_id = i.id where i.id = $1`, ice).Scan(&density, &tare); err != nil {
			t.Fatal(err)
		}
		if density != nil || tare != nil {
			t.Fatalf("en piezas quedó densidad %v o tara %v", density, tare)
		}
	})

	t.Run("pasar un insumo a piezas limpia su densidad y sus taras, y no toca la otra empresa", func(t *testing.T) {
		if err := setDensity(otherSyrup, other); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `delete from ingredient_presentations where id = $1`, bottle); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
			values ($1, $2, 'Botella', 1, 120)`, gatobobah, syrup); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `update ingredients set base_unit_id = (select id from units where code = 'pieza') where id = $1`, syrup); err != nil {
			t.Fatalf("cambiar a piezas a pelo: %v", err)
		}
		var density, tares, otherDensity int64
		if err := st.Pool.QueryRow(ctx, `select (select count(*) from ingredients where id = $1 and (density_g_per_ml is not null
			or density_method is not null or density_set_by is not null or density_set_at is not null)),
			(select count(*) from ingredient_presentations where ingredient_id = $1 and tare_g is not null),
			(select count(*) from ingredients where id = $2 and density_g_per_ml is not null)`, syrup, otherSyrup).Scan(&density, &tares, &otherDensity); err != nil {
			t.Fatal(err)
		}
		if density != 0 || tares != 0 {
			t.Fatalf("en piezas quedó densidad (%d) o tara (%d): el conteo y las compras la usarían", density, tares)
		}
		if otherDensity != 1 {
			t.Fatal("el trigger tocó el insumo de la otra empresa")
		}
	})

	t.Run("lo pesado en un conteo va junto", func(t *testing.T) {
		var def string
		if err := st.Pool.QueryRow(ctx, `select pg_get_constraintdef(oid) from pg_constraint
			where conname = 'stock_count_lines_gross_together'`).Scan(&def); err != nil {
			t.Fatalf("falta la regla que guarda bruto, empaques y tara juntos: %v", err)
		}
	})

	// Cada rama de la guarda se ejerce sola: un error en una (el join de la compra entre peso y
	// líquido) dejaría pasar un rollback que pierde ese dato.
	cleanDown := func(t *testing.T) {
		t.Helper()
		clearWhatTheDownGuards(t, st.Pool, densityVersion)
		if _, err := st.Pool.Exec(ctx, `update expense_items set voided_at = coalesce(voided_at, now()) where description = 'mig 58'`); err != nil {
			t.Fatal(err)
		}
	}
	cleanDown(t)
	var otherUser int64
	if err := st.Pool.QueryRow(ctx, `select id from users where company_id = $1 order by id limit 1`, other).Scan(&otherUser); err != nil {
		t.Fatal(err)
	}
	purchaseIn(t, st, other, otherUser, otherSyrup)
	branches := map[string]string{
		"una densidad": `update ingredients set density_g_per_ml = 1.2, density_method = 'manual', density_set_at = now(),
			density_set_by = (select id from users where company_id = ingredients.company_id order by id limit 1) where id = ` + itoa(int(otherSyrup)),
		"una tara": `update ingredient_presentations set tare_g = 5 where id = (select p.id from ingredient_presentations p
			join ingredients i on i.id = p.ingredient_id join units u on u.id = i.base_unit_id where u.kind in ('masa','volumen') limit 1)`,
		"una densidad fija en una compra": `update expense_items set density_g_per_ml = 1.1 where id = (select max(id) from expense_items)`,
		"una compra en kg de algo en ml": `insert into expense_items (company_id, expense_id, item_type, ingredient_id, description, quantity, unit_id, amount)
			select company_id, expense_id, item_type, ingredient_id, 'mig 58', 1, (select id from units where code = 'kg'), 10
			  from expense_items where id = (select max(id) from expense_items where ingredient_id = ` + itoa(int(otherSyrup)) + `)`,
	}
	for name, seed := range branches {
		t.Run("el Down se niega con "+name, func(t *testing.T) {
			if _, err := st.Pool.Exec(ctx, seed); err != nil {
				t.Fatalf("sembrar %s: %v", name, err)
			}
			if err := migrarAbajoHastaErr(t, st.Pool, densityVersion-1); err == nil {
				t.Fatalf("el Down corrió con %s: lo perdería en silencio", name)
			}
			cleanDown(t)
		})
	}

	t.Run("sin nada de eso el Down baja", func(t *testing.T) {
		cleanDown(t)
		if err := migrarAbajoHastaErr(t, st.Pool, densityVersion-1); err != nil {
			t.Fatalf("sin densidades ni taras el Down debió bajar: %v", err)
		}
		var cols int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns
			where (table_name, column_name) in (('ingredients', 'density_g_per_ml'), ('ingredient_presentations', 'tare_g'),
			                                   ('expense_items', 'density_g_per_ml'), ('stock_count_lines', 'gross_g'))`).Scan(&cols); err != nil {
			t.Fatal(err)
		}
		if cols != 0 {
			t.Fatalf("el Down dejó %d columnas", cols)
		}
		migrarArriba(t, st.Pool)
	})
}
