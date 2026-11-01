package domain

import (
	"fmt"
	"slices"
	"time"
)

// AuditAction es una acción que deja huella en la bitácora. El texto se guarda como dato, por eso
// va en inglés y no cambia: renombrarla vuelve inbuscable lo ya guardado.
type AuditAction string

// AuditGroup es el grupo con el que la bitácora cuenta y filtra las acciones.
type AuditGroup string

const (
	AuditGroupMoney      AuditGroup = "money"
	AuditGroupCancel     AuditGroup = "cancel"
	AuditGroupPrice      AuditGroup = "price"
	AuditGroupCash       AuditGroup = "cash"
	AuditGroupAttendance AuditGroup = "attendance"
)

// AllAuditGroups devuelve los grupos en el orden en que la bitácora los muestra.
func AllAuditGroups() []AuditGroup {
	return []AuditGroup{AuditGroupMoney, AuditGroupCancel, AuditGroupPrice, AuditGroupCash, AuditGroupAttendance}
}

// auditActions es la lista blanca, en orden, con el grupo de cada acción. Una devolución, un pago
// devuelto o una merma son dinero que sale: van con las cancelaciones, no con los cobros.
var auditActions = []struct {
	action AuditAction
	group  AuditGroup
}{
	{"order.created", AuditGroupMoney},
	{"order.lines_added", AuditGroupMoney},
	{"order.line_removed", AuditGroupCancel},
	{"order.lines_moved", AuditGroupMoney},
	{"order.delivered", AuditGroupMoney},
	{"order.cancelled", AuditGroupCancel},
	{"order.discount_set", AuditGroupMoney},
	{"order.platform_ref_set", AuditGroupMoney},
	{"payment.charged", AuditGroupMoney},
	{"payment.voided", AuditGroupCancel},
	{"payment.refunded", AuditGroupCancel},
	{"stock.written_off", AuditGroupCancel},
	{"cash.opened", AuditGroupCash},
	{"cash.closed", AuditGroupCash},
	// Spec 046: quien abrió se fue sin marcar su salida y alguien con permiso cerró la caja; su
	// entrada quedó «sin salida».
	{"cash.closed_without_opener_clock_out", AuditGroupCash},
	{"cash.movement", AuditGroupCash},
	{"cash.movement_corrected", AuditGroupCash},
	{"expense.created", AuditGroupCash},
	{"expense.paid", AuditGroupCash},
	{"expense.cancelled", AuditGroupCash},
	{"tip.paid_out", AuditGroupCash},
	{"price.set", AuditGroupPrice},
	{"price.deleted", AuditGroupPrice},
	{"price.synced", AuditGroupPrice},
	{"attendance.clock_in", AuditGroupAttendance},
	{"attendance.clock_out", AuditGroupAttendance},
	{"attendance.corrected", AuditGroupAttendance},
	{"attendance.manual_entry", AuditGroupAttendance},
	{"attendance.settings_changed", AuditGroupAttendance},
	{"attendance.schedule_set", AuditGroupAttendance},
	{"user.tracks_attendance_changed", AuditGroupAttendance},
	{"payment.terminal_changed", AuditGroupMoney},
	// Qué resta un producto (spec 037). Va con los precios porque es configuración del menú: un
	// grupo propio cambiaría la rejilla de contadores de la bitácora, pensada para la tableta.
	{AuditProductConsumptionMode, AuditGroupPrice},
	// Compras al almacén (spec 038). Recibir y corregir van con los demás gastos; el conteo con lo
	// dado por perdido, que también reescribe lo guardado; el método de costeo con los precios. Ningún
	// grupo nuevo: la rejilla de la bitácora es una disposición de tableta.
	{AuditExpenseReceived, AuditGroupCash},
	// El arqueo por hora (spec 048): con los demás movimientos de caja.
	{AuditExpenseCountChanged, AuditGroupCash},
	{AuditCashOpeningBackdated, AuditGroupCash},
	{AuditExpenseStockCorrected, AuditGroupCash},
	{AuditStockCounted, AuditGroupCancel},
	{AuditSettingsCostingMethod, AuditGroupPrice},
	// Lo que se echó a perder (spec 039): con lo dado por perdido. El tope de aprobación, con los
	// precios, como el método de costeo.
	{AuditWasteRecorded, AuditGroupCancel},
	{AuditWasteApproved, AuditGroupCancel},
	{AuditWasteRejected, AuditGroupCancel},
	{AuditWasteUndone, AuditGroupCancel},
	{AuditSettingsWasteThreshold, AuditGroupPrice},
	{AuditSettingsCountLimits, AuditGroupPrice},
	// Artículos del almacén (spec 042): configuración, con los precios, como el costeo. El costo
	// corregido también. Ningún grupo nuevo, por la misma razón que la 038.
	{AuditIngredientCreated, AuditGroupPrice},
	{AuditIngredientUpdated, AuditGroupPrice},
	{AuditIngredientArchived, AuditGroupPrice},
	{AuditIngredientReactivated, AuditGroupPrice},
	{AuditIngredientCostCorrected, AuditGroupPrice},
	{AuditIngredientUnitChanged, AuditGroupPrice},
	{AuditIngredientUnitChangedDetail, AuditGroupPrice},
	{AuditIngredientReplaced, AuditGroupPrice},
	{AuditPresentationCreated, AuditGroupPrice},
	{AuditPresentationUpdated, AuditGroupPrice},
	{AuditPresentationRetired, AuditGroupPrice},
	// Un aviso del conteo (spec 042): con lo dado por perdido, como el conteo.
	{AuditCountConfirmAsked, AuditGroupCancel},
	// Renglones excluidos y caja que vende (spec 049): con los demás gastos y cajas.
	{AuditExpenseItemExcluded, AuditGroupCash},
	{AuditCashSellingRegisterChanged, AuditGroupCash},
	// Editar un gasto ya guardado (spec 053): con los demás gastos.
	{AuditExpenseEdited, AuditGroupCash},
}

