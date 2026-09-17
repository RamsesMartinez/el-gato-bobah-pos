package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LectorDeMenu es lo que el servicio necesita de una plataforma: leer, y nada más.
//
// Es una interfaz de UNA sola implementación, que el principio VI normalmente prohíbe. Existe por
// una razón distinta de la abstracción: **acota por tipo lo que el servicio puede pedirle a una
// plataforma**. Si mañana alguien agrega un método de escritura al cliente de Uber, el servicio
// sigue sin poder llamarlo, y eso es justo lo que la US4 promete.
type LectorDeMenu interface {
	LeerMenu(ctx context.Context, storeID string) ([]domain.ItemDePlataforma, error)
	ListarTiendas(ctx context.Context) ([]domain.TiendaDePlataforma, error)
	ClaseDeFallo(err error) domain.ClaseDeFallo
}

// MenusDePlataformaService orquesta la lectura, el emparejamiento y la comparación.
//
// TODA consulta va por `store.QC(ctx)` y NUNCA por `store.Q`. `QC` devuelve las consultas atadas a
// la conexión que el middleware de tenant tomó para este request, con `app.company_id` fijado;
// `store.Q` toma una conexión arbitraria del pool, sin ese ajuste.
//
// La diferencia no se ve en desarrollo: la API de dev se conecta como owner —que salta RLS— y la
// base de pruebas trae un `alter database ... set app.company_id`, así que cualquier conexión
// hereda una empresa. En producción, con el rol `gatobobah_app`, una conexión sin el ajuste ve cero
// filas y todo inserto revienta contra el `not null` de `company_id`. Lo atrapa
// `TestElServicioRespetaElTenantBajoElRolDeApp`, que pide el tenant de OTRA empresa justamente para
// que el default de la base no lo enmascare.
type MenusDePlataformaService struct {
	store    *store.Store
	lectores map[string]LectorDeMenu // por nombre de plataforma: "Uber Eats", …
	ahora    func() time.Time
}

func NewMenusDePlataformaService(s *store.Store, lectores map[string]LectorDeMenu, now func() time.Time) *MenusDePlataformaService {
	if now == nil {
		now = time.Now
	}
	return &MenusDePlataformaService{store: s, lectores: lectores, ahora: now}
}

// --- Conexiones ---

type Conexion struct {
	ID              int64             `json:"id"`
	PlatformID      int16             `json:"platformId"`
	PlatformName    string            `json:"platformName"`
	ExternalStoreID string            `json:"externalStoreId"`
	Label           string            `json:"label"`
	Activa          bool              `json:"active"`
	Configurada     bool              `json:"credentialsConfigured"`
	UltimaLectura   *ResumenDeLectura `json:"lastRead"`
}

// ResumenDeLectura: de cuándo es la foto y si sirvió. Los tres estados se distinguen a propósito
// (FR-003) — «falló», «sin diferencias» y «nunca se ha leído» significan cosas opuestas y una
// pantalla mal hecha los muestra igual.
type ResumenDeLectura struct {
	ID           int64      `json:"id"`
	Estado       string     `json:"status"`
	Inicio       time.Time  `json:"startedAt"`
	Fin          *time.Time `json:"finishedAt"`
	Items        *int32     `json:"itemCount"`
	ClaseDeFallo *string    `json:"failureKind"`
	Vieja        bool       `json:"stale"`
}

