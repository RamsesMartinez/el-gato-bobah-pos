package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/secrets"
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
	store *store.Store
	// clients entrega el decisor de la empresa del contexto, armado con SUS credenciales: con uno
	// solo para todo el sistema, la empresa B aceptaría pedidos con la aplicación de la A.
	clients PlatformClients
	// cipher guarda la llave de firma: un respaldo de la base no debe servir para firmar avisos.
	cipher Cipher
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

func NewPedidosDePlataformaService(s *store.Store, clients PlatformClients, c Cipher, ambiente string, now func() time.Time) *PedidosDePlataformaService {
	if now == nil {
		now = time.Now
	}
	return &PedidosDePlataformaService{
		store: s, clients: clients, cipher: c, ambiente: ambiente,
		plazo: PlazoParaDecidir, ahora: now,
	}
}

// orderDeciderFor devuelve el decisor de la plataforma para la empresa del contexto, o por qué no hay.
func (s *PedidosDePlataformaService) orderDeciderFor(ctx context.Context, plataforma string) (DecisorDePedidos, error) {
	if s.clients == nil {
		return nil, fmt.Errorf("%w (%s)", domain.ErrPlataformaSinCredenciales, plataforma)
	}
	return s.clients.OrderDeciderFor(ctx, plataforma)
}

// EstadoDeLlave es lo que la pantalla puede saber de la llave de firma: si hay, no cuál es. Un
// secreto que se vuelve a leer desde la interfaz se filtra por una captura de pantalla, así que
// este tipo no tiene dónde llevarlo.
type EstadoDeLlave struct {
	Configurada bool `json:"configured"`
	// Rotando: la llave anterior sigue validando. Mientras nadie la retire se queda válida para
	// siempre, y RotadaEn es lo que deja ver desde cuándo.
	Rotando  bool       `json:"rotating"`
	RotadaEn *time.Time `json:"rotatedAt,omitempty"`
	// NeedsRecapture: hay llave pero este ambiente no la puede leer (un respaldo de otro ambiente).
	// Mientras siga así, ningún aviso de esta empresa valida.
	NeedsRecapture bool `json:"needsRecapture"`
}

// EstadoDeLlave dice si la empresa ya capturó la llave de esa plataforma.
func (s *PedidosDePlataformaService) EstadoDeLlave(ctx context.Context, plataforma int16) (EstadoDeLlave, error) {
	fila, err := s.store.QC(ctx).GetWebhookKeyState(ctx, plataforma)
	if errors.Is(err, pgx.ErrNoRows) {
		return EstadoDeLlave{}, nil
	}
	if err != nil {
		return EstadoDeLlave{}, fmt.Errorf("leer el estado de la llave: %w", err)
	}
	e := EstadoDeLlave{Configurada: fila.Configurada, Rotando: fila.Rotando}
	if fila.RotatedAt.Valid {
		e.RotadaEn = &fila.RotatedAt.Time
	}
	if _, err := s.currentSigningKey(ctx, plataforma); errors.Is(err, secrets.ErrUnreadable) {
		e.NeedsRecapture = true
	}
	return e, nil
}

// currentSigningKey descifra la primaria. ErrNoRows si no hay; secrets.ErrUnreadable si es de otro
// ambiente.
func (s *PedidosDePlataformaService) currentSigningKey(ctx context.Context, plataforma int16) (string, error) {
	fila, err := s.store.QC(ctx).GetWebhookKeyPrimary(ctx, plataforma)
	if err != nil {
		return "", err
	}
	empresa, err := sessionCompanyFor(ctx, fila.CompanyID)
	if err != nil {
		return "", err
	}
	claro, err := s.cipher.Decrypt(ctx, fila.KeyPrimaryEncrypted,
		domain.CredentialAAD(empresa, plataforma, domain.SecretSigningKey))
	return string(claro), err
}

