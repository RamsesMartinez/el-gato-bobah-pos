//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Un renglón que no es del almacén, ni regalo ni excluido, con importe 0 es un error de captura
// (FR-004a: cada renglón lleva su importe), y la base lo rechaza con expense_items_amount_positive.
// El servicio lo dejaba pasar y el operador veía «Error interno» al guardar un gasto con mercancía
// en el que quedó el renglón vacío que la pantalla agrega al abrir la captura.
func TestALineWithoutAmountIsAValidationErrorNotA500(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	in := app.ExpenseInput{
		ReceivedAt: "2026-07-20", CategoryID: k.category, Amount: dd("100"), Status: domain.ExpensePendiente,
		UserID: k.user,
		Items:  []app.ExpenseItemInput{{Description: "Sin descripción", Quantity: dd("1"), Amount: dd("0")}},
	}
	if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("guardar con un renglón sin importe = %v; quiere ErrValidation (400), no el CHECK de la base (500)", err)
	}
	if _, err := k.svc.PreviewPurchase(k.ctx, in); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("vista previa con un renglón sin importe = %v; quiere ErrValidation", err)
	}

	// Lo que sí se acepta con 0: el mismo renglón excluido (evidencia, spec 049).
	in.Items[0].Excluded = true
	if _, err := k.svc.CreateExpense(k.ctx, in); err != nil {
		t.Errorf("un excluido con importe 0 se guarda: %v", err)
	}
}
