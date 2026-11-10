//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

const writeScopesVersion = 112

// LA 0112 (alcances de compras y pedidos, spec 073) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Solo
// amplía el check de alcances: lo que se prueba es que la lista vive en la base, que la ampliación no
// abre un alcance cualquiera y que el Down no pierde un alcance ya dado sin avisar.
func TestMigration0112OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, writeScopesVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var company, user int64
	if err := st.Pool.QueryRow(ctx,
		`select company_id, id from users where is_active order by company_id desc, id limit 1`).Scan(&company, &user); err != nil {
		t.Fatal(err)
	}
	insert := func(scopes string) error {
		_, err := st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
			values ($1, $2, 'Claude 0112', 'abab', decode($3, 'hex'), $4::text[], now() + interval '90 days')`,
			company, user, strings.Repeat("ab", 32), scopes)
		return err
	}

	t.Run("antes de la 0112 el alcance de compras no existe", func(t *testing.T) {
		exigeViolacionDeRestriccion(t, insert(`{read,write:purchases}`), "un alcance que la 0111 no conoce")
	})

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", writeScopesVersion); err != nil {
		t.Fatal(err)
	}

	t.Run("después acepta los siete y sigue rechazando lo demás", func(t *testing.T) {
		if err := insert(`{read,write:expenses,write:cash,write:inventory,write:catalog,write:purchases,write:orders}`); err != nil {
			t.Fatalf("los siete alcances: %v", err)
		}
		exigeViolacionDeRestriccion(t, insert(`{read,write:users}`), "un alcance que no existe")
		exigeViolacionDeRestriccion(t, insert(`{write:orders}`), "un token sin lectura")
		exigeViolacionDeRestriccion(t, insert(`{read,write:orders,null}`), "un alcance nulo")
	})

	t.Run("el Down se niega con un alcance nuevo en uso y su limpieza lo deja bajar", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con un token de compras: le habría quitado el alcance sin avisar")
		}
		migrarAbajoHasta(t, st.Pool, writeScopesVersion-1)
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from api_tokens
			where 'write:purchases' = any (scopes) or 'write:orders' = any (scopes)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("quedaron %d tokens con alcances que la 0111 no conoce", n)
		}
		exigeViolacionDeRestriccion(t, insert(`{read,write:orders}`), "el check de la 0111 restaurado")
		if err := insert(`{read,write:expenses,write:cash,write:inventory,write:catalog}`); err != nil {
			t.Fatalf("la 0111 acepta sus cinco: %v", err)
		}
	})
}
