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
	// ModeUndecided: nadie ha dicho qué resta. Es un problema; ModeNone no.
	ModeUndecided ConsumptionMode = "undecided"
)

// ProductModeFacts son las columnas de un producto que deciden su modo.
type ProductModeFacts struct {
	TrackStock        bool
	HasRecipeItems    bool
	IsPackage         bool
	CompositionStatus string
}

// ModeOf deduce el modo en el mismo orden que ExpandSale: la existencia propia gana sobre una
// receta guardada al lado.
func ModeOf(p ProductModeFacts) ConsumptionMode {
	switch {
	case p.TrackStock:
		return ModeOwnStock
	case p.IsPackage:
		return ModePackage
	case p.HasRecipeItems:
		return ModeRecipe
	case p.CompositionStatus == "confirmed":
		return ModeNone
	}
	return ModeUndecided
}
