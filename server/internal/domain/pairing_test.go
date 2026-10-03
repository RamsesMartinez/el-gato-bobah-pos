package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestRankCandidatesPutsTheMostSimilarFirst(t *testing.T) {
	cands := []ProductoLocal{
		{ID: 1, Nombre: "Frappé Taro"},
		{ID: 2, Nombre: "Crepa Nutella"},
		{ID: 3, Nombre: "Arma tu Crepa"},
		{ID: 4, Nombre: "Crepa Nutella con Fresa 🍓"},
	}
	got := RankCandidates("Crepa de Nutella con Fresa", cands)
	if len(got) != 4 {
		t.Fatalf("ordenar no filtra: quería los 4, hay %d", len(got))
	}
	if got[0].ID != 4 || got[1].ID != 2 {
		t.Fatalf("el más parecido va primero: quería 4 y luego 2, quedó %d, %d", got[0].ID, got[1].ID)
	}
	if got[3].ID != 1 {
		t.Fatalf("lo que no comparte ninguna palabra va al final, quedó en último %d", got[3].ID)
	}
}

func TestRankCandidatesKeepsTiesInCatalogOrder(t *testing.T) {
	cands := []ProductoLocal{{ID: 7, Nombre: "Agua"}, {ID: 3, Nombre: "Soda"}, {ID: 5, Nombre: "Jugo"}}
	got := RankCandidates("Malteada", cands)
	if got[0].ID != 7 || got[1].ID != 3 || got[2].ID != 5 {
		t.Fatalf("un empate no debe reordenar al azar: %v", got)
	}
}

func TestGroupOf(t *testing.T) {
	cases := []struct {
		name                          string
		confirmed, proposal, excluded bool
		want                          PairingGroup
	}{
		{"pareja confirmada", true, false, false, GroupDone},
		{"confirmada gana a la decisión", true, false, true, GroupDone},
		{"propuesta", false, true, false, GroupToReview},
		{"sin nada", false, false, false, GroupUnpaired},
		{"solo existe en la plataforma", false, false, true, GroupExcluded},
		{"decisión gana a la propuesta", false, true, true, GroupExcluded},
	}
	for _, c := range cases {
		if got := GroupOf(c.confirmed, c.proposal, c.excluded); got != c.want {
			t.Errorf("%s: quería %s, fue %s", c.name, c.want, got)
		}
	}
}

func TestCountGroupsUsesTheSamePredicateAsTheList(t *testing.T) {
	groups := []PairingGroup{GroupDone, GroupDone, GroupToReview, GroupUnpaired, GroupExcluded}
	c := CountGroups(groups)
	if c.Done != 2 || c.ToReview != 1 || c.Unpaired != 1 || c.Excluded != 1 {
		t.Fatalf("conteos: %+v", c)
	}
	if c.Done+c.ToReview+c.Unpaired+c.Excluded != len(groups) {
		t.Fatalf("los conteos deben sumar la lista completa")
	}
}

