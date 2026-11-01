package domain

import (
	"errors"
	"testing"
)

func TestCanMoveSales(t *testing.T) {
	sells := SellingRegister{ID: 1, BranchID: 10, Active: true}
	other := SellingRegister{ID: 2, BranchID: 10, Active: true}
	tests := []struct {
		name            string
		current, target SellingRegister
		changes         bool
		err             error
	}{
		{"las dos cerradas, misma sucursal: se pasa", sells, other, true, nil},
		{"la que ya vende: no cambia nada", sells, sells, false, nil},
		{"la que vende con turno abierto", SellingRegister{ID: 1, BranchID: 10, Active: true, HasOpenShift: true}, other, false, ErrRegistersMustBeClosed},
		{"la nueva con turno abierto", sells, SellingRegister{ID: 2, BranchID: 10, Active: true, HasOpenShift: true}, false, ErrRegistersMustBeClosed},
		{"de otra sucursal", sells, SellingRegister{ID: 2, BranchID: 11, Active: true}, false, ErrOtherBranch},
		{"desactivada", sells, SellingRegister{ID: 2, BranchID: 10}, false, ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes, err := CanMoveSales(tt.current, tt.target)
			if changes != tt.changes {
				t.Errorf("changes = %v; quiere %v", changes, tt.changes)
			}
			if tt.err == nil && err != nil || tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("err = %v; quiere %v", err, tt.err)
			}
		})
	}
	// Los dos sentinels se distinguen en la frontera: uno es 409 (estado del negocio) y otro 400.
	if !errors.Is(ErrRegistersMustBeClosed, ErrConflict) || !errors.Is(ErrOtherBranch, ErrValidation) {
		t.Fatal("ErrRegistersMustBeClosed debe envolver ErrConflict y ErrOtherBranch, ErrValidation")
	}
}
