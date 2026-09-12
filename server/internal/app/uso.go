package app

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// UsageService mide qué se usa del sistema (spec 017).
//
// Dos caminos que no se cruzan: `Registrar` escribe con la conexión del NEGOCIO (bajo RLS, la
// empresa sale del token) y `Mapa` lee con la de la CONSOLA, que no tiene permiso sobre el grano
// fino. `Recortar` es el único que necesita al dueño, y abre su propia conexión para eso.
type UsageService struct {
	store *store.Store
	now   func() time.Time
}

func NewUsageService(st *store.Store) *UsageService {
	return &UsageService{store: st, now: time.Now}
}

// RetencionDelAgregadoEnDias: cuánto se conserva el conteo.
//
// Trece meses —y no doce— para poder comparar un mes contra el mismo mes del año pasado, que es la
// primera comparación que pide un negocio de comida.
const RetencionDelAgregadoEnDias = 396

// RetencionDeToquesEnDias: cuánto se conserva la rejilla, y es MÁS CORTO a propósito.
//
// El conteo por pantalla se mira año contra año —«¿se usa más el corte de caja que la temporada
// pasada?»—; la rejilla solo sirve para decidir un rediseño, y una rejilla de hace un año describe
// un layout que ya no existe. Conservarla más tiempo es conservar una referencia que miente, y es
// además la mitad de datos que se guarda de lo más granular que esta feature produce.
const RetencionDeToquesEnDias = 92

// LoteDeMedicion es lo que llega en un request de medición: aperturas, acciones y toques.
//
// Los tres viajan juntos porque salen de la MISMA cola del cliente, en el mismo vaciado.
type LoteDeMedicion struct {
	Eventos []domain.EventoDeUso
	Toques  []domain.Toque
}

// Descartes dice cuántos se tiraron de cada clase por no pasar la lista blanca.
//
// Ese número es el único testigo de que una versión del front dejó de medir: si nadie lo mira, el
// mapa simplemente muestra menos, que es indistinguible de «se usó menos».
type Descartes struct {
	Eventos int
	Toques  int
}

// Registrar guarda un lote de medición y devuelve cuántos se descartaron.
//
// UNA SOLA TRANSACCIÓN PARA LAS DOS TABLAS, y una sola resolución de lo que comparten: el día del
// negocio y la decisión de corte por rol. Partirlo en dos escrituras costaba dos transacciones, dos
// lecturas de la zona horaria y dos conteos de plantilla por petición —y cada petición reteniendo
// una conexión del pool el doble de tiempo, en una VM de 1 vCPU donde compite con el cobro—. Al
// tope que el limitador permite eso se multiplica por 30 por minuto y por usuario.
//
// La consecuencia de juntarlas, dicha en voz alta: si la transacción falla se pierden las dos
// mitades. Es aceptable porque las dos son mediciones y perderlas no le cuesta nada a nadie; lo que
// NO era aceptable era la versión anterior, donde un fallo al escribir las aperturas se llevaba los
// toques sin siquiera intentarlos, y el comentario decía lo contrario.
//
// El rol lo pone quien llama desde el TOKEN, nunca el cuerpo del request.
func (s *UsageService) Registrar(ctx context.Context, rol domain.Role, lote LoteDeMedicion) (Descartes, error) {
	eventos := domain.RecortarLoteDeUso(lote.Eventos)
	toques := domain.RecortarLoteDeToques(lote.Toques)
	usos := domain.PreAgregarUso(eventos, rol)
	zonas := domain.PreAgregarToques(toques, rol)

	var descartes Descartes
	validos := 0
	for _, a := range usos {
		validos += a.Veces
	}
	descartes.Eventos = len(eventos) - validos
	validos = 0
	for _, a := range zonas {
		validos += a.Veces
	}
	descartes.Toques = len(toques) - validos

	if len(usos) == 0 && len(zonas) == 0 {
		return descartes, nil
	}

	// LA SUPRESIÓN SE DECIDE AQUÍ, AL ESCRIBIR, y por eso no se puede deshacer leyendo.
	//
	// Quien leyera no podría decidirlo: la consola no tiene permiso sobre `users` para contar la
	// plantilla de un cliente, y dárselo abriría la puerta que la spec 016 cerró.
	corte, rolAGuardar, err := s.corteDeLaEmpresa(ctx, rol)
	if err != nil {
		return descartes, err
	}
	if corte == domain.NoGuardar {
		// Ni siquiera sin rol: el balde de lo suprimido no alcanza a tapar a nadie. Se pierde la
		// medición, que es lo que esta feature tiene permitido hacer.
		return descartes, nil
	}

	// EL DÍA DEL NEGOCIO, no el de UTC: con el servidor en UTC la medianoche cae a las 18:00 en
	// México, así que todo lo de la tarde-noche —donde más se mueve un lugar de comida— se contaría
	// mañana. Es el mismo defecto que 0038 arregló para la venta, y aquí habría dado un mapa que
	// miente de noche y acierta de día.
	dia := pgtype.Date{Time: domain.BusinessDate(s.now(), s.location(ctx)), Valid: true}

	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for _, a := range usos {
			var accion *string
			if a.Accion != "" {
				accion = &a.Accion
			}
			if err := q.UpsertUsageDaily(ctx, db.UpsertUsageDailyParams{
				Day: dia, Screen: a.Pantalla, Action: accion, Role: rolAGuardar, Hits: int64(a.Veces),
			}); err != nil {
				return fmt.Errorf("sumar el uso del día: %w", err)
			}
		}
		for _, a := range zonas {
			if err := q.UpsertTouchesDaily(ctx, db.UpsertTouchesDailyParams{
				Day:         dia,
				Screen:      a.Pantalla,
				Orientation: a.Orientacion,
				// La celda cabe en un `smallint` por construcción: `ToqueValido` ya la acotó a la
				// rejilla antes de llegar aquí.
				Cell: int16(a.Celda),
				Role: rolAGuardar,
				Hits: int64(a.Veces),
			}); err != nil {
				return fmt.Errorf("sumar los toques del día: %w", err)
			}
		}
		return nil
	})
	return descartes, err
}

