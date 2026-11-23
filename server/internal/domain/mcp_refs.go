package domain

import "fmt"

// LO QUE UNA ESCRITURA NOMBRA POR ID (#335).
//
// Cada herramienta de escritura declara los ids que manda; la capa del MCP revisa, con RLS, que sean de
// la empresa del token antes de la vista previa y antes de aplicar. Hacía falta porque la vista previa
// no despacha la escritura (decía que todo estaba bien con un producto ajeno) y porque varias llaves
// foráneas son de una sola columna: Postgres las revisa saltándose RLS, así que un id de otra empresa
// pasaba y la escritura quedaba apuntando a una fila ajena.

// RefKind es la clase de registro que nombra un id.
type RefKind string

const (
	RefProduct         RefKind = "product"
	RefIngredient      RefKind = "ingredient"
	RefOption          RefKind = "option"
	RefGroup           RefKind = "group"
	RefCategory        RefKind = "category"
	RefPresentation    RefKind = "presentation"
	RefCount           RefKind = "count"
	RefExpense         RefKind = "expense"
	RefExpenseCategory RefKind = "expense_category"
	RefSupplier        RefKind = "supplier"
	RefPaymentMethod   RefKind = "payment_method"
	RefRegister        RefKind = "register"
	RefCashConcept     RefKind = "cash_concept"
	RefWasteReason     RefKind = "waste_reason"
	RefOrder           RefKind = "order"
	RefPlatform        RefKind = "platform"
	RefWarehouse       RefKind = "warehouse"
	RefTerminal        RefKind = "terminal"
	RefIngredientCat   RefKind = "ingredient_category"
	RefPlace           RefKind = "place"
)

// Ref es un id que manda una escritura. WarnInactive pide que la vista previa avise si el producto
// está inactivo: cargar existencias de uno que no sale en la venta ni en el conteo confunde.
type Ref struct {
	Kind         RefKind
	ID           int64
	WarnInactive bool
}

type refInfo struct {
	noun     string // con artículo: «el producto», «la caja»
	listTool string // la lectura con la que se busca el id bueno
	table    string // la tabla que nombra Postgres cuando la FK lo frena
}

var refKinds = map[RefKind]refInfo{
	RefProduct:         {"el producto", "products_list", "products"},
	RefIngredient:      {"el insumo", "articles_search", "ingredients"},
	RefOption:          {"el extra", "modifier_options_list", "modifier_options"},
	RefGroup:           {"el grupo", "modifier_groups_list", "modifier_groups"},
	RefCategory:        {"la categoría", "categories_list", "categories"},
	RefPresentation:    {"el empaque", "presentations_list", "ingredient_presentations"},
	RefCount:           {"el conteo", "stock_count_current", "stock_counts"},
	RefExpense:         {"el gasto", "expenses_list", "expenses"},
	RefExpenseCategory: {"la categoría de gasto", "expense_categories_list", "expense_categories"},
	RefSupplier:        {"el proveedor", "suppliers_list", "suppliers"},
	RefPaymentMethod:   {"el método de pago", "payment_methods_list", "payment_methods"},
	RefRegister:        {"la caja", "cash_registers_list", "cash_registers"},
	RefCashConcept:     {"el concepto", "cash_concepts_list", "cash_concepts"},
	RefWasteReason:     {"el motivo de merma", "waste_reasons_list", "waste_reasons"},
	RefOrder:           {"el pedido", "sales_list", "orders"},
	RefPlatform:        {"la plataforma", "platform_prices_list", "delivery_platforms"},
	RefWarehouse:       {"el almacén", "warehouses_list", "warehouses"},
	RefTerminal:        {"la terminal", "card_terminals_list", "card_terminals"},
	RefIngredientCat:   {"la categoría de insumo", "articles_search", "ingredient_categories"},
	RefPlace:           {"el lugar", "articles_search", "count_places"},
}

// MissingRefMessage es el rechazo de un id que no es de la empresa. Es el mismo para uno que existe en
// otra empresa y uno que no existe en ninguna: RLS no deja ver la diferencia y el texto no la inventa.
func MissingRefMessage(r Ref) string {
	info := refKinds[r.Kind]
	return fmt.Sprintf("%s %d no existe en esta empresa. Búscalo con %s.", capitalize(info.noun), r.ID, info.listTool)
}

// InactiveProductNote es el aviso de la vista previa para un producto inactivo.
func InactiveProductNote(id int64, name string) string {
	return fmt.Sprintf("El producto %d («%s») está inactivo: no sale en la venta ni en stock_count_open. "+
		"Si se va a volver a vender, reactívalo con product_update y active: true.", id, name)
}

// ReferenceNotFound es el rechazo de la base a una referencia que no existe en la empresa: la FK
// compuesta lo frena con un 23503 que sin traducir respondía 500 «Error interno». Envuelve el error
// de la base para que quien ya lo distingue (isForeignKeyViolation) lo siga viendo.
type ReferenceNotFound struct {
	noun  string
	cause error
}

func (e ReferenceNotFound) Error() string {
	if e.noun == "" {
		return "uno de los registros que mandaste no existe en esta empresa"
	}
	return e.noun + " que mandaste no existe en esta empresa"
}

func (e ReferenceNotFound) Unwrap() error { return e.cause }

// ReferenceNotFoundIn arma el rechazo a partir de la tabla que nombró Postgres. Una tabla que no es
// de las conocidas da el texto genérico: su nombre es un interno que no se le dice a quien opera.
func ReferenceNotFoundIn(table string) ReferenceNotFound {
	return ReferenceNotFoundWrapping(table, nil)
}

// ReferenceNotFoundWrapping es ReferenceNotFoundIn con el error de la base adentro.
func ReferenceNotFoundWrapping(table string, cause error) ReferenceNotFound {
	for _, info := range refKinds {
		if info.table == table {
			return ReferenceNotFound{noun: info.noun, cause: cause}
		}
	}
	return ReferenceNotFound{cause: cause}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}
