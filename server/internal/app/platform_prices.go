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

// PlatformPricesService: los precios que un negocio corrige a mano para una plataforma.
//
// Solo se guardan las EXCEPCIONES; la ausencia de fila significa "usa el calculado". Se edita
// desde la pantalla de venta y por cualquier rol que pueda vender: el pedido ya llegó y hay que
// imprimirlo, así que mandar al cajero a buscar un gerente cuesta más de lo que protege. El rastro
// de quién lo cambió (updated_by, not null) es la mitigación.
type PlatformPricesService struct {
	store *store.Store
}

func NewPlatformPricesService(s *store.Store) *PlatformPricesService {
	return &PlatformPricesService{store: s}
}

// SetProductPrice captura o corrige el precio de un producto en una plataforma.
func (s *PlatformPricesService) SetProductPrice(ctx context.Context, productID int64, platformID int16, price decimal.Decimal, userID int64) error {
	// allowZero en false: un producto de plataforma en $0 es siempre un error de captura. Los
	// regalos se manejan con descuento sobre el pedido, no con un precio de catálogo en cero.
	p := domain.Round2(price)
	if !domain.ValidMoney(p, false) {
		return domain.ErrValidation
	}
	if err := s.perteneceProducto(ctx, productID); err != nil {
		return err
	}
	if err := s.pertenecePlataforma(ctx, platformID); err != nil {
		return err
	}
	if err := s.store.QC(ctx).UpsertProductPlatformPrice(ctx, db.UpsertProductPlatformPriceParams{
		ProductID: productID, PlatformID: platformID, Price: p, UpdatedBy: userID,
	}); err != nil {
		return err
	}
	return nil
}

// DeleteProductPrice quita la excepción: el producto vuelve al precio calculado. Devuelve si
// realmente había una fila que borrar.
//
// Existe porque un precio equivocado pero PLAUSIBLE —$14.90 donde iban $149.00— pasa todas las
// validaciones, y el check `price > 0` cierra el idioma "pon 0 para limpiar". Sin esto habría que
// entrar a la base a mano.
//
// El bool importa: quien llama invalida el menú cacheado y despierta a todas las tablets. Borrar lo
// que no existe no cambió nada, así que tampoco debe costar un refetch a todo el local.
func (s *PlatformPricesService) DeleteProductPrice(ctx context.Context, productID int64, platformID int16) (bool, error) {
	n, err := s.store.QC(ctx).DeleteProductPlatformPrice(ctx, db.DeleteProductPlatformPriceParams{
		ProductID: productID, PlatformID: platformID,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetOptionDelta hace lo mismo para el cargo de una opción de modificador.
func (s *PlatformPricesService) SetOptionDelta(ctx context.Context, optionID int64, platformID int16, delta decimal.Decimal, userID int64) error {
	// allowZero en TRUE, a diferencia de los productos: un extra sin costo ("sin cebolla") es
	// normal y su delta es 0. Lo que no vale es negativo.
	d := domain.Round2(delta)
	if !domain.ValidMoney(d, true) {
		return domain.ErrValidation
	}
	if err := s.perteneceOpcion(ctx, optionID); err != nil {
		return err
	}
	if err := s.pertenecePlataforma(ctx, platformID); err != nil {
		return err
	}
	if err := s.store.QC(ctx).UpsertOptionPlatformPrice(ctx, db.UpsertOptionPlatformPriceParams{
		OptionID: optionID, PlatformID: platformID, PriceDelta: d, UpdatedBy: userID,
	}); err != nil {
		return err
	}
	return nil
}

func (s *PlatformPricesService) DeleteOptionDelta(ctx context.Context, optionID int64, platformID int16) (bool, error) {
	n, err := s.store.QC(ctx).DeleteOptionPlatformPrice(ctx, db.DeleteOptionPlatformPriceParams{
		OptionID: optionID, PlatformID: platformID,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Comprobaciones de PERTENENCIA, bajo RLS y antes de escribir.
//
// La llave foránea no basta: los chequeos de integridad referencial de Postgres SALTAN RLS por
// diseño, así que un product_id de otra empresa entraba sin protestar. La fila quedaba con el
// company_id del atacante pero ocupando la PK global (product_id, platform_id), y a partir de ahí
// el dueño legítimo no podía capturar su propio precio —su upsert caía en ON CONFLICT DO UPDATE y
// chocaba con la política, saliendo como 500— ni borrar la fila intrusa, porque bajo RLS no la ve.
// Irreparable desde el producto, y con los ids seriales bastaba recorrerlos para ocupar el catálogo
// de todos los negocios.
//
// Se devuelve ErrNotFound —el MISMO error que un id inexistente— a propósito: distinguir "no
// existe" de "no es tuyo" convierte el endpoint en un censo de los catálogos ajenos.

func (s *PlatformPricesService) perteneceProducto(ctx context.Context, productID int64) error {
	ok, err := s.store.QC(ctx).ProductExists(ctx, productID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

func (s *PlatformPricesService) perteneceOpcion(ctx context.Context, optionID int64) error {
	ok, err := s.store.QC(ctx).OptionExists(ctx, optionID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

func (s *PlatformPricesService) pertenecePlataforma(ctx context.Context, platformID int16) error {
	if _, err := s.store.QC(ctx).GetPlatformByID(ctx, platformID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}
