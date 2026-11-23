//go:build integration

package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

const catalogMediaVersion = 117

// LA 0117 (fotos del catálogo, spec 091) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Solo agrega: un
// tipo, una columna, dos checks `not valid` y una tabla. Lo que se prueba es que una image_url vieja
// que no tiene la forma nueva no impide migrar, que la API anterior sigue escribiendo productos y
// categorías, que los checks valen para lo nuevo, y que el Down se niega con borrados pendientes.
func TestMigration0117OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, catalogMediaVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	type ent struct{ company, product, category int64 }
	var ents []ent
	rows, err := st.Pool.Query(ctx, `
		select distinct on (p.company_id) p.company_id, p.id, p.category_id
		  from products p order by p.company_id, p.id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var e ent
		if err := rows.Scan(&e.company, &e.product, &e.category); err != nil {
			t.Fatal(err)
		}
		ents = append(ents, e)
	}
	rows.Close()
	if len(ents) < 2 {
		t.Fatalf("el respaldo trae productos de %d empresas; la prueba necesita dos", len(ents))
	}
	a, b := ents[0], ents[1]

	// Una URL vieja escrita a mano, sin la forma nueva: la migración no puede tropezar con ella.
	const old = "https://otro-lado.example.com/foto.jpg"
	if _, err := st.Pool.Exec(ctx, `update products set image_url = $1 where id = $2`, old, a.product); err != nil {
		t.Fatal(err)
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", catalogMediaVersion); err != nil {
		t.Fatalf("una image_url vieja impidió migrar (la API nueva no arrancaría): %v", err)
	}

	t.Run("la URL vieja sigue como estaba", func(t *testing.T) {
		var got string
		if err := st.Pool.QueryRow(ctx, `select image_url from products where id = $1`, a.product).Scan(&got); err != nil || got != old {
			t.Fatalf("image_url = %q %v", got, err)
		}
	})

	t.Run("una fila con URL vieja se sigue editando", func(t *testing.T) {
		// Un check `not valid` se evalúa en CADA update de la fila, aunque no toque image_url: el
		// producto con la URL vieja quedaría sin poder cambiar su precio ni su nombre.
		app := restoredAppRoleStore(t)
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, strconv.FormatInt(a.company, 10)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `update products set is_favorite = not is_favorite where id = $1`, a.product); err != nil {
			t.Fatalf("editar un producto con una image_url vieja falló: %v", err)
		}
	})

	t.Run("la API anterior sigue escribiendo productos y categorías", func(t *testing.T) {
		app := restoredAppRoleStore(t)
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, strconv.FormatInt(b.company, 10)); err != nil {
			t.Fatal(err)
		}
		// Lo que la API anterior manda: ni familia ni foto.
		if _, err := tx.Exec(ctx, `update products set is_favorite = not is_favorite where id = $1`, b.product); err != nil {
			t.Fatalf("update de producto de la API anterior: %v", err)
		}
		if _, err := tx.Exec(ctx, `update categories set sort_key = sort_key where id = $1`, b.category); err != nil {
			t.Fatalf("update de categoría de la API anterior: %v", err)
		}
	})

	t.Run("los checks valen para lo que se escriba de aquí en adelante", func(t *testing.T) {
		ok := []string{
			"https://media.example.com/2/products/15/0123456789abcdef0123456789abcdef-1200.webp",
			"http://localhost:9391/media/2/products/15/0123456789abcdef0123456789abcdef-1200.webp",
		}
		for _, u := range ok {
			if _, err := st.Pool.Exec(ctx, `update products set image_url = $1 where id = $2`, u, b.product); err != nil {
				t.Errorf("rechazó %q: %v", u, err)
			}
		}
		for _, u := range []string{"https://x.example.com/foto.jpg", "javascript:alert(1)-1200.webp", "https://x/a b-1200.webp"} {
			if _, err := st.Pool.Exec(ctx, `update categories set image_url = $1 where id = $2`, u, b.category); err == nil {
				t.Errorf("aceptó %q en categories.image_url", u)
			}
			if _, err := st.Pool.Exec(ctx, `insert into categories (company_id, name, image_url) values ($1, 'Cat ' || gen_random_uuid(), $2)`, b.company, u); err == nil {
				t.Errorf("aceptó %q al crear una categoría", u)
			}
		}
		if _, err := st.Pool.Exec(ctx, `update products set image_url = null where id = $1`, b.product); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("la familia se guarda bajo RLS como el rol de la app", func(t *testing.T) {
		app := restoredAppRoleStore(t)
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, strconv.FormatInt(b.company, 10)); err != nil {
			t.Fatal(err)
		}
		tag, err := tx.Exec(ctx, `update categories set food_family = 'coffee' where id = $1`, b.category)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("update de food_family: %v filas=%d", err, tag.RowsAffected())
		}
		if _, err := tx.Exec(ctx, `update categories set food_family = 'taco' where id = $1`, b.category); err == nil {
			t.Fatal("aceptó una familia que no existe")
		}
	})

	t.Run("un pendiente de borrar solo lleva llaves de su propia empresa", func(t *testing.T) {
		good := strconv.FormatInt(a.company, 10) + "/products/9/0123456789abcdef0123456789abcdef-400.webp"
		if _, err := st.Pool.Exec(ctx, `insert into media_pending_deletions (company_id, object_key, last_error) values ($1, $2, 'x')`,
			a.company, good); err != nil {
			t.Fatalf("pendiente válido rechazado: %v", err)
		}
		foreign := strconv.FormatInt(b.company, 10) + "/products/9/0123456789abcdef0123456789abcdef-400.webp"
		if _, err := st.Pool.Exec(ctx, `insert into media_pending_deletions (company_id, object_key, last_error) values ($1, $2, 'x')`,
			a.company, foreign); err == nil {
			t.Fatal("aceptó la llave de otra empresa: el barrido borraría un archivo ajeno")
		}
		if _, err := st.Pool.Exec(ctx, `insert into media_pending_deletions (company_id, object_key, last_error) values ($1, $2, 'x')`,
			a.company, strconv.FormatInt(a.company, 10)+"/../x"); err == nil {
			t.Fatal("aceptó una llave con otra forma")
		}
	})

	t.Run("el Down se niega con borrados pendientes", func(t *testing.T) {
		err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", catalogMediaVersion-1)
		if err == nil || !strings.Contains(err.Error(), "pendientes") {
			t.Fatalf("el Down bajó con pendientes (los archivos quedarían en el bucket para siempre): %v", err)
		}
		if _, err := st.Pool.Exec(ctx, `delete from media_pending_deletions`); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", catalogMediaVersion-1); err != nil {
			t.Fatalf("Down sin pendientes: %v", err)
		}
		if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", catalogMediaVersion); err != nil {
			t.Fatalf("Up después del Down: %v", err)
		}
	})
}
