//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Lo que encontró la revisión de la tercera subida de la 094 (traspasos).

// Un producto con existencia propia se traspasa igual que un insumo.
func TestATransferOfAProductMoves(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	soda := makeProduct(t, k.st, "Refresco traspaso", decimal.NewFromInt(20), true)
	adjustIn(t, k.st, k.bodega, "producto", soda, "10")
	_, err := k.svc.CreateTransfer(k.ctx, app.TransferInput{ClientKey: uuid.New(), FromWarehouseID: k.bodega, ToWarehouseID: k.local,
		Lines: []domain.TransferLine{{ItemType: "producto", ID: soda, Qty: decimal.NewFromInt(4)}}}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if v := onHandIn(t, k.st, k.local, "product_id", soda); !v.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("el local quedó con %s refrescos, quería 4", v)
	}
	if v := onHandIn(t, k.st, k.bodega, "product_id", soda); !v.Equal(decimal.NewFromInt(6)) {
		t.Fatalf("la bodega quedó con %s refrescos, quería 6", v)
	}
}

// Una venta que tiene tomada la existencia de un producto en el local no hace esperar al traspaso:
// responde «vuelve a intentar» (409) en vez de una espera que se cruza con la venta y Postgres aborta a
// una de las dos (40P01), a veces el cobro.
func TestATransferDoesNotWaitForAProductHeldByASale(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	soda := makeProduct(t, k.st, "Refresco tomado", decimal.NewFromInt(20), true)
	adjustIn(t, k.st, k.bodega, "producto", soda, "10")
	adjustIn(t, k.st, k.local, "producto", soda, "1")

	ctx := context.Background()
	tx, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select 1 from stock_levels where warehouse_id = $1 and product_id = $2 for update`, k.local, soda); err != nil {
		t.Fatal(err)
	}

	tctx, cancel := context.WithTimeout(k.ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err = k.svc.CreateTransfer(tctx, app.TransferInput{ClientKey: uuid.New(), FromWarehouseID: k.bodega, ToWarehouseID: k.local,
		Lines: []domain.TransferLine{{ItemType: "producto", ID: soda, Qty: decimal.NewFromInt(2)}}}, k.user)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("con el producto tomado por una venta = %v (en %s); quiere 409 sin esperar", err, time.Since(start))
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("el traspaso esperó %s al producto tomado", time.Since(start))
	}
}

// A se unió a B: un traspaso con renglones A y B es el mismo artículo dos veces, y lo dice así.
func TestATransferOfAMergedPairSaysItIsTheSameArticle(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	adjustIn(t, k.st, bodega, "ingrediente", k.b, "40")
	k.merge(t, k.a, k.req(k.b))
	_, err := k.svc.CreateTransfer(k.ctx, app.TransferInput{ClientKey: uuid.New(), FromWarehouseID: bodega, ToWarehouseID: local,
		Lines: []domain.TransferLine{
			{ItemType: "ingrediente", ID: k.a, Qty: decimal.NewFromInt(1)},
			{ItemType: "ingrediente", ID: k.b, Qty: decimal.NewFromInt(1)},
		}}, k.admin)
	if !errors.Is(err, domain.ErrValidation) || err.Error() == "" || !strings.Contains(err.Error(), "dos veces") {
		t.Fatalf("A y su destino B en el mismo traspaso = %v; quiere «el mismo artículo va dos veces»", err)
	}
}

// Cada pata de un traspaso está en su origen o en su destino, nunca en un tercer almacén; y la base
// no deja un movimiento `traspaso` sin documento ni uno de otro tipo ligado a un traspaso.
func TestTransferLegsAreBoundToTheTransfer(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	res, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, k.local, "3"), k.user)
	if err != nil {
		t.Fatal(err)
	}
	var stray int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements m join stock_transfers x on x.id = m.transfer_id
		where m.transfer_id = $1 and m.warehouse_id not in (x.from_warehouse_id, x.to_warehouse_id)`, res.ID).Scan(&stray); err != nil {
		t.Fatal(err)
	}
	if stray != 0 {
		t.Fatalf("%d patas del traspaso fuera de su origen o destino", stray)
	}
	_, err = k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, warehouse_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, $2, 'ingrediente', $3, 'traspaso', 1)`, defaultCompanyID, k.local, k.leche)
	exigeViolacionDeRestriccion(t, err, "un movimiento de traspaso sin traspaso")
	_, err = k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, warehouse_id, item_type, ingredient_id, movement_type, quantity, transfer_id)
		values ($1, $2, 'ingrediente', $3, 'ajuste', 1, $4)`, defaultCompanyID, k.local, k.leche, res.ID)
	exigeViolacionDeRestriccion(t, err, "un ajuste ligado a un traspaso")
}

