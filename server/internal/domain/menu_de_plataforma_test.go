package domain

import (
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// LA LISTA DE CLASES DE FALLO VIVE EN DOS LUGARES y tiene que ser la misma: aquí en Go y en el
// `check` de la migración 0071. Si se separan, el síntoma en producción es un 23514 al intentar
// guardar una lectura fallida — o sea, la lectura falla Y el registro de por qué falló también.
func TestLasClasesDeFalloCoincidenConElCheck(t *testing.T) {
	sql, err := os.ReadFile("../../migrations/0071_menus_de_plataforma.sql")
	if err != nil {
		t.Fatalf("leer la migración: %v", err)
	}
	texto := string(sql)
	for _, c := range ClasesDeFallo() {
		if !strings.Contains(texto, "'"+string(c)+"'") {
			t.Errorf("la clase %q existe en Go y NO en el check de la migración", c)
		}
	}
	// Y al revés: un valor en el check que Go no conozca es una rama muerta que nadie puede escribir.
	inicio := strings.Index(texto, "platform_menu_reads_clase_de_fallo")
	if inicio < 0 {
		t.Fatal("no se encontró el check de clase_de_fallo en la migración")
	}
	bloque := texto[inicio : inicio+400]
	conocidas := map[string]bool{}
	for _, c := range ClasesDeFallo() {
		conocidas[string(c)] = true
	}
	for _, trozo := range strings.Split(bloque, "'") {
		if strings.ContainsAny(trozo, " (),\n") || trozo == "" {
			continue
		}
		if !conocidas[trozo] {
			t.Errorf("el check acepta %q y Go no lo conoce", trozo)
		}
	}
}

func TestClaseLocalDe(t *testing.T) {
	if c, ok := ClaseLocalDe(ItemPlatillo); !ok || c != LocalProducto {
		t.Errorf("un platillo se empareja con un producto, dio %q/%v", c, ok)
	}
	if c, ok := ClaseLocalDe(ItemOpcion); !ok || c != LocalOpcion {
		t.Errorf("una opción se empareja con una opción, dio %q/%v", c, ok)
	}
	// `grupo` existe en el esquema para no cerrar la puerta, pero esta feature no lo empareja: que
	// devuelva ok=false es lo que impide guardar una pareja de grupo a medio construir.
	if _, ok := ClaseLocalDe(ItemGrupo); ok {
		t.Error("el nivel de grupo no se empareja en esta feature y debería rechazarse")
	}
}

// --- ProponerParejas ---

func TestProponerParejas(t *testing.T) {
	abajo := []ProductoLocal{
		{ID: 1, Nombre: "Chai Latte Vainilla"},
		{ID: 2, Nombre: "Chamoyada"},
		{ID: 3, Nombre: "Frappé"},
		{ID: 4, Nombre: "Frappé"}, // homónimo a propósito
	}
	casos := []struct {
		nombre  string
		arriba  ItemDePlataforma
		quiereN int
		local   int64
	}{
		{
			nombre:  "coincide aunque cambien acentos y mayúsculas",
			arriba:  ItemDePlataforma{ID: "Chai_latte", Clase: ItemPlatillo, Nombre: "Chai latte Vainilla"},
			quiereN: 1, local: 1,
		},
		{
			nombre:  "coincide ignorando emoji",
			arriba:  ItemDePlataforma{ID: "Chamoyada_x", Clase: ItemPlatillo, Nombre: "Chamoyada 🥭"},
			quiereN: 1, local: 2,
		},
		{
			// El caso que importa: parecido NO es igual. «Hot Chicken 🔥» y «Hot Chicken - Buldak»
			// son el mismo platillo y nadie puede saberlo por el nombre — lo resuelve una persona.
			nombre:  "un nombre que solo se parece no se propone",
			arriba:  ItemDePlataforma{ID: "Hot_Chicken", Clase: ItemPlatillo, Nombre: "Hot Chicken 🔥🔥🔥🔥"},
			quiereN: 0,
		},
		{
			// Dos productos del POS con el mismo nombre: la propuesta automática no puede elegir, y
			// elegir al azar produce una pareja falsa que nadie va a auditar.
			nombre:  "con dos homónimos abajo no se propone ninguno",
			arriba:  ItemDePlataforma{ID: "Frappe", Clase: ItemPlatillo, Nombre: "Frappé"},
			quiereN: 0,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := ProponerParejas([]ItemDePlataforma{c.arriba}, abajo, nil)
			if len(got) != c.quiereN {
				t.Fatalf("propuso %d parejas, quería %d: %+v", len(got), c.quiereN, got)
			}
			if c.quiereN == 1 {
				if got[0].LocalID != c.local {
					t.Errorf("propuso el producto %d, quería %d", got[0].LocalID, c.local)
				}
				// FR-011: es una PROPUESTA, no un hecho. Aceptada en silencio produce comparaciones
				// falsas que nadie puede auditar.
				if got[0].Confirmada {
					t.Error("la propuesta llegó marcada como confirmada")
				}
			}
		})
	}
}