func (s *MenusDePlataformaService) ListarConexiones(ctx context.Context) ([]Conexion, error) {
	filas, err := s.store.QC(ctx).ListPlatformConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar conexiones de plataforma: %w", err)
	}
	// Nunca nil: un arreglo prometido se entrega aunque esté vacío. Un slice nil sale como `null`
	// y el front revienta al filtrarlo, con la API respondiendo 200 y sin un solo error en el log.
	out := make([]Conexion, 0, len(filas))
	for _, f := range filas {
		c := Conexion{
			ID: f.ID, PlatformID: f.DeliveryPlatformID, PlatformName: f.PlatformName,
			ExternalStoreID: f.ExternalStoreID, Label: f.Label, Activa: f.IsActive,
			Configurada: s.lectores[f.PlatformName] != nil,
		}
		if r, err := s.store.QC(ctx).GetLastMenuRead(ctx, f.ID); err == nil {
			c.UltimaLectura = s.resumen(r)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("última lectura de la conexión %d: %w", f.ID, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *MenusDePlataformaService) resumen(r db.GetLastMenuReadRow) *ResumenDeLectura {
	res := &ResumenDeLectura{ID: r.ID, Estado: string(r.Status), Inicio: r.StartedAt}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time
		res.Fin = &t
	}
	res.Items = r.ItemCount
	res.ClaseDeFallo = r.FailureKind
	// `Vieja` lo calcula el SERVIDOR, no la pantalla: si lo derivara el front, dos clientes con el
	// reloj distinto mostrarían cosas distintas del mismo dato.
	res.Vieja = r.StartedAt.Before(s.ahora().AddDate(0, 0, -domain.RetencionDeLecturasEnDias))
	return res
}

// TiendasDisponibles lista las tiendas que la plataforma alcanza, marcando cuáles ya están dadas
// de alta aquí.
//
// EXISTE PARA QUE NADIE TECLEE UN UUID. El identificador de tienda es el único dato del alta que una
// persona no puede producir de memoria ni deducir, y pedirlo escrito es lo que deja fuera a quien
// nunca ha usado el sistema.
//
// Marcar las ya registradas es la otra mitad: dejarlas en la lista y que el alta falle con «ya
// existe» hace que el operador crea que se equivocó de tienda.
func (s *MenusDePlataformaService) TiendasDisponibles(ctx context.Context, plataformaID int16) ([]domain.TiendaDePlataforma, error) {
	plat, err := s.store.QC(ctx).GetPlatformByID(ctx, plataformaID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Va por aquí y no por la FK: los chequeos de integridad saltan RLS, así que un id de
			// otra empresa pasaría y listaríamos tiendas contra la plataforma equivocada.
			return nil, fmt.Errorf("%w: esa plataforma no existe para esta empresa", domain.ErrValidation)
		}
		return nil, fmt.Errorf("plataforma %d: %w", plataformaID, err)
	}
	lector := s.lectores[plat.Name]
	if lector == nil {
		return nil, fmt.Errorf("%w (%s)", domain.ErrPlataformaSinCredenciales, plat.Name)
	}

	tiendas, err := lector.ListarTiendas(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar tiendas de %s: %w", plat.Name, err)
	}
	yaEstan, err := s.store.QC(ctx).ListPlatformConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("conexiones existentes: %w", err)
	}
	registradas := map[string]bool{}
	for _, c := range yaEstan {
		if c.DeliveryPlatformID == plataformaID {
			registradas[c.ExternalStoreID] = true
		}
	}
	for i := range tiendas {
		tiendas[i].YaRegistrada = registradas[tiendas[i].ID]
	}
	return tiendas, nil
}

type AltaDeConexion struct {
	PlatformID      int16
	ExternalStoreID string
	Label           string
}

