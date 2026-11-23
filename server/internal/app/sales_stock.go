package app

import (
	"context"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// salesStock y lockSalesStock son la ÚNICA puerta a la existencia con la que se vende (spec 092):
// la lista de agotados y el chequeo de la venta solo pasan por aquí. Leen el almacén de venta de la
// sucursal (spec 094): lo que está en la bodega no se vende hasta que se traspasa.
func salesStock(ctx context.Context, q *db.Queries) (domain.StockLevels, error) {
	rows, err := q.SalesStockLevels(ctx)
	if err != nil {
		return nil, err
	}
	out := domain.StockLevels{}
	for _, r := range rows {
		out[salesStockKey(string(r.ItemType), r.IngredientID, r.ProductID)] = r.OnHand
	}
	return out, nil
}

// lockSalesStock bloquea (FOR UPDATE) los renglones de existencia que pide una venta en el almacén de
// venta de la sucursal del pedido y devuelve lo que hay en ellos ya bloqueados. Lo que no tiene fila no se bloquea: vale
// cero, y cero se rechaza igual en las dos tabletas.
func lockSalesStock(ctx context.Context, q *db.Queries, branchID int64, keys map[domain.StockKey]struct{}) (domain.StockLevels, error) {
	var ings, prods []int64
	for k := range keys {
		if k.ItemType == "producto" {
			prods = append(prods, k.ID)
		} else {
			ings = append(ings, k.ID)
		}
	}
	rows, err := q.LockSalesStock(ctx, db.LockSalesStockParams{BranchID: branchID, IngredientIds: ings, ProductIds: prods})
	if err != nil {
		return nil, err
	}
	out := domain.StockLevels{}
	for _, r := range rows {
		out[salesStockKey(string(r.ItemType), r.IngredientID, r.ProductID)] = r.OnHand
	}
	return out, nil
}

func salesStockKey(itemType string, ingredientID, productID *int64) domain.StockKey {
	if productID != nil {
		return domain.StockKey{ItemType: itemType, ID: *productID}
	}
	return domain.StockKey{ItemType: itemType, ID: *ingredientID}
}

// checkSalesStock rechaza con domain.ErrOutOfStock la venta de mostrador que pide más de lo que hay
// de los productos con «vender sin existencia» apagado. Va dentro de la transacción del pedido y
// ANTES de descontar, con los candados en un orden fijo: dos ventas que revisan los mismos renglones
// se serializan sin abrazarse.
//
// ponytail: una venta revisada contra otra que no se revisa (de plataforma, o con la opción
// encendida) todavía puede cruzar candados por el trigger si tocan los mismos insumos en orden
// contrario; Postgres aborta una con 40P01 y la tableta reintenta. El riesgo ya existía entre dos
// ventas cualesquiera; el arreglo de fondo es bloquear en orden fijo todo lo que cualquier venta
// descuenta. Los extras no cuentan para el chequeo (dependen de la elección) y pueden dejar negativo
// un insumo, que está permitido.
func checkSalesStock(ctx context.Context, q *db.Queries, branchID int64, g domain.StockGraph, lines []domain.StockCheckLine) error {
	if len(lines) == 0 {
		return nil
	}
	needs := domain.StockNeeds(g, lines)
	if len(needs) == 0 {
		return nil
	}
	keys := make(map[domain.StockKey]struct{}, len(needs))
	for k := range needs {
		keys[k] = struct{}{}
	}
	levels, err := lockSalesStock(ctx, q, branchID, keys)
	if err != nil {
		return err
	}
	return domain.CheckStock(g, levels, lines)
}
