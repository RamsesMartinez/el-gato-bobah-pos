package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Densidad y tara de los artículos del almacén (spec 058). La densidad (g por ml) es lo único que
// permite pasar entre peso y líquido; nunca se asume 1 g = 1 ml, que es cierto para el agua y falso
// para el aceite y la miel.

var (
	// minDensity y maxDensity acotan lo posible para un alimento: aceites ~0.9, mieles ~1.4. Fuera de
	// ahí casi siempre es un dedazo de la pesada (gramos por kilos). Mismo rango que la columna.
	minDensity = decimal.RequireFromString("0.3")
	maxDensity = decimal.NewFromInt(3)
	// MaxTareG es el tope de lo que pesa vacío un empaque (100 kg), el mismo que la columna: sin él,
	// tara × 999 empaques desbordaba.
	MaxTareG = decimal.NewFromInt(100_000)
	// maxGrossContainers es cuántos empaques se pueden pesar juntos en una captura.
	maxGrossContainers = 999
)

// ErrDensityChanged: el cambio de unidad se confirmó con una equivalencia que ya no sale de la
// densidad vigente (alguien rehízo la pesada entre la vista previa y la confirmación).
var ErrDensityChanged = fmt.Errorf("%w: la densidad cambió mientras revisabas; vuelve a revisar", ErrConflict)

// ErrDensityInUse: hay una compra entre peso y líquido por recibir que la necesita para convertirse.
var ErrDensityInUse = fmt.Errorf("%w: hay una compra por recibir que la necesita para convertirse", ErrConflict)

// ErrGrossBelowTare: lo que marca la báscula es menos que los empaques vacíos que se dijo pesar.
var ErrGrossBelowTare = fmt.Errorf("%w: pesa menos que los empaques vacíos; revisa el peso o cuántos empaques", ErrValidation)

// ErrCountInputsChanged: la pantalla hizo la cuenta con una tara o una densidad que ya no son las
// vigentes. Se rechaza en vez de guardar una cifra distinta de la que se vio.
var ErrCountInputsChanged = fmt.Errorf("%w: lo que pesa vacío el empaque o la densidad cambiaron; vuelve a abrir el artículo", ErrConflict)

// UnitKindMismatch lleva el artículo de una compra entre peso y líquido sin densidad, para que la
// pantalla ofrezca capturarla ahí mismo.
type UnitKindMismatch struct {
	IngredientID int64
	Name         string
}

func (e UnitKindMismatch) Error() string {
	return fmt.Sprintf("%s: %s no tiene densidad para pasar entre peso y líquido", ErrUnitKindMismatch.Error(), e.Name)
}
func (e UnitKindMismatch) Unwrap() error { return ErrUnitKindMismatch }

// ValidDensity acepta una densidad en g/ml dentro de lo posible y con 4 decimales a lo más (los de la
// columna: uno más se guardaría distinto de lo que se capturó).
func ValidDensity(d decimal.Decimal) error {
	if !escalaSana(d) {
		// Sin repetir la cifra: un exponente absurdo volvía el mensaje de varios KB.
		return fmt.Errorf("%w: la densidad va de 0.30 a 3.00 gramos por ml", ErrValidation)
	}
	// «1.10000» es 1.1: los ceros de más no son decimales que se pierdan.
	if !d.Equal(d.Round(4)) || d.LessThan(minDensity) || d.GreaterThan(maxDensity) {
		return fmt.Errorf("%w: la densidad va de 0.30 a 3.00 gramos por ml; revisa la pesada: da %s", ErrValidation, d.String())
	}
	return nil
}

// DensityFromWeighing calcula la densidad de una pesada: (lleno − vacío) ÷ contenido, a 4 decimales.
func DensityFromWeighing(fullG, emptyG, contentMl decimal.Decimal) (decimal.Decimal, error) {
	if !escalaSana(fullG) || !escalaSana(emptyG) || emptyG.IsNegative() || !emptyG.LessThan(fullG) {
		return decimal.Zero, fmt.Errorf("%w: el envase vacío debe pesar menos que lleno", ErrValidation)
	}
	return densityOf(fullG.Sub(emptyG), contentMl)
}

// DensityFromLabel calcula la densidad del peso neto y el contenido de la etiqueta, para quien no
// tiene báscula: neto ÷ contenido, a 4 decimales.
func DensityFromLabel(netG, contentMl decimal.Decimal) (decimal.Decimal, error) {
	if !escalaSana(netG) || !netG.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: el peso neto debe ser mayor que cero", ErrValidation)
	}
	return densityOf(netG, contentMl)
}

func densityOf(netG, contentMl decimal.Decimal) (decimal.Decimal, error) {
	if !escalaSana(contentMl) || !contentMl.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: lo que contiene debe ser mayor que cero", ErrValidation)
	}
	d := netG.DivRound(contentMl, 4)
	if err := ValidDensity(d); err != nil {
		return decimal.Zero, err
	}
	return d, nil
}

