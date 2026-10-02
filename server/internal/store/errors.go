package store

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// branchAmbiguousCode es el SQLSTATE que lanza `branch_for_company` (0076). Propio y no `P0001`
// para no confundirlo con cualquier otro `raise exception`.
const branchAmbiguousCode = "EGB01"

// DomainError traduce a un sentinel de domain los errores que la base lanza a propósito.
//
// Una sola traducción para todos porque la sucursal se resuelve en triggers: cualquier insert de
// caja, pedido o inventario puede lanzarlo, y repartirla por los servicios dejaría fuera al primer
// camino nuevo.
func DomainError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == branchAmbiguousCode {
		return fmt.Errorf("%w (%s)", domain.ErrBranchAmbiguous, pgErr.Message)
	}
	return err
}
