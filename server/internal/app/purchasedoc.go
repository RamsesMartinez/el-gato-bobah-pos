package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Extracción de documentos de compra con un modelo de visión. Un ticket térmico, una factura
// y un pedido web no comparten estructura, y cada proveedor nuevo trae otra: escribir un
// parser por proveedor no escala. En su lugar, una sola llamada con un schema fijo devuelve
// SIEMPRE la misma forma (domain.PurchaseDoc) y el resto del sistema no sabe de qué proveedor
// vino.
//
// Lo que devuelve es un BORRADOR. Nada se escribe en el almacén ni en la contabilidad hasta
// que el operador lo confirma: una extracción equivocada cuesta una corrección, no un
// inventario corrupto.

const (
	// maxDocBytes: un ticket en PDF pesa ~10 KB y una foto de celular 3-5 MB. El techo protege
	// el límite de 32 MB por request de la API y, sobre todo, evita pagar tokens por un archivo
	// que alguien subió por error.
	maxDocBytes = 12 << 20 // 12 MiB

	// maxOutputTokens: el documento más largo que hemos visto trae ~30 renglones (~4 KB de
	// JSON). 16k deja margen de sobra para una factura de varias páginas sin permitir que un
	// documento basura genere salida ilimitada.
	maxOutputTokens = 16000

	// docExtractTimeout: es una operación interactiva (el operador espera con el tablet en la
	// mano). Por encima de esto vale más fallar y que reintente que dejarlo mirando un spinner.
	docExtractTimeout = 90 * time.Second
)

// ErrDocExtractDisabled se devuelve cuando no hay ANTHROPIC_API_KEY configurada. El POS
// funciona sin extracción (se capturan las líneas a mano), así que la ausencia de la llave
// es una feature apagada, no un error de arranque.
var ErrDocExtractDisabled = errors.New("extracción de documentos no configurada (falta ANTHROPIC_API_KEY)")

type PurchaseDocService struct {
	client anthropic.Client
	model  string
}

// NewPurchaseDocService devuelve nil cuando no hay llave: el llamador trata nil como
// "feature apagada" (ver Enabled).
func NewPurchaseDocService(apiKey, model string) *PurchaseDocService {
	if apiKey == "" {
		return nil
	}
	return &PurchaseDocService{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (s *PurchaseDocService) Enabled() bool { return s != nil }

// Extract manda el documento al modelo y devuelve el borrador estructurado más el JSON crudo.
//
// El crudo se conserva a propósito: cuando un proveedor nuevo traiga un dato que el schema de
// hoy no modela, ya lo tenemos guardado y se puede re-interpretar sin volver a pagar la
// llamada ni pedirle el papel otra vez al dueño.
func (s *PurchaseDocService) Extract(ctx context.Context, filename string, data []byte) (domain.PurchaseDoc, json.RawMessage, error) {
	var doc domain.PurchaseDoc
	if !s.Enabled() {
		return doc, nil, ErrDocExtractDisabled
	}
	if len(data) == 0 {
		return doc, nil, domain.ErrValidation
	}
	if len(data) > maxDocBytes {
		return doc, nil, fmt.Errorf("%w: el archivo pesa %d MB y el máximo es %d MB",
			domain.ErrValidation, len(data)>>20, maxDocBytes>>20)
	}
	block, err := documentBlock(filename, data)
	if err != nil {
		return doc, nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docExtractTimeout)
	defer cancel()

	// Sin thinking ni effort: el modelo es configurable por env y no todos los soportan
	// (Haiku los rechaza con 400). Una extracción con schema estricto no los necesita.
	msg, err := s.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.model),
		MaxTokens: maxOutputTokens,
		System:    []anthropic.TextBlockParam{{Text: extractSystemPrompt}},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: purchaseDocSchema()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(block, anthropic.NewTextBlock(extractUserPrompt)),
		},
	}, option.WithRequestTimeout(docExtractTimeout))
	if err != nil {
		return doc, nil, fmt.Errorf("extracción del documento: %w", err)
	}
	// El schema garantiza que el primer bloque de texto es JSON válido, pero un refusal o un
	// corte por max_tokens devuelven otra cosa: hay que mirar stop_reason antes del contenido.
	if msg.StopReason == anthropic.StopReasonRefusal {
		return doc, nil, fmt.Errorf("%w: el modelo rechazó el documento", domain.ErrValidation)
	}
	if msg.StopReason == anthropic.StopReasonMaxTokens {
		return doc, nil, errors.New("el documento excede la salida máxima: súbelo por partes")
	}
	raw := firstText(msg)
	if raw == "" {
		return doc, nil, errors.New("el modelo no devolvió contenido")
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, nil, fmt.Errorf("respuesta del modelo ilegible: %w", err)
	}
	// El crudo se devuelve tal como vino; el saneado aplica solo al borrador que se le muestra
	// al operador, para que la evidencia original quede intacta.
	//
	// Normalizar va PRIMERO: lo demás compara sumas, y un "1,234.50" sin normalizar se descarta
	// como ilegible y hace que el documento "no cuadre" por un separador de miles.
	doc.NormalizeAmounts()
	doc.PickPriceColumn()
	doc.DropAmbiguousCodes()
	doc.EnsureLists()
	return doc, json.RawMessage(raw), nil
}

