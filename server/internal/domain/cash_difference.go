package domain

import "github.com/shopspring/decimal"

// MethodDiff es un renglón de la conciliación de un corte: lo que el sistema esperaba por un método
// de pago y lo declarado al cerrar. TouchesDrawer es el snapshot de `affects_cash_drawer`.
type MethodDiff struct {
	Expected      decimal.Decimal
	Declared      decimal.Decimal
	TouchesDrawer bool
}

// DrawerCount es el arqueo de cierre del cajón. Expected es nil en los conteos de antes de la 0067,
// que no guardaban el esperado.
type DrawerCount struct {
	Expected *decimal.Decimal
	Counted  decimal.Decimal
}

// CutDiff es la diferencia de un corte en sus tres partes. Drawer es nil cuando el corte no tiene
// arqueo con esperado; DrawerCovered dice que los métodos del cajón se leen en el arqueo («ver
// arqueo») y no en la conciliación.
type CutDiff struct {
	Drawer        *decimal.Decimal `json:"drawer"`
	OtherMethods  decimal.Decimal  `json:"otherMethods"`
	Total         decimal.Decimal  `json:"total"`
	DrawerCovered bool             `json:"drawerCovered"`
}

// CutDifference es la única regla de la diferencia de un corte: la usan la lista del Histórico, el
// detalle y el cierre (#290; constitución III, «lista y resumen del mismo predicado»).
//
// Desde la 0067 el cierre guarda `declared = expected` en los métodos del cajón y la diferencia del
// cajón vive solo en el arqueo. Por eso, cuando hay arqueo con esperado, los métodos del cajón no
// suman a la conciliación: sumarlos contaría dos veces un faltante que llegara a quedar en los dos
// lados. Sin arqueo con esperado (cortes anteriores a la 0067) la conciliación es todo lo que hay, y
// el efectivo declarado sí cuenta ahí.
func CutDifference(methods []MethodDiff, drawer *DrawerCount) CutDiff {
	var out CutDiff
	if drawer != nil && drawer.Expected != nil {
		dif := Round2(drawer.Counted.Sub(*drawer.Expected))
		out.Drawer, out.DrawerCovered = &dif, true
	}
	other := decimal.Zero
	for _, m := range methods {
		if out.DrawerCovered && m.TouchesDrawer {
			continue
		}
		other = other.Add(m.Declared.Sub(m.Expected))
	}
	out.OtherMethods = Round2(other)
	out.Total = out.OtherMethods
	if out.Drawer != nil {
		out.Total = Round2(out.Total.Add(*out.Drawer))
	}
	return out
}

// CutNeedsExplanation dice si un corte queda «por explicar» (#290, decisión P1): su diferencia pasa
// la tolerancia de la empresa y nadie la explicó, ni con una nota vigente ni con el motivo de la
// apertura siguiente. No bloquea nada: marca el corte para que lo revise quien ajusta el arqueo.
// Se calcula con la tolerancia de HOY: cambiarla reclasifica cortes pasados (es estado derivado).
func CutNeedsExplanation(total, tolerance decimal.Decimal, hasNote, nextOpeningReason bool) bool {
	return total.Abs().GreaterThan(tolerance) && !hasNote && !nextOpeningReason
}

// ValidTolerance valida la tolerancia que se guarda. Ausente no es $0: un campo borrado a media
// captura apagaba el control sin que nadie lo decidiera.
func ValidTolerance(v *decimal.Decimal) (decimal.Decimal, error) {
	if v == nil {
		return decimal.Zero, ErrValidation
	}
	t := Round2(*v)
	if !ValidMoney(t, true) || t.IsNegative() {
		return decimal.Zero, ErrValidation
	}
	return t, nil
}
