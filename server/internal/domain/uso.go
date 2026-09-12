package domain

// EL USO DEL SISTEMA (spec 017): qué se puede contar y cómo se cuenta.
//
// Todo lo de aquí es puro: recibe números y cadenas, no toca base ni red. Es lo que permite probar
// la regla de anonimato sin montar un Postgres, y lo que hace que esa regla se decida en un solo
// lugar en vez de repartida por el servicio.

// MaxEventosPorLote acota lo que una tableta puede mandar de un golpe.
//
// El cliente manda de a 20; un lote de cientos solo puede venir de un bucle. El tope no protege la
// base —de eso se encargan la lista blanca y los `check` de las columnas— sino el camino: la llave
// del limitador y la línea de log se arman con lo que llega.
const MaxEventosPorLote = 50

// EventoDeUso es un hecho: alguien abrió una pantalla, o disparó una acción dentro de ella.
//
// SIN QUIÉN. No es un olvido ni algo que el servicio filtre después: este tipo no tiene dónde
// guardar a una persona, y la tabla tampoco. Lo que no existe no se llena por descuido.
type EventoDeUso struct {
	Pantalla string `json:"pantalla"`
	// Vacío = fue una apertura de pantalla. Las dos cosas son el mismo hecho —alguien hizo algo en
	// una pantalla— y por eso viajan y se guardan igual.
	Accion string `json:"accion,omitempty"`
}

// UsoAgregado es una combinación del lote con cuántas veces ocurrió.
type UsoAgregado struct {
	Pantalla string
	Accion   string
	Veces    int
}

// pantallasMedibles es la lista blanca de pantallas.
//
// Es una lista en el código y no una tabla en la base a propósito: un catálogo tendría que
// sembrarse por migración, mantenerse sincronizado con las rutas del front y, al agregar una
// pantalla, fallaría EN PRODUCCIÓN con una FK en vez de fallar al compilar. Esta se revisa en el
// diff y viaja con el binario que la valida.
//
// La consecuencia en la base es la que importa: solo entran valores de aquí, así que el número de
// combinaciones distintas está acotado por construcción y el agregado no se puede inflar.
// Son las rutas que el POS tiene HOY, ni una más: una entrada que no corresponde a ninguna pantalla
// real es un cero permanente en el mapa, y un cero que nunca sube se lee como "nadie la usa" en vez
// de como "no existe".
var pantallasMedibles = map[string]struct{}{
	"pos":        {},
	"pedidos":    {},
	"caja":       {},
	"reportes":   {},
	"ventas":     {},
	"gastos":     {},
	"catalogo":   {},
	"inventario": {},
	"usuarios":   {},
	"negocio":    {},
	"impresion":  {},
	"cuenta":     {},
}

// accionesMedibles son las acciones con nombre que vale la pena contar, por pantalla.
//
// Que sea una lista corta y no «cualquier clic» es lo que mantiene el volumen acotado y el mapa
// legible: lo que se busca es saber qué cuesta trabajo, no registrar cada toque.
var accionesMedibles = map[string]map[string]struct{}{
	"pos":        {"cobrar": {}, "agregar-producto": {}, "cancelar-pedido": {}},
	"pedidos":    {"entregar": {}, "cobrar": {}},
	"caja":       {"abrir-turno": {}, "cerrar-turno": {}, "contar-efectivo": {}, "traspaso": {}},
	"catalogo":   {"editar-producto": {}, "crear-producto": {}},
	"inventario": {"ajustar-stock": {}},
	"gastos":     {"registrar-gasto": {}},
}

// maxNombreDePantalla y maxNombreDeAccion son el espejo de los `check` de la migración 0069.
//
// Están en los dos lados a propósito: el de Go rechaza en la frontera y el de la columna cierra la
// vía de cualquier ruta futura que se salte esta validación.
const (
	maxNombreDePantalla = 40
	maxNombreDeAccion   = 60
)