// location resuelve la zona del negocio del request. Cae al default del producto si no se puede
// leer: una medición no puede fallar por eso, y el `BusinessDate` de una zona equivocada sigue
// siendo mejor que el de UTC.
func (s *UsageService) location(ctx context.Context) *time.Location {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.LoadBusinessLocation(tz)
}

// corteDeLaEmpresa decide qué se puede escribir sin señalar a una persona, mirando la plantilla
// ENTERA y no solo la del rol que mide. El porqué está en `domain.CortarPorRol`.
func (s *UsageService) corteDeLaEmpresa(ctx context.Context, rol domain.Role) (domain.DecisionDeCorte, *db.UserRole, error) {
	filas, err := s.store.QC(ctx).CountActiveUsersByRoleAll(ctx)
	if err != nil {
		return domain.NoGuardar, nil, fmt.Errorf("contar la plantilla: %w", err)
	}
	activos := make(map[domain.Role]int, len(filas))
	for _, f := range filas {
		activos[domain.Role(f.Role)] = int(f.Activos)
	}
	corte := domain.CortarPorRol(activos, rol)
	if corte != domain.GuardarConRol {
		return corte, nil, nil
	}
	r := db.UserRole(rol)
	return corte, &r, nil
}

// --- Lo que lee la consola (US1). ---

// AccionDeUso es una acción con nombre y cuántas veces ocurrió en el periodo.
type AccionDeUso struct {
	Accion string `json:"accion"`
	Veces  int64  `json:"veces"`
}

// UsoPorRol reparte el uso de una pantalla entre los roles. `Rol` nulo significa **sin corte**: ese
// uso existe pero no se puede atribuir sin identificar a una persona (FR-009).
type UsoPorRol struct {
	Rol   *string `json:"rol"`
	Veces int64   `json:"veces"`
}

// PantallaDeUso es un renglón del mapa.
//
// Las tres cifras dicen cosas distintas y el contrato las declara: `Aperturas` cuenta solo las
// vistas, `Acciones[].Veces` solo las acciones, y `PorRol[].Veces` es TODO lo de la pantalla
// repartido por rol — de modo que sum(PorRol) == Aperturas + sum(Acciones). Sin esa declaración,
// quien lea suma dos de las tres y reporta un número que no existe.
type PantallaDeUso struct {
	Pantalla  string        `json:"pantalla"`
	Aperturas int64         `json:"aperturas"`
	Acciones  []AccionDeUso `json:"acciones"`
	PorRol    []UsoPorRol   `json:"porRol"`
}

