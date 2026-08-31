//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Mover un producto de categoría desde el sistema.
//
// Hasta ahora la categoría solo se podía fijar AL CREAR el producto: el PATCH no la aceptaba, así
// que reacomodar el menú —mantenimiento normal con 1004 productos importados de FUDO, cuya
// estructura de categorías es la que ellos tenían— exigía entrar a la base a mano.
func TestCambiarLaCategoriaDeUnProducto(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)

	prod := makeProduct(t, st, "Papas de prueba", decimal.RequireFromString("40"), false)
	destino := categoriaDeEmpresa(t, st, defaultCompanyID, "Snacks de prueba")

	if err := admin.UpdateProduct(ctx, app.UpdateProductInput{
		ID: prod, Name: "Papas de prueba", Price: decimal.RequireFromString("40"),
		Active: true, CategoryID: destino,
	}); err != nil {
		t.Fatalf("mover de categoría: %v", err)
	}

	var got int64
	if err := st.Pool.QueryRow(ctx, `select category_id from products where id = $1`, prod).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != destino {
		t.Fatalf("categoría = %d, quiere %d", got, destino)
	}
}

// La categoría de OTRA empresa no se puede poner, y el que lo impide tiene que ser el servidor.
//
// Es la misma clase de defecto que cerraron 0040 y 0041: los chequeos de llave foránea de Postgres
// saltan RLS, así que `category_id` acepta cualquier id existente. Un producto apuntando a la
// categoría de otra empresa desaparece de su propio menú —el join corre bajo RLS y no encuentra la
// categoría— sin que nada avise y sin forma de arreglarlo desde la pantalla.
func TestNoSePuedeMoverUnProductoALaCategoriaDeOtraEmpresa(t *testing.T) {
	st := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	otra := makeCompany(t, st, "otra-categoria")
	prod := makeProduct(t, st, "Papas ajenas", decimal.RequireFromString("40"), false)
	ajena := categoriaDeEmpresa(t, st, otra, "Snacks de la otra")

	// Bajo el rol de app y con el tenant fijado, que es como corre un request real.
	tenantCtx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	err = app.NewAdminService(appSt).UpdateProduct(tenantCtx, app.UpdateProductInput{
		ID: prod, Name: "Papas ajenas", Price: decimal.RequireFromString("40"),
		Active: true, CategoryID: ajena,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("una categoría ajena debe rechazarse como no encontrada, fue: %v", err)
	}

	var got int64
	if err := st.Pool.QueryRow(ctx, `select category_id from products where id = $1`, prod).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got == ajena {
		t.Fatal("el producto no debió quedar en la categoría de la otra empresa")
	}
}

// Sin categoría en la petición el producto se queda donde está. Es lo que permite que el resto de
// la pantalla —renombrar, cambiar precio, activar— siga funcionando sin mandarla, y que un cliente
// viejo no mueva productos a la categoría 0 por omisión.
func TestSinCategoriaEnLaPeticionElProductoNoSeMueve(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)

	prod := makeProduct(t, st, "Papas quietas", decimal.RequireFromString("40"), false)
	var antes int64
	if err := st.Pool.QueryRow(ctx, `select category_id from products where id = $1`, prod).Scan(&antes); err != nil {
		t.Fatal(err)
	}

	if err := admin.UpdateProduct(ctx, app.UpdateProductInput{
		ID: prod, Name: "Papas quietas y con otro precio", Price: decimal.RequireFromString("55"), Active: true,
	}); err != nil {
		t.Fatalf("actualizar sin categoría: %v", err)
	}

	var despues int64
	var precio decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select category_id, price from products where id = $1`, prod).Scan(&despues, &precio); err != nil {
		t.Fatal(err)
	}
	if despues != antes {
		t.Fatalf("la categoría cambió de %d a %d sin que se pidiera", antes, despues)
	}
	if !precio.Equal(decimal.RequireFromString("55")) {
		t.Fatalf("el resto del update sí debe aplicarse; precio = %s", precio)
	}
}

func categoriaDeEmpresa(t *testing.T, st *store.Store, companyID int64, nombre string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`insert into categories (company_id, name) values ($1, $2) returning id`,
		companyID, nombre).Scan(&id); err != nil {
		t.Fatalf("categoría %s: %v", nombre, err)
	}
	return id
}