// GuardarLlave captura la llave de firma o, si ya había una, la cambia.
//
// CAMBIARLA NO TIRA LA ANTERIOR: la deja de secundaria y las dos validan. La plataforma no empieza
// a firmar con la nueva en el mismo instante en que alguien la pega aquí, y los avisos que ya
// venían en camino traen la firma vieja; tirarla de golpe los rechaza, y un aviso rechazado es un
// pedido que no entra. La anterior se retira aparte, con RetirarLlaveAnterior.
//
// `empresa` entra explícita porque es la AAD con la que se cifra; es la del usuario que captura.
func (s *PedidosDePlataformaService) GuardarLlave(ctx context.Context, empresa int64, plataforma int16, llave string) error {
	llave, err := domain.NormalizarLlaveDeFirma(llave)
	if err != nil {
		return err
	}
	// «Es la misma» se compara aquí, en claro: la base ya no puede, porque dos cifrados del mismo
	// valor salen distintos. Una vigente ilegible (de otro ambiente) no es «la misma»: se reemplaza.
	current, err := s.currentSigningKey(ctx, plataforma)
	switch {
	case err == nil && current == llave:
		return fmt.Errorf("%w: es la misma llave que ya está guardada", domain.ErrValidation)
	case err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, secrets.ErrUnreadable):
		return fmt.Errorf("leer la llave current: %w", err)
	}
	ciphertext, err := s.cipher.Encrypt(ctx, []byte(llave), domain.CredentialAAD(empresa, plataforma, domain.SecretSigningKey))
	if err != nil {
		return keyServiceError(ctx, "cifrar la llave de firma", err)
	}
	q := s.store.QC(ctx)
	// Primero se intenta rotar: si ya hay llave, es un cambio. Si no la hay, no toca ninguna fila y
	// es la primera captura. En ese orden no hace falta leer antes de escribir.
	filas, err := q.RotateWebhookKey(ctx, db.RotateWebhookKeyParams{DeliveryPlatformID: plataforma, KeyPrimaryEncrypted: ciphertext})
	if err == nil && filas == 0 {
		err = q.UpsertWebhookKey(ctx, db.UpsertWebhookKeyParams{DeliveryPlatformID: plataforma, KeyPrimaryEncrypted: ciphertext})
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503":
			// La FK compuesta: o la plataforma no existe, o es de otra empresa.
			return fmt.Errorf("%w: esa plataforma no existe para esta empresa", domain.ErrValidation)
		}
	}
	if err != nil {
		return fmt.Errorf("guardar la llave de firma: %w", err)
	}
	return nil
}

