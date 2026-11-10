//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

const apiTokensVersion = 111

// LA 0111 (tokens del servidor MCP, spec 072) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Solo agrega
// dos tablas y un permiso: lo que se prueba es que las barreras viven en la base y no solo en Go.
func TestMigration0111OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, apiTokensVersion-1)
	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", apiTokensVersion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var company, user int64
	if err := st.Pool.QueryRow(ctx,
		`select company_id, id from users where is_active order by company_id desc, id limit 1`).Scan(&company, &user); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("ab", 32)
	insertToken := func(scopes, extra string) error {
		_, err := st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
			values ($1, $2, 'Claude de prueba', 'abab', decode($3, 'hex'), $4::text[], now() + interval '90 days'`+extra+`)`,
			company, user, hash, scopes)
		return err
	}

	t.Run("los checks viven en la base", func(t *testing.T) {
		exigeViolacionDeRestriccion(t, insertToken(`{write:expenses}`, ""), "un token sin el alcance de lectura")
		exigeViolacionDeRestriccion(t, insertToken(`{read,write:users}`, ""), "un alcance que no existe")
		_, err := st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
			values ($1, $2, 'x', 'abab', decode($3, 'hex'), '{read}', now() + interval '400 days')`, company, user, hash)
		exigeViolacionDeRestriccion(t, err, "un token de más de 180 días")
		_, err = st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
			values ($1, $2, '   ', 'abab', decode($3, 'hex'), '{read}', now() + interval '30 days')`, company, user, hash)
		exigeViolacionDeRestriccion(t, err, "un nombre en blanco")
		_, err = st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at, revoked_at)
			values ($1, $2, 'x', 'abab', decode($3, 'hex'), '{read}', now() + interval '30 days', now())`, company, user, hash)
		exigeViolacionDeRestriccion(t, err, "revocado sin quién lo revocó")
	})

	var tokenID int64
	if err := st.Pool.QueryRow(ctx, `
		insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
		values ($1, $2, 'Claude de prueba', 'abab', decode($3, 'hex'), '{read,write:expenses}', now() + interval '90 days')
		returning id`, company, user, hash).Scan(&tokenID); err != nil {
		t.Fatal(err)
	}

	t.Run("la clave de idempotencia valida su forma y su estado", func(t *testing.T) {
		ins := func(key, status, result string) error {
			_, err := st.Pool.Exec(ctx, `
				insert into mcp_idempotency_keys (company_id, token_id, idem_key, tool, args_hash, status, result)
				values ($1, $2, $3, 'expense_create', decode($4, 'hex'), $5, $6::jsonb)`,
				company, tokenID, key, hash, status, nilIfEmpty(result))
			return err
		}
		exigeViolacionDeRestriccion(t, ins("corta", "pending", ""), "una clave de menos de 8")
		exigeViolacionDeRestriccion(t, ins("clave-valida-1", "otra", ""), "un estado desconocido")
		exigeViolacionDeRestriccion(t, ins("clave-valida-2", "done", ""), "terminada sin resultado")
		if err := ins("clave-valida-3", "done", `{"id":1}`); err != nil {
			t.Fatalf("una clave válida terminada se rechazó: %v", err)
		}
	})

	t.Run("revocado es terminal: la base no deja reactivarlo", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx,
			`update api_tokens set revoked_at = now(), revoked_by = user_id where id = $1`, tokenID); err != nil {
			t.Fatal(err)
		}
		for _, stmt := range []string{
			`update api_tokens set revoked_at = null, revoked_by = null where id = $1`,
			// Reescribir quién lo revocó borra evidencia aunque siga revocado.
			`update api_tokens set revoked_by = (select id from users where company_id = api_tokens.company_id and id <> api_tokens.revoked_by limit 1) where id = $1`,
		} {
			_, err := st.Pool.Exec(ctx, stmt, tokenID)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("%s: esperaba el rechazo del trigger (23514), salió %v", stmt, err)
			}
		}
	})

	t.Run("las llaves foráneas no cruzan empresas y la vigencia no va al revés", func(t *testing.T) {
		var otherUser int64
		if err := st.Pool.QueryRow(ctx, `select id from users where company_id <> $1 order by id limit 1`, company).Scan(&otherUser); err != nil {
			t.Fatal(err)
		}
		_, err := st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at)
			values ($1, $2, 'x', 'abab', decode($3, 'hex'), '{read}', now() + interval '30 days')`, company, otherUser, hash)
		exigeViolacionDeRestriccion(t, err, "un token de la empresa A para una persona de la B")
		_, err = st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, expires_at, revoked_at, revoked_by)
			values ($1, $2, 'x', 'abab', decode($3, 'hex'), '{read}', now() + interval '30 days', now(), $4)`, company, user, hash, otherUser)
		exigeViolacionDeRestriccion(t, err, "revocado por una persona de otra empresa")
		_, err = st.Pool.Exec(ctx, `
			insert into api_tokens (company_id, user_id, name, secret_hint, secret_hash, scopes, created_at, expires_at)
			values ($1, $2, 'x', 'abab', decode($3, 'hex'), '{read}', now(), now() - interval '1 day')`, company, user, hash)
		exigeViolacionDeRestriccion(t, err, "un token que vence antes de crearse")
	})

	t.Run("el rol de la app no puede renombrar ni borrar un token", func(t *testing.T) {
		app := restoredAppRoleStore(t)
		conn := conexionDeEmpresa(t, app, company)
		for _, stmt := range []string{
			`update api_tokens set name = 'otro' where id = $1`,
			`update api_tokens set scopes = '{read,write:cash}' where id = $1`,
			`delete from api_tokens where id = $1`,
		} {
			_, err := conn.Exec(ctx, stmt, tokenID)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				t.Errorf("%s: esperaba 42501 (sin permiso), salió %v", stmt, err)
			}
		}
	})

	t.Run("el permiso 37 existe y solo lo trae el administrador", func(t *testing.T) {
		var code, area string
		if err := st.Pool.QueryRow(ctx, `select code, area from permissions where id = 37`).Scan(&code, &area); err != nil {
			t.Fatal(err)
		}
		if code != string(domain.PermIntegrationsManage) || area != "business" {
			t.Fatalf("permiso 37 = %s/%s", code, area)
		}
		rows, err := st.Pool.Query(ctx, `select s.code from system_role_permissions sp
			join system_roles s on s.id = sp.system_role_id where sp.permission_id = 37 order by 1`)
		if err != nil {
			t.Fatal(err)
		}
		var roles []string
		for rows.Next() {
			var r string
			_ = rows.Scan(&r)
			roles = append(roles, r)
		}
		rows.Close()
		if strings.Join(roles, ",") != "admin" {
			t.Fatalf("lo traen: %v", roles)
		}
	})

	t.Run("los permisos de un admin incluyen el nuevo y los roles se siguen listando", func(t *testing.T) {
		appSt := restoredAppRoleStore(t)
		var admin, adminCompany int64
		if err := st.Pool.QueryRow(ctx, `
			select u.id, u.company_id from users u join roles r on r.id = u.role_id
			 where r.system_role_id = 1 and u.is_active and r.active order by u.id limit 1`).Scan(&admin, &adminCompany); err != nil {
			t.Fatal(err)
		}
		tctx, release, err := appSt.AcquireTenant(ctx, adminCompany)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		svc := app.NewRolesService(appSt)
		perms, err := svc.EffectivePermissions(tctx, admin)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range perms {
			found = found || p == domain.PermIntegrationsManage
		}
		if !found {
			t.Fatal("el administrador no recibió integrations.manage por herencia")
		}
		// La versión anterior de la API no conoce el permiso: effectivePermissions lo descarta con
		// KnownPermission. Aquí se prueba que leer el catálogo con la fila 37 presente no truena.
		if _, err := svc.List(tctx); err != nil {
			t.Fatalf("listar roles con el permiso nuevo en la base: %v", err)
		}
	})

	t.Run("el Down se niega con tokens y su limpieza lo deja bajar", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con un token en la base: lo habría perdido sin avisar")
		}
		migrarAbajoHasta(t, st.Pool, apiTokensVersion-1)
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from permissions where id = 37`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("el Down dejó el permiso 37")
		}
		if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", apiTokensVersion); err != nil {
			t.Fatal(err)
		}
	})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
