//go:build integration

package integration

import (
	"context"
	"net/url"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// inTheThreeCases corre una prueba de aislamiento en los tres contextos donde RLS ya falló en este
// proyecto, todos bajo el rol de la aplicación:
//
//   - otra empresa: una sesión de la empresa AJENA pide lo que es de la dueña.
//   - conexión reciclada: el pool devuelve una conexión que ya tuvo empresa y se soltó. Tras el
//     `reset` el ajuste vale cadena vacía, no NULL (lo que arregló la 0074).
//   - sin empresa: una conexión virgen, sin ajuste.
//
// `prueba` recibe el store y el contexto; arma ahí su servicio y comprueba que no alcanza nada de
// la dueña. Existe para que nadie tenga que acordarse de los tres: una prueba de aislamiento que
// solo cubre «otra empresa» pasa en verde con el defecto de la 0074 adentro.
func inTheThreeCases(t *testing.T, owner, other int64, check func(t *testing.T, st *store.Store, ctx context.Context)) {
	t.Helper()
	t.Run("otra empresa", func(t *testing.T) {
		st := appRoleStore(t)
		ctx, release, err := st.AcquireTenant(context.Background(), other)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		check(t, st, ctx)
	})
	t.Run("conexión reciclada", func(t *testing.T) {
		// Una sola conexión en el pool: la que se suelta es la que se vuelve a entregar, sin
		// depender de qué conexión elija el pool.
		st := appRoleStoreWithOneConn(t)
		_, release, err := st.AcquireTenant(context.Background(), owner)
		if err != nil {
			t.Fatal(err)
		}
		release()
		check(t, st, context.Background())
	})
	t.Run("sin empresa", func(t *testing.T) {
		check(t, appRoleStore(t), context.Background())
	})
}

// appRoleStoreDeUnaConexion es appRoleStore con el pool limitado a una conexión.
func appRoleStoreWithOneConn(t *testing.T) *store.Store {
	t.Helper()
	u, _ := url.Parse(testURL(t))
	u.User = url.UserPassword("gatobobah_app", appRolePassword)
	q := u.Query()
	q.Set("pool_max_conns", "1")
	u.RawQuery = q.Encode()
	st, err := store.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("app-role store de una conexión: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}