// RetirarLlaveAnterior termina el cambio de llave: la anterior deja de validar. Se hace cuando la
// plataforma ya firma con la nueva; antes, los avisos en camino se pierden.
func (s *PedidosDePlataformaService) RetirarLlaveAnterior(ctx context.Context, plataforma int16) error {
	filas, err := s.store.QC(ctx).FinishWebhookKeyRotation(ctx, plataforma)
	if err != nil {
		return fmt.Errorf("retirar la llave anterior: %w", err)
	}
	if filas == 0 {
		return domain.ErrNotFound
	}
	return nil
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

// AvisoRecibido dice qué dejó un aviso, para que quien lo recibió avise a las tabletas.
type AvisoRecibido struct {
	Empresa int64
	// PedidoNuevo es falso en un reintento, en un segundo aviso del mismo pedido y en cualquier
	// aviso que no sea de pedido: ninguno de ésos trae nada que la tableta no tenga ya.
	PedidoNuevo bool
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
func (s *PedidosDePlataformaService) RecibirAviso(ctx context.Context, plataforma string, in AvisoEntrante) (AvisoRecibido, error) {
	if in.Ambiente != "" && !equalFold(in.Ambiente, s.ambiente) {
		return AvisoRecibido{}, fmt.Errorf("%w: llegó de %q y aquí es %q", domain.ErrAmbienteEquivocado, in.Ambiente, s.ambiente)
	}
	var sobre sobreDelAviso
	if err := json.Unmarshal(in.Crudo, &sobre); err != nil {
		return AvisoRecibido{}, fmt.Errorf("%w: el cuerpo no es JSON", domain.ErrValidation)
	}
	if sobre.EventID == "" || sobre.Meta.StoreID == "" {
		return AvisoRecibido{}, fmt.Errorf("%w: el aviso no trae identificador ni tienda", domain.ErrValidation)
	}

	conexion, empresa, err := s.resolverTienda(ctx, plataforma, sobre.Meta.StoreID, in)
	if err != nil {
		return AvisoRecibido{}, err
	}

	// A partir de aquí TODO corre con el tenant fijado en la empresa que la firma identificó.
	ctx, soltar, err := s.store.AcquireTenant(ctx, empresa)
	if err != nil {
		return AvisoRecibido{}, fmt.Errorf("tomar la conexión de la empresa: %w", err)
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
		return AvisoRecibido{Empresa: empresa}, nil
	}
	if err != nil {
		return AvisoRecibido{}, fmt.Errorf("registrar el aviso: %w", err)
	}

	nuevo, claseDeFallo, err := s.procesar(ctx, plataforma, conexion, sobre)
	desenlace := "procesado"
	var fallo *string
	if err != nil {
		desenlace, fallo = "fallido", ptr(string(claseDeFallo))
	}
	if e := s.store.QC(ctx).MarkWebhookEventDone(ctx, db.MarkWebhookEventDoneParams{
		ID: eventoID, Outcome: &desenlace, FailureKind: fallo,
	}); e != nil {
		return AvisoRecibido{}, fmt.Errorf("cerrar el aviso: %w", e)
	}
	if err != nil {
		return AvisoRecibido{}, err
	}
	return AvisoRecibido{Empresa: empresa, PedidoNuevo: nuevo}, nil
}

// resolverTienda traduce (plataforma, id de tienda de allá) → (conexión, empresa).
//
// EN DOS PASOS, Y EL ORDEN ES LO QUE MANTIENE EL SECRETO DENTRO DE RLS:
//
//  1. Las CANDIDATAS salen de una vista con dueño propio, sin empresa fijada — porque la empresa es
//     el resultado, no la entrada. Esa vista no trae ninguna llave: lo más que revela es qué id de
//     tienda pertenece a qué empresa.
//  2. La LLAVE de cada candidata se lee ya con el tenant fijado, o sea bajo RLS y por la vía
//     ordinaria. El secreto nunca sale del aislamiento.
//
// Sin ese corte, la vista tendría que devolver las llaves de firma y un `select` sin filtro sobre
// ella las volcaría todas — y quien las tuviera podría firmar avisos a nombre de cualquier empresa.
//
// SI MÁS DE UNA CANDIDATA VALIDA, SE RECHAZA. El diseño decía que «cuando hay varias, decide la
// firma», y eso solo es cierto si las llaves difieren. Con UNA sola aplicación de la plataforma —el
// despliegue de hoy— la llave es un atributo de esa aplicación, no de la empresa: dos empresas
// capturan la MISMA del mismo tablero. Ahí quien se lleva el pedido lo decidiría el orden físico de
// las filas, que cambia con un VACUUM, y el pedido y su dinero entrarían a la empresa equivocada sin
// que nada falle. Una ambigüedad no se arregla reintentando: elegir a ciegas es peor que no elegir.
func (s *PedidosDePlataformaService) resolverTienda(ctx context.Context, plataforma, tienda string, in AvisoEntrante) (int64, int64, error) {
	candidatas, err := s.store.Q.CandidatasDelAviso(ctx, db.CandidatasDelAvisoParams{
		ExternalStoreID: tienda, PlatformName: plataforma,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("buscar la tienda del aviso: %w", err)
	}

	var duenias []db.CandidatasDelAvisoRow
	for _, c := range candidatas {
		if !c.IsActive {
			continue
		}
		llaves, err := s.llavesDe(ctx, c.CompanyID, c.ConnectionID)
		if err != nil {
			// Una candidata cuya llave no se puede leer NO se salta en silencio: podría ser la
			// dueña, y saltarla convertiría un problema de permisos en un «firma inválida» que
			// manda a quien depure a buscar en el lugar equivocado.
			return 0, 0, fmt.Errorf("leer la llave de la conexión %d: %w", c.ConnectionID, err)
		}
		if llaves.Verifican(in.Crudo, in.Firma) {
			duenias = append(duenias, c)
		}
	}

	switch len(duenias) {
	case 1:
		return duenias[0].ConnectionID, duenias[0].CompanyID, nil
	case 0:
		// MISMO ERROR para «no conocemos esa tienda» y «la firma no cuadra», a propósito:
		// distinguirlos le diría a quien prueba a ciegas cuándo va acertando el identificador de
		// una tienda real.
		return 0, 0, domain.ErrFirmaInvalida
	default:
		// Se responde lo MISMO que a una firma inválida —no se delata que la tienda existe ni que
		// hay dos— pero queda el evento de seguridad, que es lo único que permite notarlo.
		logging.SecurityEvent(ctx, "webhook_tienda_ambigua",
			"platform", plataforma, "candidatas", len(duenias))
		return 0, 0, fmt.Errorf("%w: %d empresas validan ese aviso", domain.ErrFirmaInvalida, len(duenias))
	}
}

// llavesDe lee la llave de una conexión CON el tenant de su empresa fijado. Es lo que mantiene el
// secreto bajo RLS: la vista que resuelve candidatas no lo entrega.
func (s *PedidosDePlataformaService) llavesDe(ctx context.Context, empresa, conexion int64) (domain.LlavesDeFirma, error) {
	ctxT, soltar, err := s.store.AcquireTenant(ctx, empresa)
	if err != nil {
		return domain.LlavesDeFirma{}, err
	}
	defer soltar()

	fila, err := s.store.QC(ctxT).LlaveDeFirmaDeLaConexion(ctxT, conexion)
	if errors.Is(err, pgx.ErrNoRows) {
		// Sin llave capturada esa empresa no puede recibir avisos todavía. No es un error del
		// sistema: es una configuración que falta, y su aviso simplemente no valida.
		return domain.LlavesDeFirma{}, nil
	}
	if err != nil {
		return domain.LlavesDeFirma{}, err
	}
	// La empresa es la de la CANDIDATA con la que se fijó la sesión, no la de la fila: si RLS
	// dejara pasar la llave de otra empresa, no descifra.
	if _, err := sessionCompanyFor(ctxT, fila.CompanyID); err != nil {
		return domain.LlavesDeFirma{}, err
	}
	aad := domain.CredentialAAD(empresa, fila.DeliveryPlatformID, domain.SecretSigningKey)
	primary, err := s.cipher.Decrypt(ctxT, fila.KeyPrimaryEncrypted, aad)
	if errors.Is(err, secrets.ErrUnreadable) {
		// Una llave de otro ambiente es como no tener llave: sus avisos no validan. NO se devuelve
		// error: tumbaría la resolución de TODAS las candidatas y la plataforma reintentaría para
		// siempre. Queda el evento, y la pantalla dice «por recapturar».
		logging.SecurityEvent(ctx, "webhook_key_unreadable", "connection_id", conexion)
		return domain.LlavesDeFirma{}, nil
	}
	if err != nil {
		return domain.LlavesDeFirma{}, err
	}
	llaves := domain.LlavesDeFirma{Primaria: string(primary)}
	if fila.KeySecondaryEncrypted != nil {
		secondary, err := s.cipher.Decrypt(ctxT, fila.KeySecondaryEncrypted, aad)
		if err != nil && !errors.Is(err, secrets.ErrUnreadable) {
			return domain.LlavesDeFirma{}, err
		}
		llaves.Secundaria = string(secondary)
	}
	return llaves, nil
}

// procesar hace lo que el tipo de aviso pida. Devuelve la clase del fallo para registrarla sin
// guardar jamás el mensaje crudo del error: el de una API ajena puede traer la dirección del
// cliente adentro.
func (s *PedidosDePlataformaService) procesar(ctx context.Context, plataforma string, conexion int64, sobre sobreDelAviso) (nuevo bool, _ domain.ClaseDeFallo, _ error) {
	switch domain.ClasificarAviso(sobre.EventType) {
	case domain.AvisoPedidoNuevo:
		return s.registrarPedido(ctx, plataforma, conexion, sobre)
	default:
		// Se confirma y no se procesa. Los demás tipos llegan en fases posteriores; no confirmarlos
		// haría que la plataforma los reintente para siempre.
		return false, "", nil
	}
}

func (s *PedidosDePlataformaService) registrarPedido(ctx context.Context, plataforma string, conexion int64, sobre sobreDelAviso) (nuevo bool, _ domain.ClaseDeFallo, _ error) {
	decisor, err := s.orderDeciderFor(ctx, plataforma)
	if err != nil {
		return false, domain.FalloSinCredenciales, fmt.Errorf("no hay cliente de %s para esta empresa: %w", plataforma, err)
	}
	crudo, err := decisor.TraerDetalleDePedido(ctx, sobre.ResourceHref)
	if err != nil {
		// NO se confirma: es lo que hace que la plataforma reintente. Confirmar un pedido que no se
		// pudo leer lo pierde en silencio.
		return false, decisor.ClaseDeFallo(err), fmt.Errorf("traer el detalle del pedido: %w", err)
	}

	pedido, err := domain.LeerPedidoDePlataforma(crudo)
	if err != nil {
		return false, domain.FalloDetalleIlegible, fmt.Errorf("interpretar el pedido: %w", err)
	}

	// El catálogo para emparejar: lo que ya se emparejó a mano en la feature anterior.
	parejas, err := s.store.QC(ctx).ListItemLinks(ctx, conexion)
	if err != nil {
		return false, domain.FalloMapeoImposible, fmt.Errorf("leer el emparejamiento: %w", err)
	}
	// Solo las parejas de PRODUCTO: una opción ligada a una opción del POS (0077) no es un producto,
	// y tratarla como tal cargaría la venta a lo que tenga ese número en el catálogo.
	porItem := make(map[string]int64, len(parejas))
	for _, p := range parejas {
		if p.ProductID != nil && p.LocalKind == string(domain.LocalProducto) {
			porItem[p.ExternalID] = *p.ProductID
		}
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
		nuevo = true
		return insertarRenglones(ctx, q, id, 0, pedido.Renglones, porItem)
	})
	if err != nil {
		return false, domain.FalloMapeoImposible, fmt.Errorf("registrar el pedido: %w", err)
	}
	return nuevo, "", nil
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

// PedidoAceptado es lo que la tableta necesita para imprimir sin volver a pedir nada.
type PedidoAceptado struct {
	ID          int64  `json:"id"`
	Number      int    `json:"number"`
	PlatformRef string `json:"platformRef"`
}

// Aceptar confirma el pedido a la plataforma y lo mete al POS ya pagado.
//
// EL ORDEN ES: PRIMERO LA PLATAFORMA, DESPUÉS NUESTRA BASE. Es lo menos malo de dos caminos malos.
// Si se creara el pedido local primero y la plataforma rechazara la aceptación, quedaría una venta
// en el corte por un pedido que la plataforma va a cancelar sola en minutos — dinero que el negocio
// nunca recibió. Al revés, si la plataforma acepta y la escritura local falla, no se pierde nada:
// el aviso crudo y el detalle siguen guardados y el pedido sigue pendiente en la pantalla.
//
// ponytail: reintentar tras ese fallo vuelve a llamar a la plataforma, y no sabemos todavía qué
// contesta ante un pedido ya aceptado — la documentación no lo dice y no hay forma de generar un
// pedido de prueba para averiguarlo. El techo es ese: si contesta error, el operador tiene que
// decidirlo desde la aplicación de la plataforma. Se resuelve el día que se pueda probar.
func (s *PedidosDePlataformaService) Aceptar(ctx context.Context, entranteID, usuarioID int64) (PedidoAceptado, error) {
	ent, err := s.store.QC(ctx).GetIncomingOrder(ctx, entranteID)
	if err != nil {
		return PedidoAceptado{}, fmt.Errorf("%w: ese pedido no existe", domain.ErrNotFound)
	}
	if ent.State != db.PlatformOrderStatePendiente {
		return PedidoAceptado{}, fmt.Errorf("%w (está %s)", domain.ErrPedidoYaDecidido, ent.State)
	}
	decisor, err := s.orderDeciderFor(ctx, ent.PlatformName)
	if err != nil {
		return PedidoAceptado{}, err
	}
	if err := decisor.AceptarPedido(ctx, ent.ExternalOrderID, ""); err != nil {
		// La plataforma NO confirmó: no se crea nada. Aceptar aquí y no allá es la peor
		// combinación posible — la cocina prepara y la plataforma cancela.
		return PedidoAceptado{}, fmt.Errorf("%w: %s no confirmó la aceptación", domain.ErrConflict, ent.PlatformName)
	}

	renglones, err := s.store.QC(ctx).ListLinesOfIncomingOrders(ctx, []int64{entranteID})
	if err != nil {
		return PedidoAceptado{}, fmt.Errorf("leer los renglones: %w", err)
	}

	plataformaID, sucursal, err := s.plataformaDeLaConexion(ctx, ent.ConnectionID)
	if err != nil {
		return PedidoAceptado{}, err
	}
	// EL MÉTODO DE PAGO DISTINGUE EN LÍNEA DE CONTRA ENTREGA. Meterlos en el mismo cajón hace que
	// el corte pida efectivo que nadie recibió. Hoy solo llegan pedidos pagados en la aplicación;
	// el día que lleguen de los otros, esto es lo que cambia.
	metodo, err := s.store.QC(ctx).GetPlatformPaymentMethod(ctx, db.GetPlatformPaymentMethodParams{
		DeliveryPlatformID: &plataformaID, EnEfectivo: false,
	})
	if err != nil {
		return PedidoAceptado{}, fmt.Errorf("%w: falta el método de pago de %s", domain.ErrValidation, ent.PlatformName)
	}

	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	fecha := pgtype.Date{Time: domain.BusinessDate(s.ahora(), domain.LoadBusinessLocation(tz)), Valid: true}
	total := valor(ent.Total)

	var creado PedidoAceptado
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// `daily_number` = 0 MIENTRAS NO HAY TURNO, y no es un valor inventado al azar: el único
		// `orders_folio_turno_key` es por (empresa, turno, número), y con el turno en NULL Postgres
		// trata cada fila como distinta. El folio real se lo da el turno al reclamarlo. Hasta
		// entonces, la identidad del pedido es su folio en la plataforma, que además es el que el
		// cliente dice por teléfono.
		sesion, numero, err := s.turnoYFolio(ctx, q, sucursal)
		if err != nil {
			return err
		}
		ord, err := q.CreateOrder(ctx, db.CreateOrderParams{
			ClientUuid:         uuid.New(),
			BusinessDate:       fecha,
			DailyNumber:        numero,
			ServiceType:        ent.ServiceType,
			DeliveryPlatformID: &plataformaID,
			CustomerName:       ent.CustomerName,
			RegisterSessionID:  sesion,
			// QUIEN ACEPTA ES QUIEN LO ABRIÓ, y por eso la fila se crea aquí y no al recibir: un
			// pedido que llega solo no tiene quién lo abrió, y `opened_by` es not null. Inventar un
			// usuario de sistema para llenarla es el parche que ya se rechazó una vez.
			OpenedBy:         usuarioID,
			Subtotal:         total,
			Total:            total,
			Status:           db.OrderStatusAbierta,
			PlatformOrderRef: &ent.ExternalOrderID,
			PlatformRefSetBy: &usuarioID,
			PlatformRefSetAt: pgtype.Timestamptz{Time: s.ahora(), Valid: true},
			// La de la tienda, también cuando no hay turno: sin ella el trigger pondría «la única de
			// la empresa», que con dos sucursales truena.
			BranchID: &sucursal,
		})
		if err != nil {
			return err
		}
		generico, err := productoGenerico(ctx, q)
		if err != nil {
			return err
		}
		if err := copiarRenglones(ctx, q, ord.ID, generico, renglones); err != nil {
			return err
		}
		// YA PAGADO POR LA PLATAFORMA. Dejarlo por cobrar inventa un faltante en el corte por un
		// dinero que el negocio sí recibió, solo que no por la caja.
		if err := q.CreatePlatformOrderPayment(ctx, db.CreatePlatformOrderPaymentParams{
			OrderID: ord.ID, PaymentMethodID: metodo, Amount: total,
			RegisterSessionID: sesion, ReceivedBy: &usuarioID,
			Reference: &ent.ExternalOrderID, ClientUuid: ptrUUID(uuid.New()),
		}); err != nil {
			return err
		}
		filas, err := q.AcceptIncomingOrder(ctx, db.AcceptIncomingOrderParams{
			ID: entranteID, DecidedBy: &usuarioID, OrderID: &ord.ID,
		})
		if err != nil {
			return err
		}
		if filas == 0 {
			// Otra tableta ganó entre la lectura y este update. La transacción se deshace entera.
			return domain.ErrPedidoYaDecidido
		}
		creado = PedidoAceptado{ID: ord.ID, Number: int(ord.DailyNumber), PlatformRef: ent.ExternalOrderID}
		return nil
	})
	if err != nil {
		return PedidoAceptado{}, err
	}
	return creado, nil
}

// turnoYFolio devuelve el turno abierto de la sucursal de la tienda y el folio que le toca, o
// (nil, 0) si no hay turno. Es el turno de ESA sucursal y no «el» de la empresa: con dos, un pedido
// de la tienda de una no puede caer en el corte de la otra.
//
// Aceptar NO exige turno abierto: la cocina no puede esperar a que alguien abra caja. Es la
// decisión del dueño del 2026-09-16, y lo que la paga es que al ABRIR turno se vea de un vistazo
// qué pedidos quedan considerados en esa apertura.
func (s *PedidosDePlataformaService) turnoYFolio(ctx context.Context, q *db.Queries, sucursal int64) (*int64, int32, error) {
	sess, err := q.GetOpenPrimarySessionOfBranch(ctx, sucursal)
	if err != nil {
		return nil, 0, nil //nolint:nilerr // sin turno abierto NO es un error: es el camino normal de madrugada
	}
	num, err := q.NextFolioNumber(ctx, sess.ID)
	if err != nil {
		return nil, 0, err
	}
	return &sess.ID, num, nil
}

// ReclamarHuerfanos le da turno y folio real a los pedidos que se aceptaron sin turno abierto.
//
// UNO POR UNO Y RENUMERANDO, no un update en bloque, y ésa es la parte que no era obvia. Mientras
// el pedido no tiene turno su `daily_number` es 0 y no choca con nada porque el único
// `orders_folio_turno_key` es por (empresa, turno, número) y los NULL son distintos entre sí. En el
// momento de asignarle el turno, ese 0 compite con los folios reales de ese turno — y dos pedidos
// huérfanos chocan entre ellos.
func ReclamarPedidosDePlataformaHuerfanos(ctx context.Context, q *db.Queries, sesionID int64, fecha pgtype.Date) error {
	ids, err := q.ListOrphanPlatformOrders(ctx, db.ListOrphanPlatformOrdersParams{BusinessDate: fecha, SessionID: sesionID})
	if err != nil {
		return fmt.Errorf("buscar pedidos de plataforma sin turno: %w", err)
	}
	for _, id := range ids {
		num, err := q.NextFolioNumber(ctx, sesionID)
		if err != nil {
			return err
		}
		if err := q.ClaimPlatformOrder(ctx, db.ClaimPlatformOrderParams{
			ID: id, RegisterSessionID: &sesionID, DailyNumber: num,
		}); err != nil {
			return fmt.Errorf("darle turno al pedido %d: %w", id, err)
		}
	}
	return nil
}

// plataformaDeLaConexion devuelve la plataforma y la sucursal de la tienda que recibió el pedido.
func (s *PedidosDePlataformaService) plataformaDeLaConexion(ctx context.Context, conexion int64) (int16, int64, error) {
	c, err := s.store.QC(ctx).GetPlatformConnection(ctx, conexion)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: la tienda del pedido ya no está conectada", domain.ErrNotFound)
	}
	return c.DeliveryPlatformID, c.BranchID, nil
}

