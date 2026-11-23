package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// #335: la FK compuesta que frena un id de otra empresa respondía 500 «Error interno» sin causa. Con
// RLS, Postgres no dice el valor, solo la tabla: de ahí sale «el producto que mandaste no existe en esta
// empresa», y quien ya distinguía el 23503 lo sigue viendo.
func TestDomainErrorTranslatesAMissingReference(t *testing.T) {
	pg := &pgconn.PgError{Code: "23503", Detail: `Key is not present in table "products".`}
	got := DomainError(pg)
	var ref domain.ReferenceNotFound
	if !errors.As(got, &ref) || !strings.Contains(got.Error(), "el producto") {
		t.Fatalf("no se tradujo: %v", got)
	}
	var back *pgconn.PgError
	if !errors.As(got, &back) || back.Code != "23503" {
		t.Fatal("el 23503 se perdió: isForeignKeyViolation dejaría de verlo")
	}
	// Borrar algo que otro renglón usa también es 23503, pero no es «no existe»: se queda como estaba.
	inUse := &pgconn.PgError{Code: "23503", Detail: `Key (id)=(5) is still referenced from table "order_lines".`}
	if got := DomainError(inUse); got != error(inUse) {
		t.Fatalf("un borrado frenado se tradujo como referencia inexistente: %v", got)
	}
}

// fakeConn acepta todo: lo que se prueba es si la conexión anota que pudo haber escrito.
type fakeConn struct{}

func (fakeConn) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (fakeConn) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("sin filas")
}
func (fakeConn) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

// #335: una escritura que falló sin confirmar nada deja su llave en «failed»; una que pudo haber
// confirmado, en «unknown». Fuera de una transacción cada sentencia se confirma sola, así que una que no
// es de lectura cuenta como «pudo haber escrito»; una lectura no.
func TestWriteTrackerSeesAutocommitWrites(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"-- name: GetProduct :one\nselect id from products where id = $1", false},
		{"  SELECT 1", false},
		{"-- name: InsertMovement :one\ninsert into stock_movements (x) values ($1)", true},
		{"update products set price = $1", true},
		{"with x as (insert into a values (1) returning id) select id from x", true},
		{"-- name: EnsureCompanyRoles :exec\nselect ensure_company_roles($1)", true},
		{"select evaluate_low_stock($1, $2)", true},
	}
	for _, c := range cases {
		ctx, tr := TrackWrites(context.Background())
		_, _ = translating{conn: fakeConn{}}.Exec(ctx, c.sql)
		if tr.MayHaveWritten() != c.want {
			t.Errorf("%q: pudo haber escrito = %v, quería %v", c.sql, tr.MayHaveWritten(), c.want)
		}
	}
	// Sin rastreador en el contexto no pasa nada (las peticiones de la pantalla no lo llevan).
	_, _ = translating{conn: fakeConn{}}.Exec(context.Background(), "insert into a values (1)")
}
