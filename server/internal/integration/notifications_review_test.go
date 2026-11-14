//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Lo que encontró la revisión de código de la 084 en avisos, destinatarios, lista de compra y
// configuración. Cada prueba nombra el fallo que atrapa.

// Las lecturas y escrituras por persona, el deshacer, el cierre con alias y la lista de compra no
// cruzan de empresa en ninguno de los tres casos donde RLS ya falló (constitución IV).
func TestNotificationPathsAreIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "otra-084r")
	ana := makeUser(t, st, "ana_084r", "cajero")
	gerente := makeUser(t, st, "gerente_084r", "gerente")
	setEmail(t, st, gerente, "gerente084r@ejemplo.mx")
	_, out, ctx := asNotifications(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Aislado revisión 084")
	alertOn(t, st, ing, "5", true, true)
	adjust(t, st, "ingrediente", ing, "9")
	rep := reportOut(t, out, ctx, ing, ana)
	nf, _, err := out.ReportNotFound(ctx, "popote de papel", newKey(), ana)
	if err != nil {
		t.Fatal(err)
	}
	reads := func() int {
		var n int
		if err := st.Pool.QueryRow(context.Background(), `select count(*) from notification_reads where notification_id in ($1, $2)`, rep.NotificationID, nf.NotificationID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewNotificationsService(as, clock)
		so := app.NewStockOutService(as, clock)
		before := reads()
		_ = svc.MarkRead(ctx, gerente, []int64{rep.NotificationID, nf.NotificationID})
		_ = svc.BannerSeen(ctx, gerente, rep.NotificationID)
		if reads() != before {
			t.Fatal("otra empresa marcó como leído un aviso ajeno")
		}
		if err := so.UndoOut(ctx, rep.ID, ana, true); err == nil {
			t.Fatal("otra empresa deshizo el reporte ajeno")
		}
		if err := svc.Resolve(ctx, gerente, managerPerms084, nf.NotificationID, app.ResolveCmd{Action: "alias", IngredientID: &ing}); err == nil {
			t.Fatal("otra empresa ligó un nombre a un insumo ajeno")
		}
		if lines, err := svc.ShoppingList(ctx); err == nil {
			for _, l := range lines {
				if l.ID == ing {
					t.Fatal("otra empresa ve la lista de compra ajena")
				}
			}
		}
		if v, err := svc.Recipients(ctx); err == nil {
			for _, p := range v.People {
				if p.UserID == gerente {
					t.Fatal("otra empresa ve a quién le llegan los avisos ajenos")
				}
			}
		}
	})
	if got := ingredientOnHand(t, st, ing); !got.IsZero() {
		t.Fatalf("la existencia ajena cambió a %s", got)
	}
}

// Quien solo reporta no cierra avisos: descartar, dar de alta y ligar como otro nombre cuelgan de
// configurar el almacén. Si alguien mueve esa barrera, un cajero descarta lo que nadie compró.
func TestOnlyWhoConfiguresClosesNotFound(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ana := makeUser(t, st, "ana_084rc", "cajero")
	svc, out, ctx := asNotifications(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Permisos 084rc")
	for _, cmd := range []app.ResolveCmd{{Action: "dismiss"}, {Action: "created", IngredientID: &ing}, {Action: "alias", IngredientID: &ing}} {
		nf, _, err := out.ReportNotFound(ctx, "cosa "+cmd.Action, newKey(), ana)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Resolve(ctx, ana, cashierPerms084, nf.NotificationID, cmd); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("un cajero hizo %q: %v", cmd.Action, err)
		}
	}
}

// La misma persona dos veces en la lista de destinatarios era un 500 (o filas repetidas).
func TestRecipientsRejectTheSamePersonTwice(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_084rr", "admin")
	svc, _, ctx := asNotifications(t, defaultCompanyID)
	v, err := svc.Recipients(ctx)
	if err != nil || len(v.People) == 0 {
		t.Fatalf("%+v %v", v, err)
	}
	v.People = append(v.People, v.People[0])
	if err := svc.SetRecipients(ctx, v, admin); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("la misma persona dos veces: %v", err)
	}
}

// Un producto también trae su versión: sin compararla, la última escritura ganaba sin aviso y el
// mínimo que otra tableta acababa de capturar se perdía.
func TestAStaleProductConfigIsRejected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	gerente := makeUser(t, st, "gerente_084rs", "gerente")
	svc, _, ctx := asNotifications(t, defaultCompanyID)
	p := makeProduct(t, st, "Agua 084rs", qty("20"), true)
	in := lowStockIn(true, "5")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	in.ExpectedUpdatedAt = &old
	if _, err := svc.ConfigureLowStock(ctx, "producto", p, in, gerente, false); !errors.Is(err, domain.ErrArticleStale) {
		t.Fatalf("una versión vieja del producto: %v", err)
	}
}

// En el lote, un renglón que no trae el mínimo lo deja como está: la tabla mandaba el que leyó al
// abrir y regresaba el que otra persona capturó en la ficha mientras tanto.
func TestABatchRowWithoutMinimumKeepsIt(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	gerente := makeUser(t, st, "gerente_084rb", "gerente")
	svc, _, ctx := asNotifications(t, defaultCompanyID)
	ing := pieceIngredient(t, st, "Lote conserva 084rb")
	adjust(t, st, "ingrediente", ing, "50")
	if _, err := svc.ConfigureLowStock(ctx, "ingrediente", ing, lowStockIn(false, "8"), gerente, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfigureBatch(ctx, []app.LowStockBatchItem{{ItemType: "ingrediente", ID: ing, Enabled: true, KeepMin: true}}, gerente, false); err != nil {
		t.Fatal(err)
	}
	var min *decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select min_stock from ingredients where id = $1`, ing).Scan(&min); err != nil || min == nil || !min.Equal(qty("8")) {
		t.Fatalf("el lote borró el mínimo: %v %v", min, err)
	}
}

func newKey() uuid.UUID { return uuid.New() }
