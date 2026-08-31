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
	OpenedBy    string          `json:"openedBy"`
	Methods     string          `json:"methods"`
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
func (s *SalesService) List(ctx context.Context, f domain.SalesFilter) (*SalesPage, error) {
	desde, hasta := fecha(f.Range.From), fecha(f.Range.To)
	rows, err := s.store.QC(ctx).ListSales(ctx, db.ListSalesParams{
		Desde:       desde,
		Hasta:       hasta,
		Status:      estadoNull(f.Status),
		ServiceType: tipoNull(f.ServiceType),
		Sort:        f.Sort,
		Dir:         f.Dir,
		Lim:         f.Limit,
		Off:         f.Offset,
	})
	if err != nil {
		return nil, err
	}
	total, err := s.store.QC(ctx).CountSales(ctx, db.CountSalesParams{
		Desde: desde, Hasta: hasta, Status: estadoNull(f.Status), ServiceType: tipoNull(f.ServiceType),
	})
	if err != nil {
		return nil, err
	}

	out := make([]SaleRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SaleRow{
			ID: r.ID, DailyNumber: r.DailyNumber, FolioName: derefStr(r.FolioName),
			Date:     r.BusinessDate.Time.Format("2006-01-02"),
			OpenedAt: r.OpenedAt, CompletedAt: momento(r.CompletedAt),
			Status: string(r.Status), ServiceType: string(r.ServiceType),
			Customer: texto(r.CustomerName), Total: domain.Round2(r.Total),
			DeliveryFee: domain.Round2(r.DeliveryFee), Refund: domain.Round2(r.RefundAmount),
			Tips: domain.Round2(r.Tips), Platform: texto(r.Platform), OpenedBy: texto(r.OpenedByName),
			Methods: string(r.Methods),
		})
	}
	return &SalesPage{Range: rango(f.Range), Items: out, Total: total}, nil
}

// Summary arma el resumen. Son tres consultas y no una porque `order_payments` y `order_lines` son
// las dos 1:N con `orders`: unirlas en la misma consulta multiplica las filas y duplica las sumas.
func (s *SalesService) Summary(ctx context.Context, f domain.SalesFilter) (*SalesSummaryView, error) {
	desde, hasta := fecha(f.Range.From), fecha(f.Range.To)
	tipo := tipoNull(f.ServiceType)

	porEstado, err := s.store.QC(ctx).SalesTotalsByStatus(ctx, db.SalesTotalsByStatusParams{
		Desde: desde, Hasta: hasta, ServiceType: tipo,
	})
	if err != nil {
		return nil, err
	}
	totales := make([]domain.StatusTotals, 0, len(porEstado))
	for _, r := range porEstado {
		totales = append(totales, domain.StatusTotals{
			Status: string(r.Status), Count: int(r.Ventas),
			Total: r.Total, Tips: r.Propinas, DeliveryFee: r.Envios,
		})
	}

	porMetodo, err := s.store.QC(ctx).SalesTotalsByMethod(ctx, db.SalesTotalsByMethodParams{
		Desde: desde, Hasta: hasta, ServiceType: tipo,
	})
	if err != nil {
		return nil, err
	}
	metodos := make([]MethodTotals, 0, len(porMetodo))
	for _, r := range porMetodo {
		metodos = append(metodos, MethodTotals{
			MethodID: r.MethodID, Method: r.Method, Payments: r.Pagos,
			Total: domain.Round2(r.Total), Tips: domain.Round2(r.Propinas),
		})
	}

	canceladas, err := s.store.QC(ctx).SalesCancelledLines(ctx, db.SalesCancelledLinesParams{
		Desde: desde, Hasta: hasta, ServiceType: tipo,
	})
	if err != nil {
		return nil, err
	}

	return &SalesSummaryView{
		Range:          rango(f.Range),
		SalesSummary:   domain.SummarizeSales(totales),
		ByMethod:       metodos,
		CancelledLines: domain.ConceptCount{Count: int(canceladas.Lineas), Amount: domain.Round2(canceladas.Monto)},
	}, nil
}

// Timezone del negocio, para que el preset se resuelva en el día del local y no en UTC. Si no se
// puede leer cae a UTC en vez de fallar: la pantalla de análisis no se cae por un ajuste mal
// escrito, y el peor caso es el rango corrido que ya se tenía antes de que esto existiera.
func (s *SalesService) Location(ctx context.Context) *time.Location {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		return time.UTC
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
