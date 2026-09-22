package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// RetencionDeLecturasEnDias: cuántos días se conservan las fotos del menú. Medido: 233 items por
// lectura; con una diaria son ~21 mil renglones por conexión en este plazo.
//
// La poda CONSERVA SIEMPRE la más reciente de cada conexión, sin importar su edad — si no, una
// tienda que nadie vuelve a leer se queda sin ninguna fila y la pantalla la muestra igual que una
// que nunca se leyó, que es justo la distinción que FR-003 exige.
const RetencionDeLecturasEnDias = 92

// ClaseDeItem: los tres niveles que tiene un menú, y que el POS y las plataformas modelan igual —
// el POS con products / modifier_groups / modifier_options, Uber con item / modifier_group_ids /
// modifier_options (donde una opción, a su vez, ES un item).
type ClaseDeItem string

const (
	ItemPlatillo ClaseDeItem = "platillo"
	ItemGrupo    ClaseDeItem = "grupo"
	ItemOpcion   ClaseDeItem = "opcion"
)

// ClaseDeItemValida reporta si c es uno de los tres niveles. Un valor desconocido se RECHAZA; nunca
// cae a un default, porque el default silencioso emparejaría una opción como si fuera platillo y el
// mapeo resultante jamás empata con nada (principio V).
func ClaseDeItemValida(c ClaseDeItem) bool {
	return c == ItemPlatillo || c == ItemGrupo || c == ItemOpcion
}

// ClaseLocal: qué es el lado de ACÁ de un emparejamiento. Hoy solo hay dos valores; el día que
// exista inventario de insumos, un platillo de la plataforma podrá corresponder a una receta.
type ClaseLocal string

const (
	LocalProducto ClaseLocal = "producto"
	LocalOpcion   ClaseLocal = "opcion_de_modificador"
)

// ClaseLocalDe devuelve qué debe ser el lado local para una clase de la plataforma, y si la
// combinación es válida. Un platillo se empareja con un producto y una opción con una opción:
// cruzarlos produce una pareja que pasa los tipos y nunca empata.
func ClaseLocalDe(c ClaseDeItem) (ClaseLocal, bool) {
	switch c {
	case ItemPlatillo:
		return LocalProducto, true
	case ItemOpcion:
		return LocalOpcion, true
	default:
		// `grupo` existe en el esquema para no cerrar la puerta, pero esta feature no lo empareja.
		return "", false
	}
}

// ClaseDeFallo: por qué no sirvió una lectura. **Es una lista cerrada y eso es un control de
// seguridad**, no una convención de estilo: existe para que el error crudo de una API ajena —que
// puede traer la dirección completa, y DiDi manda su secreto en el query string— nunca llegue a la
// base ni a la bitácora (FR-022).
//
// El espejo de esta lista vive en el `check` de la migración 0071, y
// TestLasClasesDeFalloCoincidenConElCheck vigila que no se separen.
type ClaseDeFallo string

const (
	// nolint:gosec // G101: es el NOMBRE de una clase de fallo, no una credencial.
	FalloSinCredenciales   ClaseDeFallo = "sin_credenciales"
	FalloAuthRechazada     ClaseDeFallo = "auth_rechazada"
	FalloTiempoAgotado     ClaseDeFallo = "tiempo_agotado"
	FalloRespuestaInvalida ClaseDeFallo = "respuesta_invalida"
	FalloMenuVacio         ClaseDeFallo = "menu_vacio"
	FalloMenuTruncado      ClaseDeFallo = "menu_truncado"
)

