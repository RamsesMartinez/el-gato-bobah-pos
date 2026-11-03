//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// -reset-admin DE UNA EMPRESA LE CAMBIABA LA CONTRASEÑA AL ADMIN DE OTRA.
//
// La escritura filtraba solo por usuario y corría como dueño, que salta RLS: en producción hay dos
// empresas con un usuario «admin» cada una, y resetear el de una reescribía los dos. Si la empresa
// pedida todavía no tenía admin, peor: como el update sí encontraba una fila —la de la otra— no se
// creaba el suyo, y el comando terminaba en éxito. Desde que la contraseña dejó el .env, este es el
// camino con el que nace el primer admin.
func TestResetAdminTouchesOnlyItsOwnCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	pedida := makeCompany(t, st, "reset-pedida")
	otra := makeCompany(t, st, "reset-otra")
	ajeno := makeUserIn(t, st, otra, "admin", "admin")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = 'hash-de-la-otra' where id = $1`, ajeno); err != nil {
		t.Fatal(err)
	}

	nuevo := func(hash string) db.CreateUserParams {
		u := "admin"
		return db.CreateUserParams{Name: "Administrador", Username: &u, Role: string(domain.RoleAdmin), PasswordHash: &hash}
	}
	contar := func(empresa int64, hash string) int {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from users where company_id = $1 and username = 'admin' and password_hash = $2`,
			empresa, hash).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Primero sin admin en la pedida (crear), luego con él (actualizar): los dos caminos del comando.
	for _, hash := range []string{"hash-nuevo-1", "hash-nuevo-2"} {
		if err := app.ResetCompanyAdmin(ctx, appRoleStore(t), pedida, nuevo(hash)); err != nil {
			t.Fatalf("resetear el admin: %v", err)
		}
		if contar(pedida, hash) != 1 {
			t.Fatalf("la empresa pedida no quedó con su admin y la contraseña nueva (%s)", hash)
		}
		if contar(otra, "hash-de-la-otra") != 1 {
			t.Fatal("resetear el admin de una empresa le cambió la contraseña al admin de OTRA")
		}
	}
}
