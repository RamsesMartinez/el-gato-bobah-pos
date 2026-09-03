package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Reglas de la pantalla de Ventas: qué rango se está mirando, qué filtros son válidos y cómo se
// clasifica cada peso del resumen. Todo puro y sin I/O, para que se pruebe sin base de datos.

const dateOnly = "2006-01-02"

const (
	// MaxSalesRangeDays acota el rango que se puede pedir de una vez. Sin cota, un "del 2020 a hoy"
	// escanea sin límite en el gigabyte de RAM del VPS y tumba la API para todos.
	MaxSalesRangeDays = 366
	// MaxSalesPageSize: tope de filas por página. Serializar cien mil ventas no es una consulta
	// lenta, es un proceso muerto.
	MaxSalesPageSize = 100
	// DiasDelPreset30: cuántos días mide "30d", contando hoy.
	DiasDelPreset30 = 30
)

// Range es el intervalo de FECHAS DE NEGOCIO que la pantalla está mirando, inclusivo en los dos
// extremos. Fechas y no instantes: el día de negocio ya es la unidad con la que el sistema agrupa
// ventas, cortes y gastos.
type Range struct {
	From time.Time
	To   time.Time
}

// ResolveRange traduce un preset ("hoy", "ayer", "semana", "mes", "30d", "rango") a fechas concretas.
//
// Se resuelve en la zona del NEGOCIO y no en UTC. No es un detalle: a las 19:00 de México ya son
// las 01:00 del día siguiente en UTC, así que "hoy" calculado en UTC devuelve el día equivocado
// justo en la hora de más venta. Es el mismo defecto que reinició los folios a media cena.
//
// Un preset desconocido se RECHAZA en vez de caer a "hoy": una pantalla que se ve correcta y
// reporta un rango que nadie pidió es peor que un error, porque nadie la audita.
func ResolveRange(preset string, from, to time.Time, now time.Time, loc *time.Location) (Range, error) {
	if loc == nil {
		loc = time.UTC
	}
	hoy := BusinessDate(now, loc)

	// Una fecha que se manda y no se usa es una pantalla que miente. `from`/`to` solo significan
	// algo con `preset=rango`; descartarlas en silencio con cualquier otro preset hace que
	// `?preset=hoy&from=2026-01-01` conteste HOY con la pantalla viéndose perfecta — el mismo modo
	// de falla que el principio V nombra para un parámetro malformado, y peor, porque nadie lo audita.
	if preset != "rango" && (!from.IsZero() || !to.IsZero()) {
		return Range{}, fmt.Errorf("%w: las fechas solo aplican con un rango libre, no con %q", ErrValidation, preset)
	}

	switch preset {
	case "hoy", "":
		return Range{From: hoy, To: hoy}, nil
	case "ayer":
		ayer := hoy.AddDate(0, 0, -1)
		return Range{From: ayer, To: ayer}, nil
	case "semana":
		// Desde el LUNES: es como cuenta la semana quien opera el negocio. Con domingo, el "esta
		// semana" del lunes por la mañana saldría vacío.
		diasDesdeLunes := (int(hoy.Weekday()) + 6) % 7
		return Range{From: hoy.AddDate(0, 0, -diasDesdeLunes), To: hoy}, nil
	case "mes":
		return Range{From: time.Date(hoy.Year(), hoy.Month(), 1, 0, 0, 0, 0, time.UTC), To: hoy}, nil
	case "30d":
		// La ventana con la que nace la pantalla de Reportes, y son TREINTA días CONTANDO HOY. El
		// handler la armaba restando 30 al día de fin, que son treinta y uno: el encabezado decía
		// "últimos 30 días" y la tabla sumaba uno más. Nadie ve un día de diferencia, y por eso
		// muerde al comparar dos periodos "de 30 días".
		return Range{From: hoy.AddDate(0, 0, -(DiasDelPreset30 - 1)), To: hoy}, nil
	case "rango":
		return rangoLibre(from, to, hoy)
	default:
		return Range{}, fmt.Errorf("%w: rango desconocido (%q)", ErrValidation, preset)
	}
}

func rangoLibre(from, to, hoy time.Time) (Range, error) {
	if from.IsZero() || to.IsZero() {
		return Range{}, fmt.Errorf("%w: un rango libre necesita fecha de inicio y de fin", ErrValidation)
	}
	f := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	t := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if t.Before(f) {
		// Invertido devolvería CERO filas sin error, y el operador creería que no vendió.
		return Range{}, fmt.Errorf("%w: la fecha de inicio es posterior a la de fin", ErrValidation)
	}
	if dias := int(t.Sub(f).Hours()/24) + 1; dias > MaxSalesRangeDays {
		return Range{}, fmt.Errorf("%w: el rango es de %d días y el máximo es %d", ErrValidation, dias, MaxSalesRangeDays)
	}
	// Un día que no ha pasado no tiene ventas, así que un rango que lo incluye devuelve una pantalla
	// vacía —o corta— que se lee como "no vendimos nada". La pantalla lo topa con el calendario, pero
	// el calendario no impide TECLEAR la fecha: la única barrera real es esta.
	if t.After(hoy) {
		return Range{}, fmt.Errorf("%w: el %s no ha pasado todavía", ErrValidation, t.Format(dateOnly))
	}
	return Range{From: f, To: t}, nil
}