// documentBlock arma el bloque según el tipo real del archivo. Se detecta por contenido y no
// por extensión: el nombre lo pone quien sube el archivo y puede mentir.
func documentBlock(filename string, data []byte) (anthropic.ContentBlockParamUnion, error) {
	b64 := base64.StdEncoding.EncodeToString(data)
	mime := http.DetectContentType(data)
	switch {
	case strings.HasPrefix(mime, "application/pdf"):
		return anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: b64}), nil
	case mime == "image/jpeg", mime == "image/png", mime == "image/gif", mime == "image/webp":
		return anthropic.NewImageBlockBase64(mime, b64), nil
	default:
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf(
			"%w: %s no es PDF ni imagen (detectado %s)", domain.ErrValidation, filename, mime)
	}
}

func firstText(m *anthropic.Message) string {
	for _, b := range m.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			return t.Text
		}
	}
	return ""
}

// El prompt describe FENÓMENOS, no proveedores: "varias columnas de precio", "renglones que
// no son mercancía", "cantidades fraccionarias". Así un proveedor nuevo no requiere editarlo.
// Cada regla está aquí porque un documento real la rompía.
const extractSystemPrompt = `Extraes datos de documentos de compra de un restaurante: tickets de tienda, facturas y pedidos web de cualquier proveedor. Devuelves SIEMPRE la misma estructura JSON.

Reglas:

1. IMPORTES: transcribe lo que se COBRÓ, no el precio de lista. Muchos documentos imprimen varias columnas o varios precios por renglón (precio normal, precio de promoción, precio final; o precio con impuesto y precio sin impuesto). Copia los números tal como aparecen, sin recalcular.

1a. amount es el IMPORTE DEL RENGLÓN y solo se llena si el documento lo imprime. Muchos documentos imprimen únicamente el precio por unidad (marcado "c/u", "cada uno" o similar) y no el importe de la línea: en ese caso pon el precio en unitPrice, la cantidad en qty, y DEJA amount VACÍO. El sistema multiplica. No multipliques tú.

1b. CUANDO UN RENGLÓN IMPRIME DOS PRECIOS (por ejemplo con impuesto y sin impuesto, o lista y promoción), transcribe LOS DOS: el que creas cobrado en unitPrice/amount, y el otro en unitPriceAlt/amountAlt. No intentes decidir cuál cuadra con el total — de eso se encarga el sistema después comparando contra el subtotal impreso. Si el renglón solo trae un precio, deja los campos Alt vacíos.

2. NO INVENTES NADA. Si un dato no está en el documento, deja el campo como cadena vacía. Es correcto y esperado devolver campos vacíos. Nunca deduzcas una fecha, un total ni un código que no esté impreso. Si la fecha viene incompleta (por ejemplo un día y mes sin año), deja issuedOn vacío y anota lo que decía en extra.

3. RENGLONES QUE NO SON MERCANCÍA van en "charges", no en "lines": impuestos, envío, descuentos, bolsas, propinas, donativos, redondeos. Los descuentos van con importe negativo. Usa como label el texto literal del documento.

3b. affectsTotal indica si ese cargo MUEVE el total o solo lo DESGLOSA:
   - affectsTotal=true cuando sumarlo (o restarlo) es necesario para llegar al total: descuentos, envío, bolsas, propinas, donativos.
   - affectsTotal=false cuando es un desglose de algo YA incluido en los importes de los renglones. Los tickets de mostrador en México imprimen los precios con impuesto incluido y luego listan "TOTAL IVA" o "IEPS" como información: ahí affectsTotal=false. La pista es aritmética: si la suma de los renglones ya da el total por sí sola, entonces cualquier impuesto listado es desglose, no cargo.

4. CANTIDADES: pueden ser fraccionarias cuando el artículo se vende por peso (por ejemplo 0.280 con unidad kg). Respeta los decimales. Si el documento no imprime cantidad, deja qty vacío.

5. ESTADO DEL RENGLÓN: los pedidos marcan renglones que no se surtieron o que se surtieron con otra cantidad. Usa "no_disponible" cuando el documento indica que el artículo no se entregó, "ajustado" cuando indica que la cantidad o el peso cambió respecto a lo pedido, y "comprado" cuando confirma la entrega. Si el documento no dice nada del renglón, deja el estado vacío.

6. rawCode: solo el código que identifica ese artículo específico. Muchos tickets imprimen un número que en realidad es el departamento y se repite entre artículos distintos: si ves el mismo código en renglones con nombres de productos diferentes, NO es un identificador de artículo — déjalo vacío y menciónalo en warnings.

7. rawName: el texto del renglón tal cual, incluso si viene truncado o abreviado. No lo corrijas ni lo expandas.

8. packQty/packUnit: el contenido de UNA unidad de compra cuando el nombre del artículo lo indica ("... 432 g" → 432 y "g"; "... 5.1 l" → 5.1 y "l"; "2 KG FRESA" → 2 y "kg"; "6/186GR" → 1116 y "g" solo si es claramente 6 piezas de 186 g, si dudas déjalo vacío). Si el nombre no lo dice, vacío.

9. suggestedName: el nombre del artículo limpio y buscable, en minúsculas, sin marca comercial ni gramaje, para darlo de alta en un catálogo de inventario ("Harina para pastel Great Value vainilla 432 g" → "harina para pastel vainilla"). Si el nombre original está tan truncado que no se entiende qué es, déjalo vacío.

10. warnings: todo lo que no pudiste leer con confianza, en español y por renglón. Es preferible una advertencia a un dato inventado.

11. extra: cualquier otro dato del documento que la estructura no contemple (RFC, sucursal, terminal, caja, número de socio, cliente, régimen fiscal, texto de la fecha si venía incompleta). No descartes información.

12. currency: código ISO 4217 de tres letras (MXN, USD). Un símbolo como "$" no es un código: si el documento solo trae el símbolo, usa MXN.`

