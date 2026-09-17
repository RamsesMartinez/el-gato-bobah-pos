package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// DecisorDePedidos es lo que el servicio necesita de una plataforma para recibir y decidir.
//
// Interfaz de una sola implementación, igual que LectorDeMenu y por la misma razón que no es
// abstracción especulativa: **acota por tipo lo que el servicio puede pedirle a la plataforma**.
// Aquí el recorte importa más, porque este paquete sí tiene permitido escribir: si mañana alguien
// agrega al cliente de Uber un método que toque el menú, el servicio sigue sin poder llamarlo.
type DecisorDePedidos interface {
	TraerDetalleDePedido(ctx context.Context, liga string) ([]byte, error)
	AceptarPedido(ctx context.Context, pedidoID, referenciaPropia string) error
	RechazarPedido(ctx context.Context, pedidoID, motivo, explicacion string) error
	ClaseDeFallo(err error) domain.ClaseDeFallo
}

// AvisoEntrante es lo que la puerta pública entrega al servicio: los bytes crudos y sus cabeceras.
// El servicio decide si son auténticos; el handler no interpreta nada antes.
type AvisoEntrante struct {
	Crudo    []byte
	Firma    string
	Ambiente string
}

// PedidosDePlataformaService recibe los pedidos y los deja listos para que alguien decida.
type PedidosDePlataformaService struct {
	store     *store.Store
	decisores map[string]DecisorDePedidos // por nombre de plataforma
	// ambiente es "sandbox" o "production", el de ESTE sistema. Un aviso que declara otro se
	// rechaza: procesarlo mezclaría un pedido real con datos de prueba.
	ambiente string
	// plazo es cuánto da la plataforma para decidir. Va como constante del servicio y no como
	// variable de entorno: lo fija la plataforma, no la configuración de este sistema.
	plazo time.Duration
	ahora func() time.Time
}

// PlazoParaDecidir: la plataforma cancela sola a los 11.5 minutos, y a los 90 segundos sin
// respuesta llama por teléfono al local. Se descuenta un margen para que el reloj de la pantalla no
// prometa tiempo que ya no existe.
const PlazoParaDecidir = 11*time.Minute + 30*time.Second

func NewPedidosDePlataformaService(s *store.Store, decisores map[string]DecisorDePedidos, ambiente string, now func() time.Time) *PedidosDePlataformaService {
	if now == nil {
		now = time.Now
	}
	return &PedidosDePlataformaService{
		store: s, decisores: decisores, ambiente: ambiente,
		plazo: PlazoParaDecidir, ahora: now,
	}
}

// sobreDelAviso es lo poco que se lee del cuerpo ANTES de autenticarlo: lo justo para saber a quién
// preguntarle si el cuerpo es auténtico. Nada de esto se cree hasta que la firma valide.
type sobreDelAviso struct {
	EventType string `json:"event_type"`
	EventID   string `json:"event_id"`
	Meta      struct {
		StoreID    string `json:"user_id"`
		ResourceID string `json:"resource_id"`
	} `json:"meta"`
	ResourceHref string `json:"resource_href"`
}