const (
	// AuditExpenseItemExcluded: un renglón de compra se excluyó o se volvió a incluir.
	AuditExpenseItemExcluded AuditAction = "expense.item_excluded"
	// AuditCashSellingRegisterChanged: la venta pasó a otra caja.
	AuditCashSellingRegisterChanged AuditAction = "cash.selling_register_changed"
	// AuditExpenseEdited: se corrigió un gasto ya guardado: su encabezado, renglones agregados o
	// quitados, su importe y las decisiones ante un conteo.
	AuditExpenseEdited AuditAction = "expense.edited"
)

const (
	// AuditIngredientCreated: se dio de alta un artículo del almacén.
	AuditIngredientCreated AuditAction = "ingredient.created"
	// AuditIngredientUpdated: cambió el nombre, la categoría, el lugar o el mínimo de un artículo.
	AuditIngredientUpdated AuditAction = "ingredient.updated"
	// AuditIngredientArchived: un artículo dejó de ofrecerse.
	AuditIngredientArchived AuditAction = "ingredient.archived"
	// AuditIngredientReactivated: un artículo archivado volvió a ofrecerse.
	AuditIngredientReactivated AuditAction = "ingredient.reactivated"
	// AuditIngredientCostCorrected: se corrigió a mano el costo de un artículo.
	AuditIngredientCostCorrected AuditAction = "ingredient.cost_corrected"
	// AuditIngredientUnitChanged: cambió en qué se mide un artículo, en el mismo artículo.
	AuditIngredientUnitChanged AuditAction = "ingredient.unit_changed"
	// AuditIngredientUnitChangedDetail: los valores originales de una tabla convertida por un cambio
	// de unidad, ligados al evento principal por su batch_id. Partido para no crecer sin tope.
	AuditIngredientUnitChangedDetail AuditAction = "ingredient.unit_changed.detail"
	// AuditIngredientReplaced: un artículo se reemplazó por otro en otra unidad y quedó archivado.
	AuditIngredientReplaced AuditAction = "ingredient.replaced"
	// AuditPresentationCreated: se agregó un empaque a un artículo.
	AuditPresentationCreated AuditAction = "presentation.created"
	// AuditPresentationUpdated: cambió el nombre o la cantidad de un empaque.
	AuditPresentationUpdated AuditAction = "presentation.updated"
	// AuditPresentationRetired: un empaque dejó de llegar así, o volvió.
	AuditPresentationRetired AuditAction = "presentation.retired"
	// AuditCountConfirmAsked: el conteo pidió confirmar una cifra fuera de lo razonable. Deja rastro de
	// quien prueba cifras para adivinar lo esperado (riesgo aceptado por el dueño, spec 042).
	AuditCountConfirmAsked AuditAction = "stock.count_confirm_asked"
)