func (s *MenusDePlataformaService) CrearConexion(ctx context.Context, in AltaDeConexion) (int64, error) {
	if in.ExternalStoreID == "" || len(in.ExternalStoreID) > 200 {
		return 0, fmt.Errorf("%w: el id de la tienda va entre 1 y 200 caracteres", domain.ErrValidation)
	}
	if in.Label == "" || len(in.Label) > 60 {
		return 0, fmt.Errorf("%w: la etiqueta de la tienda va entre 1 y 60 caracteres", domain.ErrValidation)
	}
	id, err := s.store.QC(ctx).CreatePlatformConnection(ctx, db.CreatePlatformConnectionParams{
		DeliveryPlatformID: in.PlatformID,
		ExternalStoreID:    in.ExternalStoreID,
		Label:              in.Label,
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			switch pg.Code {
			case "23505":
				return 0, domain.ErrConexionDuplicada
			case "23503":
				// La FK compuesta: o la plataforma no existe, o es de otra empresa. Las dos son un
				// dato malo del cliente, no una falla del servidor — 422 y no 500.
				return 0, fmt.Errorf("%w: esa plataforma no existe para esta empresa", domain.ErrValidation)
			}
		}
		return 0, fmt.Errorf("crear conexión de plataforma: %w", err)
	}

	// LOS MÉTODOS DE COBRO DE ESA PLATAFORMA SE CREAN AQUÍ, y este es el lugar correcto por lo que
	// dice el propio comentario de `SeedBasePaymentMethods`: los deja fuera porque «vender por Uber
	// exige que ese negocio haya hecho su propia vinculación con la plataforma». Conectar la tienda
	// ES esa vinculación.
	//
	// Sin esto, aceptar el primer pedido falla con «falta el método de pago» y el operador no tiene
	// desde dónde arreglarlo: los métodos de plataforma no se crean desde ninguna pantalla. Se
	// descubrió al escribir la prueba de aceptar con una empresa nueva — la empresa del negocio ya
	// los tenía de antes y eso lo tapaba.
	//
	// No tumba el alta si falla: la conexión ya existe y sirve para leer el menú, que es la feature
	// anterior. Lo que no se puede es aceptar pedidos, y eso se ve al intentarlo.
	nombre, err := s.store.QC(ctx).GetPlatformByID(ctx, in.PlatformID)
	if err == nil {
		if e := s.store.QC(ctx).SeedPlatformPaymentMethods(ctx, db.SeedPlatformPaymentMethodsParams{
			DeliveryPlatformID: &in.PlatformID,
			NombreEnLinea:      nombre.Name + " en línea",
			NombreEfectivo:     nombre.Name + " efectivo",
		}); e != nil {
			logging.SecurityEvent(ctx, "metodos_de_plataforma_no_sembrados",
				"connection_id", id, "platform_id", in.PlatformID)
		}
	}
	return id, nil
}

// ParejasQueSePierden dice cuántos emparejamientos se lleva dar de baja la tienda. La pantalla lo
// muestra ANTES de confirmar: son decisiones manuales de una sesión completa y no se reconstruyen.
func (s *MenusDePlataformaService) ParejasQueSePierden(ctx context.Context, conexionID int64) (int64, error) {
	n, err := s.store.QC(ctx).CountLinksOfConnection(ctx, conexionID)
	if err != nil {
		return 0, fmt.Errorf("contar parejas de la conexión %d: %w", conexionID, err)
	}
	return n, nil
}

func (s *MenusDePlataformaService) BorrarConexion(ctx context.Context, id int64) error {
	n, err := s.store.QC(ctx).DeletePlatformConnection(ctx, id)
	if err != nil {
		return fmt.Errorf("borrar conexión %d: %w", id, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Lectura ---

// DispararLectura encola una lectura y **devuelve de inmediato**.
//
// La lectura corre en una goroutine con SU PROPIO contexto, no el del request: el del request muere
// al responder, y con él moriría la lectura a media descarga. El menú real pesa 211 KB y leerlo
// dentro del request dejaría una conexión ocupada un segundo largo — SC-006 exige que esto no toque
// el tiempo de respuesta de la captura de un pedido en el mostrador.
func (s *MenusDePlataformaService) DispararLectura(ctx context.Context, companyID, conexionID int64) (int64, time.Time, error) {
	con, err := s.store.QC(ctx).GetPlatformConnection(ctx, conexionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, time.Time{}, domain.ErrNotFound
		}
		return 0, time.Time{}, fmt.Errorf("conexión %d: %w", conexionID, err)
	}
	lector := s.lectores[con.PlatformName]
	if lector == nil {
		return 0, time.Time{}, fmt.Errorf("%w (%s)", domain.ErrPlataformaSinCredenciales, con.PlatformName)
	}
	corriendo, err := s.store.QC(ctx).HasRunningMenuRead(ctx, conexionID)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("lecturas en curso de %d: %w", conexionID, err)
	}
	if corriendo {
		// Dos lecturas de la misma tienda gastan dos tokens para escribir la misma foto, y Uber
		// invalida el más viejo a partir del 101 en una hora.
		return 0, time.Time{}, domain.ErrLecturaEnCurso
	}

	lectura, err := s.store.QC(ctx).StartMenuRead(ctx, conexionID)
	if err != nil {
		// El chequeo de arriba y este insert son dos statements: dos peticiones a la vez pasan las
		// dos por el `if`. Quien lo impide de verdad es el índice único parcial de la 0071.
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return 0, time.Time{}, domain.ErrLecturaEnCurso
		}
		return 0, time.Time{}, fmt.Errorf("abrir la lectura de %d: %w", conexionID, err)
	}

	// El contexto del request muere al responder, y con él moriría la lectura a media descarga; la
	// goroutine arma el suyo con timeout propio. Es lo que SC-006 pide: leer 211 KB de un tercero
	// dentro del request dejaría una conexión ocupada un segundo largo.
	go s.correrLectura(companyID, conexionID, lectura.ID, con.ExternalStoreID, lector) //nolint:gosec // G118: ver arriba
	return lectura.ID, lectura.StartedAt, nil
}

