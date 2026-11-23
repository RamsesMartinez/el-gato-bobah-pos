package httpapi

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El objeto `sales` del alta y el PATCH (spec 092) viaja completo o no viaja. Un objeto a medias se
// rechaza: rellenar lo que falta con falso o vacío apagaría lugares de venta o borraría el precio
// web sin que nadie lo pidiera (el defecto del PATCH parcial de la 090).
func TestParseSales(t *testing.T) {
	full := `{"counter":false,"web":true,"webPrice":"39.50","allowOversell":false,"description":" De arroz "}`
	cases := []struct {
		name    string
		raw     string
		wantNil bool
		wantErr bool
	}{
		{"ausente: no se toca", ``, true, false},
		{"null: no se toca", `null`, true, false},
		{"completo", full, false, false},
		{"precio web null es heredar", `{"counter":true,"web":true,"webPrice":null,"allowOversell":true,"description":""}`, false, false},
		{"falta counter", `{"web":true,"webPrice":null,"allowOversell":true,"description":""}`, false, true},
		{"falta webPrice", `{"counter":true,"web":true,"allowOversell":true,"description":""}`, false, true},
		{"falta allowOversell", `{"counter":true,"web":true,"webPrice":null,"description":""}`, false, true},
		{"falta description", `{"counter":true,"web":true,"webPrice":null,"allowOversell":true}`, false, true},
		{"precio que no es número", `{"counter":true,"web":true,"webPrice":"abc","allowOversell":true,"description":""}`, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var raw json.RawMessage
			if c.raw != "" {
				raw = json.RawMessage(c.raw)
			}
			got, err := parseSales(raw)
			if c.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("se esperaba ErrValidation, fue %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != c.wantNil {
				t.Fatalf("nil = %v, se esperaba %v", got == nil, c.wantNil)
			}
		})
	}
	got, _ := parseSales(json.RawMessage(full))
	if got.Counter || !got.Web || got.WebPrice == nil || got.WebPrice.String() != "39.5" || got.AllowOversell || got.Description != " De arroz " {
		t.Fatalf("se leyó %+v", got)
	}
}