// DensityFactor es la equivalencia de un cambio de unidad entre peso y líquido: cuánto de la base
// nueva vale una de la vieja. g → ml: 1 ÷ densidad; ml → g: la densidad. 6 decimales, los de la
// equivalencia de un cambio de unidad.
func DensityFactor(fromKind, toKind string, d decimal.Decimal) (decimal.Decimal, error) {
	switch {
	case fromKind == "masa" && toKind == "volumen":
		return decimal.NewFromInt(1).DivRound(d, 6), nil
	case fromKind == "volumen" && toKind == "masa":
		return d.Round(6), nil
	}
	return decimal.Zero, fmt.Errorf("%w: la densidad solo pasa entre peso y líquido", ErrValidation)
}

// ValidTare acepta lo que pesa vacío un empaque: mayor que cero, hasta 100 kg, a 2 decimales.
func ValidTare(g decimal.Decimal) error {
	if !escalaSana(g) || !g.Equal(g.Round(2)) || !g.IsPositive() || g.GreaterThan(MaxTareG) {
		return fmt.Errorf("%w: lo que pesa vacío va de 0.01 g a 100 kg", ErrValidation)
	}
	return nil
}

// GrossInput es una pesada del conteo: lo que marcó la báscula en g, lo que pesa vacío el empaque,
// cuántos empaques se pusieron y en qué se guarda el artículo.
type GrossInput struct {
	GrossG     decimal.Decimal
	TareG      decimal.Decimal
	Containers int
	BaseKind   string
	Density    *decimal.Decimal
}

// NetFromGross es lo pesado en la unidad base: (bruto − tara × empaques), en g, y a ml con la
// densidad si el artículo se guarda en ml. A 4 decimales, como lo contado. Un neto cero es un «no
// hay» válido; uno negativo es una pesada mal hecha y se rechaza en vez de contar de menos.
func NetFromGross(in GrossInput) (decimal.Decimal, error) {
	if in.Containers < 1 || in.Containers > maxGrossContainers {
		return decimal.Zero, fmt.Errorf("%w: los empaques pesados van de 1 a %d", ErrValidation, maxGrossContainers)
	}
	if err := ValidTare(in.TareG); err != nil {
		return decimal.Zero, err
	}
	if !escalaSana(in.GrossG) || in.GrossG.IsNegative() {
		return decimal.Zero, fmt.Errorf("%w: el peso de la báscula no es válido", ErrValidation)
	}
	net := in.GrossG.Sub(in.TareG.Mul(decimal.NewFromInt(int64(in.Containers))))
	if net.IsNegative() {
		return decimal.Zero, ErrGrossBelowTare
	}
	switch in.BaseKind {
	case "masa":
		return Round4(net), nil
	case "volumen":
		if in.Density == nil || !in.Density.IsPositive() {
			return decimal.Zero, ErrUnitKindMismatch
		}
		return net.DivRound(*in.Density, 4), nil
	}
	return decimal.Zero, fmt.Errorf("%w: solo un artículo en g o ml se cuenta pesando con empaque", ErrValidation)
}

// UnitChangeFactor es la equivalencia con que se cambia la unidad de un artículo. Entre peso y líquido
// la pone la densidad: un factor capturado a mano reabriría «1 g = 1 ml» por la puerta de atrás. Uno
// que viene de la vista previa tiene que coincidir con el derivado; si no, alguien rehízo la pesada
// mientras se revisaba (ErrDensityChanged). Los demás cambios siguen con la equivalencia capturada.
func UnitChangeFactor(fromKind, toKind string, factor decimal.Decimal, density *decimal.Decimal) (decimal.Decimal, error) {
	massVolume := (fromKind == "masa" && toKind == "volumen") || (fromKind == "volumen" && toKind == "masa")
	if !massVolume {
		return factor, ValidUnitChange(fromKind, toKind, factor)
	}
	if density == nil || !density.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: primero captura la densidad para pasar entre peso y líquido", ErrValidation)
	}
	derived, err := DensityFactor(fromKind, toKind, *density)
	if err != nil {
		return decimal.Zero, err
	}
	if !factor.IsZero() && !factor.Round(6).Equal(derived) {
		return decimal.Zero, ErrDensityChanged
	}
	return derived, nil
}

// ClosedFromWeighed es lo cerrado de una pesada ya guardada: lo contado menos lo pesado, con la tara y
// la densidad que se usaron al capturar (#204). ok es falso si esa cuenta ya no sale o deja cerrados
// negativos: en ese caso la pantalla muestra solo el total, en vez de un desglose inventado.
func ClosedFromWeighed(counted decimal.Decimal, in GrossInput) (decimal.Decimal, bool) {
	net, err := NetFromGross(in)
	if err != nil {
		return decimal.Zero, false
	}
	closed := Round4(counted.Sub(net))
	if closed.IsNegative() {
		return decimal.Zero, false
	}
	return closed, true
}
