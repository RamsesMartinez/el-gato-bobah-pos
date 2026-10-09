//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// EL DEFECTO QUE ESTO PREVIENE: un WithTx dentro de otro sobre la conexión de la empresa.
//
// Postgres ignora el segundo BEGIN con un aviso y el COMMIT de adentro confirma la transacción de
// afuera a medias, soltando sus candados. Pasó con el cierre de caja: listar las cuentas vivas
// (que barren borradores en su propia transacción) soltaba el candado del turno y un pedido de
// plataforma aceptado en ese instante quedaba fuera del corte firmado.
func TestANestedWithTxOnTheTenantConnectionIsRejected(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	c, soltar, err := st.AcquireTenant(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	var adentro error
	if err := st.WithTx(c, func(_ *db.Queries) error {
		adentro = st.WithTx(c, func(_ *db.Queries) error { return nil })
		return nil
	}); err != nil {
		t.Fatalf("la transacción de afuera: %v", err)
	}
	if adentro == nil {
		t.Fatal("un WithTx anidado se aceptó: su COMMIT habría confirmado la transacción de afuera a medias")
	}
}
