package httpapi

import (
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
	"withdrawal_user_id": "la ruta lee a la persona con RLS",
}

// refsExemptTools son las escrituras que no declaran refs a propósito, con el porqué. Hoy ninguna.
var refsExemptTools = map[string]string{}

// #335: toda escritura que recibe un id declara sus referencias, para que la vista previa y el aplicar
// rechacen uno de otra empresa con su nombre. Una herramienta nueva sin ellas truena aquí, no en
// producción con un 500.
func TestEveryWriteToolWithIDsDeclaresItsRefs(t *testing.T) {
	for name, schema := range mcpWriteSchemas {
		if _, ok := refsExemptTools[name]; ok {
			continue
		}
		ids := idProperties(schema, "")
		if len(ids) > 0 && !mcpWriteHasRefs[name] {
			t.Errorf("%s recibe %v y no declara refs", name, ids)
		}
	}
}

func idProperties(s *jsonschema.Schema, prefix string) []string {
	if s == nil {
		return nil
	}
	var out []string
	for k, p := range s.Properties {
		if (strings.HasSuffix(k, "_id") || k == "id") && refsExempt[k] == "" {
			out = append(out, prefix+k)
		}
		out = append(out, idProperties(p, prefix+k+".")...)
		out = append(out, idProperties(p.Items, prefix+k+"[].")...)
	}
	return out
}
