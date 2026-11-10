package domain

import (
	"strconv"

	"github.com/shopspring/decimal"
)

// Reglas puras de las escrituras que la spec 073 abrió a los agentes.

// CloseMethod es un método de pago del turno tal como lo ve quien cierra: con el arqueo ciego, sin
// lo esperado.
type CloseMethod struct {
	ID          int
	Name        string
	InDrawer    bool // su dinero está en el cajón: se cuenta con el cajón, no se declara aparte
	AutoDeclare bool // lo declara el sistema (plataformas): nadie lo cuenta
}

// CloseRequirements es lo que un cierre tiene que traer dictado.
type CloseRequirements struct {
	DrawerNeedsCount bool
	Methods          []CloseMethod
	// Terminals son las terminales cuyo corte pide el turno (arqueo por terminal).
	Terminals []CloseTerminal
}

// CloseTerminal es una terminal de tarjeta que se arquea al cerrar.
type CloseTerminal struct {
	ID   int64
	Name string
}

// CloseDeclarations es lo que trae un cierre.
type CloseDeclarations struct {
	DrawerCounted bool
	Declared      map[int]bool
	Terminals     map[int64]bool
}

// DeclarationsFrom lee lo que trae el cierre. Un «0.00» es una cifra dictada: el método no vendió.
func DeclarationsFrom(drawerCounted bool, declared, terminals map[string]string) CloseDeclarations {
	out := CloseDeclarations{DrawerCounted: drawerCounted, Declared: map[int]bool{}, Terminals: map[int64]bool{}}
	for k := range declared {
		if id, err := strconv.Atoi(k); err == nil {
			out.Declared[id] = true
		}
	}
	for k := range terminals {
		if id, err := strconv.ParseInt(k, 10, 64); err == nil {
			out.Terminals[id] = true
		}
	}
	return out
}

// MissingCloseDeclarations nombra lo que falta dictar para cerrar, en el orden del turno.
//
// La pantalla no deja enviar el cierre sin capturar cada método, y por eso el servidor toma un
// método ausente como cero. Un agente sí puede omitirlo, y peor, puede «ayudar» llenándolo con lo que
// el sistema espera: el arqueo ciego se perdería sin que nada fallara. Por MCP, cada cifra va dictada.
func MissingCloseDeclarations(req CloseRequirements, given CloseDeclarations) []string {
	var missing []string
	if req.DrawerNeedsCount && !given.DrawerCounted {
		missing = append(missing, "el cajón (efectivo contado)")
	}
	for _, m := range req.Methods {
		if m.InDrawer || m.AutoDeclare {
			continue
		}
		if !given.Declared[m.ID] {
			missing = append(missing, m.Name)
		}
	}
	for _, tm := range req.Terminals {
		if !given.Terminals[tm.ID] {
			missing = append(missing, "la terminal "+tm.Name)
		}
	}
	return missing
}

// CancelPayment es un cobro de un pedido que se va a cancelar.
type CancelPayment struct {
	Method string
	Amount decimal.Decimal
	IsCard bool
}

// CancelRefundLine es lo que sale del corte por un método.
type CancelRefundLine struct {
	Method string          `json:"method"`
	Amount decimal.Decimal `json:"amount"`
}

// CancelSummaryView es lo que la vista previa de cancelar dice antes de aplicar. Informa: la regla de
// verdad (devolución confirmada, folio) la aplica el servicio de pedidos.
type CancelSummaryView struct {
	NeedsRefund    bool               `json:"needs_refund"`
	NeedsCardFolio bool               `json:"needs_card_folio"`
	ByMethod       []CancelRefundLine `json:"by_method"`
	Total          decimal.Decimal    `json:"total"`
}

// CancelSummary agrupa los cobros por método, en el orden en que aparecen.
func CancelSummary(payments []CancelPayment) CancelSummaryView {
	out := CancelSummaryView{ByMethod: []CancelRefundLine{}}
	idx := map[string]int{}
	for _, p := range payments {
		out.NeedsRefund = true
		out.NeedsCardFolio = out.NeedsCardFolio || p.IsCard
		out.Total = out.Total.Add(p.Amount)
		i, ok := idx[p.Method]
		if !ok {
			i = len(out.ByMethod)
			idx[p.Method] = i
			out.ByMethod = append(out.ByMethod, CancelRefundLine{Method: p.Method})
		}
		out.ByMethod[i].Amount = out.ByMethod[i].Amount.Add(p.Amount)
	}
	out.Total = Round2(out.Total)
	for i := range out.ByMethod {
		out.ByMethod[i].Amount = Round2(out.ByMethod[i].Amount)
	}
	return out
}

// ExpenseStatusFromPayments es el estado con que un agente captura un gasto: pagado si los pagos,
// redondeados como los guarda el servidor, cubren el total redondeado; si no, pendiente. La pantalla
// lo decide en su hoja; el agente no tiene hoja, y comparar sin redondear dejaba pendiente un gasto
// que el servidor guarda cubierto.
func ExpenseStatusFromPayments(total decimal.Decimal, payments []decimal.Decimal) string {
	if len(payments) == 0 {
		return ExpensePendiente
	}
	paid := decimal.Zero
	for _, p := range payments {
		paid = paid.Add(Round2(p))
	}
	if paid.GreaterThanOrEqual(Round2(total)) {
		return ExpensePagada
	}
	return ExpensePendiente
}
