package uber

import (
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL MENÚ DE UBER ES UN GRAFO PLANO, NO UN ÁRBOL.
//
// La respuesta trae cuatro listas al mismo nivel —`menus`, `categories`, `items`,
// `modifier_groups`— y las relaciones son por id. Modelarlo como árbol y después intentar mapearlo
// es, según la investigación previa, «el error estructural más caro de esta familia».
type menuResponse struct {
	Categories     []categoria `json:"categories"`
	Items          []item      `json:"items"`
	ModifierGroups []grupo     `json:"modifier_groups"`
}

type traducciones struct {
	Translations map[string]string `json:"translations"`
}

// texto devuelve el nombre en el idioma que haya. Uber entrega el menú en español bajo la llave
// "en" en esta tienda, así que elegir por código de idioma dejaría los nombres vacíos.
func (t traducciones) texto() string {
	for _, k := range []string{"es", "es_mx", "en", "en_us"} {
		if v, ok := t.Translations[k]; ok && v != "" {
			return v
		}
	}
	for _, v := range t.Translations {
		if v != "" {
			return v
		}
	}
	return ""
}

type categoria struct {
	ID       string       `json:"id"`
	Title    traducciones `json:"title"`
	Entities []entidad    `json:"entities"`
}

// entidad referencia un item desde una categoría.
//
// `Type` VIENE AUSENTE la mayoría de las veces y su default es ITEM: medido en la tienda real, 60
// de 81 referencias no lo traen. Un lector que solo cuente las explícitas reporta 21 platillos
// donde hay 65 — este proyecto ya cometió ese error una vez.
type entidad struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

func (e entidad) esItem() bool { return e.Type == "" || e.Type == "ITEM" }

type item struct {
	ID           string       `json:"id"`
	Title        traducciones `json:"title"`
	ExternalData string       `json:"external_data"`
	PriceInfo    struct {
		Price int64 `json:"price"` // centavos
	} `json:"price_info"`
	SuspensionInfo struct {
		Suspension *struct {
			SuspendUntil int64 `json:"suspend_until"`
		} `json:"suspension"`
	} `json:"suspension_info"`
}

// disponible: un item está agotado solo si su suspensión **no ha vencido**.
//
// Medido en la tienda real: 25 items traen `suspend_until` y TODOS son del pasado — suspensiones
// que ya expiraron y cuyos platillos sí se venden. Tratar la presencia del campo como «agotado»
// apagaría los 25 en el reporte, y la pantalla diría que hay 25 diferencias de disponibilidad que
// no existen.
func (i item) disponible(ahora time.Time) bool {
	s := i.SuspensionInfo.Suspension
	if s == nil || s.SuspendUntil == 0 {
		return true
	}
	return time.Unix(s.SuspendUntil, 0).Before(ahora)
}

type grupo struct {
	ID      string       `json:"id"`
	Title   traducciones `json:"title"`
	Options []struct {
		ID string `json:"id"`
	} `json:"modifier_options"`
}

// Aplanar convierte la respuesta de Uber en la lista de items que el resto del sistema entiende.
//
// LA CLASE SE DECIDE POR DÓNDE SE REFERENCIA EL ITEM, no por un campo, porque no hay campo: **un
// modificador ES un item**. Los 233 items de la tienda real son 65 platillos, 157 opciones y 11
// huérfanos que nadie referencia, y solo se distinguen por su procedencia.
//
// Tres reglas que salen de datos medidos, no de la documentación:
//
//   - Un item referenciado desde una categoría Y desde un grupo gana `platillo`. Hoy no pasa en la
//     tienda real (cero casos de 233), pero el grafo plano no lo impide.
//   - Un item referenciado desde VARIAS categorías se emite UNA vez. Medido: 16 de 65 platillos
//     están en más de una, porque «Los Favoritos» es un estante y no un platillo nuevo.
//   - Una opción que vive en varios grupos también se emite una vez: es la misma opción, aparezca
//     donde aparezca, y por eso la pareja se guarda por id y no por (grupo, opción).
//
// Los huérfanos se descartan: existen en el menú y no se ven en la app, así que compararlos
// reportaría diferencias de platillos que ningún cliente puede pedir.
func (m menuResponse) Aplanar(ahora time.Time) []domain.ItemDePlataforma {
	clase := make(map[string]domain.ClaseDeItem, len(m.Items))
	for _, g := range m.ModifierGroups {
		for _, o := range g.Options {
			if _, ya := clase[o.ID]; !ya {
				clase[o.ID] = domain.ItemOpcion
			}
		}
	}
	// Después de las opciones, para que un item que está en los dos lados quede como platillo.
	for _, c := range m.Categories {
		for _, e := range c.Entities {
			if e.esItem() {
				clase[e.ID] = domain.ItemPlatillo
			}
		}
	}

	// Orden estable por el orden de `items`, que es el que entrega la plataforma: un orden que
	// cambia entre lecturas haría que dos fotos idénticas se vieran distintas en un diff.
	salida := make([]domain.ItemDePlataforma, 0, len(clase))
	for _, it := range m.Items {
		k, referenciado := clase[it.ID]
		if !referenciado {
			continue // huérfano: existe y no se ve en la app.
		}
		salida = append(salida, domain.ItemDePlataforma{
			ID:       it.ID,
			Clase:    k,
			Nombre:   it.Title.texto(),
			Centavos: it.PriceInfo.Price,
			Activo:   it.disponible(ahora),
		})
	}
	return salida
}