// AuditProductConsumptionMode: cambió qué resta un producto al venderse.
const AuditProductConsumptionMode AuditAction = "product.consumption_mode"

const (
	// AuditExpenseCountChanged: alguien cambió si un pago de gasto ya registrado entra al arqueo.
	AuditExpenseCountChanged AuditAction = "expense.count_changed"
	// AuditCashOpeningBackdated: un corte se abrió desde una hora anterior a la actual.
	AuditCashOpeningBackdated AuditAction = "cash.opening_backdated"
	// AuditExpenseReceived: llegó mercancía de una compra y sumó a lo guardado.
	AuditExpenseReceived AuditAction = "expense.received"
	// AuditExpenseStockCorrected: se corrigió, agregó, cerró o revirtió lo que llegó de una compra.
	AuditExpenseStockCorrected AuditAction = "expense.stock_corrected"
	// AuditStockCounted: se cerró un conteo y lo guardado quedó con lo contado.
	AuditStockCounted AuditAction = "stock.counted"
	// AuditSettingsCostingMethod: cambió el método de costeo o la palabra del almacén.
	AuditSettingsCostingMethod AuditAction = "settings.costing_method"
	// AuditWasteRecorded: se registró algo que se echó a perder (restó o quedó por aprobar).
	AuditWasteRecorded AuditAction = "waste.recorded"
	// AuditWasteApproved: se aprobó una merma que pasaba el tope, y restó.
	AuditWasteApproved AuditAction = "waste.approved"
	// AuditWasteRejected: se rechazó una merma por aprobar; nunca restó.
	AuditWasteRejected AuditAction = "waste.rejected"
	// AuditWasteUndone: se deshizo una merma y volvió lo que había restado.
	AuditWasteUndone AuditAction = "waste.undone"
	// AuditSettingsWasteThreshold: cambió el tope a partir del cual una merma espera aprobación.
	AuditSettingsWasteThreshold AuditAction = "settings.waste_threshold"
	// AuditSettingsCountLimits: cambiaron los límites del conteo (máximo y cifra para confirmar).
	AuditSettingsCountLimits AuditAction = "settings.count_limits_changed"
)

// AllAuditActions devuelve la lista blanca de acciones, en orden fijo.
func AllAuditActions() []AuditAction {
	out := make([]AuditAction, len(auditActions))
	for i, a := range auditActions {
		out[i] = a.action
	}
	return out
}

// ValidAuditAction dice si `s` es una acción de la lista blanca.
func ValidAuditAction(s string) bool {
	return slices.Contains(AllAuditActions(), AuditAction(s))
}

// AuditGroupOf devuelve el grupo de una acción.
func AuditGroupOf(a AuditAction) (AuditGroup, bool) {
	for _, x := range auditActions {
		if x.action == a {
			return x.group, true
		}
	}
	return "", false
}

// ActionsOfGroup devuelve las acciones de un grupo, para filtrar la bitácora por él.
func ActionsOfGroup(g AuditGroup) []AuditAction {
	var out []AuditAction
	for _, x := range auditActions {
		if x.group == g {
			out = append(out, x.action)
		}
	}
	return out
}

// ActorKind dice quién hizo la acción: una persona, una plataforma de reparto o el sistema.
type ActorKind string