// correrLectura hace el trabajo fuera del request. Tiene condición de término siempre: el contexto
// lleva timeout propio, así que ninguna goroutine se queda colgada esperando a una plataforma que
// no responde (principio II).
func (s *MenusDePlataformaService) correrLectura(companyID, conexionID, lecturaID int64, storeID string, lector LectorDeMenu) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	items, err := lector.LeerMenu(ctx, storeID)
	if err != nil {
		clase := lector.ClaseDeFallo(err)
		// SE REGISTRA LA CLASE, NUNCA EL MENSAJE. El error de una API ajena puede traer la
		// dirección completa, y DiDi transmite su app_secret en el query string (FR-022).
		// PARES SUELTOS, no un mapa: la firma es estilo slog (`args ...any`) y un mapa entero cae
		// bajo `!BADKEY`. El evento se emite igual, pero un grep por `failure_kind=` no empata
		// nunca, y el principio V pide clave estable justamente para eso.
		logging.SecurityEvent(ctx, "platform_menu_read_failed",
			"connection_id", conexionID, "failure_kind", string(clase))
		s.cerrarConFallo(ctx, companyID, lecturaID, clase)
		return
	}

	err = s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		for _, it := range items {
			if err := q.InsertMenuItem(ctx, db.InsertMenuItemParams{
				ReadID: lecturaID, ExternalID: it.ID, Kind: db.PlatformItemKind(it.Clase),
				Name: it.Nombre, PriceCents: it.Centavos, Available: it.Activo,
			}); err != nil {
				return err
			}
		}
		n := int32(len(items))
		return q.FinishMenuReadOK(ctx, db.FinishMenuReadOKParams{
			ID: lecturaID, ItemCount: &n,
		})
	})
	if err != nil {
		s.cerrarConFallo(ctx, companyID, lecturaID, domain.FalloRespuestaInvalida)
	}
}

func (s *MenusDePlataformaService) cerrarConFallo(ctx context.Context, companyID, lecturaID int64, clase domain.ClaseDeFallo) {
	// Contexto propio: si el de la lectura ya venció, cerrarla con el mismo dejaría la fila en
	// `en_curso` para siempre y la pantalla diría «se está leyendo» eternamente.
	cierre, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	texto := string(clase)
	_ = s.store.WithTenant(cierre, companyID, func(q *db.Queries) error {
		return q.FailMenuRead(cierre, db.FailMenuReadParams{
			ID: lecturaID, FailureKind: &texto,
		})
	})
}

func (s *MenusDePlataformaService) ListarLecturas(ctx context.Context, conexionID int64, limite int32) ([]ResumenDeLectura, error) {
	filas, err := s.store.QC(ctx).ListMenuReads(ctx, db.ListMenuReadsParams{ConnectionID: conexionID, Limit: limite})
	if err != nil {
		return nil, fmt.Errorf("listar lecturas de %d: %w", conexionID, err)
	}
	out := make([]ResumenDeLectura, 0, len(filas))
	for _, f := range filas {
		out = append(out, *s.resumen(db.GetLastMenuReadRow(f)))
	}
	return out, nil
}

// TiempoMaximoDeLectura es lo que puede tardar una lectura antes de darse por perdida. Tiene que
// ser mayor que el timeout de `correrLectura`, o cerraría lecturas que todavía van en camino.
const TiempoMaximoDeLectura = 5 * time.Minute

