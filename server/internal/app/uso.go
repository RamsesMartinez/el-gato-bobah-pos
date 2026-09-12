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
}

func NewUsageService(st *store.Store) *UsageService {
	return &UsageService{store: st}
}

// retenciónEnDías: cuánto vive cada cosa.
//
// El grano fino dura poco porque su trabajo es permitir recontar si mañana se cuenta distinto, y
// darle dónde caer a las coordenadas del futuro. El agregado dura trece meses —y no doce— para
// poder comparar un mes contra el mismo mes del año pasado, que es la primera comparación que pide
// un negocio de comida.
const (
	RetencionDeEventosEnDias   = 14
	RetencionDelAgregadoEnDias = 396 // 13 meses
)

// Registrar guarda un lote de eventos y devuelve cuántos se descartaron por no estar en la lista
// blanca.
//
// Ese número es el único testigo de que una versión del front dejó de medir: si nadie lo mira, el
// mapa simplemente muestra menos, que es indistinguible de «se usó menos».
//
// El rol lo pone quien llama desde el TOKEN, nunca el cuerpo del request.
func (s *UsageService) Registrar(ctx context.Context, rol domain.Role, lote []domain.EventoDeUso) (int, error) {
	lote = domain.RecortarLoteDeUso(lote)
	agregado := domain.PreAgregarUso(lote)

	validos := 0
	for _, a := range agregado {
		validos += a.Veces
	}
	descartados := len(lote) - validos
	if len(agregado) == 0 {
		return descartados, nil
	}

	// LA SUPRESIÓN SE DECIDE AQUÍ, AL ESCRIBIR, y por eso no se puede deshacer leyendo.
	//
	// Quien leyera no podría decidirlo: la consola no tiene permiso sobre `users` para contar la
	// plantilla de un cliente, y dárselo abriría la puerta que la spec 016 cerró.
	rolAGuardar, err := s.rolQueSePuedeGuardar(ctx, rol)
	if err != nil {
		return descartados, err
	}

	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		for _, a := range agregado {
			var accion *string
			if a.Accion != "" {
				accion = &a.Accion
			}
			// El grano fino guarda UNO POR TOQUE, no uno por combinación: el día que lleve
			// coordenadas, un punto por dedo es justo lo que hace que sirva.
			for i := 0; i < a.Veces; i++ {
				if err := q.InsertUsageEvent(ctx, db.InsertUsageEventParams{
					Screen: a.Pantalla, Action: accion, Role: rolAGuardar,
				}); err != nil {
					return fmt.Errorf("guardar evento de uso: %w", err)
				}
			}
			if err := q.UpsertUsageDaily(ctx, db.UpsertUsageDailyParams{
				Screen: a.Pantalla, Action: accion, Role: rolAGuardar, Hits: int64(a.Veces),
			}); err != nil {
				return fmt.Errorf("sumar el uso del día: %w", err)
			}
		}
		return nil
	})
	return descartados, err
}

// rolQueSePuedeGuardar devuelve el rol, o nil si guardarlo identificaría a una persona.
func (s *UsageService) rolQueSePuedeGuardar(ctx context.Context, rol domain.Role) (*db.UserRole, error) {
	if !domain.RolMedible(rol) {
		return nil, nil
	}
	activos, err := s.store.QC(ctx).CountActiveUsersByRole(ctx, string(rol))
	if err != nil {
		return nil, fmt.Errorf("contar usuarios del rol: %w", err)
	}
	if !domain.CorteDeRolPermitido(int(activos)) {
		return nil, nil
	}
	r := db.UserRole(rol)
	return &r, nil
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
		sort.Slice(p.PorRol, func(i, j int) bool { return p.PorRol[i].Veces > p.PorRol[j].Veces })
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
	eventos, err := s.store.Q.DeleteOldUsageEvents(ctx, RetencionDeEventosEnDias)
	if err != nil {
		return fmt.Errorf("recortar eventos de uso: %w", err)
	}
	agregado, err := s.store.Q.DeleteOldUsageDaily(ctx, RetencionDelAgregadoEnDias)
	if err != nil {
		return fmt.Errorf("recortar el agregado de uso: %w", err)
	}
	if eventos > 0 || agregado > 0 {
		slog.Info("uso recortado", "eventos", eventos, "agregado", agregado)
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
func (s *UsageService) RecortarPeriodicamente(ctx context.Context, cada time.Duration) {
	recortar := func() {
		if err := s.Recortar(ctx); err != nil && ctx.Err() == nil {
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
