package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func TestDomainErrorTranslatesOnlyTheBranchCode(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		wantAmbiguous bool
	}{
		{"la sucursal no se pudo resolver", &pgconn.PgError{Code: "EGB01", Message: "branch_ambiguous"}, true},
		{"envuelto por un servicio", fmt.Errorf("crear pedido: %w", &pgconn.PgError{Code: "EGB01"}), true},
		{"otro raise exception", &pgconn.PgError{Code: "P0001"}, false},
		{"llave foránea", &pgconn.PgError{Code: "23503"}, false},
		{"error que no es de Postgres", errors.New("red caída"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DomainError(c.err)
			if errors.Is(got, domain.ErrBranchAmbiguous) != c.wantAmbiguous {
				t.Fatalf("DomainError(%v) = %v; ¿sucursal ambigua? quería %v", c.err, got, c.wantAmbiguous)
			}
			if !c.wantAmbiguous && got != c.err {
				t.Fatalf("un error ajeno a la sucursal se modificó: %v → %v", c.err, got)
			}
		})
	}
}
