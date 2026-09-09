package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// SalesService atiende la pantalla de Ventas: el análisis de lo que ya pasó.
//
// Vive aparte de BackofficeService, que ya cubre caja, gastos, almacén y reportes en casi mil
// renglones. Es un tipo concreto y no una interfaz: no hay un segundo consumidor, así que una
// interfaz aquí sería la abstracción especulativa que la constitución prohíbe.
type SalesService struct {
	store *store.Store
	now   func() time.Time
}

func NewSalesService(s *store.Store, now func() time.Time) *SalesService {
	if now == nil {
		now = time.Now
	}
	return &SalesService{store: s, now: now}
}

// SaleRow es un renglón de la tabla. Los montos van como decimal para que el JSON lleve el string
// exacto y el cliente no los pase por float.
type SaleRow struct {
	ID          int64 `json:"id"`
	DailyNumber int32 `json:"dailyNumber"`
	// FolioName es el nombre con el que se cantó el pedido. Es como el cliente pide su ticket
	// para facturar —recuerda "Tigre", no "#187"—, así que viaja en la lista y no solo en el detalle.
	FolioName   string          `json:"folioName"`
	Date        string          `json:"date"`
	OpenedAt    time.Time       `json:"openedAt"`
	CompletedAt *time.Time      `json:"completedAt"`
	Status      string          `json:"status"`
	ServiceType string          `json:"serviceType"`
	Customer    string          `json:"customer"`
	Total       decimal.Decimal `json:"total"`
	DeliveryFee decimal.Decimal `json:"deliveryFee"`
	Refund      decimal.Decimal `json:"refund"`
	Tips        decimal.Decimal `json:"tips"`
	Platform    string          `json:"platform"`
	// PlatformOrderRef viaja en la lista para que el dueño no tenga que abrir cada pedido con el
	// documento de pago en la mano. Vacío = sin capturar, que es lo que el filtro de pendientes lista.
	PlatformOrderRef string `json:"platformOrderRef"`
	OpenedBy         string `json:"openedBy"`
	Methods          string `json:"methods"`
}

// SalesPage: la tabla y su total, para el paginador.
type SalesPage struct {
	Range SalesRange `json:"range"`
	Items []SaleRow  `json:"items"`
	Total int64      `json:"total"`
}

// SalesRange viaja en las DOS respuestas y se pinta en la pantalla. Sirve para dos cosas de un
// renglón: el operador ve qué rango está mirando, y si la lista y el resumen cayeran en lados
// distintos de la medianoche la divergencia se ve, en vez de quedar como un descuadre sin causa.
type SalesRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MethodTotals: cuánto se COBRÓ por cada medio de pago. No es lo mismo que lo vendido — una venta
// mandada a cocina sin cobrar suma al total y no aparece aquí.
type MethodTotals struct {
	MethodID int16           `json:"methodId"`
	Method   string          `json:"method"`
	Payments int32           `json:"payments"`
	Total    decimal.Decimal `json:"total"`
	Tips     decimal.Decimal `json:"tips"`
}

// SalesSummaryView es el resumen de arriba. Agrega al de dominio el desglose por método y las
// líneas canceladas, que salen de otras dos consultas.
type SalesSummaryView struct {
	Range SalesRange `json:"range"`
	domain.SalesSummary
	ByMethod       []MethodTotals      `json:"byMethod"`
	CancelledLines domain.ConceptCount `json:"cancelledLines"`
}

