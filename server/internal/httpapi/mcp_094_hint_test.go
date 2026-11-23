package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// Un traspaso que deja el origen en negativo responde NEEDS_CONFIRM, el mismo código que el conteo. La
// pista del conteo le dice al agente «repite con confirmed: true», un campo que stock_transfer no
// tiene: el agente reintentaba con un argumento que se ignora y el traspaso nunca pasaba. La del
// traspaso nombra confirm_negative y lo que sí hay en el origen.
func TestATransferShortfallHintNamesConfirmNegative(t *testing.T) {
	body := []byte(`{"error":{"code":"NEEDS_CONFIRM","message":"el origen no tiene suficiente",` +
		`"details":{"lines":[{"itemType":"ingrediente","id":7,"available":"14"}]}}}`)
	got := toolHint(http.StatusConflict, body, "warehouses_list")
	for _, want := range []string{"confirm_negative: true", "insumo 7: hay 14"} {
		if !strings.Contains(got, want) {
			t.Errorf("la pista del traspaso no dice %q: %s", want, got)
		}
	}
	if strings.Contains(got, "confirmed: true") {
		t.Errorf("la pista del traspaso pide el campo del conteo: %s", got)
	}
}
