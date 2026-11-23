//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// TRASPASOS ENTRE ALMACENES (spec 094, historia 5).

type transferKit struct {
	st            *store.Store
	svc           *app.BackofficeService
	ctx           context.Context
	user          int64
	local, bodega int64
	leche         int64
}

func newTransferKit(t *testing.T) *transferKit {
	t.Helper()
	st := newTestStore(t)
	k := &transferKit{st: st}
	k.local, k.bodega = comoEnLaSegundaSubida(t, st, defaultCompanyID)
	k.user = makeUser(t, st, "traspasa", "gerente")
	k.leche = ingredientIn(t, st, defaultCompanyID, "Leche traspaso", "ml")
	if _, err := st.Pool.Exec(context.Background(), `update ingredients set current_cost = 0.025 where id = $1`, k.leche); err != nil {
		t.Fatal(err)
	}
	adjustIn(t, st, k.local, "ingrediente", k.leche, "10")
	adjustIn(t, st, k.bodega, "ingrediente", k.leche, "20")
	k.svc, k.ctx = asTenant(t, defaultCompanyID)
	return k
}

func (k *transferKit) in(from, to int64, qty string) app.TransferInput {
	return app.TransferInput{ClientKey: uuid.New(), FromWarehouseID: from, ToWarehouseID: to,
		Lines: []domain.TransferLine{{ItemType: "ingrediente", ID: k.leche, Qty: decimal.RequireFromString(qty)}}}
}

// El escenario del spec: Local 10 y Bodega 20; traspasar 6 de Bodega a Local deja 16 y 14, con dos
// movimientos ligados al mismo traspaso y el costo vigente del artículo en las dos patas.
func TestATransferMovesBetweenWarehouses(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	res, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, k.local, "6"), k.user)
	if err != nil {
		t.Fatal(err)
	}
	if v := onHandIn(t, k.st, k.local, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(16)) {
		t.Fatalf("Local quedó en %s, quería 16", v)
	}
	if v := onHandIn(t, k.st, k.bodega, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(14)) {
		t.Fatalf("Bodega quedó en %s, quería 14", v)
	}
	var n int
	var costs []string
	rows, err := k.st.Pool.Query(context.Background(), `select unit_cost::text from stock_movements
		where transfer_id = $1 and movement_type = 'traspaso' order by quantity`, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		costs = append(costs, c)
		n++
	}
	rows.Close()
	if n != 2 || costs[0] != costs[1] || costs[0] == "" {
		t.Fatalf("el traspaso dejó %d movimientos con costos %v; quiere dos ligados con el mismo costo vigente", n, costs)
	}
	list, err := k.svc.ListTransfers(k.ctx, 10)
	if err != nil || len(list) != 1 || len(list[0].Lines) != 1 || list[0].FromName != "Bodega" {
		t.Fatalf("la lista de traspasos = %+v, %v", list, err)
	}
}

