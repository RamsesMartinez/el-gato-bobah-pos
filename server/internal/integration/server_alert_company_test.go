//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// El aviso de un 500 (spec 070) dice el nombre de la empresa del que hizo la petición. Se lee con
// la sesión de ESA empresa, bajo el rol de la aplicación: nunca el nombre de otra, y lo que haya
// quedado pegado en la conexión no cambia la respuesta.
func TestServerAlertCompanyNameIsAlwaysTheAskedCompany(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	var other int64
	if err := owner.Pool.QueryRow(ctx, `insert into companies (slug, name) values ('aviso-ajena', 'Ajena del aviso') returning id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	var ownerName string
	if err := owner.Pool.QueryRow(ctx, `select name from companies where id = $1`, defaultCompanyID).Scan(&ownerName); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewCompanyService(st)
		if got, err := svc.Name(ctx, other); err != nil || got != "Ajena del aviso" {
			t.Fatalf("Name(otra) = %q, %v", got, err)
		}
		if got, err := svc.Name(ctx, defaultCompanyID); err != nil || got != ownerName {
			t.Fatalf("Name(dueña) = %q, %v; quería %q", got, err, ownerName)
		}
	})
}

// Una empresa que no existe no inventa un nombre: el correo cae al id.
func TestServerAlertCompanyNameOfAMissingCompanyFails(t *testing.T) {
	t.Parallel()
	_ = newTestStore(t)
	if got, err := app.NewCompanyService(appRoleStore(t)).Name(context.Background(), 987654); err == nil {
		t.Fatalf("una empresa inexistente devolvió nombre %q", got)
	}
}
