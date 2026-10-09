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

// NINGÚN CAMINO QUE REPARTE NOMBRE SE LLEVA EL DE UNA CUENTA VIVA (research R-3).
//
// El escenario que lo vuelve determinista: la cuenta viva se llama «Persa», la bolsa está vacía (la
// vuelta se agotó) y todo nombre salvo «Persa» ya se cantó en el turno. Sin excluir los vivos, el
// único nombre fresco es «Persa» y se lo llevaría el pedido: dos «Persa» en la barra, uno de ellos
// ya dicho a un cliente.
func TestNoPathTakesALiveDraftName(t *testing.T) {
	k := newDraftsKit(t)
	ctx := k.ctx
	session := abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café de los nombres", pesos("30"), false)
	live := k.newDraftNamed(t, "Persa", addOf(cafe, "1"))
	if *live.FolioName != "Persa" {
		t.Fatalf("la cuenta salió %s", *live.FolioName)
	}
	// Bolsa agotada y todo lo demás cantado en el turno.
	if _, err := k.st.Pool.Exec(context.Background(), `
		insert into folio_consumido (scheme, name) select 'razas', n from unnest($1::text[]) n on conflict do nothing`,
		domain.NombresDelEsquema(domain.EsquemaRazas)); err != nil {
		t.Fatal(err)
	}
	var otros []string
	for _, n := range domain.NombresDelEsquema(domain.EsquemaRazas) {
		if n != "Persa" {
			otros = append(otros, n)
		}
	}
	if _, err := k.st.Pool.Exec(context.Background(), `
		insert into orders (client_uuid, business_date, daily_number, service_type, opened_by, subtotal, total, status,
		                    register_session_id, folio_name)
		select gen_random_uuid(), $3, 1000 + ord, 'mostrador', $2, 0, 0, 'entregada', $1, n
		from unnest($4::text[]) with ordinality as t(n, ord)`, session, k.user, fixedNow, otros); err != nil {
		t.Fatal(err)
	}

	base := func(name string) string {
		// «Persa 2» es Persa con número de vuelta: para esta prueba cuenta como el mismo animal.
		if i := strings.LastIndexByte(name, ' '); i > 0 && strings.Trim(name[i+1:], "0123456789") == "" {
			return name[:i]
		}
		return name
	}

	t.Run("la pantalla no lo ofrece", func(t *testing.T) {
		names, err := k.orders.NombresDisponibles(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if contieneNombre(names, "Persa") {
			t.Fatal("GET /pos/folio-names ofrece «Persa», que es de una cuenta viva")
		}
	})

	t.Run("un pedido creado directo no se lo lleva (vaciar la bolsa con cuentas vivas)", func(t *testing.T) {
		o, err := k.orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
			Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}})
		if err != nil {
			t.Fatal(err)
		}
		if base(o.FolioName) == "Persa" {
			t.Fatalf("el pedido salió %q: se llevó el nombre de la cuenta viva", o.FolioName)
		}
	})

	t.Run("tampoco proponiéndolo", func(t *testing.T) {
		o, err := k.orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
			FolioName: "Persa", Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}})
		if err != nil {
			t.Fatal(err)
		}
		if base(o.FolioName) == "Persa" {
			t.Fatalf("el pedido salió %q", o.FolioName)
		}
	})

	t.Run("pasar productos a un pedido nuevo no se lo lleva", func(t *testing.T) {
		from, err := k.orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
			Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("2")}}})
		if err != nil {
			t.Fatal(err)
		}
		res, err := k.orders.MoveLines(ctx, app.MoveLinesCmd{ClientUUID: uuid.New(), FromOrderID: from.ID, ActorID: k.user,
			Lines: []domain.SelectedPieces{{LineID: from.Lines[0].ID, Qty: pesos("1")}}})
		if err != nil {
			t.Fatal(err)
		}
		if base(res.To.FolioName) == "Persa" {
			t.Fatalf("el pedido nuevo de «Pasar» salió %q", res.To.FolioName)
		}
	})
}

// EL NOMBRE AMARRADO VIAJA AL PEDIDO (D-2): ni otro animal ni otro número salvo que el turno ya lo
// haya cantado, y entonces «Persa 2» (research R-3).
func TestBoundFolioNameReachesTheOrder(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café amarrado", pesos("30"), false)
	create := func(t *testing.T, bound string) string {
		t.Helper()
		o, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
			BoundFolioName: bound, Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: pesos("1")}}})
		if err != nil {
			t.Fatal(err)
		}
		return o.FolioName
	}
	// «Persa» ya está fuera de la bolsa: lo sacó la cuenta al nacer. Aun así es suyo.
	if _, err := k.st.Pool.Exec(context.Background(), `insert into folio_consumido (scheme, name) values ('razas', 'Persa')`); err != nil {
		t.Fatal(err)
	}
	if got := create(t, "Persa"); got != "Persa" {
		t.Fatalf("el pedido salió %q: el nombre amarrado se respeta aunque ya esté fuera de la bolsa", got)
	}
	if got := create(t, "Persa"); got != "Persa 2" {
		t.Fatalf("con «Persa» ya cantado en el turno salió %q, quería «Persa 2» (nunca otro animal)", got)
	}
}

