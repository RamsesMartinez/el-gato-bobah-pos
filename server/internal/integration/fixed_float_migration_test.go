//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// fixedFloatVersion es la migración del fondo fijo (spec 035). Su número es PROVISIONAL: se mueve al
// siguiente libre de develop al fusionar, y esta constante con él.
const fixedFloatVersion = 90

// cierreViejo siembra como owner un turno cerrado con su conteo, sin pasar por el servicio: la API
// de hoy ya lee las columnas nuevas y no corre sobre el esquema anterior. `deja` vacío = un cierre de
// antes de la decisión del 2026-10-10, sin fondo tecleado.
func cierreViejo(t *testing.T, st *store.Store, company, reg, user int64, contado, deja string) {
	t.Helper()
	ctx := context.Background()
	var left any
	if deja != "" {
		left = deja
	}
	var id int64
	if err := st.Pool.QueryRow(ctx, `insert into register_sessions (company_id, register_id, opened_by, business_date, status, closed_at, closed_by, float_left)
		values ($1, $2, $3, current_date, 'cerrada', now(), $3, $4::numeric) returning id`, company, reg, user, left).Scan(&id); err != nil {
		t.Fatalf("sembrar el turno: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into session_cash_counts (company_id, session_id, moment, total, expected, created_by)
		values ($1, $2, 'cierre', $3::numeric, 0, $4)`, company, id, contado, user); err != nil {
		t.Fatalf("sembrar el conteo: %v", err)
	}
}

// LOS CIERRES QUE YA EXISTEN NO CAMBIAN (FR-009, SC-003). La migración solo agrega: un cierre que
// guardó un fondo tecleado lo conserva, uno de antes de esa decisión sigue dejando lo contado, y a
// ninguno se le inventa un retiro. Dos empresas, porque un «para cada caja» con una sola es un no-op.
func TestMigrationFixedFloatKeepsPastClosesIntact(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	// La empresa B se crea ANTES de bajar el esquema: `CreateCompany` siembra lo que exige el esquema
	// de hoy (los motivos de merma de la 0094), y las filas de la empresa sobreviven a bajarlo.
	coB := makeCompany(t, st, "mig-ff-b")
	migrarAbajoHasta(t, st.Pool, fixedFloatVersion-1)

	cajeroA := makeUser(t, st, "cajero_mig_ff_a", "cajero")
	regA := principalRegister(t, st)
	cierreViejo(t, st, defaultCompanyID, regA, cajeroA, "1000", "700")
	cierreViejo(t, st, defaultCompanyID, regA, cajeroA, "900", "")

	cajeroB := makeUserIn(t, st, coB, "cajero_mig_ff_b", "cajero")
	var regB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (company_id, name, is_primary) values ($1, 'Principal B', true) returning id`, coB).Scan(&regB); err != nil {
		t.Fatal(err)
	}
	cierreViejo(t, st, coB, regB, cajeroB, "2000", "300")

	type cierre struct {
		id   int64
		left *decimal.Decimal
	}
	leer := func() []cierre {
		rows, err := st.Pool.Query(ctx, `select id, float_left from register_sessions where status = 'cerrada' order by id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []cierre
		for rows.Next() {
			var c cierre
			if err := rows.Scan(&c.id, &c.left); err != nil {
				t.Fatal(err)
			}
			out = append(out, c)
		}
		return out
	}
	antes := leer()
	if len(antes) != 3 {
		t.Fatalf("esperaba 3 cierres sembrados, hay %d", len(antes))
	}

	migrarArriba(t, st.Pool)

	despues := leer()
	for i := range antes {
		a, d := antes[i], despues[i]
		if (a.left == nil) != (d.left == nil) || (a.left != nil && !a.left.Equal(*d.left)) {
			t.Errorf("el cierre %d cambió su fondo al migrar: %v → %v", a.id, a.left, d.left)
		}
	}
	var retiros, snapshots, fijos int
	if err := st.Pool.QueryRow(ctx, `select (select count(*) from register_cash_movements where kind = 'retiro'),
		(select count(*) from register_sessions where fixed_float_at_close is not null),
		(select count(*) from cash_registers where fixed_float is not null)`).Scan(&retiros, &snapshots, &fijos); err != nil {
		t.Fatalf("leer las columnas nuevas: %v", err)
	}
	if retiros != 0 || snapshots != 0 || fijos != 0 {
		t.Fatalf("la migración inventó datos: %d retiros, %d snapshots, %d fondos fijos", retiros, snapshots, fijos)
	}
}

// EL DOWN SE NIEGA si perdería un retiro registrado o un fondo configurado sin rastro.
func TestMigrationFixedFloatDownRefusesToLoseData(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_down_ff", "cajero")
	reg := principalRegister(t, st)
	abre := dec("0")
	sess, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &abre, Motivo: "prueba"}, cajero)
	if err != nil {
		t.Fatal(err)
	}
	guards := []struct {
		name, setup, undo string
		args              []any
	}{
		{"un retiro", `insert into register_cash_movements (session_id, kind, amount, concept, user_id, recipient_user_id, recipient_name)
			values ($1, 'retiro', 10, 'Retiro al cierre', $2, $2, 'Test')`, `delete from register_cash_movements where kind = 'retiro'`, []any{sess.ID, cajero}},
		{"un fondo fijo", `update cash_registers set fixed_float = 1000 where id = $1`, `update cash_registers set fixed_float = null`, []any{reg}},
		{"un snapshot de fondo fijo", `update register_sessions set fixed_float_at_close = 1000 where id = $1`, `update register_sessions set fixed_float_at_close = null`, []any{sess.ID}},
	}
	for _, g := range guards {
		t.Run(g.name, func(t *testing.T) {
			if _, err := st.Pool.Exec(ctx, g.setup, g.args...); err != nil {
				t.Fatalf("preparar: %v", err)
			}
			err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", fixedFloatVersion-1)
			if _, uerr := st.Pool.Exec(ctx, g.undo); uerr != nil {
				t.Fatalf("deshacer: %v", uerr)
			}
			if err == nil {
				migrarArriba(t, st.Pool)
				t.Fatalf("el Down corrió con %s: lo habría perdido sin rastro", g.name)
			}
			if !strings.Contains(err.Error(), "fondo fijo") {
				t.Fatalf("el Down se negó, pero no por su guarda: %v", err)
			}
		})
	}
	t.Run("sin nada que perder, baja y vuelve a subir", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, fixedFloatVersion-1)
		migrarArriba(t, st.Pool)
	})
}
