package httpapi

import (
	"strconv"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Los ids que manda cada escritura del MCP (#335). apply los revisa contra la empresa del token antes
// de la vista previa y antes de aplicar. Un id que no es mayor que cero no se revisa: lo rechaza antes
// la validación de argumentos de cada herramienta. Las unidades y las denominaciones son catálogos del
// sistema, iguales en toda empresa, y no van aquí.
//
// TestEveryWriteToolWithIDsDeclaresItsRefs vigila que una herramienta nueva que recibe ids no se quede
// sin su función.

type refList []domain.Ref

func (l *refList) add(kind domain.RefKind, id int64) {
	if id > 0 {
		*l = append(*l, domain.Ref{Kind: kind, ID: id})
	}
}

func (l *refList) opt(kind domain.RefKind, id *int64) {
	if id != nil {
		l.add(kind, *id)
	}
}

// product agrega un producto con el aviso de inactivo en la vista previa.
func (l *refList) product(id int64) {
	if id > 0 {
		*l = append(*l, domain.Ref{Kind: domain.RefProduct, ID: id, WarnInactive: true})
	}
}

func (l *refList) payments(ps []paymentArg) {
	for _, p := range ps {
		l.add(domain.RefPaymentMethod, int64(p.MethodID))
		l.opt(domain.RefRegister, p.RegisterID)
	}
}

// keys agrega las llaves numéricas de un mapa {id: monto}; una que no es número la rechaza la ruta.
func (l *refList) keys(kind domain.RefKind, m map[string]string) {
	for k := range m {
		if id, err := strconv.ParseInt(k, 10, 64); err == nil {
			l.add(kind, id)
		}
	}
}

func refsCashMovement(in cashMovementArgs) []domain.Ref {
	var l refList
	l.add(domain.RefRegister, in.RegisterID)
	l.opt(domain.RefCashConcept, in.ConceptID)
	return l
}

func refsExpenseCreate(in expenseCreateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefExpenseCategory, in.CategoryID)
	l.opt(domain.RefSupplier, in.SupplierID)
	l.payments(in.Payments)
	return l
}

func refsExpensePay(in expensePayArgs) []domain.Ref {
	var l refList
	l.add(domain.RefExpense, in.ExpenseID)
	l.payments([]paymentArg{in.paymentArg})
	return l
}

func refsExpenseCancel(in expenseCancelArgs) []domain.Ref {
	var l refList
	l.add(domain.RefExpense, in.ExpenseID)
	return l
}

func refsWaste(in wasteArgs) []domain.Ref {
	var l refList
	l.add(domain.RefWasteReason, in.ReasonID)
	l.opt(domain.RefIngredient, in.IngredientID)
	if in.ProductID != nil {
		l.product(*in.ProductID)
	}
	return l
}

func refsStockAdjust(in stockAdjustArgs) []domain.Ref {
	var l refList
	l.opt(domain.RefIngredient, in.IngredientID)
	if in.ProductID != nil {
		l.product(*in.ProductID)
	}
	l.opt(domain.RefWarehouse, in.WarehouseID)
	return l
}

func refsProductUpdate(in productUpdateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefProduct, in.ProductID)
	l.opt(domain.RefCategory, in.CategoryID)
	return l
}

func refsOptionUpdate(in optionUpdateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOption, in.OptionID)
	return l
}

func refsPlatformPrice(in platformPriceArgs) []domain.Ref {
	var l refList
	l.add(domain.RefPlatform, int64(in.PlatformID))
	l.add(domain.RefProduct, in.ProductID)
	l.add(domain.RefOption, in.OptionID)
	return l
}

func (l *refList) purchaseLine(ingredient, product, presentation *int64) {
	l.opt(domain.RefIngredient, ingredient)
	if product != nil {
		l.product(*product)
	}
	l.opt(domain.RefPresentation, presentation)
}

func refsPurchaseCreate(in purchaseCreateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefExpenseCategory, in.CategoryID)
	l.opt(domain.RefSupplier, in.SupplierID)
	l.payments(in.Payments)
	for _, ln := range in.Lines {
		l.purchaseLine(ln.IngredientID, ln.ProductID, ln.PresentationID)
	}
	return l
}

func refsPurchaseReceive(in purchaseReceiveArgs) []domain.Ref {
	var l refList
	l.add(domain.RefExpense, in.ExpenseID)
	return l
}

func refsCountCapture(in countCaptureArgs) []domain.Ref {
	var l refList
	l.add(domain.RefCount, in.CountID)
	for _, ln := range in.Lines {
		l.opt(domain.RefIngredient, ln.IngredientID)
		if ln.ProductID != nil {
			l.product(*ln.ProductID)
		}
	}
	return l
}

func refsCountID(in countIDArgs) []domain.Ref {
	var l refList
	l.add(domain.RefCount, in.CountID)
	return l
}

func refsOrderCancel(in orderCancelArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	return l
}

func refsCashOpen(in cashOpenArgs) []domain.Ref {
	var l refList
	l.add(domain.RefRegister, in.RegisterID)
	return l
}