// EventoDeUsoValido dice si ese par (pantalla, acción) se puede contar.
//
// Una acción vacía significa apertura de pantalla, no «acción desconocida».
func EventoDeUsoValido(pantalla, accion string) bool {
	if len(pantalla) == 0 || len(pantalla) > maxNombreDePantalla {
		return false
	}
	if _, ok := pantallasMedibles[pantalla]; !ok {
		return false
	}
	if accion == "" {
		return true
	}
	if len(accion) > maxNombreDeAccion {
		return false
	}
	_, ok := accionesMedibles[pantalla][accion]
	return ok
}

// RolMedible dice si ese rol es uno de los que el sistema tiene.
//
// Son CUATRO y no tres: `mesero` existe desde la primera migración, y el spec lo había olvidado.
// Una lista de tres lo manda al camino de error en vez de contarlo, y el mapa se queda ciego justo
// con el rol del que menos se sabe.
func RolMedible(r Role) bool { return r.Valid() }

// PreAgregarUso agrupa el lote por (pantalla, acción) y cuenta cuántas veces ocurrió cada uno.
//
// NO es una optimización cosmética. Cada `update` deja en Postgres la versión vieja de la fila
// muerta, y las filas del día son pocas y calientes: medido sobre el esquema real, 135 filas
// recibiendo 2,000 incrementos de a uno pasan de 64 kB a 232 kB antes de que autovacuum llegue —y
// con decenas de miles de filas vivas, autovacuum tarda días en disparar—. Agrupar convierte hasta
// 50 escrituras físicas en una.
//
// Descarta lo que no está en la lista blanca: lo que no se puede contar no llega a la base.
func PreAgregarUso(lote []EventoDeUso) []UsoAgregado {
	if len(lote) == 0 {
		return nil
	}
	type llave struct{ pantalla, accion string }
	// El orden importa para que el resultado sea estable entre corridas: sin él, dos lotes iguales
	// producen `update`s en orden distinto y dos transacciones concurrentes pueden interbloquearse.
	orden := make([]llave, 0, len(lote))
	veces := make(map[llave]int, len(lote))
	for _, e := range lote {
		if !EventoDeUsoValido(e.Pantalla, e.Accion) {
			continue
		}
		k := llave{e.Pantalla, e.Accion}
		if _, visto := veces[k]; !visto {
			orden = append(orden, k)
		}
		veces[k]++
	}
	agregado := make([]UsoAgregado, 0, len(orden))
	for _, k := range orden {
		agregado = append(agregado, UsoAgregado{Pantalla: k.pantalla, Accion: k.accion, Veces: veces[k]})
	}
	return agregado
}

// RecortarLoteDeUso deja el lote en el tope. Lo que sobra se pierde, que es lo que esta feature
// tiene permitido hacer con una medición.
func RecortarLoteDeUso(lote []EventoDeUso) []EventoDeUso {
	if len(lote) <= MaxEventosPorLote {
		return lote
	}
	return lote[:MaxEventosPorLote]
}

// CorteDeRolPermitido dice si se puede guardar el rol, o si hay que dejarlo en blanco.
//
// Con menos de dos usuarios activos de ese rol en esa empresa, decir «el rol gerente hizo estas 40
// acciones» es decir su nombre. Es la mitad que hace real la promesa de no guardar la identidad:
// sin esto, la promesa se rompe en la pantalla aunque la columna no exista.
//
// Se decide con un número y no leyendo la base para que la regla sea pura y esté en un solo lugar
// — y porque tiene que aplicarse al ESCRIBIR, que es el único momento en que no se puede deshacer.
// Quien lo intentara al leer no podría: la consola no tiene permiso para contar la plantilla de un
// cliente, y dárselo abriría la puerta que la spec 016 cerró.
func CorteDeRolPermitido(usuariosActivosDelRol int) bool {
	return usuariosActivosDelRol >= 2
}