// MapaDeUso es lo que la consola pinta.
type MapaDeUso struct {
	Periodo   RangoDeUso      `json:"periodo"`
	Pantallas []PantallaDeUso `json:"pantallas"`
}

// RangoDeUso dice qué periodo se está mirando. Viaja en la respuesta porque una pantalla que no
// dice qué rango muestra invita a leer el número equivocado.
type RangoDeUso struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
}

// Mapa devuelve el uso del periodo, ordenado de la pantalla más usada a la menos.
//
// Las pantallas sin uso VIAJAN con cero y no se omiten: «qué no usa nadie» es la mitad de la
// pregunta que esta feature vino a responder.
func (s *UsageService) Mapa(ctx context.Context, desde, hasta time.Time, empresa *int64) (MapaDeUso, error) {
	if err := domain.RangoDeUsoValido(desde, hasta, RetencionDelAgregadoEnDias); err != nil {
		return MapaDeUso{}, err
	}
	filas, err := s.store.Q.SumUsageForMap(ctx, db.SumUsageForMapParams{
		Desde:   pgtype.Date{Time: desde, Valid: true},
		Hasta:   pgtype.Date{Time: hasta, Valid: true},
		Company: empresa,
	})
	if err != nil {
		return MapaDeUso{}, fmt.Errorf("leer el uso: %w", err)
	}

	porPantalla := map[string]*PantallaDeUso{}
	porRol := map[string]map[string]int64{} // pantalla -> rol ("" = sin corte) -> veces
	for _, f := range filas {
		p, ok := porPantalla[f.Screen]
		if !ok {
			p = &PantallaDeUso{Pantalla: f.Screen, Acciones: []AccionDeUso{}, PorRol: []UsoPorRol{}}
			porPantalla[f.Screen] = p
			porRol[f.Screen] = map[string]int64{}
		}
		if f.Action == nil {
			p.Aperturas += f.Veces
		} else {
			p.Acciones = append(p.Acciones, AccionDeUso{Accion: *f.Action, Veces: f.Veces})
		}
		rol := ""
		if f.Role != nil {
			rol = string(*f.Role)
		}
		porRol[f.Screen][rol] += f.Veces
	}

	// Las pantallas que nadie abrió: nacen en cero para que el mapa pueda decir «esta no la usa
	// nadie», que es justo lo que no se puede ver mirando la aplicación.
	for _, nombre := range domain.PantallasMedibles() {
		if _, ok := porPantalla[nombre]; !ok {
			porPantalla[nombre] = &PantallaDeUso{Pantalla: nombre, Acciones: []AccionDeUso{}, PorRol: []UsoPorRol{}}
		}
	}

	pantallas := make([]PantallaDeUso, 0, len(porPantalla))
	for nombre, p := range porPantalla {
		for rol, veces := range porRol[nombre] {
			r := &rol
			if rol == "" {
				r = nil
			}
			p.PorRol = append(p.PorRol, UsoPorRol{Rol: r, Veces: veces})
		}
		// Con desempate por nombre: `porRol` se llena recorriendo un map de Go, cuyo orden es
		// aleatorio a propósito, así que dos empates pintarían distinto en dos cargas seguidas. Una
		// tabla que se reordena sola es una tabla que nadie puede comparar de un día para otro.
		sort.Slice(p.PorRol, func(i, j int) bool {
			if p.PorRol[i].Veces != p.PorRol[j].Veces {
				return p.PorRol[i].Veces > p.PorRol[j].Veces
			}
			return nombreDeRol(p.PorRol[i].Rol) < nombreDeRol(p.PorRol[j].Rol)
		})
		sort.Slice(p.Acciones, func(i, j int) bool { return p.Acciones[i].Veces > p.Acciones[j].Veces })
		pantallas = append(pantallas, *p)
	}
	// De más a menos usada, contando todo lo que pasó en ella. El empate se rompe por nombre para
	// que dos cargas seguidas pinten el mismo orden: una tabla que se reordena sola es una tabla
	// que nadie puede comparar de un día para otro.
	sort.Slice(pantallas, func(i, j int) bool {
		ti, tj := totalDe(pantallas[i]), totalDe(pantallas[j])
		if ti != tj {
			return ti > tj
		}
		return pantallas[i].Pantalla < pantallas[j].Pantalla
	})

	return MapaDeUso{
		Periodo:   RangoDeUso{Desde: desde.Format(time.DateOnly), Hasta: hasta.Format(time.DateOnly)},
		Pantallas: pantallas,
	}, nil
}

