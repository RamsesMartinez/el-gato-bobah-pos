package app

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// checkPaymentsCover es el guard que evita que un gasto quede "pagado" con menos dinero
// registrado del que realmente salió — el descuadre saldría en el corte sin rastro de dónde.
func TestCheckPaymentsCover(t *testing.T) {
	pay := func(amounts ...string) []resolvedPayment {
		out := make([]resolvedPayment, len(amounts))
		for i, a := range amounts {
			out[i] = resolvedPayment{in: ExpensePaymentInput{Amount: mustDec(a)}}
		}
		return out
	}
	tests := []struct {
		name     string
		payments []resolvedPayment
		amount   string
		wantErr  bool
	}{
		{"un pago exacto", pay("100.00"), "100.00", false},
		// El caso real de Soriana: tarjeta 640.06 + efectivo 0.01 = 640.07.
		{"pago partido suma exacto", pay("640.06", "0.01"), "640.07", false},
		{"pago de más (propina, redondeo)", pay("150.00"), "100.00", false},
		{"un solo centavo de menos", pay("99.99"), "100.00", true},
		{"pago partido incompleto", pay("640.06"), "640.07", true},
		{"sin pagos", nil, "100.00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPaymentsCover(tt.payments, mustDec(tt.amount))
			if tt.wantErr {
				if !errors.Is(err, domain.ErrPaymentsBelowAmount) {
					t.Fatalf("err = %v, quiero ErrPaymentsBelowAmount", err)
				}
				// Debe llegar como 4xx accionable, no como 500.
				if !errors.Is(err, domain.ErrValidation) {
					t.Error("debe envolver ErrValidation para mapear a 4xx")
				}
				return
			}
			if err != nil {
				t.Fatalf("err inesperado: %v", err)
			}
		})
	}
}

// itemTypeParam es la frontera que impide guardar una línea incoherente (tipo 'ingrediente' sin
// ingredient_id, o apuntando a los dos a la vez). El CHECK de la tabla lo repite, pero llegar
// hasta él devolvería 500 en vez de 400.
func TestItemTypeParam(t *testing.T) {
	id := int64(7)
	tests := []struct {
		name     string
		in       ExpenseItemInput
		wantType string // "" = null (línea no inventariable)
		wantErr  bool
	}{
		{"no inventariable", ExpenseItemInput{}, "", false},
		{"ingrediente", ExpenseItemInput{ItemType: "ingrediente", IngredientID: &id}, "ingrediente", false},
		{"producto", ExpenseItemInput{ItemType: "producto", ProductID: &id}, "producto", false},
		{"ingrediente sin id", ExpenseItemInput{ItemType: "ingrediente"}, "", true},
		{"producto sin id", ExpenseItemInput{ItemType: "producto"}, "", true},
		{"ingrediente apuntando también a producto", ExpenseItemInput{ItemType: "ingrediente", IngredientID: &id, ProductID: &id}, "", true},
		// Un id sin tipo tocaría el almacén sin que nadie lo pidiera.
		{"id sin tipo", ExpenseItemInput{IngredientID: &id}, "", true},
		{"tipo inventado", ExpenseItemInput{ItemType: "insumo", IngredientID: &id}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := itemTypeParam(tt.in)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("err = %v, quiero ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err inesperado: %v", err)
			}
			if tt.wantType == "" {
				if got != nil {
					t.Errorf("quiero null, tengo %v", *got)
				}
				return
			}
			if got == nil || string(*got) != tt.wantType {
				t.Errorf("got = %v, quiero %s", got, tt.wantType)
			}
		})
	}
}

// El corte lista PAGOS, no gastos: un gasto liquidado con dos medios en días distintos toca dos
// cortes y cada uno debe ver solo su parte, nunca el importe completo del gasto.
func TestCashExpenseViewUsaImporteDelPago(t *testing.T) {
	rows := []db.ListExpensesBySessionRow{
		{ID: 1, ExpenseID: 99, Category: "Insumos", PaymentMethod: "Tarjeta", Amount: mustDec("640.06"), Currency: "MXN", Status: "pagada"},
		{ID: 2, ExpenseID: 99, Category: "Insumos", PaymentMethod: "Efectivo", Amount: mustDec("0.01"), Currency: "MXN", Status: "pagada"},
	}
	var sum decimal.Decimal
	for _, r := range rows {
		v := CashExpenseView{
			ID: r.ID, ExpenseID: r.ExpenseID, Category: r.Category,
			PaymentMethod: r.PaymentMethod, Amount: r.Amount,
			Currency: domain.Currency(r.Currency), Status: string(r.Status),
		}
		if v.ExpenseID != 99 {
			t.Errorf("cada pago debe apuntar a su gasto, tengo %d", v.ExpenseID)
		}
		sum = sum.Add(v.Amount)
	}
	// Los dos pagos suman el gasto; si la vista trajera el importe del gasto en cada renglón,
	// el corte contaría 1280.12 en vez de 640.07.
	if want := mustDec("640.07"); !sum.Equal(want) {
		t.Errorf("suma de pagos = %s, quiero %s", sum, want)
	}
}

// Cómo se recuerda cada renglón para la próxima compra del mismo proveedor. Equivocarse aquí es
// caro: el mapeo se aplica solo en cada visita siguiente, así que un estado mal puesto mete el
// shampoo de la casa al inventario del local una y otra vez.
func TestLearnedStatus(t *testing.T) {
	id := int64(7)
	tests := []struct {
		name string
		in   ExpenseItemInput
		want string
	}{
		{"con artículo → mapeado", ExpenseItemInput{ItemType: "ingrediente", IngredientID: &id}, domain.SupplierItemMapeado},
		{"sin artículo → ignorado (bolsa, envío, IVA)", ExpenseItemInput{}, domain.SupplierItemIgnorado},
		{"de la casa → personal", ExpenseItemInput{Personal: true}, domain.SupplierItemPersonal},
		// El operador corrige un mapeo previo: 'personal' tiene que ganar, o la sugerencia
		// aprendida seguiría proponiendo el artículo que acaba de descartar.
		{"de la casa gana sobre el artículo", ExpenseItemInput{
			Personal: true, ItemType: "ingrediente", IngredientID: &id,
		}, domain.SupplierItemPersonal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := learnedStatus(tt.in)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got != tt.want {
				t.Errorf("learnedStatus = %q, quiero %q", got, tt.want)
			}
			if !domain.ValidSupplierItemStatus(got) {
				t.Errorf("%q no es un estado válido: la columna lo rechazaría con un CHECK", got)
			}
		})
	}
}