func TestProponerParejasNoRepiteLoYaEmparejado(t *testing.T) {
	arriba := []ItemDePlataforma{{ID: "Chamoyada_x", Clase: ItemPlatillo, Nombre: "Chamoyada"}}
	abajo := []ProductoLocal{{ID: 2, Nombre: "Chamoyada"}}
	if got := ProponerParejas(arriba, abajo, map[string]bool{"Chamoyada_x": true}); len(got) != 0 {
		t.Fatalf("volvió a proponer algo ya emparejado: %+v", got)
	}
}

// --- Comparar ---

func pesos(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestComparar(t *testing.T) {
	// Un producto de cada clase de diferencia, más uno que coincide en todo.
	arriba := []ItemDePlataforma{
		{ID: "caro", Clase: ItemPlatillo, Nombre: "Crepizza", Centavos: 13900, Activo: true},
		{ID: "agotado", Clase: ItemPlatillo, Nombre: "Ferrero", Centavos: 9900, Activo: false},
		{ID: "huerfano", Clase: ItemPlatillo, Nombre: "Dedos de queso", Centavos: 7500, Activo: true},
		{ID: "igual", Clase: ItemPlatillo, Nombre: "Mokaccino", Centavos: 8775, Activo: true},
	}
	abajo := []ProductoLocal{
		{ID: 1, Nombre: "Crepizza", PrecioDePlataforma: pesos("128.25"), Activo: true},
		{ID: 2, Nombre: "Ferrero", PrecioDePlataforma: pesos("99"), Activo: true},
		{ID: 3, Nombre: "Mokaccino", PrecioDePlataforma: pesos("87.75"), Activo: true},
		{ID: 4, Nombre: "Papas", PrecioDePlataforma: pesos("78.30"), Activo: true},
	}
	parejas := []Pareja{
		{ExternalID: "caro", LocalID: 1, Confirmada: true},
		{ExternalID: "agotado", LocalID: 2, Confirmada: true},
		{ExternalID: "igual", LocalID: 3, Confirmada: true},
	}

	difs := Comparar(arriba, abajo, parejas)
	por := map[ClaseDeDiferencia][]Diferencia{}
	for _, d := range difs {
		por[d.Clase] = append(por[d.Clase], d)
	}

	// FR-018: el que coincide en todo NO aparece. La pantalla lista lo que difiere.
	for _, d := range difs {
		if d.ExternalID == "igual" {
			t.Errorf("apareció un platillo que coincide en todo: %+v", d)
		}
	}
	if n := len(por[DifPrecio]); n != 1 {
		t.Fatalf("diferencias de precio: %d, quería 1", n)
	}
	if p := por[DifPrecio][0]; !p.PrecioArriba.Equal(pesos("139")) || !p.PrecioAbajo.Equal(pesos("128.25")) {
		t.Errorf("la diferencia de precio trae %s/%s, quería 139/128.25", p.PrecioArriba, p.PrecioAbajo)
	}
	// FR-016: disponibilidad es OTRA clase, no un precio con asterisco. Se arreglan en lugares
	// distintos y mezclarlas manda a buscar al lugar equivocado.
	if n := len(por[DifDisponibilidad]); n != 1 {
		t.Errorf("diferencias de disponibilidad: %d, quería 1", n)
	}
	if n := len(por[DifSoloEnPlataforma]); n != 1 || por[DifSoloEnPlataforma][0].ExternalID != "huerfano" {
		t.Errorf("solo en la plataforma: %+v", por[DifSoloEnPlataforma])
	}
	if n := len(por[DifSoloEnCatalogo]); n != 1 || por[DifSoloEnCatalogo][0].LocalID != 4 {
		t.Errorf("solo en el catálogo: %+v", por[DifSoloEnCatalogo])
	}
	// FR-019: ninguna diferencia propone una acción. Un platillo que solo existe arriba puede ser
	// deliberado, y sugerir borrarlo convierte el reporte en una trampa. Se verifica por
	// construcción: el tipo Diferencia no tiene dónde ponerla.
}

// Una pareja SIN CONFIRMAR no se usa para comparar: es una propuesta, y comparar contra ella
// reportaría diferencias de un emparejamiento que nadie aceptó.
func TestCompararIgnoraLasParejasSinConfirmar(t *testing.T) {
	arriba := []ItemDePlataforma{{ID: "x", Clase: ItemPlatillo, Nombre: "Crepizza", Centavos: 13900, Activo: true}}
	abajo := []ProductoLocal{{ID: 1, Nombre: "Crepizza", PrecioDePlataforma: pesos("139"), Activo: true}}
	difs := Comparar(arriba, abajo, []Pareja{{ExternalID: "x", LocalID: 1, Confirmada: false}})

	var soloArriba, soloAbajo int
	for _, d := range difs {
		switch d.Clase {
		case DifSoloEnPlataforma:
			soloArriba++
		case DifSoloEnCatalogo:
			soloAbajo++
		default:
			t.Errorf("con la pareja sin confirmar no debería salir %q", d.Clase)
		}
	}
	if soloArriba != 1 || soloAbajo != 1 {
		t.Errorf("esperaba los dos lados sin emparejar, dio %d/%d", soloArriba, soloAbajo)
	}
}

// El caso de FR-014a: si la plataforma cambiara el id de un platillo, sale como dos renglones
// visibles —uno de cada lado— y NUNCA como una pareja adivinada.
func TestSiCambiaElIdDeArribaSalenLosDosLados(t *testing.T) {
	arriba := []ItemDePlataforma{{ID: "id_nuevo", Clase: ItemPlatillo, Nombre: "Crepizza", Centavos: 13900, Activo: true}}
	abajo := []ProductoLocal{{ID: 1, Nombre: "Crepizza", PrecioDePlataforma: pesos("139"), Activo: true}}
	difs := Comparar(arriba, abajo, []Pareja{{ExternalID: "id_viejo", LocalID: 1, Confirmada: true}})
	if len(difs) != 2 {
		t.Fatalf("esperaba dos renglones visibles, dio %d: %+v", len(difs), difs)
	}
}

// Un catálogo vacío del lado de arriba NO produce «sobra todo»: esta función ni siquiera debería
// recibirlo, pero si lo recibe se comporta de la única forma honesta — reporta el catálogo como no
// publicado, sin inventar nada. Quien rechaza la lectura vacía es el servicio (FR-005).
func TestCompararConCeroItemsArribaNoInventa(t *testing.T) {
	abajo := []ProductoLocal{{ID: 1, Nombre: "Crepizza", PrecioDePlataforma: pesos("139"), Activo: true}}
	difs := Comparar(nil, abajo, nil)
	if len(difs) != 1 || difs[0].Clase != DifSoloEnCatalogo {
		t.Fatalf("esperaba un solo renglón «solo en el catálogo», dio %+v", difs)
	}
}
