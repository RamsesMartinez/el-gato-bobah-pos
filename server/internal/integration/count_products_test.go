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

// PRODUCTOS CON EXISTENCIA PROPIA EN EL CONTEO (spec 069, issue #252): la lata que se vende tal
// cual se cuenta igual que un artículo, a ciegas y contra lo que había al capturar.

type productCountKit struct {
	st    *store.Store
	svc   *app.BackofficeService
	ctx   context.Context
	user  int64
	lata  int64 // existencia propia, en −2
	agua  int64 // existencia propia, nunca movida: sin fila en stock_levels
	latte int64 // receta: no entra al conteo
}

func newProductCountKit(t *testing.T) *productCountKit {
	t.Helper()
	st := newTestStore(t)
	k := &productCountKit{st: st}
	k.user = makeUser(t, st, "contador_productos", "admin")
	k.lata = makeProduct(t, st, "Lata conteo", dd("25"), true)
	adjust(t, st, "producto", k.lata, "-2")
	k.agua = makeProduct(t, st, "Agua conteo", dd("15"), true)
	k.latte = makeProduct(t, st, "Latte conteo", dd("55"), false)
	cafe := ingredientIn(t, st, defaultCompanyID, "Café latte conteo", "g")
	recipe := makeRecipe(t, st, map[int64]string{cafe: "18"})
	if _, err := st.Pool.Exec(context.Background(), `update products set recipe_id = $2, composition_status = 'confirmed', composition_confirmed_at = now() where id = $1`, k.latte, recipe); err != nil {
		t.Fatal(err)
	}
	k.svc, k.ctx = asTenant(t, defaultCompanyID)
	return k
}

// sellProduct simula lo que una venta resta mientras se cuenta.
func (k *productCountKit) sellProduct(t *testing.T, product int64, q string) {
	t.Helper()
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, item_type, product_id, movement_type, quantity)
		values ($1, 'producto', $2, 'venta', $3)`, defaultCompanyID, product, q); err != nil {
		t.Fatal(err)
	}
}

func (k *productCountKit) open(t *testing.T) app.CountView {
	t.Helper()
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	return c
}

func productIn(c app.CountView, id int64) *app.CountProductItem {
	for i := range c.Products {
		if c.Products[i].ProductID == id {
			return &c.Products[i]
		}
	}
	return nil
}

// El criterio de aceptación del issue #252.
func TestAProductAtMinusTwoCountedAtFifteenEndsAtFifteen(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	if _, err := k.st.Pool.Exec(context.Background(), `update products set current_cost = 9.5 where id = $1`, k.lata); err != nil {
		t.Fatal(err)
	}
	c := k.open(t)
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("15")}, k.user); err != nil {
		t.Fatalf("capturar 15: %v", err)
	}
	if _, err := k.svc.CloseCount(k.ctx, c.ID, k.user); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	if got := onHand(t, k.st, k.lata); !got.Equal(dd("15")) {
		t.Fatalf("la lata quedó en %s; quiere 15 (se contaron 15 con el sistema en −2)", got)
	}
	var n int
	var qty decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*), coalesce(sum(quantity), 0) from stock_movements
		where product_id = $1 and movement_type = 'ajuste' and stock_count_id = $2 and user_id = $3 and branch_id is not null`,
		k.lata, c.ID, k.user).Scan(&n, &qty); err != nil {
		t.Fatal(err)
	}
	if n != 1 || !qty.Equal(dd("17")) {
		t.Fatalf("ajustes del conteo = %d por %s; quiere uno de +17 ligado al conteo, con quién y sucursal", n, qty)
	}
	var cost *decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select unit_cost from stock_count_product_lines where count_id = $1 and product_id = $2`,
		c.ID, k.lata).Scan(&cost); err != nil {
		t.Fatal(err)
	}
	if cost == nil || !cost.Equal(dd("9.5")) {
		t.Errorf("costo guardado al cerrar = %v; quiere 9.5 (sin él, el faltante cambiaría con el costo)", cost)
	}
}

func TestTheCountListsOnlyProductsThatCountTheirOwnStock(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	combo := makeProduct(t, k.st, "Combo con interruptor", dd("80"), true)
	nothing := makeProduct(t, k.st, "No se cuenta conteo", dd("5"), false)
	archived := makeProduct(t, k.st, "Archivada conteo", dd("25"), true)
	ctx := context.Background()
	if _, err := k.st.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, combo); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update products set composition_status = 'confirmed', composition_confirmed_at = now() where id = $1`, nothing); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update products set is_active = false where id = $1`, archived); err != nil {
		t.Fatal(err)
	}
	c := k.open(t)
	for _, id := range []int64{k.lata, k.agua} {
		p := productIn(c, id)
		if p == nil {
			t.Fatalf("el producto %d con existencia propia no está en el conteo", id)
		}
		if p.Category == "" || p.CountedQty != nil {
			t.Errorf("producto %d: categoría %q, contado %v; quiere su categoría y sin contar", id, p.Category, p.CountedQty)
		}
	}
	for name, id := range map[string]int64{"receta": k.latte, "paquete": combo, "no se cuenta": nothing, "archivado": archived} {
		if productIn(c, id) != nil {
			t.Errorf("un producto de %s entró al conteo", name)
		}
	}
}

func TestCapturingAProductIsWholePiecesAndBlind(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	c := k.open(t)

	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("2.5")}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("2.5 latas = %v; quiere ErrValidation", err)
	}
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("-1")}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("−1 latas = %v; quiere ErrValidation", err)
	}
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.latte, app.CountCapture{Qty: dd("3")}, k.user); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("capturar un producto de receta = %v; quiere ErrNotFound", err)
	}
	// Un archivado no sale en la lista ni en «sin contar»: capturarlo lo ajustaría a escondidas.
	archived := makeProduct(t, k.st, "Archivada captura", dd("20"), true)
	if _, err := k.st.Pool.Exec(context.Background(), `update products set is_active = false where id = $1`, archived); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, archived, app.CountCapture{Qty: dd("3")}, k.user); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("capturar un producto archivado = %v; quiere ErrNotFound", err)
	}
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("2000000")}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("sobre el máximo de piezas = %v; quiere ErrValidation", err)
	}
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("900000"), Confirmable: true}, k.user); !errors.Is(err, domain.ErrCountNeedsConfirm) {
		t.Errorf("cifra rara con aviso = %v; quiere ErrCountNeedsConfirm", err)
	}

	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("10")}, k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("12")}, k.user); err != nil {
		t.Fatal(err)
	}
	cur, err := k.svc.CurrentCount(k.ctx)
	if err != nil || cur == nil {
		t.Fatalf("conteo abierto: %v", err)
	}
	if p := productIn(*cur, k.lata); p == nil || p.CountedQty == nil || !p.CountedQty.Equal(dd("12")) {
		t.Fatalf("recapturar no reemplazó lo contado: %+v", p)
	}
	var system decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select system_qty from stock_count_product_lines where count_id = $1 and product_id = $2`,
		c.ID, k.lata).Scan(&system); err != nil || !system.Equal(dd("-2")) {
		t.Fatalf("lo que el sistema tenía al capturar = %s (%v); quiere −2", system, err)
	}
	if err := k.svc.UncaptureCountProduct(k.ctx, c.ID, k.lata); err != nil {
		t.Fatal(err)
	}
	cur, _ = k.svc.CurrentCount(k.ctx)
	if p := productIn(*cur, k.lata); p == nil || p.CountedQty != nil {
		t.Fatalf("quitar la cifra no la dejó sin contar: %+v", p)
	}
}

