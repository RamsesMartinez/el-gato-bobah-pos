package domain

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Mapeo aprendido de "lo que dice el papel del proveedor" a "lo que hay en el inventario".
//
// Las descripciones de los tickets vienen truncadas y abreviadas ("MM 2K FRESA",
// "ACEITEVEGETA", "SALCH HOT DOG PAVO") y nunca coinciden literalmente con el nombre del
// ingrediente. NormalizeItemName produce la llave estable con la que se busca y se compara.

// Estados de una fila del catálogo por proveedor.
const (
	SupplierItemPendiente = "pendiente" // visto, sin decidir: es la cola de revisión
	SupplierItemMapeado   = "mapeado"   // apunta a un ingrediente o producto
	SupplierItemIgnorado  = "ignorado"  // decidido que no es inventariable (bolsa, envío, IVA)
	// SupplierItemPersonal: no es del local (el shampoo que se colgó de la compra del Sam's).
	// Separado de 'ignorado' porque una bolsa de plástico tampoco es inventariable pero SÍ es
	// gasto del negocio; esto no lo es, y su importe no debe sumar al gasto.
	SupplierItemPersonal = "personal"
)

// ValidSupplierItemStatus rechaza basura en la frontera.
func ValidSupplierItemStatus(s string) bool {
	switch s {
	case SupplierItemPendiente, SupplierItemMapeado, SupplierItemIgnorado, SupplierItemPersonal:
		return true
	}
	return false
}

// NormalizeItemName reduce el texto de un renglón a una llave comparable: minúsculas, sin
// acentos, sin signos y con los espacios colapsados.
//
// Es lo que hace que "COCA COLA 600ML" del ticket de una tienda empate con el "Coca Cola
// 600 ml" que se registró comprando en otra. Se conservan los dígitos porque el gramaje es
// justo lo que distingue dos presentaciones del mismo producto ("catsup 200 g" vs "catsup
// 3.8 l"), y el punto decimal por lo mismo.
func NormalizeItemName(s string) string {
	// NFD separa la letra de su diacrítico para poder descartar el diacrítico solo; se usa la
	// forma genérica en vez de una tabla de vocales españolas para que un nombre con ü, ç o un
	// símbolo de marca no se cuele sin normalizar.
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := true // arranca en true para no dejar espacio inicial
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r): // marca diacrítica: se descarta
			continue
		case r == '\'', r == '’': // apóstrofo: va DENTRO de la palabra en las marcas
			continue // ("Smucker's" → "smuckers", no "smucker s")
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.':
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// SupplierItemKey es la llave con la que se aprende: el código del proveedor cuando identifica
// al artículo, y si no el nombre normalizado. Refleja el índice único de supplier_items.
//
// Existe porque los códigos no son universales: un ticket de club de precios trae SKU único,
// una tienda de autoservicio trae código de departamento repetido (inútil como llave) y un
// pedido web no trae ninguno.
func SupplierItemKey(rawCode, rawName string) string {
	if c := strings.TrimSpace(rawCode); c != "" {
		return c
	}
	return NormalizeItemName(rawName)
}