// Dos clases más, propias de RECIBIR un pedido (spec 021). Viven aquí y no en un tipo aparte
// porque son el mismo vocabulario: «por qué no sirvió lo que vino de la plataforma». Lo que cambia
// es la lista de cada `check`, y por eso hay dos funciones espejo y no una.
const (
	// FalloDetalleIlegible: el detalle llegó pero no se pudo interpretar. Distinto de una respuesta
	// inválida: aquí el HTTP salió bien y lo que no cuadra es la forma del pedido.
	FalloDetalleIlegible ClaseDeFallo = "detalle_ilegible"
	// FalloMapeoImposible: se entendió el pedido y no se pudo registrar.
	FalloMapeoImposible ClaseDeFallo = "mapeo_imposible"
)

// ClasesDeFalloDePedido son las del `check` de platform_webhook_events (0073). No incluye las de
// menú —un pedido no puede venir «vacío» ni «truncado» en el sentido de un menú— y sí las dos de
// arriba.
func ClasesDeFalloDePedido() []ClaseDeFallo {
	return []ClaseDeFallo{
		FalloSinCredenciales, FalloAuthRechazada, FalloTiempoAgotado,
		FalloRespuestaInvalida, FalloDetalleIlegible, FalloMapeoImposible,
	}
}

// ClasesDeFallo son todas, en el orden del `check`. Se exporta para que el test de espejo la
// compare contra el archivo de migración.
func ClasesDeFallo() []ClaseDeFallo {
	return []ClaseDeFallo{
		FalloSinCredenciales, FalloAuthRechazada, FalloTiempoAgotado,
		FalloRespuestaInvalida, FalloMenuVacio, FalloMenuTruncado,
	}
}

// ItemDePlataforma es un platillo u opción tal como lo publica la plataforma. NO es un producto del
// catálogo y no se mezcla con él: el precio viene en centavos, como lo entrega la API.
type ItemDePlataforma struct {
	ID       string      `json:"externalId"`
	Clase    ClaseDeItem `json:"kind"`
	Nombre   string      `json:"name"`
	Centavos int64       `json:"priceCents"`
	Activo   bool        `json:"available"`
}

// TiendaDePlataforma es una tienda tal como la lista la plataforma, para que alguien la ELIJA en vez
// de teclear su identificador.
//
// Trae lo mínimo con lo que una persona distingue sus sucursales: el nombre y la ciudad. La
// respuesta de la plataforma incluye además el correo del titular, su teléfono y la dirección
// exacta, y nada de eso cruza esta frontera — la pantalla no lo necesita y el repositorio es
// público (FR-022 es sobre secretos, esto es lo mismo con datos personales).
type TiendaDePlataforma struct {
	ID     string `json:"externalStoreId"`
	Nombre string `json:"name"`
	Ciudad string `json:"city"`
	// PDVConectado: si esa tienda ya tiene un punto de venta conectado del lado de la plataforma.
	// Sin mostrarlo, alguien conecta la misma dos veces y no entiende por qué la otra dejó de
	// recibir.
	PDVConectado bool `json:"posConnected"`
	// YaRegistrada: si esta empresa ya la dio de alta aquí. La alternativa —dejarla en la lista y
	// que el alta falle con «ya existe»— hace que el operador crea que se equivocó de tienda.
	YaRegistrada bool `json:"alreadyAdded"`
}

// ProductoLocal es el lado del POS de la comparación, con su precio YA calculado para la
// plataforma: la excepción capturada si existe, y si no `base × (1 + markup)`. Esa regla vive en
// app/menu.go y no se duplica aquí.
type ProductoLocal struct {
	ID                 int64           `json:"id"`
	Nombre             string          `json:"name"`
	PrecioDePlataforma decimal.Decimal `json:"platformPrice"`
	Activo             bool            `json:"active"`
}

// Pareja es un emparejamiento guardado. Sin `ConfirmadaEn` es una PROPUESTA, no un hecho.
type Pareja struct {
	ExternalID string      `json:"externalId"`
	Clase      ClaseDeItem `json:"kind"`
	LocalID    int64       `json:"localId"`
	ClaseLocal ClaseLocal  `json:"localKind"`
	Confirmada bool        `json:"confirmed"`
	// ConfirmadaEn permite saber CUÁL fue la última, que es lo único que hace correcto un
	// «deshacer el último». La lista llega ordenada por nombre, así que sin esta marca hay que
	// adivinar — y adivinar aquí no falla ruidoso: deshace la pareja equivocada y el operador se
	// entera semanas después, cuando reaparece como pendiente.
	ConfirmadaEn *time.Time `json:"confirmedAt,omitempty"`
}

