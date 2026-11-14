//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL CENTRO DE AVISOS (spec 084, historias 2 y 5): lo leído es por persona, «No lo encuentro» solo lo
// ve quien configura el almacén, y cerrar un aviso a mano deja su motivo y quién.

func asNotifications(t *testing.T, company int64) (*app.NotificationsService, *app.StockOutService, context.Context) {
	t.Helper()
	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.NewNotificationsService(as, clock), app.NewStockOutService(as, clock), ctx
}

var managerPerms084 = domain.PermissionsFor(domain.RoleGerente)
var cashierPerms084 = domain.PermissionsFor(domain.RoleCajero)

func TestTheBellIsPerPerson(t *testing.T) {
	st := newTestStore(t)
	ana := makeUser(t, st, "ana_084", "cajero")
	marisol := makeUser(t, st, "marisol_084", "gerente")
	ramiro := makeUser(t, st, "ramiro_084", "gerente")
	svc, out, ctx := asNotifications(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Vasos campana 084")
	adjust(t, st, "ingrediente", ing, "4")
	if _, err := st.Pool.Exec(ctx, `update ingredients set low_stock_banner = true where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	rep := reportOut(t, out, ctx, ing, ana)
	if _, _, err := out.ReportNotFound(ctx, "servitoallas", uuid.New(), ana); err != nil {
		t.Fatal(err)
	}

	t.Run("cada quien su contador; «No lo encuentro» solo para quien configura", func(t *testing.T) {
		if n, _ := svc.Unread(ctx, marisol, managerPerms084); n != 2 {
			t.Fatalf("el gerente tiene %d sin leer", n)
		}
		if n, _ := svc.Unread(ctx, ana, cashierPerms084); n != 1 {
			t.Fatalf("la cajera tiene %d sin leer; «No lo encuentro» no es para ella", n)
		}
		if err := svc.MarkRead(ctx, marisol, []int64{rep.NotificationID}); err != nil {
			t.Fatal(err)
		}
		if n, _ := svc.Unread(ctx, marisol, managerPerms084); n != 1 {
			t.Fatalf("tras leer, el gerente tiene %d", n)
		}
		if n, _ := svc.Unread(ctx, ramiro, managerPerms084); n != 2 {
			t.Fatalf("leer uno bajó el contador de otra persona: %d", n)
		}
	})

	t.Run("la franja no sale a quien reportó y sale una vez a los demás", func(t *testing.T) {
		mine, err := svc.List(ctx, ana, cashierPerms084, true, 50)
		if err != nil {
			t.Fatal(err)
		}
		if mine[0].Banner {
			t.Fatal("la franja salió en la tableta de quien lo reportó")
		}
		theirs, err := svc.List(ctx, ramiro, managerPerms084, true, 50)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(theirs, func(n app.NotificationView) bool { return n.ID == rep.NotificationID })
		if i < 0 || !theirs[i].Banner {
			t.Fatal("la franja no salió a otra persona")
		}
		if err := svc.BannerSeen(ctx, ramiro, rep.NotificationID); err != nil {
			t.Fatal(err)
		}
		again, _ := svc.List(ctx, ramiro, managerPerms084, true, 50)
		if again[slices.IndexFunc(again, func(n app.NotificationView) bool { return n.ID == rep.NotificationID })].Banner {
			t.Fatal("la franja volvió a salir después de verla")
		}
	})

	t.Run("una lista vacía sale como [] en el JSON, nunca null", func(t *testing.T) {
		closed, err := svc.List(ctx, ramiro, managerPerms084, false, 50)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(closed)
		if !bytes.Equal(raw, []byte("[]")) {
			t.Fatalf("JSON crudo %s", raw)
		}
	})
}

func TestResolvingNotificationsByHand(t *testing.T) {
	st := newTestStore(t)
	ana := makeUser(t, st, "ana_084r", "cajero")
	marisol := makeUser(t, st, "marisol_084r", "gerente")
	svc, out, ctx := asNotifications(t, defaultCompanyID)

	t.Run("«ya llegó» cierra un «se acabó» con su motivo y quién", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Llegó 084")
		rep := reportOut(t, out, ctx, ing, ana)
		if err := svc.Resolve(ctx, marisol, managerPerms084, rep.NotificationID, app.ResolveCmd{Action: "arrived"}); err != nil {
			t.Fatal(err)
		}
		var status, reason string
		var by int64
		if err := st.Pool.QueryRow(ctx, `select status, resolved_reason, resolved_by from notifications where id = $1`, rep.NotificationID).Scan(&status, &reason, &by); err != nil {
			t.Fatal(err)
		}
		if status != "resolved" || reason != "arrived" || by != marisol {
			t.Fatalf("%s %s %d", status, reason, by)
		}
		if err := svc.Resolve(ctx, marisol, managerPerms084, rep.NotificationID, app.ResolveCmd{Action: "arrived"}); !errors.Is(err, domain.ErrNotificationClosed) {
			t.Fatalf("cerrar dos veces: %v", err)
		}
	})

	t.Run("«ya llegó» pide poder recibir mercancía", func(t *testing.T) {
		ing := pieceIngredient(t, st, "Sin permiso 084")
		rep := reportOut(t, out, ctx, ing, ana)
		if err := svc.Resolve(ctx, ana, cashierPerms084, rep.NotificationID, app.ResolveCmd{Action: "arrived"}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("la cajera cerró el aviso: %v", err)
		}
	})

	t.Run("«Es otro nombre de…» agrega el texto a los otros nombres con la regla de siempre", func(t *testing.T) {
		toalla := pieceIngredient(t, st, "Toalla de papel 084")
		nf, _, err := out.ReportNotFound(ctx, "servitoallas", uuid.New(), ana)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Resolve(ctx, marisol, managerPerms084, nf.NotificationID, app.ResolveCmd{Action: "alias", IngredientID: &toalla}); err != nil {
			t.Fatal(err)
		}
		found, err := out.Search(ctx, "servitoallas")
		if err != nil || len(found) != 1 || found[0].ID != toalla {
			t.Fatalf("la búsqueda no lo encontró por el otro nombre: %v %v", found, err)
		}
		// Un texto que ya es el nombre de otro insumo choca, como en la ficha.
		otro := pieceIngredient(t, st, "Servilletas 084r")
		nf2, _, _ := out.ReportNotFound(ctx, "servilletas 084r", uuid.New(), ana)
		var taken domain.AltNameTaken
		if err := svc.Resolve(ctx, marisol, managerPerms084, nf2.NotificationID, app.ResolveCmd{Action: "alias", IngredientID: &toalla}); !errors.As(err, &taken) || taken.OwnerID != otro {
			t.Fatalf("el otro nombre chocó con %v (%v)", taken, err)
		}
	})

	t.Run("dar de alta y descartar cierran «No lo encuentro»", func(t *testing.T) {
		nueva := pieceIngredient(t, st, "Hielo 084")
		nf, _, _ := out.ReportNotFound(ctx, "hielo", uuid.New(), ana)
		if err := svc.Resolve(ctx, marisol, managerPerms084, nf.NotificationID, app.ResolveCmd{Action: "created", IngredientID: &nueva}); err != nil {
			t.Fatal(err)
		}
		nf2, _, _ := out.ReportNotFound(ctx, "cosa rara", uuid.New(), ana)
		if err := svc.Resolve(ctx, marisol, managerPerms084, nf2.NotificationID, app.ResolveCmd{Action: "dismiss"}); err != nil {
			t.Fatal(err)
		}
		var statuses []string
		rows, _ := st.Pool.Query(ctx, `select status || ':' || resolved_reason from notifications where id = any($1) order by id`, []int64{nf.NotificationID, nf2.NotificationID})
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			statuses = append(statuses, s)
		}
		if !slices.Equal(statuses, []string{"resolved:created", "dismissed:dismissed"}) {
			t.Fatalf("%v", statuses)
		}
		var audits int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'notification.resolved'`).Scan(&audits); err != nil || audits < 2 {
			t.Fatalf("bitácora: %d %v", audits, err)
		}
	})

	t.Run("una acción desconocida es 400, no un default", func(t *testing.T) {
		nf, _, _ := out.ReportNotFound(ctx, "otra cosa", uuid.New(), ana)
		if err := svc.Resolve(ctx, marisol, managerPerms084, nf.NotificationID, app.ResolveCmd{Action: "borrar"}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("%v", err)
		}
	})
}

// Los avisos de una empresa no se ven ni se cierran desde otra.
func TestNotificationsAreIsolated(t *testing.T) {
	st := newTestStore(t)
	other := makeCompany(t, st, "otra-084n")
	ana := makeUser(t, st, "ana_084i", "cajero")
	_, out, ctx := asNotifications(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Aislado 084")
	rep := reportOut(t, out, ctx, ing, ana)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewNotificationsService(as, clock)
		if list, err := svc.List(ctx, ana, managerPerms084, true, 50); err == nil && len(list) > 0 {
			t.Fatalf("otra empresa ve %d avisos", len(list))
		}
		if err := svc.Resolve(ctx, ana, managerPerms084, rep.NotificationID, app.ResolveCmd{Action: "dismiss"}); err == nil {
			t.Fatal("otra empresa cerró el aviso ajeno")
		}
	})
}