// Columnas por las que la tabla se puede ordenar. Es una whitelist a propósito: un `sort`
// desconocido se rechaza en vez de ignorarse, porque ignorarlo deja la tabla ordenada por algo
// distinto de lo que dice su encabezado.
var salesSorts = map[string]bool{
	"fecha": true, "folio": true, "total": true, "estado": true, "tipo": true,
}

// ValidSalesSort dice si una columna de orden es conocida.
func ValidSalesSort(s string) bool { return salesSorts[s] }

var salesStatuses = map[string]bool{
	StatusAbierta: true, StatusLista: true, StatusEntregada: true,
	StatusCancelada: true, StatusReembolsada: true,
}

var salesServiceTypes = map[string]bool{
	"mostrador": true, "para_llevar": true, "domicilio": true,
}

// SalesFilter es lo que la pantalla pide. Vive en domain y no en app porque son REGLAS —qué
// combinación es aceptable— y las reglas se prueban sin base de datos.
// Un valor por filtro y no una lista: en la pantalla que esto viene a reemplazar cada filtro es un
// selector simple, así que aceptar listas sería construir para un caso que nadie pidió.
type SalesFilter struct {
	Range       Range
	Status      string
	ServiceType string
	Sort        string
	Dir         string
	Limit       int32
	Offset      int32
}

// Validate rechaza lo que no se puede atender. Cada regla existe por un fallo concreto: un estado
// inventado devolvería cero filas en silencio, un `sort` desconocido ordenaría por otra cosa, y una
// página de cien mil filas es un proceso muerto.
func (f SalesFilter) Validate() error {
	if f.Status != "" && !salesStatuses[f.Status] {
		return fmt.Errorf("%w: estado de venta desconocido (%q)", ErrValidation, f.Status)
	}
	if f.ServiceType != "" && !salesServiceTypes[f.ServiceType] {
		return fmt.Errorf("%w: tipo de venta desconocido (%q)", ErrValidation, f.ServiceType)
	}
	if !ValidSalesSort(f.Sort) {
		return fmt.Errorf("%w: no se puede ordenar por %q", ErrValidation, f.Sort)
	}
	if f.Dir != "asc" && f.Dir != "desc" {
		return fmt.Errorf("%w: dirección de orden desconocida (%q)", ErrValidation, f.Dir)
	}
	if f.Limit < 1 || f.Limit > MaxSalesPageSize {
		return fmt.Errorf("%w: el tamaño de página va de 1 a %d", ErrValidation, MaxSalesPageSize)
	}
	if f.Offset < 0 {
		return fmt.Errorf("%w: la página no puede ser negativa", ErrValidation)
	}
	return nil
}

// StatusTotals es lo que la base devuelve por estado: cuántas ventas y por cuánto.
//
// El resumen se arma sobre esas pocas filas —una por estado— y no sobre las ventas una por una: la
// suma pesada la hace Postgres, que es quien puede hacerla con un índice, y en Go se queda la REGLA
// de qué cuenta como ingreso, que es lo que hay que poder probar sin base de datos.
type StatusTotals struct {
	Status      string
	Count       int
	Total       decimal.Decimal
	Tips        decimal.Decimal
	DeliveryFee decimal.Decimal
}

// ConceptCount: cuántas ventas de un concepto y por cuánto.
type ConceptCount struct {
	Count  int             `json:"count"`
	Amount decimal.Decimal `json:"amount"`
}

// SalesSummary es el resumen de arriba de la pantalla. Cada campo declara qué incluye:
//
//   - Total: ingreso REAL. No incluye canceladas ni reembolsadas, que son ingreso que no ocurrió.
//   - Tips: pass-through del personal. NO está dentro de Total.
//   - DeliveryFees: ya está DENTRO de Total; viaja aparte solo como referencia.
//
// La separación no es estética. Un resumen que pone propina, envío y total como renglones hermanos
// invita a sumarlos y a reportar un ingreso que el negocio no tuvo — la misma forma del fondo de
// caja que se contó una vez por método y dejó un turno con $4,500 de faltante sin explicación.
type SalesSummary struct {
	Count        int             `json:"count"`
	Total        decimal.Decimal `json:"total"`
	Average      decimal.Decimal `json:"average"`
	Tips         decimal.Decimal `json:"tips"`
	DeliveryFees decimal.Decimal `json:"deliveryFees"`
	Cancelled    ConceptCount    `json:"cancelled"`
	Refunded     ConceptCount    `json:"refunded"`
}

// SummarizeSales clasifica cada venta en un solo concepto y saca el promedio.
func SummarizeSales(filas []StatusTotals) SalesSummary {
	var s SalesSummary
	for _, f := range filas {
		switch f.Status {
		case StatusCancelada:
			s.Cancelled.Count += f.Count
			s.Cancelled.Amount = s.Cancelled.Amount.Add(f.Total)
		case StatusReembolsada:
			s.Refunded.Count += f.Count
			s.Refunded.Amount = s.Refunded.Amount.Add(f.Total)
		default:
			s.Count += f.Count
			s.Total = s.Total.Add(f.Total)
			s.Tips = s.Tips.Add(f.Tips)
			s.DeliveryFees = s.DeliveryFees.Add(f.DeliveryFee)
		}
	}
	if s.Count > 0 {
		s.Average = Round2(s.Total.Div(decimal.NewFromInt(int64(s.Count))))
	}
	s.Total = Round2(s.Total)
	s.Tips = Round2(s.Tips)
	s.DeliveryFees = Round2(s.DeliveryFees)
	s.Cancelled.Amount = Round2(s.Cancelled.Amount)
	s.Refunded.Amount = Round2(s.Refunded.Amount)
	return s
}
