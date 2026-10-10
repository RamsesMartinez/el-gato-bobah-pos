package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// branchAmbiguousCode es el SQLSTATE que lanza `branch_for_company` (0076). Propio y no `P0001`
// para no confundirlo con cualquier otro `raise exception`.
const branchAmbiguousCode = "EGB01"

// DomainError traduce a un sentinel de domain los errores que la base lanza a propósito.
func DomainError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == branchAmbiguousCode {
		return fmt.Errorf("%w (%s)", domain.ErrBranchAmbiguous, pgErr.Message)
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
	tag, err := t.conn.Exec(ctx, sql, args...)
	return tag, DomainError(err)
}

func (t translating) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := t.conn.Query(ctx, sql, args...)
	if err != nil {
		return rows, DomainError(err)
	}
	return translatingRows{rows}, nil
}

func (t translating) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return translatingRow{t.conn.QueryRow(ctx, sql, args...)}
}

type translatingRows struct{ pgx.Rows }

func (r translatingRows) Err() error { return DomainError(r.Rows.Err()) }

type translatingRow struct{ pgx.Row }

func (r translatingRow) Scan(dest ...any) error { return DomainError(r.Row.Scan(dest...)) }

func queriesOn(conn db.DBTX) *db.Queries { return db.New(translating{conn}) }
