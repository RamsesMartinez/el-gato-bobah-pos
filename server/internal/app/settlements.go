package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// SettlementsService captura lo que el documento de pago de una plataforma dice de un pedido, y
// resume el periodo.
//
// Vive aparte de OrdersService porque es OTRO momento y OTRO camino: el pedido se levanta en la hora
// pico y la liquidación se captura días después, con el estado de cuenta en la mano y con rol de
// administración. Es un tipo concreto y no una interfaz: no hay un segundo consumidor.
type SettlementsService struct {
	store *store.Store
}

func NewSettlementsService(s *store.Store) *SettlementsService {
	return &SettlementsService{store: s}
}

// SettlementView es la liquidación como la ve la pantalla.
type SettlementView struct {
	OrderID          int64            `json:"orderId"`
	ReportedGross    decimal.Decimal  `json:"reportedGross"`
	CommissionAmount decimal.Decimal  `json:"commissionAmount"`
	CommissionPct    *decimal.Decimal `json:"commissionPct"`
	DiscountTotal    decimal.Decimal  `json:"discountTotal"`
	DiscountPlatform decimal.Decimal  `json:"discountPlatform"`
	// DiscountRestaurant es DERIVADO (total − plataforma) y no una columna: guardarlo además serían
	// dos verdades sobre el mismo hecho, y un documento corregido que mueva una y no la otra dejaría
	// la fila contradiciéndose.
	DiscountRestaurant decimal.Decimal `json:"discountRestaurant"`
	Withholdings       decimal.Decimal `json:"withholdings"`
	NetAmount          decimal.Decimal `json:"netAmount"`
	PayoutReference    string          `json:"payoutReference"`
	DocumentRef        string          `json:"documentRef"`
	CapturedAt         time.Time       `json:"capturedAt"`
	CapturedBy         string          `json:"capturedBy"`
}

// Upsert registra la liquidación de un pedido, o la reemplaza desde un documento corregido.
func (s *SettlementsService) Upsert(ctx context.Context, orderID int64, in domain.Settlement, quien int64) (*SettlementView, error) {
	// Redondear ANTES de validar: todas las fronteras de dinero del repo lo hacen en ese orden, para
	// que un sub-centavo que redondea fuera de la cota se rechace igual que el valor explícito.
	in = in.Rounded()
	if err := in.Validate(); err != nil {
		return nil, err
	}

	// El pedido tiene que existir EN ESTA EMPRESA y ser de plataforma. Se comprueba aquí y no solo
	// con la FK: la FK compuesta rechaza el cruce de empresas con un 23503 opaco, y "ese pedido no
	// es de plataforma" no lo diría nadie.
	ord, err := s.store.QC(ctx).GetOrder(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if ord.DeliveryPlatformID == nil {
		return nil, fmt.Errorf("%w: ese pedido no es de plataforma, así que no tiene liquidación",
			domain.ErrValidation)
	}

	row, err := s.store.QC(ctx).UpsertSettlement(ctx, db.UpsertSettlementParams{
		OrderID:          orderID,
		ReportedGross:    in.ReportedGross,
		CommissionAmount: in.CommissionAmount,
		CommissionPct:    in.CommissionPct,
		DiscountTotal:    in.DiscountTotal,
		DiscountPlatform: in.DiscountPlatform,
		Withholdings:     in.Withholdings,
		NetAmount:        in.NetAmount,
		PayoutReference:  textoONil(in.PayoutReference),
		DocumentRef:      textoONil(in.DocumentRef),
		CapturedBy:       quien,
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, row.OrderID)
}

// Get devuelve la liquidación de un pedido, o ErrNotFound si todavía no se ha capturado.
//
// El ErrNotFound es LA MITAD de la feature, no un detalle: "todavía no llega el documento" y "el
// documento dice cero" no son lo mismo, y devolver ceros en los dos casos borraría la distinción
// que esta feature viene a crear.
func (s *SettlementsService) Get(ctx context.Context, orderID int64) (*SettlementView, error) {
	r, err := s.store.QC(ctx).GetSettlement(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	liq := domain.Settlement{
		ReportedGross: r.ReportedGross, CommissionAmount: r.CommissionAmount,
		CommissionPct: r.CommissionPct, DiscountTotal: r.DiscountTotal,
		DiscountPlatform: r.DiscountPlatform, Withholdings: r.Withholdings, NetAmount: r.NetAmount,
	}
	return &SettlementView{
		OrderID: r.OrderID, ReportedGross: r.ReportedGross, CommissionAmount: r.CommissionAmount,
		CommissionPct: r.CommissionPct, DiscountTotal: r.DiscountTotal,
		DiscountPlatform: r.DiscountPlatform, DiscountRestaurant: liq.DiscountRestaurant(),
		Withholdings: r.Withholdings, NetAmount: r.NetAmount,
		PayoutReference: derefStr(r.PayoutReference), DocumentRef: derefStr(r.DocumentRef),
		CapturedAt: r.CapturedAt, CapturedBy: derefStr(r.CapturedByName),
	}, nil
}

// PlatformMoneyView son las tres cifras del periodo, con el rango que de verdad se consultó.
type PlatformMoneyView struct {
	Range SalesRange `json:"range"`
	domain.PlatformMoneySummary
}

// Summary responde cuánto se vendió por plataformas, cuánto se quedó la plataforma y cuánto llegó al
// banco. Tres cifras que hoy no existen, y que NO se derivan una de otra.
func (s *SettlementsService) Summary(ctx context.Context, f domain.SalesFilter) (*PlatformMoneyView, error) {
	r, err := s.store.QC(ctx).PlatformSettlementSummary(ctx, db.PlatformSettlementSummaryParams{
		Desde: fecha(f.Range.From), Hasta: fecha(f.Range.To),
	})
	if err != nil {
		return nil, err
	}
	return &PlatformMoneyView{
		Range: rango(f.Range),
		PlatformMoneySummary: domain.SummarizePlatformMoney(domain.PlatformTotals{
			OrdersVendido:    int(r.Pedidos),
			Vendido:          r.Vendido,
			OrdersLiquidados: int(r.Liquidados),
			Comision:         r.Comision,
			Retenciones:      r.Retenciones,
			Neto:             r.Neto,
			SinLiquidar:      int(r.SinLiquidar),
			SinFolio:         int(r.SinFolio),
		}),
	}, nil
}

// textoONil traduce el vacío de la frontera a NULL. El check del esquema rechaza la cadena vacía a
// propósito: una referencia de depósito "" no es una referencia, es la ausencia de una.
func textoONil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