// ClaseDeDiferencia: de qué tipo es lo que no coincide. Precio y disponibilidad son clases
// distintas porque se arreglan en lugares distintos (FR-016).
type ClaseDeDiferencia string

const (
	DifPrecio           ClaseDeDiferencia = "precio"
	DifDisponibilidad   ClaseDeDiferencia = "disponibilidad"
	DifSoloEnPlataforma ClaseDeDiferencia = "solo_en_plataforma"
	DifSoloEnCatalogo   ClaseDeDiferencia = "solo_en_catalogo"
	// DifPrecioIlegible: la plataforma publicó un importe que no cabe en una columna de dinero —un
	// dedazo de ceros en el portal, por ejemplo. NO se puede callar: sin este renglón, el platillo
	// sale como que coincide en precio, y un reporte que dice «todo está bien» sobre un precio
	// absurdo es peor que uno que falla.
	DifPrecioIlegible ClaseDeDiferencia = "precio_ilegible"
)

// Diferencia es un renglón de lo que no coincide.
//
// NO LLEVA NINGUNA ACCIÓN SUGERIDA, y es deliberado (FR-019): un platillo que solo existe arriba
// puede ser una decisión del negocio, y proponer borrarlo convierte un reporte en una trampa.
type Diferencia struct {
	Clase        ClaseDeDiferencia `json:"kind"`
	ExternalID   string            `json:"externalId,omitempty"`
	NombreArriba string            `json:"platformName,omitempty"`
	LocalID      int64             `json:"localId,omitempty"`
	NombreAbajo  string            `json:"localName,omitempty"`
	PrecioArriba decimal.Decimal   `json:"platformPrice,omitempty"`
	PrecioAbajo  decimal.Decimal   `json:"catalogPrice,omitempty"`
	ActivoArriba bool              `json:"platformAvailable,omitempty"`
	ActivoAbajo  bool              `json:"catalogActive,omitempty"`
}

// Sentinels. El mapeo a HTTP vive SOLO en httpapi.Error (principio II).
var (
	// ErrLecturaVacia: la plataforma respondió bien y sin productos. No es «el menú está vacío»:
	// es una lectura que no sirve, y compararla diría que sobra todo el catálogo — el peor reporte
	// posible y el que más invita a una acción destructiva.
	ErrLecturaVacia = errors.New("la lectura del menú volvió sin productos")
	// ErrPlataformaSinCredenciales: no se puede leer porque nadie configuró esta plataforma.
	// Distinto de «no hay diferencias».
	ErrPlataformaSinCredenciales = errors.New("la plataforma no tiene credenciales configuradas")
	// ErrLecturaEnCurso: ya hay una lectura corriendo para esta tienda. Dos a la vez gastan dos
	// tokens para escribir la misma foto, y Uber invalida el más viejo a partir del 101 por hora.
	ErrLecturaEnCurso = errors.New("ya hay una lectura en curso para esta tienda")
	// ErrSinLecturaValida: nunca se ha leído con éxito, así que no hay contra qué emparejar.
	ErrSinLecturaValida = errors.New("no hay ninguna lectura válida de esta tienda")
	// ErrParejaOcupada: ese item de la plataforma ya apunta a otro producto (FR-012).
	ErrParejaOcupada = errors.New("ese platillo de la plataforma ya está emparejado")
	// ErrConexionDuplicada: esa tienda ya está registrada para esa plataforma.
	ErrConexionDuplicada = errors.New("esa tienda ya está registrada")
	// ErrItemInexistente: se intentó emparejar un id que no existe en la última lectura. Sustituye
	// a la FK que platform_item_links deliberadamente no tiene.
	ErrItemInexistente = errors.New("ese platillo no existe en la última lectura del menú")
)

