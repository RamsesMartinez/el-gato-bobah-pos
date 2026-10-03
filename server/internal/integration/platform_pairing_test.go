//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL EMPAREJAMIENTO REDISEÑADO (spec 026), bajo el rol de la aplicación.

type pairingFixture struct {
	st     *store.Store
	svc    *app.MenusDePlataformaService
	ctx    context.Context
	appSt  *store.Store
	lector *lectorFalso
	conn   int64
	user   int64
	mango  int64 // producto «Chamoyada de Mango»
	hielo  int64 // opción «Con hielo»
}

func newPairingFixture(t *testing.T) *pairingFixture {
	t.Helper()
	st := newTestStore(t)
	f := &pairingFixture{st: st, lector: &lectorFalso{items: menuFalso()}}
	f.mango = makeProduct(t, st, "Chamoyada de Mango", decimal.RequireFromString("80.00"), false)
	var group int64
	if err := st.Pool.QueryRow(context.Background(),
		`insert into modifier_groups (name) values ('Hielo') returning id`).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(context.Background(),
		`insert into modifier_options (group_id, name, price_delta) values ($1, 'Con hielo', 0) returning id`, group).Scan(&f.hielo); err != nil {
		t.Fatal(err)
	}
	f.user = makeUser(t, st, "admin_emparejar", "admin")
	f.svc, f.ctx, f.appSt = servicioDePrueba(t, defaultCompanyID, f.lector)
	f.conn = conexionConLectura(t, f.svc, f.ctx, st, defaultCompanyID, f.lector, "tienda-emparejar")
	return f
}

func (f *pairingFixture) link(t *testing.T, ext string, kind domain.ClaseDeItem, local domain.ClaseLocal, id int64, capture *bool) error {
	t.Helper()
	return f.svc.GuardarPareja(f.ctx, app.AltaDePareja{
		ConexionID: f.conn, ExternalID: ext, Clase: kind, LocalID: id, ClaseLocal: local,
		UsuarioID: f.user, PrecioDeCaptura: capture,
	})
}

func groupOfItem(t *testing.T, v *app.PairingView, ext string) domain.PairingGroup {
	t.Helper()
	for _, it := range v.Items {
		if it.ExternalID == ext {
			return it.Group
		}
	}
	t.Fatalf("%q no está en la vista", ext)
	return ""
}

