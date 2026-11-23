//go:build integration

package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

const idemFailedVersion = 121

// LA 0121 («failed» en la llave de idempotencia del MCP, #335) CONTRA UN RESPALDO REAL CON DOS
// EMPRESAS. Lo que se prueba: lo que escribe la API anterior sigue entrando, «failed» exige su error,
// el rol de la API puede retomar una llave fallida con otra huella, RLS sigue separando empresas, y el
// Down suelta las fallidas sin negarse.
func TestMigration0121OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, idemFailedVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	type who struct{ company, user, token int64 }
	var ws []who
	rows, err := st.Pool.Query(ctx, `
		select distinct on (company_id) company_id, id from users where is_active order by company_id, id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var w who
		if err := rows.Scan(&w.company, &w.user); err != nil {
			t.Fatal(err)
		}
		ws = append(ws, w)
	}
	rows.Close()
	if len(ws) < 2 {
		t.Fatalf("el respaldo trae usuarios de %d empresas; la prueba necesita dos", len(ws))
	}
	hash := strings.Repeat("ab", 32)
	for i := range ws[:2] {
		if err := st.Pool.QueryRow(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
			values ($1, $2, 'Claude 0121', 'abab', decode($3, 'hex'), '{read,write:inventory}', now() + interval '30 days')
			returning id`, ws[i].company, ws[i].user, hash).Scan(&ws[i].token); err != nil {
			t.Fatal(err)
		}
	}
	a, b := ws[0], ws[1]
	ins := func(w who, key, status, result string) error {
		_, err := st.Pool.Exec(ctx, `
			insert into mcp_idempotency_keys (company_id, token_id, idem_key, tool, args_hash, status, result)
			values ($1, $2, $3, 'stock_adjust', decode($4, 'hex'), $5, $6::jsonb)`,
			w.company, w.token, key, hash, status, nilIfEmpty(result))
		return err
	}
	// Lo de antes de subir: una llave terminada y una desconocida.
	if err := ins(a, "antes-done-1", "done", `{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if err := ins(a, "antes-unknown-1", "unknown", ""); err != nil {
		t.Fatal(err)
	}
	exigeViolacionDeRestriccion(t, ins(a, "antes-failed-1", "failed", `{"error":"x"}`), "«failed» antes de la 0121")

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", idemFailedVersion); err != nil {
		t.Fatal(err)
	}

	t.Run("lo de la API anterior sigue entrando y «failed» exige su error", func(t *testing.T) {
		if err := ins(a, "despues-pending", "pending", ""); err != nil {
			t.Fatalf("una llave en curso, como la escribe la API anterior: %v", err)
		}
		if err := ins(a, "despues-unknown", "unknown", ""); err != nil {
			t.Fatalf("una llave desconocida, como la escribe la API anterior: %v", err)
		}
		if err := ins(a, "despues-failed", "failed", `{"error":"el producto 3 no existe en esta empresa"}`); err != nil {
			t.Fatalf("una llave fallida con su error: %v", err)
		}
		exigeViolacionDeRestriccion(t, ins(a, "fallida-sin-error", "failed", ""), "«failed» sin el error")
		exigeViolacionDeRestriccion(t, ins(a, "unknown-con-algo", "unknown", `{"x":1}`), "«unknown» con resultado")
	})

	t.Run("el rol de la API retoma una llave fallida de su empresa y no ve la de otra", func(t *testing.T) {
		if err := ins(b, "fallida-de-b", "failed", `{"error":"x"}`); err != nil {
			t.Fatal(err)
		}
		app := restoredAppRoleStore(t)
		retake := func(company, token int64, key string) int64 {
			t.Helper()
			tx, err := app.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck // la prueba no deja nada
			if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, strconv.FormatInt(company, 10)); err != nil {
				t.Fatal(err)
			}
			tag, err := tx.Exec(ctx, `update mcp_idempotency_keys set status = 'pending', result = null,
				args_hash = decode($3, 'hex'), tool = 'waste_record'
				where token_id = $1 and idem_key = $2 and status = 'failed'`, token, key, strings.Repeat("cd", 32))
			if err != nil {
				t.Fatalf("retomar como gatobobah_app: %v", err)
			}
			return tag.RowsAffected()
		}
		if n := retake(a.company, a.token, "despues-failed"); n != 1 {
			t.Fatalf("la empresa A retomó %d llaves suyas", n)
		}
		if n := retake(a.company, b.token, "fallida-de-b"); n != 0 {
			t.Fatal("la empresa A retomó la llave fallida de la empresa B")
		}
		// Lo que evita una escritura doble: una llave «unknown» (pudo aplicarse) o «pending» no se retoma.
		for _, k := range []string{"despues-unknown", "despues-pending"} {
			if n := retake(a.company, a.token, k); n != 0 {
				t.Fatalf("se retomó la llave %s, que no falló", k)
			}
		}
	})

	t.Run("el Down suelta las fallidas sin negarse y deja las demás", func(t *testing.T) {
		if err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", idemFailedVersion-1); err != nil {
			t.Fatalf("el Down se negó: %v", err)
		}
		var failed, kept int64
		if err := st.Pool.QueryRow(ctx, `select count(*) filter (where status = 'failed'),
			count(*) filter (where idem_key in ('antes-done-1', 'antes-unknown-1', 'despues-pending'))
			from mcp_idempotency_keys where token_id in ($1, $2)`, a.token, b.token).Scan(&failed, &kept); err != nil {
			t.Fatal(err)
		}
		if failed != 0 || kept != 3 {
			t.Fatalf("tras el Down: %d fallidas (quería 0) y %d de las demás (quería 3)", failed, kept)
		}
		exigeViolacionDeRestriccion(t, ins(a, "tras-el-down", "failed", `{"error":"x"}`), "«failed» tras el Down")
		var grants int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.column_privileges
			where table_name = 'mcp_idempotency_keys' and grantee = 'gatobobah_app' and privilege_type = 'UPDATE'
			  and column_name in ('args_hash', 'tool')`).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		if grants != 0 {
			t.Fatalf("tras el Down gatobobah_app sigue pudiendo reescribir %d columnas de la huella", grants)
		}
	})
}