const extractUserPrompt = `Extrae este documento de compra a la estructura JSON indicada. Transcribe, no interpretes: campos vacíos donde el documento no diga nada.`

// purchaseDocSchema describe la salida. Los structured outputs exigen additionalProperties:false
// y todas las propiedades en required, así que cada campo viene SIEMPRE presente (vacío cuando
// no aplica). Eso es justo lo que hace la estructura homogénea entre proveedores: el consumidor
// nunca pregunta si el campo existe.
func purchaseDocSchema() map[string]any {
	str := map[string]any{"type": "string"}
	obj := func(props map[string]any) map[string]any {
		req := make([]string, 0, len(props))
		for k := range props {
			req = append(req, k)
		}
		// El orden de required no importa para la validación, pero un orden estable mantiene el
		// schema byte-idéntico entre llamadas y así el prompt caching puede reusar el prefijo.
		slices.Sort(req)
		return map[string]any{
			"type": "object", "properties": props, "required": req, "additionalProperties": false,
		}
	}
	arr := func(items map[string]any) map[string]any {
		return map[string]any{"type": "array", "items": items}
	}
	// extra es lista de pares y no un objeto libre porque additionalProperties:false prohíbe
	// claves arbitrarias; la lista deja pasar datos que no anticipamos sin romper el schema.
	keyValue := obj(map[string]any{"key": str, "value": str})

	return obj(map[string]any{
		"kind": map[string]any{
			"type": "string", "enum": []string{"ticket", "factura", "pedido", "otro"},
		},
		"supplier": str,
		"folio":    str,
		"issuedOn": map[string]any{
			"type":        "string",
			"description": "YYYY-MM-DD, o cadena vacía si el documento no trae fecha completa",
		},
		"currency": str,
		"subtotal": str,
		"total":    str,
		"lines": arr(obj(map[string]any{
			"rawCode":       str,
			"rawName":       str,
			"qty":           str,
			"unit":          str,
			"unitPrice":     str,
			"amount":        str,
			"unitPriceAlt":  str,
			"amountAlt":     str,
			"packQty":       str,
			"packUnit":      str,
			"suggestedName": str,
			"note":          str,
			"status": map[string]any{
				"type": "string", "enum": []string{"comprado", "no_disponible", "ajustado", ""},
			},
		})),
		"charges": arr(obj(map[string]any{
			"label":  str,
			"amount": str,
			"affectsTotal": map[string]any{
				"type":        "boolean",
				"description": "true si el cargo mueve el total; false si solo desglosa algo ya incluido en los renglones",
			},
		})),
		"payments": arr(obj(map[string]any{"method": str, "amount": str, "reference": str})),
		"extra":    arr(keyValue),
		"warnings": arr(str),
	})
}