// --- Emparejamiento automático ---

// normalizarNombre deja un nombre comparable: sin acentos, sin emoji, sin paréntesis y sin
// puntuación. Es lo que permite que «Chai latte Vainilla» empate con «Chai Latte Vainilla».
//
// NO es el mecanismo de emparejamiento, es solo la propuesta: medido contra el menú real, el
// nombre acierta 6 de 65 platillos, porque «Hot Chicken 🔥🔥🔥🔥» y «Hot Chicken - Buldak» son el
// mismo platillo con nombres que ningún normalizador va a unir.
func normalizarNombre(s string) string {
	sinParentesis := s
	for {
		i := strings.IndexByte(sinParentesis, '(')
		if i < 0 {
			break
		}
		j := strings.IndexByte(sinParentesis[i:], ')')
		if j < 0 {
			sinParentesis = sinParentesis[:i]
			break
		}
		sinParentesis = sinParentesis[:i] + " " + sinParentesis[i+j+1:]
	}
	sinAcentos, _, err := transform.String(
		transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), sinParentesis)
	if err != nil {
		sinAcentos = sinParentesis
	}
	var b strings.Builder
	espacio := false
	for _, r := range strings.ToLower(sinAcentos) {
		switch {
		case unicode.IsLetter(r) && r < 128, unicode.IsDigit(r):
			if espacio && b.Len() > 0 {
				b.WriteByte(' ')
			}
			espacio = false
			b.WriteRune(r)
		default:
			espacio = true
		}
	}
	return b.String()
}

// ProponerParejas sugiere emparejamientos por coincidencia EXACTA de nombre normalizado, y los
// devuelve marcados como propuestas (FR-011).
//
// Dos reglas que no son cosméticas:
//
//   - **Nunca empareja por aproximación.** Un nombre que se parece no es el mismo platillo, y una
//     pareja adivinada produce diferencias falsas que nadie puede auditar.
//   - **Si dos productos del POS se llaman igual, no propone ninguno.** La propuesta automática no
//     puede elegir, y elegir al azar es peor que dejarlo a la persona.
func ProponerParejas(arriba []ItemDePlataforma, abajo []ProductoLocal, yaEmparejados map[string]bool) []Pareja {
	porNombre := map[string][]ProductoLocal{}
	for _, p := range abajo {
		n := normalizarNombre(p.Nombre)
		if n == "" {
			continue
		}
		porNombre[n] = append(porNombre[n], p)
	}
	var props []Pareja
	for _, it := range arriba {
		if yaEmparejados[it.ID] {
			continue
		}
		cands := porNombre[normalizarNombre(it.Nombre)]
		if len(cands) != 1 {
			continue // cero: sin propuesta. dos o más: ambiguo, y elegir al azar es peor.
		}
		local, ok := ClaseLocalDe(it.Clase)
		if !ok {
			continue
		}
		props = append(props, Pareja{
			ExternalID: it.ID, Clase: it.Clase,
			LocalID: cands[0].ID, ClaseLocal: local, Confirmada: false,
		})
	}
	sort.Slice(props, func(i, j int) bool { return props[i].ExternalID < props[j].ExternalID })
	return props
}

// --- La comparación ---