// RecibirAviso es el camino completo de un aviso: autenticar, resolver de quién es, traer el
// detalle y registrarlo.
//
// EL ORDEN IMPORTA Y NO ES EL INTUITIVO. Primero se lee el identificador de tienda SIN CREER en el
// cuerpo, porque hace falta para saber con qué llave verificar. Solo después de que la firma valida
// se cree algo de lo que venía adentro. Es el mismo orden que usan todas las plataformas que
// entregan webhooks multi-cuenta.
//
// DEVUELVE ERROR CUANDO NO SE PUDO PROCESAR, y eso es lo que hace que la plataforma reintente. Un
// aviso confirmado que en realidad falló es un pedido perdido en silencio: el cliente espera comida
// que nadie está haciendo.
func (s *PedidosDePlataformaService) RecibirAviso(ctx context.Context, plataforma string, in AvisoEntrante) error {
	if in.Ambiente != "" && !equalFold(in.Ambiente, s.ambiente) {
		return fmt.Errorf("%w: llegó de %q y aquí es %q", domain.ErrAmbienteEquivocado, in.Ambiente, s.ambiente)
	}
	var sobre sobreDelAviso
	if err := json.Unmarshal(in.Crudo, &sobre); err != nil {
		return fmt.Errorf("%w: el cuerpo no es JSON", domain.ErrValidation)
	}
	if sobre.EventID == "" || sobre.Meta.StoreID == "" {
		return fmt.Errorf("%w: el aviso no trae identificador ni tienda", domain.ErrValidation)
	}

	conexion, empresa, err := s.resolverTienda(ctx, plataforma, sobre.Meta.StoreID, in)
	if err != nil {
		return err
	}

	// A partir de aquí TODO corre con el tenant fijado en la empresa que la firma identificó.
	ctx, soltar, err := s.store.AcquireTenant(ctx, empresa)
	if err != nil {
		return fmt.Errorf("tomar la conexión de la empresa: %w", err)
	}
	defer soltar()

	// El aviso crudo se guarda ANTES de llamar a la plataforma, en su propia transacción: si el
	// proceso se muere a media llamada, el aviso ya está en disco y el reintento lo encuentra.
	eventoID, err := s.store.QC(ctx).InsertWebhookEvent(ctx, db.InsertWebhookEventParams{
		EventID: sobre.EventID, EventType: sobre.EventType,
		ConnectionID: conexion, RawBody: ptr(string(in.Crudo)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// `on conflict do nothing` no devolvió fila: ya lo habíamos procesado. Se CONFIRMA igual —
		// no confirmarlo haría que la plataforma lo reintente para siempre.
		return nil
	}
	if err != nil {
		return fmt.Errorf("registrar el aviso: %w", err)
	}

	claseDeFallo, err := s.procesar(ctx, plataforma, conexion, sobre)
	desenlace := "procesado"
	var fallo *string
	if err != nil {
		desenlace, fallo = "fallido", ptr(string(claseDeFallo))
	}
	if e := s.store.QC(ctx).MarkWebhookEventDone(ctx, db.MarkWebhookEventDoneParams{
		ID: eventoID, Outcome: &desenlace, FailureKind: fallo,
	}); e != nil {
		return fmt.Errorf("cerrar el aviso: %w", e)
	}
	return err
}

// resolverTienda traduce (plataforma, id de tienda de allá) → (conexión, empresa).
//
// **ES LA ÚNICA CONSULTA DEL REPOSITORIO QUE CORRE POR `store.Q` A PROPÓSITO**, y la excepción está
// acotada por tres cosas:
//
//  1. Quien llama es la plataforma, no una persona: no hay sesión, no hay JWT y por lo tanto no hay
//     `app.company_id` que fijar. La empresa ES EL RESULTADO de esta consulta, no su entrada — no
//     es que `QC` sea peor aquí, es que no hay nada que pasarle.
//  2. La consulta devuelve SOLO ids y las llaves de firma. Nada del negocio sale por aquí.
//  3. Todo lo demás corre después con el tenant fijado, y `TestUnAvisoNoCaeEnLaEmpresaEquivocada`
//     lo comprueba con dos empresas.
//
// CUANDO HAY VARIAS CANDIDATAS, DECIDE LA FIRMA. Dos empresas pueden registrar el mismo id de
// tienda —pasa en el ambiente de pruebas, donde las plataformas reparten tiendas de demostración
// compartidas— y la dueña es aquella cuya llave valide el cuerpo. Es más seguro que un único
// global: la empresa la determina quién pudo firmar, no un dato que cualquiera escribe en el
// cuerpo.
func (s *PedidosDePlataformaService) resolverTienda(ctx context.Context, plataforma, tienda string, in AvisoEntrante) (int64, int64, error) {
	filas, err := s.store.Q.ResolverTiendaDeAviso(ctx, db.ResolverTiendaDeAvisoParams{
		ExternalStoreID: tienda, PlatformName: plataforma,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("resolver la tienda del aviso: %w", err)
	}
	for _, f := range filas {
		if !f.IsActive {
			continue
		}
		llaves := domain.LlavesDeFirma{Primaria: textoDe(f.KeyPrimary), Secundaria: textoDe(f.KeySecondary)}
		if llaves.Verifican(in.Crudo, in.Firma) {
			return f.ConnectionID, f.CompanyID, nil
		}
	}
	// MISMO ERROR para «no conocemos esa tienda» y «la firma no cuadra», a propósito: distinguirlos
	// le diría a quien prueba a ciegas cuándo va acertando el identificador de una tienda real.
	return 0, 0, domain.ErrFirmaInvalida
}

// procesar hace lo que el tipo de aviso pida. Devuelve la clase del fallo para registrarla sin
// guardar jamás el mensaje crudo del error: el de una API ajena puede traer la dirección del
// cliente adentro.
func (s *PedidosDePlataformaService) procesar(ctx context.Context, plataforma string, conexion int64, sobre sobreDelAviso) (domain.ClaseDeFallo, error) {
	switch domain.ClasificarAviso(sobre.EventType) {
	case domain.AvisoPedidoNuevo:
		return s.registrarPedido(ctx, plataforma, conexion, sobre)
	default:
		// Se confirma y no se procesa. Los demás tipos llegan en fases posteriores; no confirmarlos
		// haría que la plataforma los reintente para siempre.
		return "", nil
	}
}

func (s *PedidosDePlataformaService) registrarPedido(ctx context.Context, plataforma string, conexion int64, sobre sobreDelAviso) (domain.ClaseDeFallo, error) {
	decisor, ok := s.decisores[plataforma]
	if !ok {
		return domain.FalloSinCredenciales, fmt.Errorf("no hay cliente configurado para %s", plataforma)
	}
	crudo, err := decisor.TraerDetalleDePedido(ctx, sobre.ResourceHref)
	if err != nil {
		// NO se confirma: es lo que hace que la plataforma reintente. Confirmar un pedido que no se
		// pudo leer lo pierde en silencio.
		return decisor.ClaseDeFallo(err), fmt.Errorf("traer el detalle del pedido: %w", err)
	}

	pedido, err := domain.LeerPedidoDePlataforma(crudo)
	if err != nil {
		return domain.FalloDetalleIlegible, fmt.Errorf("interpretar el pedido: %w", err)
	}

	// El catálogo para emparejar: lo que ya se emparejó a mano en la feature anterior.
	parejas, err := s.store.QC(ctx).ListItemLinks(ctx, conexion)
	if err != nil {
		return domain.FalloMapeoImposible, fmt.Errorf("leer el emparejamiento: %w", err)
	}
	porItem := make(map[string]int64, len(parejas))
	for _, p := range parejas {
		porItem[p.ExternalID] = p.ProductID
	}

	ahora := s.ahora()
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		id, err := q.InsertIncomingOrder(ctx, db.InsertIncomingOrderParams{
			ConnectionID: conexion, ExternalOrderID: pedido.ID, DisplayID: opt(pedido.FolioCorto),
			PlacedAt: optTime(pedido.Colocado), DecideBefore: optTime(ahora.Add(s.plazo)),
			ServiceType:  db.ServiceType(pedido.TipoDeServicio),
			CustomerName: opt(pedido.Cliente), Total: optDec(pedido.Total),
			RawDetail: ptr(string(crudo)),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Ya estaba: otro aviso del mismo pedido ganó la carrera. No es un fallo.
			return nil
		}
		if err != nil {
			return err
		}
		return insertarRenglones(ctx, q, id, 0, pedido.Renglones, porItem)
	})
	if err != nil {
		return domain.FalloMapeoImposible, fmt.Errorf("registrar el pedido: %w", err)
	}
	return "", nil
}

// insertarRenglones baja el árbol de renglones y opciones. Una opción es un renglón que apunta a
// otro renglón, igual que en la plataforma, donde un modificador ES un item.
func insertarRenglones(ctx context.Context, q *db.Queries, pedido int64, padre int64, renglones []domain.RenglonDePlataforma, porItem map[string]int64) error {
	for _, r := range renglones {
		var padreID *int64
		if padre != 0 {
			p := padre
			padreID = &p
		}
		var producto *int64
		if id, ok := porItem[r.ItemID]; ok {
			p := id
			producto = &p
		}
		// SIN PAREJA NO IMPIDE NADA: el cliente ya pagó y rechazar su pedido por un hueco de
		// nuestra contabilidad interna no es defendible. Queda contado y visible.
		id, err := q.InsertIncomingOrderLine(ctx, db.InsertIncomingOrderLineParams{
			IncomingOrderID: pedido, ParentLineID: padreID,
			ExternalItemID: r.ItemID, ExternalName: r.Nombre,
			Quantity: r.Cantidad, UnitPrice: r.PrecioUnitario, ProductID: producto,
		})
		if err != nil {
			return err
		}
		if err := insertarRenglones(ctx, q, pedido, id, r.Opciones, porItem); err != nil {
			return err
		}
	}
	return nil
}

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optTime deja la columna en NULL cuando no hay fecha. Es lo que permite registrar la cancelación
// que llega ANTES que su notificación: ahí nunca hubo un detalle que traer y no hay con qué llenar
// esas columnas.
func optTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func optDec(d decimal.Decimal) *decimal.Decimal { return &d }

func textoDe(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func equalFold(a, b string) bool {
	return len(a) == len(b) && (a == b || lower(a) == lower(b))
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

// PedidoPendiente es lo que la tableta pinta para decidir.
type PedidoPendiente struct {
	ID           int64              `json:"id"`
	PlatformName string             `json:"platformName"`
	DisplayID    string             `json:"displayId"`
	PlacedAt     *time.Time         `json:"placedAt"`
	DecideBefore *time.Time         `json:"decideBefore"`
	ServiceType  string             `json:"serviceType"`
	CustomerName string             `json:"customerName"`
	Total        decimal.Decimal    `json:"total"`
	Lines        []RenglonPendiente `json:"lines"`
}

// RenglonPendiente es un platillo del pedido. `Matched` es lo que la pantalla usa para señalar lo
// que no tiene pareja sin impedir aceptar.
type RenglonPendiente struct {
	ExternalName string             `json:"externalName"`
	Quantity     decimal.Decimal    `json:"quantity"`
	UnitPrice    decimal.Decimal    `json:"unitPrice"`
	ProductID    *int64             `json:"productId"`
	Matched      bool               `json:"matched"`
	Options      []RenglonPendiente `json:"options"`
}

// Pendientes son los pedidos que esperan decisión, con sus renglones.
//
// `Lines` y `Options` se devuelven SIEMPRE como arreglo, nunca nil: un slice nil de Go sale como
// `null` en JSON y la pantalla hace `.filter()` al pintar. Eso ya tumbó la pantalla de pedidos de
// producción con la API respondiendo 200 y sin una línea de error en el log.
func (s *PedidosDePlataformaService) Pendientes(ctx context.Context) ([]PedidoPendiente, error) {
	filas, err := s.store.QC(ctx).ListPendingIncomingOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar los pedidos pendientes: %w", err)
	}
	out := make([]PedidoPendiente, 0, len(filas))
	ids := make([]int64, 0, len(filas))
	for _, f := range filas {
		ids = append(ids, f.ID)
		out = append(out, PedidoPendiente{
			ID: f.ID, PlatformName: f.PlatformName, DisplayID: textoDe(f.DisplayID),
			PlacedAt: cuando(f.PlacedAt), DecideBefore: cuando(f.DecideBefore),
			ServiceType: string(f.ServiceType), CustomerName: textoDe(f.CustomerName),
			Total: valor(f.Total),
			Lines: []RenglonPendiente{},
		})
	}
	if len(ids) == 0 {
		return out, nil
	}
	renglones, err := s.store.QC(ctx).ListLinesOfIncomingOrders(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("listar los renglones: %w", err)
	}
	// Se arma el árbol en un paso: los renglones vienen ordenados con los padres primero
	// (`parent_line_id nulls first`), así que al llegar a una opción su padre ya está colocado.
	porID := map[int64]*PedidoPendiente{}
	for i := range out {
		porID[out[i].ID] = &out[i]
	}
	indice := map[int64]*RenglonPendiente{}
	for _, r := range renglones {
		nuevo := RenglonPendiente{
			ExternalName: r.ExternalName, Quantity: r.Quantity, UnitPrice: r.UnitPrice,
			ProductID: r.ProductID, Matched: r.ProductID != nil,
			Options: []RenglonPendiente{},
		}
		if r.ParentLineID == nil {
			p := porID[r.IncomingOrderID]
			if p == nil {
				continue
			}
			p.Lines = append(p.Lines, nuevo)
			indice[r.ID] = &p.Lines[len(p.Lines)-1]
			continue
		}
		padre := indice[*r.ParentLineID]
		if padre == nil {
			continue
		}
		padre.Options = append(padre.Options, nuevo)
		indice[r.ID] = &padre.Options[len(padre.Options)-1]
	}
	return out, nil
}

func cuando(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func valor(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}
