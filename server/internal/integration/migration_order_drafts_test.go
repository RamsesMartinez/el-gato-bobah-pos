//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

// orderDraftsVersion es la migración de las cuentas en captura. Su número se mueve al siguiente
// libre de develop al fusionar (data-model.md), y esta constante con él.
const orderDraftsVersion = 82

// LAS CUENTAS EN CAPTURA SOBRE UN RESPALDO REAL CON DOS EMPRESAS (spec 030).
//
// Lo que un esquema sembrado desde cero no ve: que el rol de la aplicación pueda leer y escribir las
// tablas nuevas (y NO borrar una cuenta: la descartada se conserva), que una cuenta de una empresa no
// pueda colgarse del producto, el usuario, la plataforma o el pedido de otra (los chequeos de FK
// saltan RLS), que dos cuentas vivas no compartan nombre, y que el Down se niegue cuando perdería lo
// que alguien está capturando o el rastro de quién capturó un pedido.
func TestMigrationOrderDraftsOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start >= orderDraftsVersion {
		migrarAbajoHasta(t, st.Pool, orderDraftsVersion-1)
	} else if start < orderDraftsVersion-1 {
		migrarArriba(t, st.Pool)
		migrarAbajoHasta(t, st.Pool, orderDraftsVersion-1)
	}
	t.Cleanup(func() {
		if v := versionDeEsquema(t, st); v != start && start < orderDraftsVersion {
			migrarAbajoHasta(t, st.Pool, start)
		}
	})

	var ordersBefore int
	var totalBefore string
	if err := st.Pool.QueryRow(ctx, `select count(*), coalesce(sum(total), 0)::text from orders`).
		Scan(&ordersBefore, &totalBefore); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("no toca un solo pedido", func(t *testing.T) {
		var n int
		var total string
		if err := st.Pool.QueryRow(ctx, `select count(*), coalesce(sum(total), 0)::text from orders`).Scan(&n, &total); err != nil {
			t.Fatal(err)
		}
		if n != ordersBefore || total != totalBefore {
			t.Fatalf("pedidos %d→%d, total %s→%s: la migración solo crea tablas", ordersBefore, n, totalBefore, total)
		}
	})

	// Dos empresas reales, cada una con su usuario, producto, pedido y plataforma.
	type empresa struct{ id, user, product, order int64 }
	pick := func(t *testing.T, notCompany int64) empresa {
		t.Helper()
		var e empresa
		if err := st.Pool.QueryRow(ctx, `
			select o.company_id, o.opened_by, p.id, o.id
			  from orders o
			  join products p on p.company_id = o.company_id
			 where o.company_id <> $1
			 order by o.id desc limit 1`, notCompany).Scan(&e.id, &e.user, &e.product, &e.order); err != nil {
			t.Fatalf("una empresa con pedido y producto: %v", err)
		}
		return e
	}
	a := pick(t, 0)
	b := pick(t, a.id)
	var platformB int16
	if err := st.Pool.QueryRow(ctx, `select id from delivery_platforms where company_id = $1 limit 1`, b.id).Scan(&platformB); err != nil {
		t.Fatalf("una plataforma de la otra empresa: %v", err)
	}

	// insertDraft inserta como dueño, con la empresa explícita.
	insertDraft := func(tx pgx.Tx, id uuid.UUID, cols string, vals string, args ...any) error {
		all := append([]any{id, a.id, a.user}, args...)
		_, err := tx.Exec(ctx, `insert into order_drafts (id, company_id, opened_by`+cols+`) values ($1, $2, $3`+vals+`)`, all...)
		return err
	}

	t.Run("el rol de la aplicación lee, crea y cambia, pero no borra una cuenta", func(t *testing.T) {
		conn := conexionDeEmpresa(t, restoredAppRoleStore(t), a.id)
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		draft, line, op := uuid.New(), uuid.New(), uuid.New()
		steps := []struct {
			name string
			sql  string
			args []any
		}{
			{"crear la cuenta", `insert into order_drafts (id, opened_by, folio_name, folio_scheme) values ($1, $2, 'Prueba030', 'razas')`, []any{draft, a.user}},
			{"agregar un renglón", `insert into order_draft_lines (id, draft_id, product_id, qty, position) values ($1, $2, $3, 1, 1)`, []any{line, draft, a.product}},
			{"anotar el agregado", `insert into order_draft_adds (op_id, draft_id, line_id) values ($1, $2, $3)`, []any{op, draft, line}},
			{"cambiar la cuenta", `update order_drafts set customer_name = 'Mesa 4', header_version = header_version + 1 where id = $1`, []any{draft}},
			{"cambiar el renglón", `update order_draft_lines set qty = 2, version = version + 1 where id = $1`, []any{line}},
			{"quitar el renglón", `delete from order_draft_lines where id = $1`, []any{line}},
		}
		for _, s := range steps {
			if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
				t.Fatalf("%s como gatobobah_app: %v", s.name, err)
			}
		}
		var n int
		if err := tx.QueryRow(ctx, `select (select count(*) from order_drafts where id = $1) + (select count(*) from order_draft_adds where op_id = $2)`,
			draft, op).Scan(&n); err != nil || n != 2 {
			t.Fatalf("leer como gatobobah_app: n=%d err=%v", n, err)
		}
		_, err = tx.Exec(ctx, `delete from order_drafts where id = $1`, draft)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("borrar una cuenta como gatobobah_app tiene que negarse con 42501 (se conserva el rastro), fue %v", err)
		}
	})

	t.Run("una cuenta de una empresa no se cuelga de otra", func(t *testing.T) {
		cases := []struct {
			name string
			run  func(tx pgx.Tx) error
		}{
			{"usuario ajeno", func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `insert into order_drafts (id, company_id, opened_by, folio_name, folio_scheme) values ($1, $2, $3, 'X030', 'razas')`,
					uuid.New(), a.id, b.user)
				return err
			}},
			{"plataforma ajena", func(tx pgx.Tx) error {
				return insertDraft(tx, uuid.New(), ", folio_name, folio_scheme, delivery_platform_id", ", 'X030', 'razas', $4", platformB)
			}},
			{"pedido ajeno", func(tx pgx.Tx) error {
				return insertDraft(tx, uuid.New(), ", order_id", ", $4", b.order)
			}},
			{"quien puso el descuento es de otra empresa", func(tx pgx.Tx) error {
				return insertDraft(tx, uuid.New(), ", folio_name, folio_scheme, discount_amount, discount_set_by", ", 'X030', 'razas', 5, $4", b.user)
			}},
			{"producto ajeno", func(tx pgx.Tx) error {
				d := uuid.New()
				if err := insertDraft(tx, d, ", folio_name, folio_scheme", ", 'X030', 'razas'"); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `insert into order_draft_lines (id, company_id, draft_id, product_id, qty, position) values ($1, $2, $3, $4, 1, 1)`,
					uuid.New(), a.id, d, b.product)
				return err
			}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				exigeViolacionDeRestriccion(t, inRolledBackTx(t, st.Pool, c.run), c.name)
			})
		}
	})

	t.Run("dos cuentas vivas no comparten nombre; una enviada y una viva sí", func(t *testing.T) {
		err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
			if err := insertDraft(tx, uuid.New(), ", folio_name, folio_scheme", ", 'Persa030', 'razas'"); err != nil {
				t.Fatalf("la primera: %v", err)
			}
			return insertDraft(tx, uuid.New(), ", folio_name, folio_scheme", ", 'Persa030', 'razas'")
		})
		exigeViolacionDeRestriccion(t, err, "dos cuentas vivas con el mismo nombre")

		err = inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
			if err := insertDraft(tx, uuid.New(), ", folio_name, folio_scheme, status, sent_at, order_id",
				", 'Persa030', 'razas', 'enviada', now(), $4", a.order); err != nil {
				return err
			}
			return insertDraft(tx, uuid.New(), ", folio_name, folio_scheme", ", 'Persa030', 'razas'")
		})
		if err != nil {
			t.Fatalf("una enviada y una viva con el mismo nombre tienen que convivir: %v", err)
		}
	})

	t.Run("una sola «Nuevo» viva por pedido", func(t *testing.T) {
		err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
			if err := insertDraft(tx, uuid.New(), ", order_id", ", $4", a.order); err != nil {
				t.Fatalf("la primera: %v", err)
			}
			return insertDraft(tx, uuid.New(), ", order_id", ", $4", a.order)
		})
		exigeViolacionDeRestriccion(t, err, "dos «Nuevo» vivas del mismo pedido")
	})

	t.Run("los checks de pareja", func(t *testing.T) {
		cases := []struct{ name, cols, vals string }{
			{"enviada sin fecha", ", order_id, status", ", $4, 'enviada'"},
			{"fecha de envío en una viva", ", folio_name, folio_scheme, sent_at", ", 'X030', 'razas', now()"},
			{"enviada sin pedido", ", folio_name, folio_scheme, status, sent_at", ", 'X030', 'razas', 'enviada', now()"},
			{"descartada sin fecha", ", folio_name, folio_scheme, status, discard_reason", ", 'X030', 'razas', 'descartada', 'manual'"},
			{"descartada sin motivo", ", folio_name, folio_scheme, status, discarded_at", ", 'X030', 'razas', 'descartada', now()"},
			{"motivo en una viva", ", folio_name, folio_scheme, discard_reason", ", 'X030', 'razas', 'manual'"},
			{"motivo inventado", ", folio_name, folio_scheme, status, discarded_at, discard_reason", ", 'X030', 'razas', 'descartada', now(), 'porque sí'"},
			{"descuento en monto y en porcentaje", ", folio_name, folio_scheme, discount_amount, discount_percent, discount_set_by", ", 'X030', 'razas', 5, 10, $3"},
			{"descuento sin autor", ", folio_name, folio_scheme, discount_amount", ", 'X030', 'razas', 5"},
			{"porcentaje de más de 100", ", folio_name, folio_scheme, discount_percent, discount_set_by", ", 'X030', 'razas', 101, $3"},
			{"nombre sin esquema", ", folio_name", ", 'X030'"},
			{"cuenta nueva sin nombre", "", ""},
			{"folio de plataforma sin autor", ", folio_name, folio_scheme, platform_order_ref", ", 'X030', 'razas', 'A1'"},
			{"envío negativo", ", folio_name, folio_scheme, delivery_fee", ", 'X030', 'razas', -1"},
			{"cliente de 61 letras", ", folio_name, folio_scheme, customer_name", ", 'X030', 'razas', repeat('a', 61)"},
			{"estado inventado", ", folio_name, folio_scheme, status", ", 'X030', 'razas', 'otra'"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
					if strings.Contains(c.vals, "$4") {
						return insertDraft(tx, uuid.New(), c.cols, c.vals, a.order)
					}
					return insertDraft(tx, uuid.New(), c.cols, c.vals)
				})
				exigeViolacionDeRestriccion(t, err, c.name)
			})
		}
		t.Run("renglón con cantidad cero o modificadores que no son lista", func(t *testing.T) {
			for _, bad := range []string{
				`insert into order_draft_lines (id, company_id, draft_id, product_id, qty, position) values ($1, $2, $3, $4, 0, 1)`,
				`insert into order_draft_lines (id, company_id, draft_id, product_id, qty, position, modifiers) values ($1, $2, $3, $4, 1, 1, '{}')`,
				`insert into order_draft_lines (id, company_id, draft_id, product_id, qty, position, notes) values ($1, $2, $3, $4, 1, 1, repeat('a', 201))`,
			} {
				err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
					d := uuid.New()
					if err := insertDraft(tx, d, ", folio_name, folio_scheme", ", 'X030', 'razas'"); err != nil {
						return err
					}
					_, err := tx.Exec(ctx, bad, uuid.New(), a.id, d, a.product)
					return err
				})
				exigeViolacionDeRestriccion(t, err, bad)
			}
		})
	})

	t.Run("el Down se niega si perdería algo", func(t *testing.T) {
		guards := []struct{ name, cols, vals, want string }{
			{"una cuenta capturándose", ", folio_name, folio_scheme", ", 'Abajo030', 'razas'", "capturando"},
			{"una cuenta enviada", ", folio_name, folio_scheme, status, sent_at, order_id", ", 'Abajo030', 'razas', 'enviada', now(), $4", "enviada"},
		}
		for _, g := range guards {
			t.Run(g.name, func(t *testing.T) {
				id := uuid.New()
				args := []any{id, a.id, a.user}
				if strings.Contains(g.vals, "$4") {
					args = append(args, a.order)
				}
				if _, err := st.Pool.Exec(ctx, `insert into order_drafts (id, company_id, opened_by`+g.cols+`) values ($1, $2, $3`+g.vals+`)`, args...); err != nil {
					t.Fatalf("preparar: %v", err)
				}
				err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", orderDraftsVersion-1)
				if _, uerr := st.Pool.Exec(ctx, `delete from order_drafts where id = $1`, id); uerr != nil && err != nil {
					t.Fatalf("deshacer: %v", uerr)
				}
				if err == nil {
					migrarArriba(t, st.Pool)
					t.Fatalf("el Down corrió con %s: la habría perdido", g.name)
				}
				if !strings.Contains(err.Error(), g.want) {
					t.Fatalf("el Down se negó, pero no por su guarda (quería «%s»): %v", g.want, err)
				}
				if v := versionDeEsquema(t, st); v < orderDraftsVersion {
					t.Fatalf("el Down se negó pero dejó la base en %d", v)
				}
			})
		}
	})

	t.Run("sin nada que perder, baja y vuelve a subir", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, orderDraftsVersion-1)
		var exists bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass('order_drafts') is not null`).Scan(&exists); err != nil || exists {
			t.Fatalf("el Down dejó la tabla: exists=%v err=%v", exists, err)
		}
		migrarArriba(t, st.Pool)
	})
}