// CerrarLecturasColgadas marca como fallidas las que quedaron `en_curso` porque el proceso se cayó
// a media descarga.
//
// SIN ESTO NO HAY SALIDA: la fila se queda en curso para siempre, cada intento futuro responde «ya
// hay una lectura en curso» y la pantalla dice «se está leyendo» eternamente. Corre al arrancar —
// que es justo cuando un despliegue acaba de matar las que iban corriendo— y con la poda.
func (s *MenusDePlataformaService) CerrarLecturasColgadas(ctx context.Context) (int64, error) {
	n, err := s.store.QC(ctx).FailStaleRunningReads(ctx, s.ahora().Add(-TiempoMaximoDeLectura))
	if err != nil {
		return 0, fmt.Errorf("cerrar lecturas colgadas: %w", err)
	}
	return n, nil
}

// PodarLecturas borra las fotos viejas. **Conserva siempre la más reciente de cada conexión**: sin
// esa excepción, una tienda que nadie vuelve a leer se queda sin ninguna fila y la pantalla la
// muestra igual que una que nunca se leyó — dos estados que FR-003 exige distinguir.
func (s *MenusDePlataformaService) PodarLecturas(ctx context.Context) (int64, error) {
	corte := s.ahora().AddDate(0, 0, -domain.RetencionDeLecturasEnDias)
	n, err := s.store.QC(ctx).PruneMenuReads(ctx, corte)
	if err != nil {
		return 0, fmt.Errorf("podar lecturas de menú: %w", err)
	}
	return n, nil
}

// --- Emparejamiento ---

// ItemParaEmparejar es un platillo u opción de la plataforma con su pareja, si la tiene.
type ItemParaEmparejar struct {
	domain.ItemDePlataforma
	Pareja *ParejaVisible `json:"link"`
}

type ParejaVisible struct {
	LocalID      int64      `json:"localId"`
	LocalNombre  string     `json:"localName"`
	LocalKind    string     `json:"localKind"`
	Confirmada   bool       `json:"confirmed"`
	ConfirmadaEn *time.Time `json:"confirmedAt,omitempty"`
}

type Emparejamiento struct {
	LeidoEn      time.Time           `json:"readAt"`
	Items        []ItemParaEmparejar `json:"items"`
	SinEmparejar []OpcionLocal       `json:"unlinkedLocal"`
}

type OpcionLocal struct {
	ID     int64  `json:"id"`
	Nombre string `json:"name"`
}

// Emparejar arma las dos listas y calcula las PROPUESTAS, marcadas como tales (FR-011).
func (s *MenusDePlataformaService) Emparejar(ctx context.Context, conexionID int64, clase domain.ClaseDeItem) (*Emparejamiento, error) {
	lectura, err := s.ultimaLecturaValida(ctx, conexionID)
	if err != nil {
		return nil, err
	}
	arriba, err := s.itemsDeLectura(ctx, lectura.ID, clase)
	if err != nil {
		return nil, err
	}
	abajo, err := s.catalogoLocal(ctx, conexionID, clase)
	if err != nil {
		return nil, err
	}
	parejas, err := s.parejasDe(ctx, conexionID)
	if err != nil {
		return nil, err
	}

	guardadas := map[string]domain.Pareja{}
	yaEmparejado := map[string]bool{}
	emparejadoLocal := map[int64]bool{}
	for _, p := range parejas {
		guardadas[p.ExternalID] = p
		yaEmparejado[p.ExternalID] = true
		emparejadoLocal[p.LocalID] = true
	}
	for _, prop := range domain.ProponerParejas(arriba, abajo, yaEmparejado) {
		guardadas[prop.ExternalID] = prop
	}

	nombreLocal := map[int64]string{}
	for _, p := range abajo {
		nombreLocal[p.ID] = p.Nombre
	}

	out := &Emparejamiento{LeidoEn: lectura.StartedAt, Items: make([]ItemParaEmparejar, 0, len(arriba))}
	for _, it := range arriba {
		fila := ItemParaEmparejar{ItemDePlataforma: it}
		if p, hay := guardadas[it.ID]; hay {
			fila.Pareja = &ParejaVisible{
				LocalID: p.LocalID, LocalNombre: nombreLocal[p.LocalID],
				LocalKind: string(p.ClaseLocal), Confirmada: p.Confirmada,
				ConfirmadaEn: p.ConfirmadaEn,
			}
		}
		out.Items = append(out.Items, fila)
	}
	out.SinEmparejar = make([]OpcionLocal, 0)
	for _, p := range abajo {
		if !emparejadoLocal[p.ID] {
			out.SinEmparejar = append(out.SinEmparejar, OpcionLocal{ID: p.ID, Nombre: p.Nombre})
		}
	}
	return out, nil
}