// Un producto que nunca se vendió en la sucursal no tiene fila de existencia, y sin fila no hay
// candado: una venta a media cerrada se calcularía contra un cero viejo (revisión de arquitectura).
func TestCapturingAProductWithoutStockRowLocksItBeforeClosing(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	c := k.open(t)
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.agua, app.CountCapture{Qty: dd("6")}, k.user); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var rows int
	if err := k.st.Pool.QueryRow(ctx, `select count(*) from stock_levels where product_id = $1`, k.agua).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("filas de existencia del agua = %d (%v); capturar debió asegurarla", rows, err)
	}
	// Una venta del agua en curso: tiene su fila tomada.
	tx, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `select 1 from stock_levels where product_id = $1 for update`, k.agua); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CloseCount(k.ctx, c.ID, k.user); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("cerrar con una venta del agua en curso = %v; quiere ErrConflict («vuelve a intentar»)", err)
	}
	if _, err := tx.Exec(ctx, `insert into stock_movements (company_id, item_type, product_id, movement_type, quantity)
		values ($1, 'producto', $2, 'venta', -1)`, defaultCompanyID, k.agua); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CloseCount(k.ctx, c.ID, k.user); err != nil {
		t.Fatalf("cerrar después de la venta: %v", err)
	}
	if got := onHand(t, k.st, k.agua); !got.Equal(dd("5")) {
		t.Errorf("agua = %s; quiere 5 (6 contadas menos la vendida después de contar)", got)
	}
}

