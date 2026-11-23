package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Pedidos entrantes (spec 093): lo que llega de fuera y espera que el negocio lo acepte o lo
// rechace. Es la forma COMÚN a todos los orígenes: la alerta, la franja y la columna «Por aceptar»
// solo conocen este tipo, y por eso el pedido web (#328) entra sin tocarlas.

// IncomingSource es de dónde viene un pedido entrante. Lista cerrada: un origen desconocido en una
// ruta es un 400, nunca «el que sea».
type IncomingSource string

const (
	IncomingSourcePlatform IncomingSource = "platform"
)

// ParseIncomingSource rechaza lo que no esté en la lista.
func ParseIncomingSource(raw string) (IncomingSource, error) {
	switch IncomingSource(raw) {
	case IncomingSourcePlatform:
		return IncomingSourcePlatform, nil
	}
	return "", fmt.Errorf("%w: origen de pedido %q", ErrValidation, raw)
}

// IncomingStatePending es el estado guardado de un pedido que nadie ha decidido.
const IncomingStatePending = "pendiente"

// IsPending dice si un pedido todavía se puede decidir.
//
// El plazo cuenta y no solo el estado: la plataforma cancela sola al vencer, y si su aviso de
// cancelación no llega, la fila se queda «pendiente» para siempre. Con la alerta continua eso sería
// una tableta sonando sin fin por un pedido que ya no existe. Justo en el plazo ya no se decide:
// aceptar en el último segundo es aceptar algo que la plataforma ya está cancelando.
func IsPending(state string, decideBefore *time.Time, now time.Time) bool {
	if state != IncomingStatePending {
		return false
	}
	return decideBefore == nil || decideBefore.After(now)
}

// IncomingOrder es un pedido entrante como lo ve la pantalla.
type IncomingOrder struct {
	Source        IncomingSource  `json:"source"`
	ID            int64           `json:"id"`
	SourceName    string          `json:"sourceName"`
	DisplayID     string          `json:"displayId"`
	CustomerName  string          `json:"customerName"`
	CustomerPhone string          `json:"customerPhone"`
	Address       string          `json:"address"`
	ServiceType   string          `json:"serviceType"`
	Total         decimal.Decimal `json:"total"`
	PlacedAt      *time.Time      `json:"placedAt"`
	DecideBefore  *time.Time      `json:"decideBefore"`
	// Lines nunca es nil: un slice nil sale como `null` y la pantalla hace `.map()` al pintar.
	Lines []IncomingLine `json:"lines"`
}

// IncomingLine es un platillo del pedido con sus opciones colgando.
type IncomingLine struct {
	Name      string          `json:"name"`
	Quantity  decimal.Decimal `json:"quantity"`
	UnitPrice decimal.Decimal `json:"unitPrice"`
	// Matched: si tiene pareja en el catálogo. `false` no impide aceptar: el cliente ya pagó.
	Matched bool           `json:"matched"`
	Options []IncomingLine `json:"options"`
}

// SortByUrgency ordena para decidir: con plazo primero, el que vence antes arriba; sin plazo
// después, el más viejo arriba. El id desempata para que dos tabletas pinten el mismo orden.
func SortByUrgency(orders []IncomingOrder) {
	sort.SliceStable(orders, func(i, j int) bool {
		a, b := orders[i], orders[j]
		if (a.DecideBefore == nil) != (b.DecideBefore == nil) {
			return a.DecideBefore != nil
		}
		if a.DecideBefore != nil && !a.DecideBefore.Equal(*b.DecideBefore) {
			return a.DecideBefore.Before(*b.DecideBefore)
		}
		if a.PlacedAt != nil && b.PlacedAt != nil && !a.PlacedAt.Equal(*b.PlacedAt) {
			return a.PlacedAt.Before(*b.PlacedAt)
		}
		return a.ID < b.ID
	})
}

// IncomingRejectReason es el motivo de rechazo en palabras de quien opera. Se guarda el código
// propio (en `deny_reason`) y cada origen lo traduce al suyo: los 11 códigos de la plataforma
// incluyen varios que solo entiende un integrador.
type IncomingRejectReason string

type motivoDeRechazo struct {
	label      string
	plataforma string
}

var motivosDeRechazoPropios = map[IncomingRejectReason]motivoDeRechazo{
	"closed":       {"Ya cerramos", "STORE_CLOSED"},
	"out_of_stock": {"Se acabó un producto", "ITEM_AVAILABILITY"},
	"too_busy":     {"Estamos saturados", "CAPACITY"},
	"address":      {"No llegamos a esa dirección", "ADDRESS"},
	"other":        {"Otro motivo", "OTHER"},
}

// ParseIncomingRejectReason rechaza un motivo fuera de la lista en vez de caer a «other»: caer a un
// default le mandaría al cliente una explicación que nadie eligió.
func ParseIncomingRejectReason(raw string) (IncomingRejectReason, error) {
	r := IncomingRejectReason(raw)
	if _, ok := motivosDeRechazoPropios[r]; !ok {
		return "", fmt.Errorf("%w: motivo de rechazo %q", ErrValidation, raw)
	}
	return r, nil
}

// Label es el texto que lee quien opera, y la explicación que recibe la plataforma.
func (r IncomingRejectReason) Label() string { return motivosDeRechazoPropios[r].label }

// PlatformCode es el código que admite la plataforma de reparto.
func (r IncomingRejectReason) PlatformCode() string { return motivosDeRechazoPropios[r].plataforma }