// List devuelve la página de ventas del filtro.
//
// Tres caminos, y cada uno existe por una razón distinta:
//
//   - BUSCANDO un folio: una consulta propia con la igualdad literal, que es la única forma de que
//     el planner use `orders_platform_ref_busqueda`. Devuelve una fila o ninguna, así que paginar
//     no significa nada y el total sale del largo del resultado.
//   - PENDIENTES de folio: la gemela `…SinFolio`, con el predicado literal. Con el patrón
//     `narg is null or (…)` el planner no puede probar el predicado del índice parcial y cae a un
//     bitmap scan sobre la fecha — medido contra 150k pedidos.
//   - Lo demás: la consulta de siempre.
func (s *SalesService) List(ctx context.Context, f domain.SalesFilter) (*SalesPage, error) {
	desde, hasta := fecha(f.Range.From), fecha(f.Range.To)

	if f.Buscando() {
		return s.buscarPorFolio(ctx, f, desde, hasta)
	}

	var rows []db.ListSalesRow
	var total int64
	var err error
	if f.SoloSinFolio() {
		var pend []db.ListSalesSinFolioRow
		pend, err = s.store.QC(ctx).ListSalesSinFolio(ctx, db.ListSalesSinFolioParams{
			Desde: desde, Hasta: hasta, Status: estadoNull(f.Status), ServiceType: tipoNull(f.ServiceType),
			Sort: f.Sort, Dir: f.Dir, Lim: f.Limit, Off: f.Offset,
		})
		// La conversión de struct es legal porque las dos consultas seleccionan exactamente las
		// mismas columnas en el mismo orden: es lo que las hace un PAR, y si alguien toca el select
		// de una sin la otra, esto deja de compilar. Es la red que impide que el par se desincronice
		// en silencio.
		for _, r := range pend {
			rows = append(rows, db.ListSalesRow(r))
		}
		if err == nil {
			total, err = s.store.QC(ctx).CountSalesSinFolio(ctx, db.CountSalesSinFolioParams{
				Desde: desde, Hasta: hasta, Status: estadoNull(f.Status), ServiceType: tipoNull(f.ServiceType),
			})
		}
	} else {
		rows, err = s.store.QC(ctx).ListSales(ctx, db.ListSalesParams{
			Desde:       desde,
			Hasta:       hasta,
			Status:      estadoNull(f.Status),
			ServiceType: tipoNull(f.ServiceType),
			Sort:        f.Sort,
			Dir:         f.Dir,
			Lim:         f.Limit,
			Off:         f.Offset,
		})
		if err == nil {
			total, err = s.store.QC(ctx).CountSales(ctx, db.CountSalesParams{
				Desde: desde, Hasta: hasta, Status: estadoNull(f.Status), ServiceType: tipoNull(f.ServiceType),
			})
		}
	}
	if err != nil {
		return nil, err
	}

	out := make([]SaleRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, filaDeVenta(r))
	}
	return &SalesPage{Range: rango(f.Range), Items: out, Total: total}, nil
}

// filaDeVenta traduce la fila de la base al renglón de la pantalla. Vive en una sola función porque
// los tres caminos de List —normal, pendientes y búsqueda— tienen que pintar EXACTAMENTE lo mismo:
// dos traducciones distintas serían dos formas de que la misma venta se vea diferente según cómo se
// llegó a ella.
func filaDeVenta(r db.ListSalesRow) SaleRow {
	return SaleRow{
		ID: r.ID, DailyNumber: r.DailyNumber, FolioName: derefStr(r.FolioName),
		Date:     r.BusinessDate.Time.Format("2006-01-02"),
		OpenedAt: r.OpenedAt, CompletedAt: momento(r.CompletedAt),
		Status: string(r.Status), ServiceType: string(r.ServiceType),
		Customer: texto(r.CustomerName), Total: domain.Round2(r.Total),
		DeliveryFee: domain.Round2(r.DeliveryFee), Refund: domain.Round2(r.RefundAmount),
		Tips: domain.Round2(r.Tips), Platform: texto(r.Platform), OpenedBy: texto(r.OpenedByName),
		Methods: string(r.Methods), PlatformOrderRef: texto(r.PlatformOrderRef),
	}
}

// Summary arma el resumen. Son tres consultas y no una porque `order_payments` y `order_lines` son
// las dos 1:N con `orders`: unirlas en la misma consulta multiplica las filas y duplica las sumas.
func (s *SalesService) Summary(ctx context.Context, f domain.SalesFilter) (*SalesSummaryView, error) {
	desde, hasta := fecha(f.Range.From), fecha(f.Range.To)
	tipo := tipoNull(f.ServiceType)

	// BUSCANDO un folio: el resumen se deriva de LAS MISMAS FILAS que devuelve la lista, no de otra
	// consulta. Es la forma más fuerte de cumplir "la lista y el resumen describen el mismo
	// conjunto": no pueden divergir porque salen del mismo lugar.
	if f.Buscando() {
		return s.resumenDeLaBusqueda(ctx, f, desde, hasta)
	}

	porEstado, err := s.totalesPorEstado(ctx, f, desde, hasta, tipo)
	if err != nil {
		return nil, err
	}
	totales := porEstado

	porMetodo, err := s.totalesPorMetodo(ctx, f, desde, hasta, tipo)
	if err != nil {
		return nil, err
	}
	metodos := porMetodo

	lineas, monto, err := s.lineasCanceladas(ctx, f, desde, hasta, tipo)
	if err != nil {
		return nil, err
	}

	return &SalesSummaryView{
		Range:          rango(f.Range),
		SalesSummary:   domain.SummarizeSales(totales),
		ByMethod:       metodos,
		CancelledLines: domain.ConceptCount{Count: lineas, Amount: monto},
	}, nil
}