// Un reintento con la misma clave no duplica; la misma clave con otro cuerpo se rechaza.
func TestATransferIsIdempotent(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	in := k.in(k.bodega, k.local, "6")
	first, err := k.svc.CreateTransfer(k.ctx, in, k.user)
	if err != nil {
		t.Fatal(err)
	}
	again, err := k.svc.CreateTransfer(k.ctx, in, k.user)
	if err != nil || again.ID != first.ID || !again.Replay {
		t.Fatalf("el reintento = %+v, %v; quiere el mismo traspaso", again, err)
	}
	if v := onHandIn(t, k.st, k.local, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(16)) {
		t.Fatalf("el reintento movió otra vez: Local en %s", v)
	}
	// El reintento tampoco vuelve a escribir en la bitácora: un traspaso, un renglón.
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from audit_events
		where action = 'transfer.created' and entity_type = 'transfer' and entity_id = $1`, first.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("la bitácora tiene %d renglones transfer.created del traspaso (%v); quiere 1 aunque se reintente", n, err)
	}
	other := in
	other.Lines = []domain.TransferLine{{ItemType: "ingrediente", ID: k.leche, Qty: decimal.NewFromInt(7)}}
	if _, err := k.svc.CreateTransfer(k.ctx, other, k.user); !errors.Is(err, domain.ErrTransferKeyReused) {
		t.Fatalf("la misma clave con otro cuerpo = %v", err)
	}
}

// Más de lo que hay en el origen se pregunta; confirmado, el origen queda en negativo.
func TestATransferBeyondTheOriginNeedsConfirm(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	in := k.in(k.bodega, k.local, "30")
	_, err := k.svc.CreateTransfer(k.ctx, in, k.user)
	var nc domain.TransferNeedsConfirm
	if !errors.As(err, &nc) || len(nc.Lines) != 1 || !nc.Lines[0].Available.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("traspasar 30 de 20 = %v; quiere confirmar con 20 disponibles", err)
	}
	if v := onHandIn(t, k.st, k.bodega, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("sin confirmar se movió: Bodega en %s", v)
	}
	in.ConfirmNegative = true
	if _, err := k.svc.CreateTransfer(k.ctx, in, k.user); err != nil {
		t.Fatal(err)
	}
	if v := onHandIn(t, k.st, k.bodega, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(-10)) {
		t.Fatalf("confirmado, Bodega quedó en %s; quiere -10", v)
	}
}

func TestATransferRejectsWhatIsNotItsToMove(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.local, k.local, "1"), k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("origen igual a destino = %v", err)
	}
	if err := k.svc.ArchiveWarehouse(k.ctx, func() int64 {
		var id int64
		if err := k.st.Pool.QueryRow(context.Background(), `insert into warehouses (company_id, branch_id, name)
			select company_id, branch_id, 'Sótano' from warehouses where id = $1 returning id`, k.local).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}()); err != nil {
		t.Fatal(err)
	}
	var sotano int64
	if err := k.st.Pool.QueryRow(context.Background(), `select id from warehouses where name = 'Sótano'`).Scan(&sotano); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, sotano, "1"), k.user); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("a un almacén archivado = %v", err)
	}
	gone := ingredientIn(t, k.st, defaultCompanyID, "Archivado traspaso", "g")
	if _, err := k.st.Pool.Exec(context.Background(), `update ingredients set is_active = false where id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	in := k.in(k.bodega, k.local, "1")
	in.Lines[0].ID = gone
	if _, err := k.svc.CreateTransfer(k.ctx, in, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("un artículo archivado = %v", err)
	}
}

// Un traspaso de un insumo que se unió a otro va al destino de la unión, como cualquier movimiento.
func TestATransferOfAMergedIngredientGoesToTheTarget(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	adjustIn(t, k.st, bodega, "ingrediente", k.b, "40")
	k.merge(t, k.a, k.req(k.b))
	_, err := k.svc.CreateTransfer(k.ctx, app.TransferInput{ClientKey: uuid.New(), FromWarehouseID: bodega, ToWarehouseID: local,
		Lines: []domain.TransferLine{{ItemType: "ingrediente", ID: k.b, Qty: decimal.NewFromInt(10)}}}, k.admin)
	if err != nil {
		t.Fatalf("traspasar el insumo unido: %v", err)
	}
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.a); !v.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("la bodega del destino de la unión quedó en %s, quería 30 (40 − 10)", v)
	}
}

// En Existencias del almacén de venta, un negativo dice de dónde se puede traer.
func TestStockLevelsSuggestWhereToBringFrom(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	adjustIn(t, k.st, k.local, "ingrediente", k.leche, "-15")
	rows, err := k.svc.StockLevels(k.ctx, domain.WarehouseSel{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.IngredientID != nil && *r.IngredientID == k.leche {
			if len(r.TransferFrom) != 1 || r.TransferFrom[0].WarehouseID != k.bodega || !r.TransferFrom[0].OnHand.Equal(decimal.NewFromInt(20)) {
				t.Fatalf("la leche en -5 sugiere %+v; quiere traer de Bodega (20)", r.TransferFrom)
			}
			return
		}
	}
	t.Fatal("la leche no salió en Existencias")
}

func TestTransfersAreIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	other := makeCompany(t, k.st, "traspaso-ajena")
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, k.local, "1"), k.user); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		s := app.NewBackofficeService(as, clock)
		if list, err := s.ListTransfers(ctx, 50); err == nil && len(list) > 0 {
			t.Fatalf("se ven %d traspasos de otra empresa", len(list))
		}
		if _, err := s.CreateTransfer(ctx, k.in(k.bodega, k.local, "1"), k.user); err == nil {
			t.Fatal("se traspasó entre almacenes de otra empresa")
		}
	})
}
