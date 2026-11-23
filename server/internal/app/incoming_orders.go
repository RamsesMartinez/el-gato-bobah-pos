package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// IncomingOrdersService es el módulo común de pedidos entrantes (spec 093): junta lo que espera
// decisión de cada origen y despacha aceptar y rechazar al origen que corresponde.
//
// Despacha con un `switch` y no con una interfaz: hoy hay un solo origen (la plataforma), y una
// interfaz de una implementación es abstracción especulativa (constitución VI). Lo intercambiable
// es el contrato —`domain.IncomingOrder`, las rutas y el evento—, que ya no cambia cuando el pedido
// web (#328) agregue su caso aquí.
type IncomingOrdersService struct {
	platform *PedidosDePlataformaService
}

// NewIncomingOrdersService arma el módulo con sus orígenes.
func NewIncomingOrdersService(platform *PedidosDePlataformaService) *IncomingOrdersService {
	return &IncomingOrdersService{platform: platform}
}

// Pending es lo que espera decisión, de todos los orígenes, en el orden en que hay que decidirlo.
func (s *IncomingOrdersService) Pending(ctx context.Context) ([]domain.IncomingOrder, error) {
	pedidos, err := s.platform.Pendientes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.IncomingOrder, 0, len(pedidos))
	for _, p := range pedidos {
		out = append(out, domain.IncomingOrder{
			Source: domain.IncomingSourcePlatform, ID: p.ID, SourceName: p.PlatformName,
			DisplayID: p.DisplayID, CustomerName: p.CustomerName, ServiceType: p.ServiceType,
			Total: p.Total, PlacedAt: p.PlacedAt, DecideBefore: p.DecideBefore,
			Lines: renglonesEntrantes(p.Lines),
		})
	}
	domain.SortByUrgency(out)
	return out, nil
}

// renglonesEntrantes traduce los renglones de la plataforma a la forma común, sin dejar un nil.
func renglonesEntrantes(rs []RenglonPendiente) []domain.IncomingLine {
	out := make([]domain.IncomingLine, 0, len(rs))
	for _, r := range rs {
		out = append(out, domain.IncomingLine{
			Name: r.ExternalName, Quantity: r.Quantity, UnitPrice: r.UnitPrice, Matched: r.Matched,
			Options: renglonesEntrantes(r.Options),
		})
	}
	return out
}

// Accept acepta el pedido en su origen y lo mete al POS.
func (s *IncomingOrdersService) Accept(ctx context.Context, source domain.IncomingSource, id, userID int64) (PedidoAceptado, error) {
	switch source {
	case domain.IncomingSourcePlatform:
		return s.platform.Aceptar(ctx, id, userID)
	}
	return PedidoAceptado{}, fmt.Errorf("%w: origen %q", domain.ErrValidation, source)
}

// Reject rechaza el pedido en su origen con el motivo que eligió quien opera.
func (s *IncomingOrdersService) Reject(ctx context.Context, source domain.IncomingSource, id, userID int64, reason string) error {
	switch source {
	case domain.IncomingSourcePlatform:
		return s.platform.Rechazar(ctx, id, userID, reason)
	}
	return fmt.Errorf("%w: origen %q", domain.ErrValidation, source)
}

// simulatedOrderDecider confirma aceptar y rechazar sin hablar con la plataforma. SOLO DESARROLLO
// (PLATFORM_ORDERS_SIMULATED, que config.Validate rechaza fuera de desarrollo): en local no hay
// credenciales ni pedidos de prueba, y sin esto la alerta no se puede probar de punta a punta.
type simulatedOrderDecider struct{}

var errSimulatedNoDetail = errors.New("la simulación no trae detalles de pedido")

func (simulatedOrderDecider) TraerDetalleDePedido(context.Context, string) ([]byte, error) {
	return nil, errSimulatedNoDetail
}
func (simulatedOrderDecider) AceptarPedido(context.Context, string, string) error { return nil }
func (simulatedOrderDecider) RechazarPedido(_ context.Context, _ string, motivo, _ string) error {
	if !domain.MotivoDeRechazoValido(motivo) {
		return fmt.Errorf("%w: %q", domain.ErrMotivoDesconocido, motivo)
	}
	return nil
}
func (simulatedOrderDecider) ClaseDeFallo(error) domain.ClaseDeFallo {
	return domain.FalloRespuestaInvalida
}

// SimulateDecisions hace que aceptar y rechazar se confirmen sin hablar con la plataforma. Solo lo
// enciende main.go con PLATFORM_ORDERS_SIMULATED, que no arranca fuera de desarrollo.
func (s *PedidosDePlataformaService) SimulateDecisions() { s.simulated = true }
