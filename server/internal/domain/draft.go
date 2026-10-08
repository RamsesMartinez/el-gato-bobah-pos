package domain

import (
	"fmt"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/shopspring/decimal"
)

// La cuenta en captura (spec 030, D-1): lo que se está capturando, guardado en el servidor desde el
// primer producto. NUNCA es un pedido —vive en su propia tabla— y por eso ninguna consulta de
// dinero la puede contar: no hay estado nuevo en `orders` que treinta consultas tengan que
// acordarse de excluir.

// Estados de una cuenta en captura. Texto y no enum en la base: un enum no se quita en el Down.
const (
	DraftCapturing = "capturando"
	DraftSent      = "enviada"
	DraftDiscarded = "descartada"
)

// Por qué se descartó. Viaja a la base con su check: el rastro de lo abandonado (D-7) sirve para
// saber si las cuentas se pierden por descuido, por las 12 horas o porque el pedido se canceló.
const (
	DiscardManual      = "manual"
	DiscardExpired     = "expired"
	DiscardOrderClosed = "order_closed"
	DiscardEmpty       = "empty"
)

// DraftIdleLimit es cuánto vive una cuenta en captura sin que nadie la toque (D-8). Constante y no
// configuración (principio VI): es una regla del dueño, no algo que cambie por negocio.
const DraftIdleLimit = 12 * time.Hour

// MaxDraftLines acota los renglones de una cuenta. No es una regla de negocio: una mesa real no
// pasa de unas decenas. Existe para que un cliente que manda renglones en un ciclo no llene la tabla
// de una empresa ni haga que enviar a cocina arme un pedido de miles de renglones.
const MaxDraftLines = 200

// maxDraftNote es el largo máximo de la nota de un renglón, en letras (el check de la tabla dice lo
// mismo con char_length).
const maxDraftNote = 200

// DraftModifier es un modificador tal como se guarda en la cuenta: solo la opción, cuántas y la
// mitad. El nombre y el precio se resuelven en cada lectura y al enviar (R-16): un snapshot aquí no
// protege nada, porque el pedido vuelve a calcular todo al nacer.
type DraftModifier struct {
	OptionID int64  `json:"optionId"`
	Qty      int    `json:"qty"`
	Portion  string `json:"portion"`
}

// DraftLineInput es un renglón que llega de la tableta para agregarse a una cuenta.
type DraftLineInput struct {
	ProductID int64
	Qty       decimal.Decimal
	Modifiers []DraftModifier
	Notes     string
}

// DraftLine es un renglón ya guardado en la cuenta.
type DraftLine struct {
	ID        uuid.UUID
	ProductID int64
	Qty       decimal.Decimal
	Modifiers []DraftModifier
	Notes     string
}

// ValidateDraftLine valida un renglón en la frontera y lo devuelve con la cantidad ya redondeada:
// lo que se guarda es lo que se validó. Un 0.001 que redondea a 0 se rechaza aquí y no en el check
// de la tabla, que devolvería un 500.
func ValidateDraftLine(in DraftLineInput) (DraftLineInput, error) {
	if in.ProductID <= 0 {
		return in, fmt.Errorf("%w: falta el producto", ErrValidation)
	}
	qty, err := ValidateDraftQty(in.Qty)
	if err != nil {
		return in, err
	}
	in.Qty = qty
	if utf8.RuneCountInString(in.Notes) > maxDraftNote {
		return in, fmt.Errorf("%w: la nota es demasiado larga", ErrValidation)
	}
	for _, m := range in.Modifiers {
		if err := validDraftModifier(m); err != nil {
			return in, err
		}
	}
	return in, nil
}

// ValidateDraftQty valida una cantidad de la frontera y la devuelve redondeada a 2 decimales (la
// escala de la columna). Lo usa también el «+» de un renglón, que solo trae cantidad.
func ValidateDraftQty(qty decimal.Decimal) (decimal.Decimal, error) {
	// La escala se mira ANTES de redondear: redondear un exponente absurdo es el ataque de CPU que
	// escalaSana existe para cerrar.
	if !escalaSana(qty) {
		return qty, ErrValidation
	}
	qty = Round2(qty)
	if !ValidQty(qty, MaxOrderQty, false) {
		return qty, fmt.Errorf("%w: la cantidad no es válida", ErrValidation)
	}
	return qty, nil
}

func validDraftModifier(m DraftModifier) error {
	if m.OptionID <= 0 || m.Qty < 1 {
		return fmt.Errorf("%w: modificador incompleto", ErrValidation)
	}
	// Se guarda como int16 al enviarse: sin tope, 40000 hace wrap y corrompe la comanda.
	if !ValidQty(decimal.NewFromInt(int64(m.Qty)), MaxOrderQty, false) {
		return fmt.Errorf("%w: demasiadas veces la misma opción", ErrValidation)
	}
	if m.Portion != "" && m.Portion != "A" && m.Portion != "B" {
		return fmt.Errorf("%w: esa mitad no existe", ErrValidation)
	}
	return nil
}

// MergeTarget dice con qué renglón se fusiona un agregado, si con alguno.
//
// Solo se fusiona lo PELADO con lo pelado: mismo producto, sin modificadores ni nota en ninguno de
// los dos. Un café con nota es otro café para cocina, y sumarle uno sin nota le pondría la nota a
// algo que nadie pidió así. Es la regla que la tableta aplicaba en `stores/ticket.ts`; ahora vive en
// el servidor porque dos tabletas que agregan el mismo producto tienen que terminar en UN renglón.
func MergeTarget(lines []DraftLine, add DraftLineInput) (uuid.UUID, bool) {
	if len(add.Modifiers) > 0 || add.Notes != "" {
		return uuid.UUID{}, false
	}
	for _, l := range lines {
		if l.ProductID == add.ProductID && len(l.Modifiers) == 0 && l.Notes == "" {
			return l.ID, true
		}
	}
	return uuid.UUID{}, false
}

// DraftExpired dice si una cuenta lleva DraftIdleLimit o más sin cambios. Un `updatedAt` en el
// futuro (reloj adelantado) no la vence.
func DraftExpired(updatedAt, now time.Time) bool {
	return now.Sub(updatedAt) >= DraftIdleLimit
}