func refsCashClose(in cashCloseArgs) []domain.Ref {
	var l refList
	l.add(domain.RefRegister, in.RegisterID)
	l.keys(domain.RefPaymentMethod, in.Declared)
	l.keys(domain.RefTerminal, in.TerminalCounts)
	return l
}

func refsPresentationCreate(in presentationCreateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefIngredient, in.IngredientID)
	return l
}

func refsPresentationUpdate(in presentationUpdateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefPresentation, in.PresentationID)
	return l
}

func refsExpenseEdit(in expenseEditArgs) []domain.Ref {
	var l refList
	l.add(domain.RefExpense, in.ExpenseID)
	l.opt(domain.RefSupplier, in.SupplierID)
	l.opt(domain.RefExpenseCategory, in.CategoryID)
	for _, ln := range in.Add {
		l.purchaseLine(ln.IngredientID, ln.ProductID, ln.PresentationID)
	}
	return l
}

func refsIngredientCreate(in ingredientCreateArgs) []domain.Ref {
	var l refList
	l.opt(domain.RefIngredientCat, in.CategoryID)
	l.opt(domain.RefPlace, in.PlaceID)
	return l
}

func refsIngredientUpdate(in ingredientUpdateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefIngredient, in.IngredientID)
	l.opt(domain.RefIngredientCat, in.CategoryID)
	l.opt(domain.RefPlace, in.PlaceID)
	return l
}

func refsLowStock(in lowStockUpdateArgs) []domain.Ref {
	var l refList
	for _, it := range in.Items {
		// Un tipo que no es ninguno de los dos lo rechaza la validación de la herramienta.
		switch it.ItemType {
		case "producto":
			l.product(it.ID)
		case "ingrediente":
			l.add(domain.RefIngredient, it.ID)
		}
	}
	return l
}

func refsCostSet(in costSetArgs) []domain.Ref {
	var l refList
	l.add(domain.RefIngredient, in.IngredientID)
	return l
}

func refsRecipeSet(in recipeSetArgs) []domain.Ref {
	var l refList
	l.add(domain.RefProduct, in.ProductID)
	l.add(domain.RefOption, in.OptionID)
	for _, it := range in.Items {
		l.add(domain.RefIngredient, it.IngredientID)
	}
	l.opt(domain.RefProduct, in.LinkedProductID)
	for _, c := range in.Components {
		l.add(domain.RefProduct, c.ProductID)
	}
	return l
}

func refsGroupUpdate(in groupUpdateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefGroup, in.GroupID)
	return l
}

func refsOptionCreate(in optionCreateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefGroup, in.GroupID)
	return l
}

func refsGroupLink(in groupLinkArgs) []domain.Ref {
	var l refList
	l.add(domain.RefProduct, in.ProductID)
	l.add(domain.RefGroup, in.GroupID)
	return l
}

func refsGroupUnlink(in groupUnlinkArgs) []domain.Ref {
	var l refList
	l.add(domain.RefProduct, in.ProductID)
	l.add(domain.RefGroup, in.GroupID)
	return l
}

func refsProductCreate(in productCreateArgs) []domain.Ref {
	var l refList
	l.add(domain.RefCategory, in.CategoryID)
	return l
}

func refsDraftCreate(in draftCreateArgs) []domain.Ref {
	var l refList
	if in.PlatformID != nil {
		l.add(domain.RefPlatform, int64(*in.PlatformID))
	}
	for _, ln := range in.Lines {
		l.add(domain.RefProduct, ln.ProductID)
		for _, m := range ln.Modifiers {
			l.add(domain.RefOption, m.OptionID)
		}
	}
	return l
}

func refsOrderWrite(in orderWriteArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	return l
}

func refsDiscount(in discountArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	return l
}

func refsPay(in payArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	l.add(domain.RefPaymentMethod, int64(in.MethodID))
	l.opt(domain.RefTerminal, in.TerminalID)
	return l
}

func refsVoid(in voidArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	return l
}

func refsTerminal(in terminalArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	l.add(domain.RefTerminal, in.TerminalID)
	l.opt(domain.RefTerminal, in.FromTerminalID)
	return l
}

func refsRefund(in refundArgs) []domain.Ref {
	var l refList
	l.add(domain.RefOrder, in.OrderID)
	return l
}

func refsProductImageSet(in productImageSetArgs) []domain.Ref {
	var l refList
	l.add(domain.RefProduct, in.ProductID)
	return l
}

func refsProductImageRemove(in productImageRemoveArgs) []domain.Ref {
	var l refList
	l.add(domain.RefProduct, in.ProductID)
	return l
}

func refsCategoryImageSet(in categoryImageSetArgs) []domain.Ref {
	var l refList
	l.add(domain.RefCategory, in.CategoryID)
	return l
}

func refsCategoryImageRemove(in categoryImageRemoveArgs) []domain.Ref {
	var l refList
	l.add(domain.RefCategory, in.CategoryID)
	return l
}

func refsCategoryFamily(in categoryFamilyArgs) []domain.Ref {
	var l refList
	l.add(domain.RefCategory, in.CategoryID)
	return l
}