// Un almacén de OTRA SUCURSAL de la misma empresa no recibe traspasos: RLS no lo separa, lo separan el
// filtro de sucursal de la sesión y la llave compuesta.
func TestATransferToAnotherBranchIsRejected(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	var norte int64
	if err := k.st.Pool.QueryRow(context.Background(), `with b as (insert into branches (company_id, branch_number, code, name, is_active)
		values ($1, 7, 'NORTE7', 'Norte', false) returning *)
		insert into warehouses (company_id, branch_id, name, is_sales) select company_id, id, 'Local Norte', true from b returning id`,
		defaultCompanyID).Scan(&norte); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, norte, "1"), k.user); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("traspasar al almacén de otra sucursal = %v; quiere «no existe»", err)
	}
	if v := onHandIn(t, k.st, k.bodega, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("la bodega quedó en %s tras el rechazo", v)
	}
}

// La clave del cliente es por empresa: la misma clave en otra empresa es otro traspaso, nunca el de la
// primera.
func TestATransferKeyOfAnotherCompanyIsAnotherTransfer(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	in := k.in(k.bodega, k.local, "1")
	mine, err := k.svc.CreateTransfer(k.ctx, in, k.user)
	if err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, k.st, "traspaso-clave-ajena")
	oLocal, oBodega := comoEnLaSegundaSubida(t, k.st, other)
	oLeche := ingredientIn(t, k.st, other, "Leche ajena", "ml")
	// adjustIn no lleva empresa y la columna toma la de omisión: aquí va explícita.
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, warehouse_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, $2, 'ingrediente', $3, 'ajuste', 5)`, other, oBodega, oLeche); err != nil {
		t.Fatal(err)
	}
	osvc, octx := asTenant(t, other)
	ouser := makeUserIn(t, k.st, other, "traspasa-ajena", "gerente")
	theirs, err := osvc.CreateTransfer(octx, app.TransferInput{ClientKey: in.ClientKey, FromWarehouseID: oBodega, ToWarehouseID: oLocal,
		Lines: []domain.TransferLine{{ItemType: "ingrediente", ID: oLeche, Qty: decimal.NewFromInt(1)}}}, ouser)
	if err != nil {
		t.Fatalf("la otra empresa con la misma clave: %v", err)
	}
	if theirs.ID == mine.ID || theirs.Replay {
		t.Fatalf("la otra empresa recibió el traspaso de la primera (%d) en vez del suyo", mine.ID)
	}
}

// Dos peticiones con la misma clave a la vez (un doble toque, un reintento de red): una crea, la otra
// devuelve el mismo traspaso, y la mercancía se mueve una sola vez.
func TestTwoTransfersWithTheSameKeyAtOnce(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	in := k.in(k.bodega, k.local, "2")
	var wg sync.WaitGroup
	ids := make([]int64, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc, ctx := asTenant(t, defaultCompanyID)
			res, err := svc.CreateTransfer(ctx, in, k.user)
			ids[i], errs[i] = res.ID, err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil && !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("petición %d: %v", i, err)
		}
	}
	if errs[0] == nil && errs[1] == nil && ids[0] != ids[1] {
		t.Fatalf("la misma clave creó dos traspasos: %v", ids)
	}
	if v := onHandIn(t, k.st, k.bodega, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(18)) {
		t.Fatalf("la bodega quedó en %s; con un solo traspaso de 2 quiere 18", v)
	}
}
