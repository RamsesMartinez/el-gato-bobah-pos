//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// UN TRASPASO MUEVE LOTES CON SU FECHA (spec 098, historia 6).

func (k *transferKit) lotsIn(t *testing.T, warehouse int64) map[string]string {
	t.Helper()
	rows, err := k.st.Pool.Query(context.Background(), `select expires_on::text, qty::text from stock_lots
		where warehouse_id = $1 and ingredient_id = $2 and qty > 0`, warehouse, k.leche)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, q string
		if err := rows.Scan(&d, &q); err != nil {
			t.Fatal(err)
		}
		out[d] = decimal.RequireFromString(q).String()
	}
	return out
}

// Bodega: 20 sin fecha (del kit), un lote de 40 bueno y uno de 5 caducado. Traspasar 50 a Local: salen
// los 20 sin fecha y 30 del lote, que llegan a Local con su fecha; el caducado no se mueve.
func TestATransferMovesLotsWithTheirDate(t *testing.T) {
	t.Parallel()
	k := newTransferKit(t)
	ctx := context.Background()
	var today time.Time
	if err := k.st.Pool.QueryRow(ctx, `select business_today($1)`, defaultCompanyID).Scan(&today); err != nil {
		t.Fatal(err)
	}
	good, old := today.AddDate(0, 0, 12).Format(time.DateOnly), today.AddDate(0, 0, -1).Format(time.DateOnly)
	for _, m := range [][2]string{{"40", good}, {"5", old}} {
		if _, err := k.st.Pool.Exec(ctx, `insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, expires_on)
			values ($1, 'ingrediente', $2, 'compra', $3::numeric, $4::date)`, k.bodega, k.leche, m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := k.svc.CreateTransfer(k.ctx, k.in(k.bodega, k.local, "50"), k.user); err != nil {
		t.Fatal(err)
	}
	wantLots(t, k.lotsIn(t, k.bodega), map[string]string{good: "10", old: "5"}, "Bodega")
	wantLots(t, k.lotsIn(t, k.local), map[string]string{good: "30"}, "Local")
	if v := onHandIn(t, k.st, k.local, "ingredient_id", k.leche); !v.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("Local = %s, quería 60 (10 + 50)", v)
	}
}