const (
	ActorUser     ActorKind = "user"
	ActorPlatform ActorKind = "platform"
	ActorSystem   ActorKind = "system"
)

// AuditEvent es una acción a registrar. El origen (IP, aparato) lo agrega quien la registra.
type AuditEvent struct {
	Action        AuditAction
	ActorKind     ActorKind
	ActorUserID   *int64
	ActorLabel    string
	EntityType    string
	EntityID      int64
	OrderID       *int64
	SubjectUserID *int64
	BranchID      *int64
	Before        any
	After         any
}

// Validate adelanta los checks de la base para que el error salga con nombre y no como un 23514.
func (e AuditEvent) Validate() error {
	if !ValidAuditAction(string(e.Action)) {
		return fmt.Errorf("%w: acción de auditoría desconocida %q", ErrValidation, e.Action)
	}
	switch e.ActorKind {
	case ActorUser:
		if e.ActorUserID == nil {
			return fmt.Errorf("%w: una acción de persona necesita a la persona", ErrValidation)
		}
	case ActorPlatform, ActorSystem:
		if e.ActorUserID != nil {
			return fmt.Errorf("%w: una acción de %s no lleva persona", ErrValidation, e.ActorKind)
		}
	default:
		return fmt.Errorf("%w: autor desconocido %q", ErrValidation, e.ActorKind)
	}
	if e.EntityType == "" {
		return fmt.Errorf("%w: el evento no dice sobre qué fue", ErrValidation)
	}
	return nil
}

// Tamaños de página de la bitácora. Una lista corta y fija: la pantalla ofrece estos dos.
var auditPageSizes = []int{25, 50}

// maxAuditPage acota la página: sin tope, página × tamaño desborda el offset y la consulta
// responde 500 en vez de 400.
const maxAuditPage = 10_000

// maxAuditRange acota el rango de una consulta a la bitácora.
const maxAuditRange = 366 * 24 * time.Hour

// AuditFilter es lo que la bitácora general deja filtrar. From y To son días del negocio, ambos
// incluidos.
type AuditFilter struct {
	From, To    time.Time
	ActorUserID *int64
	Action      string
	Group       AuditGroup
	DeviceKey   string
	Page, Size  int
}

// Validate rechaza cualquier filtro mal formado en vez de caer a un default.
func (f AuditFilter) Validate() error {
	if !slices.Contains(auditPageSizes, f.Size) {
		return fmt.Errorf("%w: tamaño de página inválido", ErrValidation)
	}
	if f.Page < 0 || f.Page > maxAuditPage {
		return fmt.Errorf("%w: página inválida", ErrValidation)
	}
	if f.From.IsZero() || f.To.IsZero() || f.To.Before(f.From) {
		return fmt.Errorf("%w: el rango de fechas no es válido", ErrValidation)
	}
	if f.To.Sub(f.From) > maxAuditRange {
		return fmt.Errorf("%w: el rango no puede pasar de un año", ErrValidation)
	}
	if f.Action != "" && !ValidAuditAction(f.Action) {
		return fmt.Errorf("%w: acción desconocida", ErrValidation)
	}
	if f.Group != "" && !slices.Contains(AllAuditGroups(), f.Group) {
		return fmt.Errorf("%w: tipo de acción desconocido", ErrValidation)
	}
	if f.DeviceKey != "" && !deviceKeyRe.MatchString(f.DeviceKey) {
		return fmt.Errorf("%w: clave de aparato inválida", ErrValidation)
	}
	return nil
}

// Actions es la lista de acciones por la que se filtra, o nil si no se filtra por acción.
func (f AuditFilter) Actions() []AuditAction {
	if f.Group == "" && f.Action == "" {
		return nil
	}
	if f.Group == "" {
		return []AuditAction{AuditAction(f.Action)}
	}
	of := ActionsOfGroup(f.Group)
	if f.Action == "" {
		return of
	}
	if slices.Contains(of, AuditAction(f.Action)) {
		return []AuditAction{AuditAction(f.Action)}
	}
	return []AuditAction{}
}
