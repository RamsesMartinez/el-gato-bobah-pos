package domain

import "fmt"

var (
	// ErrRegistersMustBeClosed: pasar la venta con un turno abierto partiría ese turno entre dos
	// cajas, y su corte dejaría de cuadrar.
	ErrRegistersMustBeClosed = fmt.Errorf("%w: cierra las dos cajas primero", ErrConflict)
	// ErrOtherBranch: la caja es de otra sucursal; cada sucursal tiene su propia caja que vende.
	ErrOtherBranch = fmt.Errorf("%w: esa caja es de otra sucursal", ErrValidation)
)

// SellingRegister es lo que importa de una caja para decidir si puede pasar a vender.
type SellingRegister struct {
	ID           int64
	BranchID     int64
	Active       bool
	HasOpenShift bool
}

// CanMoveSales decide si la venta pasa de `current` (la que vende hoy) a `target`. `changes` es
// falso cuando target ya es la que vende: tocarla dos veces no es un error.
func CanMoveSales(current, target SellingRegister) (changes bool, err error) {
	if target.ID == current.ID {
		return false, nil
	}
	if target.BranchID != current.BranchID {
		return false, ErrOtherBranch
	}
	if !target.Active {
		return false, fmt.Errorf("%w: esa caja está desactivada", ErrValidation)
	}
	if current.HasOpenShift || target.HasOpenShift {
		return false, ErrRegistersMustBeClosed
	}
	return true, nil
}