func totalDe(p PantallaDeUso) int64 {
	total := p.Aperturas
	for _, a := range p.Acciones {
		total += a.Veces
	}
	return total
}

// --- La rejilla que lee la consola (US1 de la 019). ---

// CeldaDeToques es una zona de la pantalla y cuántas veces se tocó.
type CeldaDeToques struct {
	Celda int   `json:"celda"`
	Veces int64 `json:"veces"`
}

// FormaDeLaRejilla dice cómo se acomodan las celdas. Viaja en la respuesta porque el mismo número
// de celda es un lugar distinto en cada forma, y quien pinta necesita las dos cosas.
type FormaDeLaRejilla struct {
	Columnas int `json:"columnas"`
	Filas    int `json:"filas"`
}

// RejillaDeToques es lo que la consola pinta.
type RejillaDeToques struct {
	Pantalla    string           `json:"pantalla"`
	Orientacion string           `json:"orientacion"`
	Rejilla     FormaDeLaRejilla `json:"rejilla"`
	Periodo     RangoDeUso       `json:"periodo"`
	Celdas      []CeldaDeToques  `json:"celdas"`
	// El reparto por rol es del TOTAL de la pantalla, no de una celda: por celda, los números son
	// tan chicos que el corte por rol volvería a identificar por eliminación.
	PorRol []UsoPorRol `json:"porRol"`
}

// Rejilla devuelve los toques del periodo en una pantalla y una orientación.
//
// LAS DOS ORIENTACIONES NUNCA SE SUMAN (FR-016): se pide una y se devuelve esa. La celda 37 está a
// la derecha del centro en horizontal y abajo del centro en vertical — mezclarlas pinta un mapa que
// nadie tocó nunca, y el error sería invisible porque la rejilla se ve normal.
func (s *UsageService) Rejilla(ctx context.Context, pantalla, orientacion string, desde, hasta time.Time, empresa *int64) (RejillaDeToques, error) {
	if !domain.PantallaConToque(pantalla) {
		// Se rechaza en vez de devolver una rejilla en ceros: una rejilla vacía se lee como «aquí
		// nadie toca», que es justo la conclusión equivocada.
		return RejillaDeToques{}, fmt.Errorf("%w: esa pantalla no mide toques", domain.ErrValidation)
	}
	orientacion, err := domain.OrientacionDeToque(orientacion)
	if err != nil {
		return RejillaDeToques{}, err
	}
	if err := domain.RangoDeUsoValido(desde, hasta, RetencionDeToquesEnDias); err != nil {
		return RejillaDeToques{}, err
	}

	filas, err := s.store.Q.SumTouchesForGrid(ctx, db.SumTouchesForGridParams{
		Desde:       pgtype.Date{Time: desde, Valid: true},
		Hasta:       pgtype.Date{Time: hasta, Valid: true},
		Screen:      pantalla,
		Orientation: orientacion,
		Company:     empresa,
	})
	if err != nil {
		return RejillaDeToques{}, fmt.Errorf("leer los toques: %w", err)
	}

	// LAS 84 CELDAS NACEN EN CERO y se llenan con lo que haya. «Qué parte no toca nadie» es la
	// mitad de la pregunta que esta feature vino a responder, y una celda omitida por no tener
	// filas se pinta como un hueco en vez de como un cero.
	celdas := make([]CeldaDeToques, domain.CeldasDeLaRejilla)
	for i := range celdas {
		celdas[i].Celda = i
	}
	porRol := map[string]int64{} // "" = sin corte
	for _, f := range filas {
		if int(f.Cell) < 0 || int(f.Cell) >= len(celdas) {
			// No puede pasar —la columna tiene su `check`— pero si pasara, un índice fuera de
			// rango tumbaría la consola entera por una fila mal escrita.
			continue
		}
		celdas[f.Cell].Veces += f.Veces
		rol := ""
		if f.Role != nil {
			rol = string(*f.Role)
		}
		porRol[rol] += f.Veces
	}

	reparto := make([]UsoPorRol, 0, len(porRol))
	for rol, veces := range porRol {
		r := &rol
		if rol == "" {
			r = nil
		}
		reparto = append(reparto, UsoPorRol{Rol: r, Veces: veces})
	}
	// Con desempate por nombre: `porRol` se recorre como map, cuyo orden Go aleatoriza a propósito,
	// y una tabla que se reordena sola es una tabla que nadie puede comparar de un día para otro.
	sort.Slice(reparto, func(i, j int) bool {
		if reparto[i].Veces != reparto[j].Veces {
			return reparto[i].Veces > reparto[j].Veces
		}
		return nombreDeRol(reparto[i].Rol) < nombreDeRol(reparto[j].Rol)
	})

	columnas, filasDeLaRejilla := domain.RejillaDe(orientacion)
	return RejillaDeToques{
		Pantalla:    pantalla,
		Orientacion: orientacion,
		Rejilla:     FormaDeLaRejilla{Columnas: columnas, Filas: filasDeLaRejilla},
		Periodo:     RangoDeUso{Desde: desde.Format(time.DateOnly), Hasta: hasta.Format(time.DateOnly)},
		Celdas:      celdas,
		PorRol:      reparto,
	}, nil
}

