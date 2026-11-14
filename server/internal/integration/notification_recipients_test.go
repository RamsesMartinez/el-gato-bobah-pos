//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// DESTINATARIOS POR PERMISO (spec 084, historia 7): por omisión, quien configura el almacén y tiene
// correo; quitar a uno funciona, un correo extra recibe aunque no sea de nadie, y un rol propio sin el
// permiso no recibe nada.

func setEmail(t *testing.T, st *store.Store, user int64, email string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `update users set recovery_email = $2 where id = $1`, user, email); err != nil {
		t.Fatal(err)
	}
}

func TestRecipientsByPermission(t *testing.T) {
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_084d", "admin")
	gerente := makeUser(t, st, "gerente_084d", "gerente")
	sinCorreo := makeUser(t, st, "gerente2_084d", "gerente")
	cajero := makeUser(t, st, "cajero_084d", "cajero")
	setEmail(t, st, admin, "admin084@ejemplo.mx")
	setEmail(t, st, gerente, "gerente084@ejemplo.mx")
	setEmail(t, st, cajero, "cajero084@ejemplo.mx")
	svc, _, ctx := asNotifications(t, defaultCompanyID)

	emails := func(kind string) []string {
		t.Helper()
		got, err := svc.RecipientsFor(ctx, kind)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		return got
	}

	t.Run("por omisión: quien configura el almacén y tiene correo", func(t *testing.T) {
		if got := emails("stock"); !slices.Equal(got, []string{"admin084@ejemplo.mx", "gerente084@ejemplo.mx"}) {
			t.Fatalf("%v", got)
		}
		v, err := svc.Recipients(ctx)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(v.People, func(p app.RecipientPerson) bool { return p.UserID == sinCorreo })
		if i < 0 || v.People[i].HasEmail || slices.ContainsFunc(v.People, func(p app.RecipientPerson) bool { return p.UserID == cajero }) {
			t.Fatalf("personas %+v", v.People)
		}
		if v.MorningAt != "08:00" {
			t.Fatalf("hora %q", v.MorningAt)
		}
	})

	t.Run("quitar a uno, agregar un correo extra y elegir por tipo", func(t *testing.T) {
		in := app.RecipientsView{MorningAt: "07:30",
			People: []app.RecipientPerson{{UserID: admin, Stock: false, NotFound: true}, {UserID: gerente, Stock: true, NotFound: false}},
			Extras: []app.RecipientExtra{{Email: " Compras084@Ejemplo.mx ", Stock: true, NotFound: false}}}
		if err := svc.SetRecipients(ctx, in, admin); err != nil {
			t.Fatal(err)
		}
		if got := emails("stock"); !slices.Equal(got, []string{"compras084@ejemplo.mx", "gerente084@ejemplo.mx"}) {
			t.Fatalf("stock %v", got)
		}
		if got := emails("not_found"); !slices.Equal(got, []string{"admin084@ejemplo.mx"}) {
			t.Fatalf("no lo encuentro %v", got)
		}
		v, _ := svc.Recipients(ctx)
		if v.MorningAt != "07:30" || len(v.Extras) != 1 || v.Extras[0].Email != "compras084@ejemplo.mx" {
			t.Fatalf("%+v", v)
		}
		var audits int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'notification.recipients_changed'`).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("bitácora %d %v", audits, err)
		}
	})

	t.Run("lo inválido es 400", func(t *testing.T) {
		for name, in := range map[string]app.RecipientsView{
			"correo mal escrito":       {MorningAt: "08:00", Extras: []app.RecipientExtra{{Email: "no-es-correo", Stock: true}}},
			"hora fuera de los 15 min": {MorningAt: "08:10"},
			"persona sin el permiso":   {MorningAt: "08:00", People: []app.RecipientPerson{{UserID: cajero, Stock: true}}},
		} {
			if err := svc.SetRecipients(ctx, in, admin); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	t.Run("un rol propio sin el permiso no recibe", func(t *testing.T) {
		var role int64
		if err := st.Pool.QueryRow(ctx, `insert into roles (name, active) values ('Compras 084', true) returning id`).Scan(&role); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `update users set role_id = $2 where id = $1`, gerente, role); err != nil {
			t.Fatal(err)
		}
		if got := emails("stock"); slices.Contains(got, "gerente084@ejemplo.mx") {
			t.Fatalf("un rol sin el permiso recibe: %v", got)
		}
	})
}