func TestClosingAdjustsProductsAgainstWhatThereWasWhenCaptured(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	ctx := context.Background()
	adjust(t, k.st, "producto", k.agua, "10")
	gone := makeProduct(t, k.st, "Deja de contarse", dd("20"), true)
	adjust(t, k.st, "producto", gone, "4")
	archived := makeProduct(t, k.st, "Se archiva contada", dd("20"), true)
	adjust(t, k.st, "producto", archived, "3")
	c := k.open(t)
	for id, q := range map[int64]string{k.agua: "15", gone: "9", archived: "5"} {
		if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, id, app.CountCapture{Qty: dd(q)}, k.user); err != nil {
			t.Fatal(err)
		}
	}
	k.sellProduct(t, k.agua, "-2")
	// Con el conteo abierto, uno deja de contar su existencia (cierra en cero) y otro se archiva.
	admin, actx := asApp(t, defaultCompanyID)
	if err := admin.SaveComposition(actx, app.CompositionOfProduct, gone, app.CompositionRequest{Mode: "nothing"}, k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update products set is_active = false where id = $1`, archived); err != nil {
		t.Fatal(err)
	}

	preview, err := k.svc.PreviewCount(k.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := k.svc.CloseCount(k.ctx, c.ID, k.user)
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	if got := onHand(t, k.st, k.agua); !got.Equal(dd("13")) {
		t.Errorf("agua = %s; quiere 13 (15 contadas, 2 vendidas después)", got)
	}
	if got := onHand(t, k.st, gone); !got.IsZero() {
		t.Errorf("el que dejó de contarse quedó en %s: el conteo le revivió el contador", got)
	}
	if got := onHand(t, k.st, archived); !got.Equal(dd("5")) {
		t.Errorf("el archivado ya contado quedó en %s; quiere 5", got)
	}
	if got := onHand(t, k.st, k.lata); !got.Equal(dd("-2")) {
		t.Errorf("la lata no se contó y cambió a %s", got)
	}
	for _, v := range []app.CountCloseView{preview, closed} {
		if len(v.SkippedProducts) != 1 || v.SkippedProducts[0].ProductID != gone {
			t.Errorf("saltados = %+v; quiere el que dejó de contarse", v.SkippedProducts)
		}
		found := false
		for _, n := range v.NotCountedProducts {
			found = found || n.ProductID == k.lata
		}
		if !found {
			t.Errorf("sin contar = %+v; quiere la lata", v.NotCountedProducts)
		}
	}
	var agua *app.CountProductAdjustmentView
	for i := range closed.AdjustedProducts {
		if closed.AdjustedProducts[i].ProductID == k.agua {
			agua = &closed.AdjustedProducts[i]
		}
	}
	if agua == nil || !agua.Delta.Equal(dd("5")) || !agua.System.Equal(dd("10")) {
		t.Errorf("ajuste del agua = %+v; quiere +5 sobre 10", agua)
	}
}

func TestClosingFailsFastWhileAModeChangeHoldsTheStock(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	c := k.open(t)
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("15")}, k.user); err != nil {
		t.Fatal(err)
	}
	// Un cambio de modo en curso bloquea la existencia del producto (applyModeChange).
	ctx := context.Background()
	tx, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select 1 from stock_levels where product_id = $1 for update`, k.lata); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CloseCount(k.ctx, c.ID, k.user); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cerrar a media cambio de modo = %v; quiere ErrConflict", err)
	}
}

func TestProductCountsAreIsolated(t *testing.T) {
	t.Parallel()
	k := newProductCountKit(t)
	c := k.open(t)
	if _, err := k.svc.CaptureCountProduct(k.ctx, c.ID, k.lata, app.CountCapture{Qty: dd("15")}, k.user); err != nil {
		t.Fatal(err)
	}
	other := makeCompany(t, k.st, "conteo-productos-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		if cur, err := svc.CurrentCount(ctx); err == nil && cur != nil && productIn(*cur, k.lata) != nil {
			t.Fatal("se ven los productos de otra empresa en el conteo")
		}
		if _, err := svc.CaptureCountProduct(ctx, c.ID, k.lata, app.CountCapture{Qty: dd("1")}, k.user); err == nil {
			t.Fatal("capturó un producto en el conteo de otra empresa")
		}
		if err := svc.UncaptureCountProduct(ctx, c.ID, k.lata); err == nil {
			t.Fatal("quitó una cifra del conteo de otra empresa")
		}
		if v, err := svc.PreviewCount(ctx, c.ID); err == nil && len(v.AdjustedProducts)+len(v.NotCountedProducts) > 0 {
			t.Fatal("la vista previa enseñó productos de otra empresa")
		}
		if _, err := svc.CloseCount(ctx, c.ID, k.user); err == nil {
			t.Fatal("cerró el conteo de otra empresa")
		}
	})
	if got := onHand(t, k.st, k.lata); !got.Equal(dd("-2")) {
		t.Errorf("otra empresa movió la lata a %s", got)
	}
	var counted decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select counted_qty from stock_count_product_lines where count_id = $1 and product_id = $2`,
		c.ID, k.lata).Scan(&counted); err != nil || !counted.Equal(dd("15")) {
		t.Errorf("la cifra quedó en %s (%v); otra empresa la tocó", counted, err)
	}
}
