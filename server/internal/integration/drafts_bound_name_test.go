//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Con el turno pasado del largo de la lista, la cuenta nace numerada («Cymric 2», D-2). Un pedido
// creado por OTRO camino (directo, pasar productos) numeraba su sorteo solo contra lo cantado en el
// turno, no contra las cuentas vivas: salía «Cymric 2» con la cuenta «Cymric 2» todavía en captura,
// y al mandarla, la cuenta cambiaba a «Cymric 3» — el nombre que el cliente oyó ya no era el suyo.
func TestADirectOrderNeverTakesTheNumberedNameOfALiveDraft(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	sess := abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café del turno largo", pesos("30"), false)
	for i, n := range domain.NombresDelEsquema(domain.EsquemaPorDefecto) {
		if _, err := k.st.Pool.Exec(context.Background(), `insert into orders (company_id, client_uuid, business_date,
			daily_number, service_type, subtotal, total, opened_by, register_session_id, folio_name)
			values ($1, $2, current_date, $3, 'mostrador', 0, 0, $4, $5, $6)`,
			k.company, uuid.New(), 5000+i, k.user, sess, n); err != nil {
			t.Fatalf("sembrar %s: %v", n, err)
		}
	}
	v := k.newDraftNamed(t, "Cymric", addOf(cafe, "1"))
	if *v.FolioName != "Cymric 2" {
		t.Fatalf("la cuenta nació %q, quería «Cymric 2»", *v.FolioName)
	}
	// El sorteo es al azar: se crean pedidos hasta que uno caiga en el animal de la cuenta.
	for i := 0; i < 400; i++ {
		o, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
			Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}})
		if err != nil {
			t.Fatalf("pedido directo %d: %v", i, err)
		}
		if o.FolioName == *v.FolioName {
			t.Fatalf("un pedido directo salió como %q, el nombre de una cuenta viva", o.FolioName)
		}
		if strings.HasPrefix(o.FolioName, "Cymric") {
			break
		}
	}
	res, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Order.FolioName != *v.FolioName {
		t.Fatalf("la cuenta se capturó como %q y el pedido salió como %q", *v.FolioName, res.Order.FolioName)
	}
}