func price(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestPriceSync(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	items := []ItemDePlataforma{
		{ID: "chai", Clase: ItemPlatillo, Nombre: "Chai Latte", Centavos: 8505, Activo: true},
		{ID: "crepa-a", Clase: ItemPlatillo, Nombre: "Crepa Cajeta", Centavos: 13900, Activo: true},
		{ID: "crepa-b", Clase: ItemPlatillo, Nombre: "Crepa Nutella", Centavos: 14900, Activo: true},
		{ID: "gratis", Clase: ItemPlatillo, Nombre: "Galleta de regalo", Centavos: 0, Activo: true},
		{ID: "leche", Clase: ItemOpcion, Nombre: "Leche deslactosada", Centavos: 1500, Activo: true},
		{ID: "igual", Clase: ItemPlatillo, Nombre: "Té verde", Centavos: 6000, Activo: true},
	}
	links := []PairingLink{
		{ExternalID: "chai", Target: PriceTarget{Kind: LocalProducto, ID: 41}, Confirmed: true, CapturePrice: true},
		{ExternalID: "crepa-a", Target: PriceTarget{Kind: LocalProducto, ID: 50}, Confirmed: true, CapturePrice: true},
		{ExternalID: "crepa-b", Target: PriceTarget{Kind: LocalProducto, ID: 50}, Confirmed: true},
		{ExternalID: "gratis", Target: PriceTarget{Kind: LocalProducto, ID: 60}, Confirmed: true, CapturePrice: true},
		{ExternalID: "leche", Target: PriceTarget{Kind: LocalOpcion, ID: 9}, Confirmed: true, CapturePrice: true},
		{ExternalID: "igual", Target: PriceTarget{Kind: LocalProducto, ID: 70}, Confirmed: true, CapturePrice: true},
		{ExternalID: "desaparecido", Target: PriceTarget{Kind: LocalProducto, ID: 80}, Confirmed: true, CapturePrice: true},
		{ExternalID: "propuesta", Target: PriceTarget{Kind: LocalProducto, ID: 90}, CapturePrice: true},
	}
	current := map[PriceTarget]CurrentPrice{
		{Kind: LocalProducto, ID: 41}: {Price: price("80.00"), FromPlatform: false},
		{Kind: LocalProducto, ID: 70}: {Price: price("60.00"), FromPlatform: true},
	}

	writes, err := PriceSync(items, links, current, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	byTarget := map[PriceTarget]PriceWrite{}
	for _, w := range writes {
		byTarget[w.Target] = w
	}

	if w := byTarget[PriceTarget{Kind: LocalProducto, ID: 41}]; !w.Price.Equal(price("85.05")) || !w.Changed || w.Old == nil {
		t.Fatalf("el chai debe quedar en 85.05 y contarse como cambio desde 80: %+v", w)
	}
	if w := byTarget[PriceTarget{Kind: LocalProducto, ID: 50}]; !w.Price.Equal(price("139.00")) || w.ExternalID != "crepa-a" {
		t.Fatalf("con dos platillos al mismo producto, manda el de captura (crepa-a, 139): %+v", w)
	}
	if _, ok := byTarget[PriceTarget{Kind: LocalProducto, ID: 60}]; ok {
		t.Fatalf("un precio de cero no se escribe: la columna exige mayor que cero")
	}
	if w := byTarget[PriceTarget{Kind: LocalOpcion, ID: 9}]; !w.Price.Equal(price("15.00")) || w.Old != nil || !w.Changed {
		t.Fatalf("la opción sin precio previo se escribe y es cambio: %+v", w)
	}
	if w, ok := byTarget[PriceTarget{Kind: LocalProducto, ID: 70}]; ok && w.Changed {
		t.Fatalf("un precio que ya puso la plataforma y no cambió no es cambio: %+v", w)
	}
	if _, ok := byTarget[PriceTarget{Kind: LocalProducto, ID: 80}]; ok {
		t.Fatalf("un platillo que ya no está en la lectura no toca su precio")
	}
	if _, ok := byTarget[PriceTarget{Kind: LocalProducto, ID: 90}]; ok {
		t.Fatalf("una propuesta sin confirmar no sincroniza precio")
	}

	if _, err := PriceSync(items, links, current, 2, now); !errors.Is(err, ErrSeveralStoresSamePlatform) {
		t.Fatalf("con dos tiendas de la misma plataforma no se sincroniza: el precio es por plataforma y una pisaría a la otra; fue %v", err)
	}
}

func TestCapturePriceAfterUnlinkGoesToTheMostRecent(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	remaining := []PairingLink{
		{ExternalID: "a", CreatedAt: t0},
		{ExternalID: "c", CreatedAt: t0.Add(2 * time.Hour)},
		{ExternalID: "b", CreatedAt: t0.Add(time.Hour)},
	}
	if got, ok := CapturePriceAfterUnlink(remaining); !ok || got != "c" {
		t.Fatalf("la captura pasa a la más reciente sin preguntar: quería c, fue %q", got)
	}
	if _, ok := CapturePriceAfterUnlink(nil); ok {
		t.Fatalf("sin parejas restantes no hay a quién pasarla")
	}
}

func TestPlatformLineNotesListsWhatThePlatformSent(t *testing.T) {
	got := PlatformLineNotes([]PlatformLineOption{
		{Name: "Leche deslactosada", Quantity: price("1"), UnitPrice: price("15")},
		{Name: "Extra perla", Quantity: price("2"), UnitPrice: price("10.5")},
		{Name: "Sin hielo", Quantity: price("1"), UnitPrice: price("0")},
	})
	want := "Leche deslactosada (+$15.00) · 2× Extra perla (+$10.50) · Sin hielo"
	if got != want {
		t.Fatalf("la nota debe decir en cocina lo que pidió el cliente:\n quería %q\n fue    %q", want, got)
	}
	if PlatformLineNotes(nil) != "" {
		t.Fatalf("sin opciones no hay nota")
	}
}