// Las tres consultas del resumen, cada una con su gemela de pendientes. Elegir la variante en un
// solo lugar por consulta es lo que impide que una de las tres se quede con el filtro viejo — que
// es exactamente la divergencia que este archivo lleva advirtiendo desde su primera línea.
func (s *SalesService) totalesPorEstado(ctx context.Context, f domain.SalesFilter, desde, hasta pgtype.Date, tipo *db.ServiceType) ([]domain.StatusTotals, error) {
	out := []domain.StatusTotals{}
	if f.SoloSinFolio() {
		rows, err := s.store.QC(ctx).SalesTotalsByStatusSinFolio(ctx, db.SalesTotalsByStatusSinFolioParams{
			Desde: desde, Hasta: hasta, ServiceType: tipo,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, domain.StatusTotals{Status: string(r.Status), Count: int(r.Ventas),
				Total: r.Total, Tips: r.Propinas, DeliveryFee: r.Envios})
		}
		return out, nil
	}
	rows, err := s.store.QC(ctx).SalesTotalsByStatus(ctx, db.SalesTotalsByStatusParams{
		Desde: desde, Hasta: hasta, ServiceType: tipo,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, domain.StatusTotals{Status: string(r.Status), Count: int(r.Ventas),
			Total: r.Total, Tips: r.Propinas, DeliveryFee: r.Envios})
	}
	return out, nil
}

func (s *SalesService) totalesPorMetodo(ctx context.Context, f domain.SalesFilter, desde, hasta pgtype.Date, tipo *db.ServiceType) ([]MethodTotals, error) {
	out := []MethodTotals{}
	if f.SoloSinFolio() {
		rows, err := s.store.QC(ctx).SalesTotalsByMethodSinFolio(ctx, db.SalesTotalsByMethodSinFolioParams{
			Desde: desde, Hasta: hasta, ServiceType: tipo,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, MethodTotals{MethodID: r.MethodID, Method: r.Method, Payments: r.Pagos,
				Total: domain.Round2(r.Total), Tips: domain.Round2(r.Propinas)})
		}
		return out, nil
	}
	rows, err := s.store.QC(ctx).SalesTotalsByMethod(ctx, db.SalesTotalsByMethodParams{
		Desde: desde, Hasta: hasta, ServiceType: tipo,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, MethodTotals{MethodID: r.MethodID, Method: r.Method, Payments: r.Pagos,
			Total: domain.Round2(r.Total), Tips: domain.Round2(r.Propinas)})
	}
	return out, nil
}

func (s *SalesService) lineasCanceladas(ctx context.Context, f domain.SalesFilter, desde, hasta pgtype.Date, tipo *db.ServiceType) (int, decimal.Decimal, error) {
	if f.SoloSinFolio() {
		r, err := s.store.QC(ctx).SalesCancelledLinesSinFolio(ctx, db.SalesCancelledLinesSinFolioParams{
			Desde: desde, Hasta: hasta, ServiceType: tipo,
		})
		if err != nil {
			return 0, decimal.Zero, err
		}
		return int(r.Lineas), domain.Round2(r.Monto), nil
	}
	r, err := s.store.QC(ctx).SalesCancelledLines(ctx, db.SalesCancelledLinesParams{
		Desde: desde, Hasta: hasta, ServiceType: tipo,
	})
	if err != nil {
		return 0, decimal.Zero, err
	}
	return int(r.Lineas), domain.Round2(r.Monto), nil
}

