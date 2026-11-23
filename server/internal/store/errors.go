package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// branchAmbiguousCode es el SQLSTATE que lanza `branch_for_company` (0076). Propio y no `P0001`
// para no confundirlo con cualquier otro `raise exception`.
const branchAmbiguousCode = "EGB01"

// mergedIngredientCode es el SQLSTATE de las guardas de la 0104: algo nuevo apunta a un insumo que se
// unió a otro. El mensaje trae «ingredient_merged|<destino>».
const mergedIngredientCode = "EGB02"

// uniqueViolationCode es el SQLSTATE de una llave única repetida (el nombre de un menú público que
// otra empresa ganó en la carrera, spec 097).
const uniqueViolationCode = "23505"

// warehouseMissingCode y warehouseNotWritableCode son los de la 0120: la sucursal no tiene almacén
// de venta (`sales_warehouse_of`), y un movimiento o un cambio de almacén de venta que la base todavía
// no admite mientras guarde las existencias con llave por sucursal.
const (
	warehouseMissingCode     = "EGB03"
	warehouseNotWritableCode = "EGB04"
)

// DomainError traduce a un sentinel de domain los errores que la base lanza a propósito.
func DomainError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case branchAmbiguousCode:
		return fmt.Errorf("%w (%s)", domain.ErrBranchAmbiguous, pgErr.Message)
	case mergedIngredientCode:
		if _, target, ok := strings.Cut(pgErr.Message, "|"); ok && target != "" {
			return domain.IngredientMergedInto{Target: target}
		}
		return domain.ErrIngredientMerged
	case warehouseMissingCode:
		return fmt.Errorf("%w (%s)", domain.ErrWarehouseMissing, pgErr.Message)
	case warehouseNotWritableCode:
		return domain.ErrWarehouseNotWritableYet
	case uniqueViolationCode:
		// Otra empresa ganó la carrera por el mismo nombre de menú (spec 097). Solo ESA restricción: el
		// índice de «un vigente por empresa» no se disfraza de «no disponible».
		if pgErr.ConstraintName == "public_menu_names_name_key" {
			return domain.ErrMenuNameUnavailable
		}
	case "23503":
		// Insert o update con una referencia que no existe en la empresa (#335). Con RLS, Postgres no
		// dice el valor, solo la tabla. El borrado frenado («is still referenced») no es esto.
		const marker = `is not present in table "`
		if _, rest, ok := strings.Cut(pgErr.Detail, marker); ok {
			table, _, _ := strings.Cut(rest, `"`)
			return domain.ReferenceNotFoundWrapping(table, err)
		}
	}
	return err
}

// translating envuelve la conexión que reciben las Queries para que TODO error pase por
// DomainError antes de salir de store.
//
// En la conexión y no en cada servicio porque la sucursal se resuelve en triggers y en consultas
// de lectura: cualquier insert de caja, pedido o inventario, y cualquier lectura de existencias,
// puede lanzarlo. Traducirlo servicio por servicio dejaría fuera al primer camino nuevo, y
// traducirlo en httpapi obligaría a esa capa a conocer a store.
type translating struct{ conn db.DBTX }

func (t translating) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.noteStatement(ctx, sql)
	tag, err := t.conn.Exec(ctx, sql, args...)
	return tag, DomainError(err)
}

func (t translating) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	t.noteStatement(ctx, sql)
	rows, err := t.conn.Query(ctx, sql, args...)
	if err != nil {
		return rows, DomainError(err)
	}
	return translatingRows{rows}, nil
}

func (t translating) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	t.noteStatement(ctx, sql)
	return translatingRow{t.conn.QueryRow(ctx, sql, args...)}
}

type translatingRows struct{ pgx.Rows }

func (r translatingRows) Err() error { return DomainError(r.Rows.Err()) }

type translatingRow struct{ pgx.Row }

func (r translatingRow) Scan(dest ...any) error { return DomainError(r.Row.Scan(dest...)) }

func queriesOn(conn db.DBTX) *db.Queries { return db.New(translating{conn}) }
