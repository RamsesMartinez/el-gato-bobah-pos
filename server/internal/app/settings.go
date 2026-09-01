package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// SettingsService lee y escribe los ajustes de negocio (fila única). Concreto a propósito
// (§8): un solo consumidor, sin interfaz especulativa.
type SettingsService struct {
	store *store.Store
}

func NewSettingsService(s *store.Store) *SettingsService {
	return &SettingsService{store: s}
}

// BusinessSettings son los ajustes del negocio y la identidad que va en el encabezado del ticket.
// Los campos opcionales viajan como string vacío y no como null: el ticket omite el renglón cuando
// está vacío, y así el front no tiene que distinguir null de "".
//
// HasLogo/LogoUpdatedAt describen el logo sin traerlo: el binario se pide por su propio endpoint.
// Meterlo aquí serían 256 KB en cada lectura del costo de envío, que ocurre en cada cobro.
type BusinessSettings struct {
	DeliveryFee      decimal.Decimal `json:"deliveryFee"`
	BusinessName     string          `json:"businessName"`
	Address          string          `json:"address"`
	Phone            string          `json:"phone"`
	FooterNote       string          `json:"footerNote"`
	HeaderNote       string          `json:"headerNote"`
	AutoPrintOnClose bool            `json:"autoPrintOnClose"`
	// Timezone: nombre IANA. Decide de qué DÍA es una venta, un corte o un gasto — la base guarda
	// instantes en UTC, pero la fecha es una decisión de calendario y depende de dónde está el local.
	Timezone string `json:"timezone"`
	// PrintFreeModifiers: si el ticket lista los adicionales que no cuestan. Encendido por default
	// — cocina los usa para preparar y el cliente para reclamar; apagarlo solo acorta el papel.
	PrintFreeModifiers bool `json:"printFreeModifiers"`
	// PrintKitchenTicket: si al mandar el pedido sale una comanda SIN precios para cocina. Apagado
	// por default: en un local donde la cocina está pegada al mostrador sería papel que duplica lo
	// que el cocinero ya ve. Lo enciende el negocio que tiene la cocina en otro cuarto.
	PrintKitchenTicket bool `json:"printKitchenTicket"`
	// KitchenCanCharge: si el tablero de Pedidos puede cobrar. Apagado = /pedidos solo prepara.
	KitchenCanCharge bool       `json:"kitchenCanCharge"`
	HasLogo          bool       `json:"hasLogo"`
	LogoUpdatedAt    *time.Time `json:"logoUpdatedAt"`
}

func (s *SettingsService) Get(ctx context.Context) (BusinessSettings, error) {
	row, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	if err != nil {
		// Empresa sin fila de ajustes aún (p. ej. tenant recién provisionado): default sin costo
		// de envío, en vez de un 500 en el camino del cobro.
		if errors.Is(err, pgx.ErrNoRows) {
			// Sin zona, las fechas se calcularían en UTC y la cena caería en el día siguiente: el
			// default acompaña al de la columna en vez de dejar el campo vacío.
			return BusinessSettings{DeliveryFee: decimal.Zero, Timezone: domain.DefaultTimezone, PrintFreeModifiers: true}, nil
		}
		return BusinessSettings{}, err
	}
	bs := BusinessSettings{
		DeliveryFee:        row.DeliveryFee,
		BusinessName:       row.BusinessName,
		Address:            derefStr(row.Address),
		Phone:              derefStr(row.Phone),
		FooterNote:         derefStr(row.FooterNote),
		HeaderNote:         derefStr(row.HeaderNote),
		AutoPrintOnClose:   row.AutoPrintOnClose,
		Timezone:           row.Timezone,
		PrintFreeModifiers: row.PrintFreeModifiers,
		PrintKitchenTicket: row.PrintKitchenTicket,
		KitchenCanCharge:   row.KitchenCanCharge,
		HasLogo:            row.HasLogo,
	}
	if row.LogoUpdatedAt.Valid {
		bs.LogoUpdatedAt = &row.LogoUpdatedAt.Time
	}
	return bs, nil
}

// SetDeliveryFee valida y guarda el costo de envío por defecto. allowZero: envío gratis es
// un ajuste válido. El monto se acota en la frontera (numeric(10,2)) → un valor absurdo cae 400.
func (s *SettingsService) SetDeliveryFee(ctx context.Context, fee decimal.Decimal, userID int64) (BusinessSettings, error) {
	fee = domain.Round2(fee)
	if !domain.ValidMoney(fee, true) {
		return BusinessSettings{}, domain.ErrValidation
	}
	row, err := s.store.QC(ctx).UpdateDeliveryFee(ctx, db.UpdateDeliveryFeeParams{DeliveryFee: fee, UpdatedBy: &userID})
	if err != nil {
		return BusinessSettings{}, err
	}
	return BusinessSettings{DeliveryFee: row.DeliveryFee}, nil
}

