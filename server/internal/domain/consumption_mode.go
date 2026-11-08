package domain

// ConsumptionMode es qué se resta al vender un producto (spec 037). No se guarda: se deduce de lo que
// usa ExpandSale, para que la pantalla y la venta no puedan contradecirse.
type ConsumptionMode string

const (
	// ModeOwnStock es la existencia del producto mismo. Solo se lee: ya no se ofrece como opción,
	// los productos que se compran hechos son receta de un insumo por pieza.
	ModeOwnStock ConsumptionMode = "own_stock"
	ModeRecipe   ConsumptionMode = "recipe"
	ModePackage  ConsumptionMode = "package"
	// ModeNone: confirmado a propósito que no gasta insumos.
	ModeNone ConsumptionMode = "none"
	// ModeOptions: el producto no resta nada y lo restan sus opciones (cada sabor su lata, spec 069).
	// Se deduce del catálogo de hoy: si alguien apaga la última opción que resta, se lee ModeNone.
	ModeOptions ConsumptionMode = "options"
	// ModeUndecided: nadie ha dicho qué resta. Es un problema; ModeNone no.
	ModeUndecided ConsumptionMode = "undecided"
)

// ProductModeFacts son las columnas de un producto que deciden su modo.
type ProductModeFacts struct {
	TrackStock        bool
	HasRecipeItems    bool
	IsPackage         bool
	CompositionStatus string
	// OptionsDeduct: alguna opción activa, en un grupo ligado y activo, resta algo del almacén.
	OptionsDeduct bool
}

// ModeOf deduce el modo en el mismo orden que ExpandSale: un paquete se expande por sus piezas
// aunque traiga existencia propia, y la existencia propia gana sobre una receta guardada al lado.
func ModeOf(p ProductModeFacts) ConsumptionMode {
	switch {
	case p.IsPackage:
		return ModePackage
	case p.TrackStock:
		return ModeOwnStock
	case p.HasRecipeItems:
		return ModeRecipe
	case p.CompositionStatus == "confirmed" && p.OptionsDeduct:
		return ModeOptions
	case p.CompositionStatus == "confirmed":
		return ModeNone
	}
	return ModeUndecided
}

// CountsOwnStock dice si el producto entra al conteo de existencias: solo el que lleva su propia
// existencia. Los que restan por receta, paquete u opciones no se cuentan (decisión del issue #252).
func CountsOwnStock(p ProductModeFacts) bool {
	return ModeOf(p) == ModeOwnStock
}