// copiarRenglones pasa los renglones del pedido entrante al pedido del POS.
//
// EL PRECIO ES EL DE LA PLATAFORMA, no el del catálogo: la dirección de la verdad en precio es de
// arriba hacia abajo. Un renglón sin pareja entra igual, ligado al producto genérico de la empresa
// (0077) con el nombre que mandó la plataforma: rechazar un pedido pagado por un hueco de nuestra
// contabilidad interna no es defendible, y sin producto el renglón no sigue el camino de los demás.
//
// Las opciones van en la NOTA del renglón: `order_line_modifiers` exige una opción del catálogo y
// la de la plataforma puede no tenerla. Así cocina lee lo que el cliente pidió.
func copiarRenglones(ctx context.Context, q *db.Queries, pedido, generico int64, renglones []db.ListLinesOfIncomingOrdersRow) error {
	opciones := map[int64][]domain.PlatformLineOption{}
	for _, r := range renglones {
		if r.ParentLineID != nil {
			opciones[*r.ParentLineID] = append(opciones[*r.ParentLineID], domain.PlatformLineOption{
				Name: r.ExternalName, Quantity: r.Quantity, UnitPrice: r.UnitPrice,
			})
		}
	}
	for _, r := range renglones {
		if r.ParentLineID != nil {
			continue
		}
		producto := r.ProductID
		if producto == nil {
			producto = &generico
		}
		var nota *string
		if n := domain.PlatformLineNotes(opciones[r.ID]); n != "" {
			nota = &n
		}
		if _, err := q.CreateOrderLine(ctx, db.CreateOrderLineParams{
			OrderID:     pedido,
			ProductID:   producto,
			ProductName: r.ExternalName,
			Quantity:    r.Quantity,
			UnitPrice:   r.UnitPrice,
			LineTotal:   r.Quantity.Mul(r.UnitPrice).Round(2),
			Notes:       nota,
		}); err != nil {
			return err
		}
	}
	return nil
}

func ptrUUID(u uuid.UUID) *uuid.UUID { return &u }

// productoGenerico devuelve el producto al que va un renglón de plataforma sin pareja: el de la
// empresa de la sesión, creado si faltara (una empresa de antes de la 0077 que se saltó ambos
// caminos). Crearlo es mejor que rechazar un pedido ya pagado.
func productoGenerico(ctx context.Context, q *db.Queries) (int64, error) {
	id, err := q.GetPlatformUnpairedProduct(ctx)
	if err != nil {
		return 0, fmt.Errorf("producto genérico: %w", err)
	}
	return id, nil
}
