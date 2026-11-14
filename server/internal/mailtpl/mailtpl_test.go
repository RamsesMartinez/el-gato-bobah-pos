package mailtpl

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "reescribe los archivos de referencia")

func sample() StockEmail {
	return StockEmail{
		BusinessName: "Taquería La Esquina", Period: "Viernes 14 nov · 20:35 a 20:40", Title: "3 avisos nuevos",
		Subtitle: "2 cosas se acabaron y 1 no está en el sistema", LogoURL: "https://app.ejemplo.mx/logo192.png",
		OpenURL: "https://app.ejemplo.mx/almacen?tab=compra", Footer: "Te llega porque recibes los avisos de inventario.",
		Groups: []Group{
			{Title: "Se acabó", Items: []Item{{Kind: "stock_out", Name: "Bolsa grande", Detail: "Lo reportó Ana a las 20:32",
				Buy: "4 × Paquete de 100", Supplier: "Desechables del Centro", Why: "se usan ~22 pzas al día, quedan 0 pzas, objetivo 14 días"}}},
			{Title: "No lo encuentran", Items: []Item{{Kind: "not_found", Name: "«servitoallas»", Detail: "Lo buscó Luis a las 20:41"}}},
		},
	}
}

// El correo sale igual que su referencia; si la plantilla cambia a propósito, se regenera con -update.
func TestStockEmailMatchesItsGolden(t *testing.T) {
	got, err := RenderStock(sample())
	if err != nil {
		t.Fatal(err)
	}
	golden := "testdata/stock.golden.html"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("sin referencia (corre con -update): %v", err)
	}
	if got != string(want) {
		t.Fatal("el correo cambió respecto a su referencia; si fue a propósito, corre go test ./internal/mailtpl -update")
	}
}

// Gmail recorta lo que pasa de 102 KB y esconde el resto detrás de «Ver mensaje completo».
func TestStockEmailStaysUnderGmailClip(t *testing.T) {
	e := sample()
	for i := 0; i < 60; i++ {
		e.Groups[0].Items = append(e.Groups[0].Items, e.Groups[0].Items[0])
	}
	got, err := RenderStock(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) >= 102*1024 {
		t.Fatalf("con 61 avisos el correo pesa %d bytes", len(got))
	}
}

func TestStockEmailCarriesTheBrand(t *testing.T) {
	got, _ := RenderStock(sample())
	for _, want := range []string{"#c62b21", `src="https://app.ejemplo.mx/logo192.png"`, "Taquería La Esquina", "4 × Paquete de 100", "Abrir en el sistema"} {
		if !strings.Contains(got, want) {
			t.Errorf("el correo no lleva %q", want)
		}
	}
}

// Lo que escribió una persona («No lo encuentro», el nombre de un insumo) se escapa: un texto con
// etiquetas no puede inyectar HTML en el correo de otra.
func TestStockEmailEscapesWhatPeopleWrote(t *testing.T) {
	e := sample()
	e.Groups[1].Items[0].Name = `<img src=x onerror=alert(1)>`
	got, _ := RenderStock(e)
	if strings.Contains(got, "<img src=x") {
		t.Fatal("el texto de una persona entró como HTML")
	}
}
