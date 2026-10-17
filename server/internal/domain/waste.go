package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// WasteStatus es el estado de un registro de merma (spec 039, data-model).
type WasteStatus string

const (
	WasteApplied  WasteStatus = "applied"  // restó del almacén
	WastePending  WasteStatus = "pending"  // pasó el tope: espera aprobación y no resta
	WasteRejected WasteStatus = "rejected" // la rechazaron: nunca restó
	WasteCovered  WasteStatus = "covered"  // un conteo cerrado ya reflejó la pérdida
	WasteUndone   WasteStatus = "undone"   // se deshizo: devolvió lo que había restado
)

// WasteAction es lo que se le puede hacer a un registro de merma.
type WasteAction string

const (
	WasteApprove WasteAction = "approve"
	WasteReject  WasteAction = "reject"
	WasteUndo    WasteAction = "undo"
	WasteCover   WasteAction = "cover"
)

var wasteTransitions = map[WasteStatus]map[WasteAction]WasteStatus{
	WasteApplied: {WasteUndo: WasteUndone},
	WastePending: {WasteApprove: WasteApplied, WasteReject: WasteRejected, WasteUndo: WasteUndone, WasteCover: WasteCovered},
}

// NextWasteStatus decide a qué estado lleva una acción. Lo que no está en la tabla es un conflicto:
// aprobar dos veces restaría dos veces, y deshacer algo rechazado devolvería lo que nunca salió.
func NextWasteStatus(from WasteStatus, a WasteAction) (WasteStatus, error) {
	if to, ok := wasteTransitions[from][a]; ok {
		return to, nil
	}
	return "", fmt.Errorf("%w: esta merma ya no se puede %s", ErrConflict, wasteActionWords[a])
}

var wasteActionWords = map[WasteAction]string{
	WasteApprove: "aprobar", WasteReject: "rechazar", WasteUndo: "deshacer", WasteCover: "cubrir",
}

// WasteNeedsApproval dice si una merma espera aprobación. Sin tope, nunca. Con tope, la que cuesta
// más que él, y también la de un artículo sin costo: no se sabe cuánto vale y podría ser cualquier
// cosa.
func WasteNeedsApproval(threshold, totalCost *decimal.Decimal) bool {
	if threshold == nil {
		return false
	}
	if totalCost == nil {
		return true
	}
	return totalCost.GreaterThan(*threshold)
}

// WasteCanUndo permite deshacer solo el mismo día del local y mientras ese artículo no se haya
// contado después: un conteo cerrado ya corrigió lo guardado, y devolver la merma inventaría lo que
// el conteo quitó.
func WasteCanUndo(recordedAt, now time.Time, loc *time.Location, countedAfter bool) error {
	if countedAfter {
		return fmt.Errorf("%w: ya se contó después; lo corrige el siguiente conteo", ErrConflict)
	}
	ry, rm, rd := recordedAt.In(loc).Date()
	ny, nm, nd := now.In(loc).Date()
	if ry != ny || rm != nm || rd != nd {
		return fmt.Errorf("%w: solo se deshace el mismo día", ErrConflict)
	}
	return nil
}

// LossSplit parte lo perdido de un artículo entre dos conteos.
type LossSplit struct {
	Registered  decimal.Decimal // la merma que restó en el periodo
	Unexplained decimal.Decimal // lo que el conteo encontró de menos
	Surplus     decimal.Decimal // lo que el conteo encontró de más
	Lost        decimal.Decimal // registrado + sin explicar
}

// SplitLoss parte la pérdida de un periodo. La merma registrada ya restó de lo que el sistema tenía
// al capturar el conteo, así que lo que el conteo encontró de menos ES lo sin explicar: restarle
// otra vez la merma la contaría dos veces (research R1).
func SplitLoss(registered, system, counted decimal.Decimal) LossSplit {
	diff := Round4(system.Sub(counted))
	s := LossSplit{Registered: Round4(registered), Unexplained: decimal.Zero, Surplus: decimal.Zero}
	if diff.IsPositive() {
		s.Unexplained = diff
	} else {
		s.Surplus = diff.Neg()
	}
	s.Lost = Round4(s.Registered.Add(s.Unexplained))
	return s
}

var reasonAccents = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// NormalizeReasonName es la forma con la que dos motivos se consideran el mismo: sin mayúsculas,
// acentos ni espacios de más. Es la misma que el índice único de la base (0094) y que
// IngredientNameTaken; si cambia una, cambian las tres. Los espacios se juntan con strings.Fields y
// no con `\s` de regexp: el de Go no ve el espacio duro y el de Postgres sí.
func NormalizeReasonName(name string) string {
	return strings.Join(strings.Fields(reasonAccents.Replace(strings.ToLower(name))), " ")
}

// WasteReasonExists dice que ya hay un motivo con ese nombre; lleva su id para que la pantalla lo
// ofrezca en vez de crear otro igual. Es un conflicto (409).
type WasteReasonExists struct{ ID int64 }

func (e WasteReasonExists) Error() string { return "ese motivo ya existe" }
func (e WasteReasonExists) Unwrap() error { return ErrConflict }

// ValidWasteReason valida el nombre (1–40) y la descripción obligatoria (1–200) de un motivo.
func ValidWasteReason(name, description string) error {
	n := utf8.RuneCountInString(strings.TrimSpace(name))
	if n < 1 || n > 40 {
		return fmt.Errorf("%w: el motivo lleva un nombre de 1 a 40 letras", ErrValidation)
	}
	dsc := utf8.RuneCountInString(strings.TrimSpace(description))
	if dsc < 1 || dsc > 200 {
		return fmt.Errorf("%w: el motivo lleva una descripción de 1 a 200 letras", ErrValidation)
	}
	return nil
}
