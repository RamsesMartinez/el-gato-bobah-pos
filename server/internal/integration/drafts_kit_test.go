//go:build integration

package integration

import (
	"context"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// draftsKit arma lo que comparten las pruebas de la cuenta en captura (spec 030): la base de la
// prueba, el rol de la aplicación con la empresa puesta (RLS de verdad) y los dos servicios.
//
// Los servicios corren bajo `gatobobah_app`, no como dueño: una cuenta en captura es una tabla de
// empresa, y lo que se prueba como dueño pasa en verde con el grant o la política borrados.
type draftsKit struct {
	st      *store.Store // dueño: solo para sembrar y mirar por debajo
	appSt   *store.Store
	ctx     context.Context
	orders  *app.OrdersService
	drafts  *app.DraftsService
	user    int64
	company int64
}

func newDraftsKit(t *testing.T) *draftsKit {
	t.Helper()
	st := newTestStore(t)
	k := &draftsKit{st: st, company: defaultCompanyID}
	k.user = makeUser(t, st, "cajero_cuentas", "cajero")
	k.appSt = appRoleStore(t)
	k.ctx = k.tenant(t, defaultCompanyID)
	k.orders = app.NewOrdersService(k.appSt, clock)
	k.drafts = app.NewDraftsService(k.appSt, k.orders)
	return k
}

// tenant devuelve un contexto con su propia conexión de la empresa. Una goroutine por contexto: la
// conexión que guarda no se comparte.
func (k *draftsKit) tenant(t *testing.T, company int64) context.Context {
	t.Helper()
	ctx, release, err := k.appSt.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatalf("AcquireTenant(%d): %v", company, err)
	}
	t.Cleanup(release)
	return ctx
}

func addOf(productID int64, qty string) app.DraftLineCmd {
	return app.DraftLineCmd{OpID: uuid.New(), ProductID: productID, Qty: decimal.RequireFromString(qty)}
}

// newDraft crea una cuenta con sus renglones y falla la prueba si no se pudo.
func (k *draftsKit) newDraft(t *testing.T, lines ...app.DraftLineCmd) *app.DraftView {
	t.Helper()
	v, created, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Lines: lines, Actor: k.user})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created {
		t.Fatal("Create con un id nuevo dijo que ya existía")
	}
	return v
}

func lineQty(t *testing.T, v *app.DraftView, productID int64) decimal.Decimal {
	t.Helper()
	total := decimal.Zero
	for _, l := range v.Lines {
		if l.ProductID == productID {
			total = total.Add(l.Qty)
		}
	}
	return total
}

// draftStatus mira la cuenta por debajo (como dueño), sin pasar por el servicio que se prueba.
func draftStatus(t *testing.T, st *store.Store, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := st.Pool.QueryRow(context.Background(), `select status from order_drafts where id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("estado de la cuenta %s: %v", id, err)
	}
	return s
}

var _ = domain.ErrNotFound

func (k *draftsKit) newDraftNamed(t *testing.T, name string, lines ...app.DraftLineCmd) *app.DraftView {
	t.Helper()
	v, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), FolioName: name, Lines: lines, Actor: k.user})
	if err != nil {
		t.Fatalf("Create(%s): %v", name, err)
	}
	return v
}