type AltaDePareja struct {
	ConexionID int64
	ExternalID string
	Clase      domain.ClaseDeItem
	LocalID    int64
	ClaseLocal domain.ClaseLocal
	UsuarioID  int64
	// Reemplazar una pareja YA CONFIRMADA hay que pedirlo aparte. Sin esto, un `PUT` pisa en
	// silencio una decisión que alguien tomó a mano, y el emparejamiento es una sesión completa de
	// trabajo que no se reconstruye. La pantalla lo pregunta antes de mandarlo.
	Reemplazar bool
}

// GuardarPareja confirma o cambia un emparejamiento.
func (s *MenusDePlataformaService) GuardarPareja(ctx context.Context, in AltaDePareja) error {
	if !domain.ClaseDeItemValida(in.Clase) {
		return fmt.Errorf("%w: clase de item desconocida (%q)", domain.ErrValidation, in.Clase)
	}
	esperada, ok := domain.ClaseLocalDe(in.Clase)
	if !ok {
		return fmt.Errorf("%w: el nivel %q no se empareja en esta versión", domain.ErrValidation, in.Clase)
	}
	// Un platillo se empareja con un producto y una opción con una opción. Cruzarlos produce una
	// fila que pasa los tipos y **nunca empata con nada**, sin que nadie vea un error.
	if in.ClaseLocal != esperada {
		return fmt.Errorf("%w: un %q se empareja con %q, no con %q", domain.ErrValidation, in.Clase, esperada, in.ClaseLocal)
	}

	lectura, err := s.ultimaLecturaValida(ctx, in.ConexionID)
	if err != nil {
		return err
	}
	// ESTA VALIDACIÓN SUSTITUYE A LA FK QUE NO EXISTE. `platform_item_links` no referencia
	// `platform_menu_items` a propósito —para que podar lecturas no borre parejas— y el precio de
	// eso es que nada en la base impide guardar una pareja contra un id que no está arriba. Sin
	// esto, un desajuste de codificación escribe una fila que jamás empata, el PUT responde 200 y
	// la pantalla sigue diciendo «sin pareja».
	existe, err := s.store.QC(ctx).MenuItemExistsInRead(ctx, db.MenuItemExistsInReadParams{
		ReadID: lectura.ID, ExternalID: in.ExternalID, Kind: db.PlatformItemKind(in.Clase),
	})
	if err != nil {
		return fmt.Errorf("verificar un item de la lectura %d: %w", lectura.ID, err)
	}
	if !existe {
		return domain.ErrItemInexistente
	}

	// Una pareja CONFIRMADA no se pisa sin decirlo. El upsert de abajo sobrescribe sin chistar, y
	// el contrato promete un 409 en este caso: sin este chequeo la promesa era solo del documento.
	if !in.Reemplazar {
		previa, err := s.store.QC(ctx).GetItemLink(ctx, db.GetItemLinkParams{
			ConnectionID: in.ConexionID, ExternalID: in.ExternalID,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("pareja previa en la conexión %d: %w", in.ConexionID, err)
		}
		if err == nil && previa.ConfirmedAt.Valid && previa.ProductID != in.LocalID {
			return domain.ErrParejaOcupada
		}
	}

	// Sin usuario se guarda NULL, no cero: `confirmed_by` referencia `users(id)` y un cero viola la
	// FK. El `check` de la tabla ya permite confirmar sin saber quién —el día que un proceso
	// automático empareje, no habrá persona— y lo que no puede es reventar con un error que además
	// dice otra cosa.
	var confirmadaPor *int64
	if in.UsuarioID != 0 {
		confirmadaPor = &in.UsuarioID
	}
	err = s.store.QC(ctx).UpsertItemLink(ctx, db.UpsertItemLinkParams{
		ConnectionID: in.ConexionID, ExternalID: in.ExternalID,
		Kind: db.PlatformItemKind(in.Clase), ProductID: in.LocalID,
		LocalKind: string(in.ClaseLocal), ConfirmedBy: confirmadaPor,
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			// Se nombra la constraint en vez de adivinar: las dos FK de esta tabla dan 23503, y
			// mandar siempre el mensaje del producto hacía que un usuario inexistente se reportara
			// como «ese producto no es de esta empresa» — una pista falsa que cuesta una hora.
			if strings.Contains(pg.ConstraintName, "producto_de_la_empresa") {
				// La FK compuesta con company_id. Los chequeos de integridad saltan RLS, así que
				// esta es la barrera real contra emparejar el catálogo de otra empresa.
				return fmt.Errorf("%w: ese producto no es de esta empresa", domain.ErrValidation)
			}
			return fmt.Errorf("%w: el emparejamiento apunta a algo que ya no existe", domain.ErrValidation)
		}
		return fmt.Errorf("guardar una pareja en la conexión %d: %w", in.ConexionID, err)
	}
	return nil
}

func (s *MenusDePlataformaService) BorrarPareja(ctx context.Context, conexionID int64, externalID string) error {
	n, err := s.store.QC(ctx).DeleteItemLink(ctx, db.DeleteItemLinkParams{ConnectionID: conexionID, ExternalID: externalID})
	if err != nil {
		return fmt.Errorf("deshacer una pareja de la conexión %d: %w", conexionID, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Comparación ---

type Comparacion struct {
	LeidoEn      time.Time           `json:"readAt"`
	Vieja        bool                `json:"stale"`
	Diferencias  []domain.Diferencia `json:"differences"`
	SinEmparejar int                 `json:"unpaired"`
}

// Diferencias compara la última foto válida con el catálogo.
func (s *MenusDePlataformaService) Diferencias(ctx context.Context, conexionID int64, clases map[domain.ClaseDeDiferencia]bool) (*Comparacion, error) {
	lectura, err := s.ultimaLecturaValida(ctx, conexionID)
	if err != nil {
		return nil, err
	}
	arriba, err := s.itemsDeLectura(ctx, lectura.ID, domain.ItemPlatillo)
	if err != nil {
		return nil, err
	}
	// UNA LECTURA VACÍA NO LLEGA A Comparar. La función pura no puede distinguir «el menú está
	// vacío» de «la lectura se rompió», y darle esa decisión sería esconderla donde no se ve.
	if len(arriba) == 0 {
		return nil, domain.ErrLecturaVacia
	}
	abajo, err := s.catalogoLocal(ctx, conexionID, domain.ItemPlatillo)
	if err != nil {
		return nil, err
	}
	parejas, err := s.parejasDe(ctx, conexionID)
	if err != nil {
		return nil, err
	}

	todas := domain.Comparar(arriba, abajo, parejas)
	out := &Comparacion{
		LeidoEn:     lectura.StartedAt,
		Vieja:       lectura.StartedAt.Before(s.ahora().AddDate(0, 0, -domain.RetencionDeLecturasEnDias)),
		Diferencias: make([]domain.Diferencia, 0, len(todas)),
	}
	for _, d := range todas {
		if d.Clase == domain.DifSoloEnPlataforma || d.Clase == domain.DifSoloEnCatalogo {
			out.SinEmparejar++
		}
		if clases == nil || clases[d.Clase] {
			out.Diferencias = append(out.Diferencias, d)
		}
	}
	return out, nil
}

// --- Compartido ---

func (s *MenusDePlataformaService) ultimaLecturaValida(ctx context.Context, conexionID int64) (db.GetLastOKMenuReadRow, error) {
	r, err := s.store.QC(ctx).GetLastOKMenuRead(ctx, conexionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Sin foto válida no se empareja ni se compara. Devolver una lista vacía haría creer
			// que el menú coincide en todo (FR-003).
			return r, domain.ErrSinLecturaValida
		}
		return r, fmt.Errorf("última lectura válida de %d: %w", conexionID, err)
	}
	return r, nil
}

func (s *MenusDePlataformaService) itemsDeLectura(ctx context.Context, lecturaID int64, clase domain.ClaseDeItem) ([]domain.ItemDePlataforma, error) {
	filas, err := s.store.QC(ctx).ListMenuItemsOfRead(ctx, db.ListMenuItemsOfReadParams{
		ReadID: lecturaID, Kind: db.PlatformItemKind(clase),
	})
	if err != nil {
		return nil, fmt.Errorf("items de la lectura %d: %w", lecturaID, err)
	}
	out := make([]domain.ItemDePlataforma, 0, len(filas))
	for _, f := range filas {
		out = append(out, domain.ItemDePlataforma{
			ID: f.ExternalID, Clase: domain.ClaseDeItem(f.Kind), Nombre: f.Name,
			Centavos: f.PriceCents, Activo: f.Available,
		})
	}
	return out, nil
}

// catalogoLocal arma el lado del POS **con el precio de ESA plataforma** (FR-017): la excepción
// capturada si existe, y si no `base × (1 + markup)`.
//
// Reusa domain.PlatformPrice y no recalcula la regla: duplicarla haría que la comparación y el
// ticket se separaran, y entonces uno de los dos miente sin que nadie sepa cuál.
func (s *MenusDePlataformaService) catalogoLocal(ctx context.Context, conexionID int64, clase domain.ClaseDeItem) ([]domain.ProductoLocal, error) {
	con, err := s.store.QC(ctx).GetPlatformConnection(ctx, conexionID)
	if err != nil {
		return nil, fmt.Errorf("conexión %d: %w", conexionID, err)
	}
	plat, err := s.store.QC(ctx).GetPlatformByID(ctx, con.DeliveryPlatformID)
	if err != nil {
		return nil, fmt.Errorf("plataforma %d: %w", con.DeliveryPlatformID, err)
	}

	if clase == domain.ItemOpcion {
		filas, err := s.store.QC(ctx).ListActiveOptionsForCompare(ctx)
		if err != nil {
			return nil, fmt.Errorf("opciones del catálogo: %w", err)
		}
		deltas, err := s.store.QC(ctx).GetOptionPlatformPrices(ctx, plat.ID)
		if err != nil {
			return nil, fmt.Errorf("deltas por plataforma: %w", err)
		}
		manual := map[int64]decimal.Decimal{}
		for _, d := range deltas {
			manual[d.OptionID] = d.PriceDelta
		}
		out := make([]domain.ProductoLocal, 0, len(filas))
		for _, f := range filas {
			var m *decimal.Decimal
			if v, hay := manual[f.ID]; hay {
				m = &v
			}
			out = append(out, domain.ProductoLocal{
				ID: f.ID, Nombre: f.Name, Activo: true,
				PrecioDePlataforma: domain.PlatformPrice(f.PriceDelta, plat.PriceMarkupPct, m),
			})
		}
		return out, nil
	}

	filas, err := s.store.QC(ctx).ListActiveProductsForCompare(ctx)
	if err != nil {
		return nil, fmt.Errorf("productos del catálogo: %w", err)
	}
	precios, err := s.store.QC(ctx).GetProductPlatformPrices(ctx, plat.ID)
	if err != nil {
		return nil, fmt.Errorf("precios por plataforma: %w", err)
	}
	manual := map[int64]decimal.Decimal{}
	for _, p := range precios {
		manual[p.ProductID] = p.Price
	}
	out := make([]domain.ProductoLocal, 0, len(filas))
	for _, f := range filas {
		var m *decimal.Decimal
		if v, hay := manual[f.ID]; hay {
			m = &v
		}
		out = append(out, domain.ProductoLocal{
			ID: f.ID, Nombre: f.Name, Activo: true,
			PrecioDePlataforma: domain.PlatformPrice(f.Price, plat.PriceMarkupPct, m),
		})
	}
	return out, nil
}

func (s *MenusDePlataformaService) parejasDe(ctx context.Context, conexionID int64) ([]domain.Pareja, error) {
	filas, err := s.store.QC(ctx).ListItemLinks(ctx, conexionID)
	if err != nil {
		return nil, fmt.Errorf("parejas de la conexión %d: %w", conexionID, err)
	}
	out := make([]domain.Pareja, 0, len(filas))
	for _, f := range filas {
		p := domain.Pareja{
			ExternalID: f.ExternalID, Clase: domain.ClaseDeItem(f.Kind),
			LocalID: f.ProductID, ClaseLocal: domain.ClaseLocal(f.LocalKind),
			Confirmada: f.ConfirmedAt.Valid,
		}
		if f.ConfirmedAt.Valid {
			c := f.ConfirmedAt.Time
			p.ConfirmadaEn = &c
		}
		out = append(out, p)
	}
	return out, nil
}
