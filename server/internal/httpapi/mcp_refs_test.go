package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// refsExempt son los ids de escritura que no se revisan contra la empresa, con el porqué.
var refsExempt = map[string]string{
	"unit_id":            "las unidades son del sistema, iguales en toda empresa",
	"base_unit_id":       "las unidades son del sistema",
	"denomination_id":    "las denominaciones son del sistema",
	"draft_id":           "es un UUID; la ruta lee la cuenta con RLS y responde 404",
	"item_id":            "renglón de un gasto que ya se revisó como de la empresa",
	"payment_id":         "pago de un pedido que ya se revisó como de la empresa",
	"line_id":            "renglón de un pedido que ya se revisó como de la empresa",
	"withdrawal_user_id": "InsertWithdrawal la lee de users con RLS (where u.id = … and u.is_active)",
}

// #335: toda escritura declara CADA id que recibe, para que la vista previa y el aplicar rechacen uno de
// otra empresa con su nombre. La prueba manda un id distinto en cada campo `*_id` del esquema (también
// dentro de listas) y exige que refs los devuelva todos: un campo nuevo sin revisar truena aquí, no en
// producción.
func TestEveryWriteToolWithIDsDeclaresItsRefs(t *testing.T) {
	for name, schema := range mcpWriteSchemas {
		next := int64(1000)
		want := map[int64]string{}
		args := sampleArgs(schema, "", &next, want)
		if len(want) == 0 {
			continue
		}
		probe, ok := mcpWriteRefsProbe[name]
		if !ok {
			t.Errorf("%s recibe ids y no declara refs", name)
			continue
		}
		raw, _ := json.Marshal(args)
		refs, err := probe(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := map[int64]bool{}
		for _, r := range refs {
			got[r.ID] = true
		}
		for id, field := range want {
			if !got[id] {
				t.Errorf("%s no revisa %s", name, field)
			}
		}
	}
}

// sampleArgs arma argumentos con un id único en cada campo de id no exento y anota cuál es cuál.
func sampleArgs(s *jsonschema.Schema, prefix string, next *int64, want map[int64]string) map[string]any {
	out := map[string]any{}
	for k, p := range s.Properties {
		switch {
		case strings.HasSuffix(k, "_id") || k == "id":
			if refsExempt[k] != "" {
				continue
			}
			*next++
			out[k] = *next
			want[*next] = prefix + k
		case k == "item_type":
			out[k] = "ingrediente"
		case p.Items != nil && len(p.Items.Properties) > 0:
			out[k] = []any{sampleArgs(p.Items, prefix+k+"[].", next, want)}
		case len(p.Properties) > 0:
			out[k] = sampleArgs(p, prefix+k+".", next, want)
		}
	}
	return out
}