// Comparar produce las diferencias entre lo publicado y el catálogo, usando SOLO las parejas
// confirmadas.
//
// Es pura: sin base de datos y sin HTTP. Por eso se puede probar con tablas, y por eso una lectura
// vacía NO llega hasta aquí — el servicio la rechaza antes, porque esta función no puede distinguir
// «el menú está vacío» de «la lectura se rompió», y darle esa decisión sería esconderla donde no se
// ve (FR-005).
//
// Lo que coincide NO se devuelve (FR-018): la pantalla lista lo que difiere, no el catálogo entero.
func Comparar(arriba []ItemDePlataforma, abajo []ProductoLocal, parejas []Pareja) []Diferencia {
	localPorID := map[int64]ProductoLocal{}
	for _, p := range abajo {
		localPorID[p.ID] = p
	}
	// Qué ids existen HOY arriba. Hace falta para no dar por emparejado un producto cuya pareja
	// apunta a un platillo que ya no está publicado.
	existeArriba := map[string]bool{}
	for _, it := range arriba {
		existeArriba[it.ID] = true
	}
	// Solo las confirmadas: comparar contra una propuesta sin confirmar reportaría diferencias de
	// una pareja que nadie aceptó.
	//
	// Y solo las que apuntan a algo que SIGUE arriba. Una pareja huérfana —porque borraron el
	// platillo en el portal, o porque la plataforma le cambió el id— marcaría el producto local
	// como emparejado y lo ESCONDERÍA del reporte: el operador vería desaparecer un producto de la
	// lista sin que nada indique por qué. Lo atrapó TestSiCambiaElIdDeArribaSalenLosDosLados.
	parejaDe := map[string]Pareja{}
	emparejadoLocal := map[int64]bool{}
	for _, pa := range parejas {
		if !pa.Confirmada || !existeArriba[pa.ExternalID] {
			continue
		}
		parejaDe[pa.ExternalID] = pa
		emparejadoLocal[pa.LocalID] = true
	}

	var difs []Diferencia
	for _, it := range arriba {
		pa, tiene := parejaDe[it.ID]
		if !tiene {
			difs = append(difs, Diferencia{
				Clase: DifSoloEnPlataforma, ExternalID: it.ID, NombreArriba: it.Nombre,
			})
			continue
		}
		local, existe := localPorID[pa.LocalID]
		if !existe {
			// La pareja apunta a un producto que ya no está activo: se reporta como si el platillo
			// no tuviera pareja, que es lo que el operador tiene que resolver.
			difs = append(difs, Diferencia{
				Clase: DifSoloEnPlataforma, ExternalID: it.ID, NombreArriba: it.Nombre,
			})
			continue
		}
		pesos, err := PesosDeCentavos(it.Centavos)
		if err != nil {
			// Se REPORTA, no se descarta. `PesosDeCentavos` existe para rechazar lo que no cabe, y
			// tragarse su rechazo convierte el reporte en una mentira tranquilizadora.
			difs = append(difs, Diferencia{
				Clase: DifPrecioIlegible, ExternalID: it.ID, NombreArriba: it.Nombre,
				LocalID: local.ID, NombreAbajo: local.Nombre,
				PrecioAbajo: local.PrecioDePlataforma,
			})
		} else if !pesos.Equal(local.PrecioDePlataforma) {
			difs = append(difs, Diferencia{
				Clase: DifPrecio, ExternalID: it.ID, NombreArriba: it.Nombre,
				LocalID: local.ID, NombreAbajo: local.Nombre,
				PrecioArriba: pesos, PrecioAbajo: local.PrecioDePlataforma,
			})
		}
		// Disponibilidad APARTE del precio: son dos problemas distintos y se arreglan en lugares
		// distintos (FR-016).
		if it.Activo != local.Activo {
			difs = append(difs, Diferencia{
				Clase: DifDisponibilidad, ExternalID: it.ID, NombreArriba: it.Nombre,
				LocalID: local.ID, NombreAbajo: local.Nombre,
				ActivoArriba: it.Activo, ActivoAbajo: local.Activo,
			})
		}
	}
	for _, p := range abajo {
		if !emparejadoLocal[p.ID] {
			difs = append(difs, Diferencia{
				Clase: DifSoloEnCatalogo, LocalID: p.ID, NombreAbajo: p.Nombre,
			})
		}
	}
	return difs
}
