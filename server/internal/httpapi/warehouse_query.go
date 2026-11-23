package httpapi

import (
	"fmt"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// warehouseFromQuery lee ?warehouseId=. Ausente es el almacén de venta; presente y vacío se rechaza:
// viene de una pantalla que interpoló algo sin valor, y leerlo como «el de venta» le enseñaría cifras
// de un almacén que no pidió (constitución V).
func warehouseFromQuery(r *http.Request, allowAll bool) (domain.WarehouseSel, error) {
	v := r.URL.Query()
	if v.Has("warehouseId") && v.Get("warehouseId") == "" {
		return domain.WarehouseSel{}, fmt.Errorf("%w: almacén vacío", domain.ErrValidation)
	}
	return domain.ParseWarehouseParam(v.Get("warehouseId"), allowAll)
}
