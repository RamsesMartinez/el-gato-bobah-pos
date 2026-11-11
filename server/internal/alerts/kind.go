// Package alerts manda por correo las respuestas 500 de la API (spec 070), agrupadas para que una
// ráfaga no llene el buzón y sin que el request espere nada.
package alerts

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrorKind names the kind of an error for grouping: the SQLSTATE if Postgres is in the chain,
// else the type of the innermost error. El texto no sirve: trae ids, y cada pedido sería otro error.
func ErrorKind(err error) string {
	if err == nil {
		return "sin error"
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return "pg:" + pg.Code
	}
	for {
		var next error
		switch u := err.(type) {
		case interface{ Unwrap() error }:
			next = u.Unwrap()
		case interface{ Unwrap() []error }:
			if errs := u.Unwrap(); len(errs) > 0 {
				next = errs[0]
			}
		}
		if next == nil {
			return fmt.Sprintf("%T", err)
		}
		err = next
	}
}
