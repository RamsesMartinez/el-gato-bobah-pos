package uber

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func menuDePrueba(t *testing.T) menuResponse {
	t.Helper()
	crudo, err := os.ReadFile("testdata/menu.json")
	if err != nil {
		t.Fatal(err)
	}
	var m menuResponse
	if err := json.Unmarshal(crudo, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// EL CONTEO ES EL DEFECTO QUE YA SE COMETIÓ. Un modificador ES un item, así que pedir "los items" y
// contarlos como platillos da 233 donde hay 65; y `type` viene ausente en la mayoría de las
// referencias, así que contar solo las explícitas da 21.
func TestAplanarClasificaPorProcedenciaYNoPorElCampoType(t *testing.T) {
	items := menuDePrueba(t).Aplanar(time.Now())

	var platillos, opciones int
	vistos := map[string]int{}
	for _, it := range items {
		vistos[it.ID]++
		switch it.Clase {
		case domain.ItemPlatillo:
			platillos++
		case domain.ItemOpcion:
			opciones++
		default:
			t.Errorf("clase inesperada %q en %s", it.Clase, it.ID)
		}
	}
	if platillos != 5 {
		t.Errorf("platillos: %d, quería 5 — si dio 1, está contando solo las entidades con `type` explícito", platillos)
	}
	if opciones != 5 {
		t.Errorf("opciones: %d, quería 5", opciones)
	}
	// El huérfano no sale: existe en el menú y ningún cliente puede pedirlo, así que compararlo
	// reportaría una diferencia de algo que no se vende.
	if _, hay := vistos["Item_que_nadie_refe"]; hay {
		t.Error("salió el item huérfano: nadie lo referencia y no se ve en la app")
	}
	// Un platillo en dos categorías —«Los más pedidos» es un estante, no un platillo nuevo— se
	// emite UNA vez. Medido en la tienda real: 16 de 65 están en más de una.
	if vistos["Limonada_de_Mango_"] != 1 {
		t.Errorf("Limonada_de_Mango_ salió %d veces; está en dos categorías y es un solo platillo", vistos["Limonada_de_Mango_"])
	}
	// Y una opción que vive en dos grupos, también: es la misma opción aparezca donde aparezca, y
	// por eso la pareja se guarda por id y no por (grupo, opción).
	if vistos["Chico"] != 1 {
		t.Errorf("la opción Chico salió %d veces; vive en dos grupos y es una sola", vistos["Chico"])
	}
}

// Una opción que vive en varios grupos sigue siendo opción: la clase la decide de dónde viene la
// referencia, no cuántas veces aparece.
func TestUnaOpcionEnVariosGruposSigueSiendoOpcion(t *testing.T) {
	for _, it := range menuDePrueba(t).Aplanar(time.Now()) {
		if it.ID == "Chico" && it.Clase != domain.ItemOpcion {
			t.Errorf("Chico debería ser opción, dio %q", it.Clase)
		}
	}
}

// EL ID SE CONSERVA COMPLETO (FR-020). Los de Uber vienen truncados a 20 caracteres y traen
// acentos y emoji; recortarlos o normalizarlos rompe el emparejamiento en la siguiente lectura.
func TestAplanarConservaElIdTalCual(t *testing.T) {
	items := menuDePrueba(t).Aplanar(time.Now())
	var encontrado bool
	for _, it := range items {
		if it.ID == "Salsa_de_la_casa_🌶" {
			encontrado = true
		}
	}
	if !encontrado {
		t.Error("se perdió o se normalizó un id con emoji: la pareja guardada dejaría de empatar")
	}
}

// LA SUSPENSIÓN VENCIDA NO ES UN AGOTADO. Medido en la tienda real: 25 items traen `suspend_until`
// y todos son del pasado. Tratar la presencia del campo como «agotado» apagaría los 25 y la
// pantalla reportaría 25 diferencias de disponibilidad que no existen.
func TestLaSuspensionVencidaNoApagaElPlatillo(t *testing.T) {
	porID := map[string]domain.ItemDePlataforma{}
	for _, it := range menuDePrueba(t).Aplanar(time.Now()) {
		porID[it.ID] = it
	}
	if !porID["Sandwich_de_tres_q"].Activo {
		t.Error("un platillo con suspensión VENCIDA salió como agotado: son 25 en la tienda real")
	}
	if porID["Refresco_de_Toronja"].Activo {
		t.Error("un platillo con suspensión VIGENTE salió como disponible")
	}
	if !porID["Limonada_de_Fresa_"].Activo {
		t.Error("un platillo sin suspensión salió como agotado")
	}
}

// El precio viaja en centavos hasta domain, que es donde se convierte una sola vez.
func TestAplanarDejaElPrecioEnCentavos(t *testing.T) {
	for _, it := range menuDePrueba(t).Aplanar(time.Now()) {
		if it.ID == "Limonada_de_Mango_" && it.Centavos != 9900 {
			t.Errorf("el precio llegó como %d; debe ser 9900 centavos, sin convertir", it.Centavos)
		}
	}
}