func TestBoardGroupsAndCountsComeFromTheSameRows(t *testing.T) {
	f := newPairingFixture(t)
	v, err := f.svc.Pairing(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if g := groupOfItem(t, v, "Chamoyada_de_Mango"); g != domain.GroupToReview {
		t.Fatalf("un nombre igual a un solo producto es propuesta: %s", g)
	}
	if g := groupOfItem(t, v, "Con_hielo"); g != domain.GroupToReview {
		t.Fatalf("las opciones también se proponen, contra opciones: %s", g)
	}
	porGrupo := map[domain.PairingGroup]int{}
	for _, it := range v.Items {
		porGrupo[it.Group]++
		if it.Price != "" && !strings.Contains(it.Price, ".") {
			t.Fatalf("precio sin dos decimales: %q", it.Price)
		}
	}
	if v.Counts.ToReview != porGrupo[domain.GroupToReview] || v.Counts.Unpaired != porGrupo[domain.GroupUnpaired] ||
		v.Counts.Done+v.Counts.ToReview+v.Counts.Unpaired+v.Counts.Excluded != len(v.Items) {
		t.Fatalf("los conteos no salen de la lista: %+v contra %v", v.Counts, porGrupo)
	}
	crudo, _ := json.Marshal(v)
	if strings.Contains(string(crudo), `"priceChanges":null`) || strings.Contains(string(crudo), `"items":null`) {
		t.Fatalf("un arreglo vacío debe salir como [], no null: %s", crudo)
	}
}

func TestBatchConfirmsOnlyCurrentProposals(t *testing.T) {
	f := newPairingFixture(t)
	res, err := f.svc.ConfirmBatch(f.ctx, f.conn, f.user, []string{"Chamoyada_de_Mango", "Con_hielo", "Dedos_de_queso"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Confirmed) != 2 || len(res.Skipped) != 1 || res.Skipped[0] != "Dedos_de_queso" {
		t.Fatalf("se confirman las dos propuestas y se omite lo que no tenía: %+v", res)
	}
	again, err := f.svc.ConfirmBatch(f.ctx, f.conn, f.user, []string{"Chamoyada_de_Mango"})
	if err != nil || len(again.Confirmed) != 0 {
		t.Fatalf("confirmar dos veces no duplica ni truena: %+v %v", again, err)
	}
	v, _ := f.svc.Pairing(f.ctx, f.conn)
	for _, it := range v.Items {
		if it.ExternalID == "Con_hielo" && (it.Link == nil || it.Link.LocalKind != string(domain.LocalOpcion) || it.Link.LocalID != f.hielo) {
			t.Fatalf("la opción quedó ligada a una opción del POS, no a un producto: %+v", it.Link)
		}
	}
}

func TestSeveralPlatformItemsToOneProductRequireTheCapturePrice(t *testing.T) {
	f := newPairingFixture(t)
	if err := f.link(t, "Chamoyada_de_Mango", domain.ItemPlatillo, domain.LocalProducto, f.mango, nil); err != nil {
		t.Fatal(err)
	}
	err := f.link(t, "Chamoyada_de_Fresa", domain.ItemPlatillo, domain.LocalProducto, f.mango, nil)
	if !errors.Is(err, domain.ErrCapturePriceRequired) {
		t.Fatalf("el segundo platillo al mismo producto exige elegir el precio de captura: %v", err)
	}
	yes := true
	if err := f.link(t, "Chamoyada_de_Fresa", domain.ItemPlatillo, domain.LocalProducto, f.mango, &yes); err != nil {
		t.Fatal(err)
	}
	capturas := map[string]bool{}
	v, _ := f.svc.Pairing(f.ctx, f.conn)
	for _, it := range v.Items {
		if it.Link != nil {
			capturas[it.ExternalID] = it.Link.CapturePrice
		}
	}
	if !capturas["Chamoyada_de_Fresa"] || capturas["Chamoyada_de_Mango"] {
		t.Fatalf("el precio de captura es uno solo y pasó a la elegida: %v", capturas)
	}

	// Quitar la que lo daba lo pasa a la que queda, sin preguntar.
	if err := f.svc.BorrarPareja(f.ctx, f.conn, "Chamoyada_de_Fresa"); err != nil {
		t.Fatal(err)
	}
	v, _ = f.svc.Pairing(f.ctx, f.conn)
	for _, it := range v.Items {
		if it.ExternalID == "Chamoyada_de_Mango" && (it.Link == nil || !it.Link.CapturePrice) {
			t.Fatalf("al quitar la pareja que daba el precio de captura, debe pasar a la que queda")
		}
	}
}

func TestLinkKindsCannotCross(t *testing.T) {
	f := newPairingFixture(t)
	if err := f.link(t, "Con_hielo", domain.ItemOpcion, domain.LocalProducto, f.mango, nil); !errors.Is(err, domain.ErrLinkKindMismatch) {
		t.Fatalf("una opción de la plataforma ligada a un producto se rechaza: %v", err)
	}
	otra := makeCompany(t, f.st, "otra-opcion")
	ajena := optionID(t, f.st, otra)
	if err := f.link(t, "Con_hielo", domain.ItemOpcion, domain.LocalOpcion, ajena, nil); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("una opción de otra empresa se rechaza por la llave compuesta: %v", err)
	}
}

func TestExclusionsPersistAndPairingClearsThem(t *testing.T) {
	f := newPairingFixture(t)
	if err := f.svc.ExcludeItem(f.ctx, f.conn, f.user, "Dedos_de_queso"); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.Pairing(f.ctx, f.conn)
	if g := groupOfItem(t, v, "Dedos_de_queso"); g != domain.GroupExcluded {
		t.Fatalf("«solo existe en la plataforma» lo saca de Sin pareja: %s", g)
	}
	if n, err := f.svc.ParejasQueSePierden(f.ctx, f.conn); err != nil || n != 1 {
		t.Fatalf("el aviso antes de borrar la tienda cuenta las decisiones: %d %v", n, err)
	}
	queso := makeProduct(t, f.st, "Dedos de queso", decimal.RequireFromString("60"), false)
	if err := f.link(t, "Dedos_de_queso", domain.ItemPlatillo, domain.LocalProducto, queso, nil); err != nil {
		t.Fatal(err)
	}
	v, _ = f.svc.Pairing(f.ctx, f.conn)
	if g := groupOfItem(t, v, "Dedos_de_queso"); g != domain.GroupDone {
		t.Fatalf("emparejar gana y borra la decisión: %s", g)
	}
	if n, _ := f.svc.ParejasQueSePierden(f.ctx, f.conn); n != 1 {
		t.Fatalf("la decisión se borró al emparejar; queda solo la pareja: %d", n)
	}
}

func TestAGoodReadCopiesThePlatformPriceAndLocksIt(t *testing.T) {
	f := newPairingFixture(t)
	yes := true
	if err := f.link(t, "Chamoyada_de_Mango", domain.ItemPlatillo, domain.LocalProducto, f.mango, &yes); err != nil {
		t.Fatal(err)
	}
	plat := plataformaUber(t, f.st, defaultCompanyID)
	prices := app.NewPlatformPricesService(f.appSt)
	if err := prices.SetProductPrice(f.ctx, f.mango, plat, decimal.RequireFromString("80.00"), f.user); err != nil {
		t.Fatalf("antes de la lectura el precio todavía se captura a mano: %v", err)
	}

	// Sin aviso, el menú cacheado dura 24 horas y las tabletas cobrarían con el precio viejo.
	avisos := make(chan int64, 1)
	f.svc.OnPricesSynced(func(_ context.Context, company int64) { avisos <- company })
	if _, _, err := f.svc.DispararLecturaPor(f.ctx, defaultCompanyID, f.conn, f.user); err != nil {
		t.Fatal(err)
	}
	esperarLectura(t, f.svc, f.ctx, f.conn, f.lector)
	esperarPrecio(t, f.st, f.mango, plat, "99.00")
	select {
	case c := <-avisos:
		if c != defaultCompanyID {
			t.Fatalf("el aviso debe ser para la empresa de la tienda, fue %d", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("copiar precios no avisó al POS: el menú cacheado seguiría con el precio viejo")
	}

	var source string
	if err := f.st.Pool.QueryRow(context.Background(),
		`select source from product_platform_prices where product_id = $1 and platform_id = $2`, f.mango, plat).Scan(&source); err != nil || source != "platform" {
		t.Fatalf("el precio copiado queda como de la plataforma: %q %v", source, err)
	}
	v, _ := f.svc.Pairing(f.ctx, f.conn)
	if len(v.Changes) != 1 || v.Changes[0].New != "99.00" || v.Changes[0].Old == nil || *v.Changes[0].Old != "80.00" {
		t.Fatalf("el aviso dice qué cambió: %+v", v.Changes)
	}
	if err := prices.SetProductPrice(f.ctx, f.mango, plat, decimal.RequireFromString("70.00"), f.user); !errors.Is(err, domain.ErrPlatformPriceManaged) {
		t.Fatalf("un precio que pone la plataforma no se captura a mano, ni con la pantalla vieja: %v", err)
	}
	if _, err := prices.DeleteProductPrice(f.ctx, f.mango, plat); !errors.Is(err, domain.ErrPlatformPriceManaged) {
		t.Fatalf("tampoco se borra a mano: %v", err)
	}
	// Y el menú del POS lo marca, para que el diálogo de precio lo muestre bloqueado.
	doc, err := app.NewMenuService(f.appSt, clock).Build(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.PlatformSynced[plat][f.mango]; !ok {
		t.Fatalf("el menú debe decir que ese precio lo pone la plataforma: %v", doc.PlatformSynced)
	}
}

func TestTwoStoresOfTheSamePlatformDoNotCopyPrices(t *testing.T) {
	f := newPairingFixture(t)
	yes := true
	if err := f.link(t, "Chamoyada_de_Mango", domain.ItemPlatillo, domain.LocalProducto, f.mango, &yes); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CrearConexion(f.ctx, app.AltaDeConexion{
		PlatformID: plataformaUber(t, f.st, defaultCompanyID), ExternalStoreID: "segunda-tienda", Label: "Segunda",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.DispararLecturaPor(f.ctx, defaultCompanyID, f.conn, f.user); err != nil {
		t.Fatal(err)
	}
	esperarLectura(t, f.svc, f.ctx, f.conn, f.lector)
	var n int
	if err := f.st.Pool.QueryRow(context.Background(), `select count(*) from product_platform_prices where product_id = $1`, f.mango).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("con dos tiendas de la misma plataforma, una pisaría el precio de la otra: no se copia nada")
	}
}

func esperarPrecio(t *testing.T, st *store.Store, product int64, plat int16, want string) {
	t.Helper()
	for range 100 {
		var got string
		err := st.Pool.QueryRow(context.Background(),
			`select price::text from product_platform_prices where product_id = $1 and platform_id = $2`, product, plat).Scan(&got)
		if err == nil && got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("el precio no llegó a %s", want)
}

func TestPairingIsIsolatedInTheThreeCases(t *testing.T) {
	f := newPairingFixture(t)
	if err := f.svc.ExcludeItem(f.ctx, f.conn, f.user, "Dedos_de_queso"); err != nil {
		t.Fatal(err)
	}
	otra := makeCompany(t, f.st, "otra-emparejar")
	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewMenusDePlataformaService(st, fixedClients{}, nil)
		if v, err := svc.Pairing(ctx, f.conn); err == nil && len(v.Items) > 0 {
			t.Fatalf("otra sesión vio el emparejamiento de la tienda ajena")
		}
		if c, err := svc.Candidates(ctx, f.conn, "Chamoyada_de_Mango", ""); err == nil && len(c) > 0 {
			t.Fatalf("otra sesión vio candidatos de la tienda ajena")
		}
		if n, err := svc.ParejasQueSePierden(ctx, f.conn); err == nil && n > 0 {
			t.Fatalf("otra sesión contó las decisiones de la tienda ajena: %d", n)
		}
	})
}