// resumenDeLaBusqueda deriva el resumen de las MISMAS filas que la lista devuelve.
//
// `ByMethod` va vacío a propósito y no es una omisión: buscando un folio hay a lo más UN pedido, y
// su medio de pago ya está en la columna del renglón. Repetirlo como tile sería la misma cifra en
// dos lugares, que es lo que este archivo evita en todo lo demás.
func (s *SalesService) resumenDeLaBusqueda(ctx context.Context, f domain.SalesFilter, desde, hasta pgtype.Date) (*SalesSummaryView, error) {
	pagina, err := s.buscarPorFolio(ctx, f, desde, hasta)
	if err != nil {
		return nil, err
	}
	totales := make([]domain.StatusTotals, 0, len(pagina.Items))
	for _, r := range pagina.Items {
		totales = append(totales, domain.StatusTotals{
			Status: r.Status, Count: 1, Total: r.Total, Tips: r.Tips, DeliveryFee: r.DeliveryFee,
		})
	}
	return &SalesSummaryView{
		Range:        rango(f.Range),
		SalesSummary: domain.SummarizeSales(totales),
		ByMethod:     []MethodTotals{},
	}, nil
}

// buscarPorFolio atiende el caso de pegar el identificador del documento de pago.
//
// Casi siempre devuelve UNA fila, pero puede devolver más: la unicidad del folio es por empresa Y
// PLATAFORMA, así que dos plataformas pueden usar el mismo identificador y las dos entran. Con los
// formatos reales —UUID de 36 en Uber, entero de 19 en DiDi, de 10 en Rappi— la colisión es
// prácticamente imposible, pero el esquema la permite a propósito: rechazarla tiraría una captura
// legítima. Por eso la consulta es `:many` y esto itera en vez de asumir una sola fila; convertirla
// a `:one` restauraría una garantía que nunca existió.
//
// Un folio que nadie capturó devuelve la lista VACÍA y no un error: "este renglón del documento
// todavía no está registrado" es una respuesta legítima, y es exactamente lo que el dueño necesita.
func (s *SalesService) buscarPorFolio(ctx context.Context, f domain.SalesFilter, desde, hasta pgtype.Date) (*SalesPage, error) {
	rows, err := s.store.QC(ctx).FindSaleByPlatformRef(ctx, db.FindSaleByPlatformRefParams{
		// El folio RECORTADO: la columna lo guarda así, y la comparación es una igualdad exacta.
		Folio: ptrDe(f.FolioBuscado()), Desde: desde, Hasta: hasta,
	})
	if err != nil {
		return nil, err
	}
	out := make([]SaleRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, filaDeVenta(db.ListSalesRow(r)))
	}
	return &SalesPage{Range: rango(f.Range), Items: out, Total: int64(len(out))}, nil
}

// Timezone del negocio, para que el preset se resuelva en el día del local y no en UTC. Si no se
// puede leer cae a UTC en vez de fallar: la pantalla de análisis no se cae por un ajuste mal
// escrito, y el peor caso es el rango corrido que ya se tenía antes de que esto existiera.
func (s *SalesService) Location(ctx context.Context) *time.Location {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		// El default del producto, no UTC. Es la misma llamada, el mismo modo de falla y el mismo
		// corrimiento de seis horas que se corrigió en la fecha de negocio del arqueo: aquí decide
		// qué día se le muestra al gerente como "hoy", así que un preset resuelto en UTC le enseña
		// un rango que nadie pidió después de las 18:00 locales.
		tz = domain.DefaultTimezone
	}
	return domain.LoadBusinessLocation(tz)
}

// Now expone el reloj del servicio para que el handler resuelva el preset con el mismo instante que
// usaría el resto del sistema (los tests lo fijan).
func (s *SalesService) Now() time.Time { return s.now() }

// --- conversiones a los tipos de pgx/sqlc ---

func fecha(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func rango(r domain.Range) SalesRange {
	return SalesRange{From: r.From.Format("2006-01-02"), To: r.To.Format("2006-01-02")}
}

// Vacío = "sin filtrar". sqlc traduce un `sqlc.narg` de enum a un puntero, así que nil es el que
// hace verdadero el `is null` de la consulta.
func estadoNull(s string) *db.OrderStatus {
	if s == "" {
		return nil
	}
	v := db.OrderStatus(s)
	return &v
}

func tipoNull(s string) *db.ServiceType {
	if s == "" {
		return nil
	}
	v := db.ServiceType(s)
	return &v
}

func texto(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// momento traduce un timestamptz que puede venir nulo. Una venta abierta no tiene hora de cierre, y
// mandar el cero de Go la pintaría como cerrada el año 1.
func momento(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// ptrDe es el puntero a un valor local, para los parámetros nullable de sqlc.
func ptrDe(v string) *string { return &v }
