package app

import (
	"context"
	"errors"

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

type BusinessSettings struct {
	DeliveryFee decimal.Decimal `json:"deliveryFee"`
}

func (s *SettingsService) Get(ctx context.Context) (BusinessSettings, error) {
	row, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	if err != nil {
		// Empresa sin fila de ajustes aún (p. ej. tenant recién provisionado): default sin costo
		// de envío, en vez de un 500 en el camino del cobro.
		if errors.Is(err, pgx.ErrNoRows) {
			return BusinessSettings{DeliveryFee: decimal.Zero}, nil
		}
		return BusinessSettings{}, err
	}
	return BusinessSettings{DeliveryFee: row.DeliveryFee}, nil
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