// EL NOMBRE QUE SE CANTA AL NACER ES EL QUE LLEVA EL PEDIDO, TAMBIÉN CUANDO EL DÍA YA DIO LA VUELTA
// A LA LISTA (D-2).
//
// Lo encontró el e2e del ambiente de pruebas (caso 23): pasado el largo de la lista en un turno,
// la bolsa vuelve a ofrecer nombres ya cantados hoy. La cuenta nacía «Pixie-bob» —eso decían su
// ficha, su ticket y la cuenta impresa— y al mandarla a cocina el pedido salía «Pixie-bob 2». Al
// cliente se le dijo un nombre y la comanda canta otro. En un local con más pedidos por turno que
// nombres en la lista, pasa todos los días.
func TestADraftBornAfterTheListRanOutKeepsItsNameInTheOrder(t *testing.T) {
	k := newDraftsKit(t)
	session := abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café de la vuelta", pesos("30"), false)
	lista := domain.NombresDelEsquema(domain.EsquemaRazas)
	// Todos los nombres de la lista ya se cantaron en el turno, y la bolsa está vacía.
	if _, err := k.st.Pool.Exec(context.Background(), `
		insert into folio_consumido (scheme, name) select 'razas', n from unnest($1::text[]) n on conflict do nothing`,
		lista); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(context.Background(), `
		insert into orders (client_uuid, business_date, daily_number, service_type, opened_by, subtotal, total, status,
		                    register_session_id, folio_name)
		select gen_random_uuid(), $3, 1000 + ord, 'mostrador', $2, 0, 0, 'entregada', $1, n
		from unnest($4::text[]) with ordinality as t(n, ord)`, session, k.user, fixedNow, lista); err != nil {
		t.Fatal(err)
	}

	for _, propuesto := range []string{"", "Persa"} {
		d := k.newDraftNamed(t, propuesto, addOf(cafe, "1"))
		nacio := *d.FolioName
		if contieneNombre(lista, nacio) {
			t.Fatalf("la cuenta nació %q, un nombre ya cantado hoy: al mandarla cambiaría de nombre", nacio)
		}
		res, err := k.drafts.Send(k.ctx, d.ID, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if res.Order.FolioName != nacio {
			t.Fatalf("la cuenta nació %q y el pedido salió %q: la comanda canta un nombre que el cliente no oyó",
				nacio, res.Order.FolioName)
		}
	}
}

// DESCARTAR UNA CUENTA CON NÚMERO DE VUELTA DEVUELVE SU ANIMAL A LA BOLSA (D-7).
//
// La bolsa guarda animales, no «Persa 2». Soltar el nombre con número no soltaría nada.
func TestDiscardingANumberedDraftReleasesItsAnimal(t *testing.T) {
	k := newDraftsKit(t)
	session := abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café que se descarta", pesos("30"), false)
	lista := domain.NombresDelEsquema(domain.EsquemaRazas)
	if _, err := k.st.Pool.Exec(context.Background(), `
		insert into orders (client_uuid, business_date, daily_number, service_type, opened_by, subtotal, total, status,
		                    register_session_id, folio_name)
		select gen_random_uuid(), $3, 1000 + ord, 'mostrador', $2, 0, 0, 'entregada', $1, n
		from unnest($4::text[]) with ordinality as t(n, ord)`, session, k.user, fixedNow, lista); err != nil {
		t.Fatal(err)
	}
	d := k.newDraftNamed(t, "Persa", addOf(cafe, "1"))
	if *d.FolioName != "Persa 2" {
		t.Fatalf("la cuenta nació %q, quería «Persa 2»", *d.FolioName)
	}
	if err := k.drafts.Discard(k.ctx, d.ID, k.user); err != nil {
		t.Fatal(err)
	}
	var queda int
	if err := k.st.Pool.QueryRow(context.Background(),
		`select count(*) from folio_consumido where scheme = 'razas' and name = 'Persa'`).Scan(&queda); err != nil {
		t.Fatal(err)
	}
	if queda != 0 {
		t.Fatal("descartar «Persa 2» dejó a «Persa» fuera de la bolsa")
	}
}
