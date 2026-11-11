package alerts

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type rootErr struct{}

func (rootErr) Error() string { return "raíz" }

func TestErrorKind(t *testing.T) {
	pg := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "sin error"},
		{"errors.New", errors.New("x"), "*errors.errorString"},
		{"envuelto hasta un tipo propio", fmt.Errorf("cobrar: %w", fmt.Errorf("guardar: %w", rootErr{})), "alerts.rootErr"},
		{"postgres en la cadena", fmt.Errorf("cobrar pedido 12: %w", pg), "pg:23505"},
		{"join toma el primero", errors.Join(rootErr{}, errors.New("y")), "alerts.rootErr"},
		{"fmt sin %w", fmt.Errorf("pedido %d", 7), "*errors.errorString"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ErrorKind(c.err); got != c.want {
				t.Fatalf("ErrorKind = %q, quería %q", got, c.want)
			}
		})
	}
}

// Dos pedidos distintos con el mismo fallo deben caer en la misma ventana.
func TestErrorKindIgnoresTheMessage(t *testing.T) {
	a := fmt.Errorf("pedido 1: %w", &pgconn.PgError{Code: "23505"})
	b := fmt.Errorf("pedido 2: %w", &pgconn.PgError{Code: "23505"})
	if ErrorKind(a) != ErrorKind(b) {
		t.Fatal("el mismo error de base con otro texto cambió de tipo")
	}
}