// TicketLogo es el binario del logo del encabezado. Se lee aparte de BusinessSettings porque son
// hasta 256 KB que no deben viajar en cada lectura del costo de envío.
type TicketLogo struct {
	Bytes     []byte
	Mime      string
	UpdatedAt time.Time
}

// Logo devuelve el logo subido. ok=false significa que el negocio no ha subido ninguno: es el
// estado normal de un negocio recién dado de alta, no un error — el front cae a su logo default.
func (s *SettingsService) Logo(ctx context.Context) (TicketLogo, bool, error) {
	row, err := s.store.QC(ctx).GetTicketLogo(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TicketLogo{}, false, nil
		}
		return TicketLogo{}, false, err
	}
	// El check business_settings_logo_pair garantiza que van juntos; se comprueban los dos de
	// todos modos porque de aquí sale el Content-Type que se le promete al navegador.
	if row.LogoBytes == nil || row.LogoMime == nil {
		return TicketLogo{}, false, nil
	}
	logo := TicketLogo{Bytes: row.LogoBytes, Mime: *row.LogoMime}
	if row.LogoUpdatedAt.Valid {
		logo.UpdatedAt = row.LogoUpdatedAt.Time
	}
	return logo, true, nil
}

// SetBusinessInfo guarda la identidad que sale en el ticket y el interruptor de impresión
// automática. Valida en domain ANTES de tocar el store: un texto que no cabe en 80mm se rechaza
// como 400, no como un check violado de Postgres convertido en 500.
func (s *SettingsService) SetBusinessInfo(ctx context.Context, info domain.BusinessInfo, print domain.PrintSettings, timezone string, userID int64) (BusinessSettings, error) {
	if err := info.Validate(); err != nil {
		return BusinessSettings{}, err
	}
	// La zona se valida AQUÍ y no donde se usa: donde se usa está el camino de una venta, y ahí un
	// nombre mal escrito cae a UTC para no tumbar el cobro. Si nunca se rechazara al guardar, ese
	// fallback silencioso correría los cortes durante meses sin que nadie lo notara.
	if !domain.ValidTimezone(timezone) {
		return BusinessSettings{}, domain.ErrInvalidTimezone
	}
	err := s.store.QC(ctx).UpdateBusinessInfo(ctx, db.UpdateBusinessInfoParams{
		Timezone:           timezone,
		PrintFreeModifiers: print.PrintFreeModifiers,
		PrintKitchenTicket: print.PrintKitchenTicket,
		KitchenCanCharge:   print.KitchenCanCharge,
		BusinessName:       strings.TrimSpace(info.Name),
		Address:            strings.TrimSpace(info.Address),
		Phone:              strings.TrimSpace(info.Phone),
		HeaderNote:         strings.TrimSpace(info.HeaderNote),
		FooterNote:         strings.TrimSpace(info.FooterNote),
		AutoPrintOnClose:   print.AutoPrintOnClose,
		UpdatedBy:          &userID,
	})
	if err != nil {
		return BusinessSettings{}, err
	}
	return s.Get(ctx)
}

// SetLogo valida la imagen y la guarda. El mime que se persiste es el DETECTADO por contenido: es
// el que después se le promete al navegador en el Content-Type.
func (s *SettingsService) SetLogo(ctx context.Context, data []byte, userID int64) (BusinessSettings, error) {
	mime, err := domain.ValidateLogo(data)
	if err != nil {
		return BusinessSettings{}, err
	}
	if err := s.store.QC(ctx).SetTicketLogo(ctx, db.SetTicketLogoParams{
		LogoBytes: data, LogoMime: &mime, UpdatedBy: &userID,
	}); err != nil {
		return BusinessSettings{}, err
	}
	return s.Get(ctx)
}

// ClearLogo devuelve el ticket al logo por default. Idempotente: quitar un logo que no existe no
// es un error, el operador quiere el estado final y no una lección sobre el estado previo.
func (s *SettingsService) ClearLogo(ctx context.Context, userID int64) (BusinessSettings, error) {
	if err := s.store.QC(ctx).ClearTicketLogo(ctx, &userID); err != nil {
		return BusinessSettings{}, err
	}
	return s.Get(ctx)
}
