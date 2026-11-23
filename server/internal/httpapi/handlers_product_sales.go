package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// parseSales lee el objeto `sales` del alta y el PATCH de un producto (spec 092). Ausente o null =
// no tocar. Presente, trae todos sus campos: un objeto a medias se rechaza en vez de rellenar con
// falso o vacío, que apagaría lugares de venta o borraría el precio web sin que nadie lo pidiera.
func parseSales(raw json.RawMessage) (*app.SalesInput, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	// Por llaves y no por punteros: un `webPrice: null` presente (heredar) y uno ausente (objeto a
	// medias) se decodifican igual a un puntero nil.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("%w: sales ilegible", domain.ErrValidation)
	}
	for _, k := range []string{"counter", "web", "webPrice", "allowOversell", "description"} {
		if _, ok := fields[k]; !ok {
			return nil, fmt.Errorf("%w: sales viaja completo (counter, web, webPrice, allowOversell, description)", domain.ErrValidation)
		}
	}
	var b struct {
		Counter       bool   `json:"counter"`
		Web           bool   `json:"web"`
		AllowOversell bool   `json:"allowOversell"`
		Description   string `json:"description"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("%w: sales ilegible", domain.ErrValidation)
	}
	in := &app.SalesInput{Counter: b.Counter, Web: b.Web, AllowOversell: b.AllowOversell, Description: b.Description}
	if wp := fields["webPrice"]; !bytes.Equal(bytes.TrimSpace(wp), []byte("null")) {
		var p decimal.Decimal
		if err := json.Unmarshal(wp, &p); err != nil {
			return nil, fmt.Errorf("%w: el precio web no es un número", domain.ErrValidation)
		}
		in.WebPrice = &p
	}
	return in, nil
}