// --- El recorte (US4). ---

// Recortar borra lo que ya pasó su retención.
//
// Corre con la conexión que traiga el store: en producción se le pasa una de DUEÑO, porque el rol
// de la aplicación está bajo RLS y solo borraría las filas de SU empresa — el recorte se quedaría a
// medias en una instalación multi-empresa y sin que nada fallara.
//
// Borra un día a la vez en la práctica (se llama a diario), así que no necesita lotes: lo que
// elimina son las filas de un solo día, no un año de golpe.
func (s *UsageService) Recortar(ctx context.Context) error {
	filas, err := s.store.Q.DeleteOldUsageDaily(ctx, RetencionDelAgregadoEnDias)
	if err != nil {
		return fmt.Errorf("recortar el uso: %w", err)
	}
	if filas > 0 {
		slog.Info("uso recortado", "filas", filas)
	}

	// Los toques van con SU retención, más corta. No es un detalle de afinación: una rejilla de
	// hace un año describe un layout que ya no existe, y conservarla es conservar una referencia
	// que miente. Si las dos compartieran constante, la mitad de lo guardado estaría describiendo
	// una pantalla rediseñada.
	toques, err := s.store.Q.DeleteOldTouchesDaily(ctx, RetencionDeToquesEnDias)
	if err != nil {
		return fmt.Errorf("recortar los toques: %w", err)
	}
	if toques > 0 {
		slog.Info("toques recortados", "filas", toques)
	}
	return nil
}

// RecortarPeriodicamente corre el recorte AL ARRANCAR y luego cada `cada`, hasta que el contexto se
// cancele.
//
// El «al arrancar» es lo que hace que exista. Un `time.Ticker` de 24 horas se reinicia con el
// proceso, y este repo redespliega en CADA merge: un binario que no vive un día entero seguido no
// dispararía nunca el recorte, en silencio, y la promesa de que el volumen no crece quedaría siendo
// una promesa que nadie cumple. Lo encontró la revisión de arquitectura; el ticker solo, que era lo
// obvio, era justo lo que no funcionaba aquí.
//
// Un fallo se registra y NO detiene el ciclo: que la base esté ocupada un día no puede dejar el
// recorte apagado para siempre.
//
// La conexión de DUEÑO se abre y se cierra EN CADA PASADA, no se sostiene entre una y otra: es un
// asa que salta RLS viviendo en el mismo proceso que los handlers, y mantenerla abierta un día
// entero para correr un `delete` es superficie regalada. `abrir` la provee quien llama.
func RecortarPeriodicamente(ctx context.Context, cada time.Duration, abrir func(context.Context) (*store.Store, error)) {
	recortar := func() {
		st, err := abrir(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("sin conexión para recortar el uso", "error", err)
			}
			return
		}
		defer st.Close()
		if err := NewUsageService(st).Recortar(ctx); err != nil && ctx.Err() == nil {
			slog.Error("no se pudo recortar el uso", "error", err)
		}
	}
	recortar()

	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// La condición de término que el principio II exige: sin esto es una goroutine que
			// sobrevive al apagado y mantiene viva una conexión de dueño.
			return
		case <-t.C:
			recortar()
		}
	}
}

// nombreDeRol ordena el "sin corte" al final, que es donde se lee mejor: primero quiénes fueron y
// al último lo que no se puede atribuir.
func nombreDeRol(r *string) string {
	if r == nil {
		return "~sin corte"
	}
	return *r
}
