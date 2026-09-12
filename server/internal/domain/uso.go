package domain

import (
	"fmt"
	"sort"
	"time"
)

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
//
// SOLO LO QUE ALGUIEN DISPARA HOY. La primera versión de esta lista tenía doce acciones y el front
// enganchaba cuatro: las otras ocho habrían salido en el mapa como ceros permanentes, que se leen
// como «nadie la usa» y no como «nadie la midió» — el mismo modo de falla que el comentario de
// arriba describe para las pantallas. Agregar una es una línea aquí y una llamada allá, juntas.
var accionesMedibles = map[string]map[string]struct{}{
	"pos":      {"cobrar": {}},
	"pedidos":  {"cobrar": {}},
	"caja":     {"abrir-turno": {}, "cerrar-turno": {}, "contar-efectivo": {}, "traspaso": {}},
	"catalogo": {"editar-producto": {}},
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

// rolesPorPantalla es el ESPEJO de lo que el router deja abrir a cada rol.
//
// Sin esto, la lista blanca acota el conjunto de valores pero no su coherencia: un mesero con su
// propio token puede reportar treinta aperturas por minuto de la pantalla de usuarios —que un GET
// suyo recibiría con 403— y el mapa diría que es la más usada del sistema. No es una fuga de datos:
// es una feature de medición que se puede llenar de mentiras desde adentro, y lo que mide deja de
// servir para decidir.
//
// Es una COPIA de lo que `RequireRole` impone en el router y hay que moverla con él. El test
// `TestTodaPantallaMedibleTieneRoles` impide la mitad barata de la desincronización —una pantalla
// nueva sin roles— y el resto vive en esta nota.
var rolesPorPantalla = map[string][]Role{
	"pos":        {RoleAdmin, RoleGerente, RoleCajero, RoleMesero},
	"pedidos":    {RoleAdmin, RoleGerente, RoleCajero, RoleMesero},
	"cuenta":     {RoleAdmin, RoleGerente, RoleCajero, RoleMesero},
	"caja":       {RoleAdmin, RoleGerente, RoleCajero},
	"ventas":     {RoleAdmin, RoleGerente},
	"reportes":   {RoleAdmin, RoleGerente},
	"gastos":     {RoleAdmin, RoleGerente},
	"inventario": {RoleAdmin, RoleGerente},
	"catalogo":   {RoleAdmin, RoleGerente},
	"usuarios":   {RoleAdmin},
	"negocio":    {RoleAdmin},
	"impresion":  {RoleAdmin, RoleGerente},
}

// PantallaPermitidaParaRol dice si ese rol puede siquiera abrir esa pantalla.
func PantallaPermitidaParaRol(pantalla string, rol Role) bool {
	roles, ok := rolesPorPantalla[pantalla]
	if !ok {
		return false
	}
	return rol.In(roles...)
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
// Descarta lo que no está en la lista blanca Y lo que ese rol no podría haber abierto: un evento
// incoherente con quien lo manda es una mentira, y una medición llena de mentiras no sirve para
// decidir, que es lo único para lo que existe.
func PreAgregarUso(lote []EventoDeUso, rol Role) []UsoAgregado {
	if len(lote) == 0 {
		return nil
	}
	type llave struct{ pantalla, accion string }
	// El orden importa para que el resultado sea estable entre corridas: sin él, dos lotes iguales
	// producen `update`s en orden distinto y dos transacciones concurrentes pueden interbloquearse.
	orden := make([]llave, 0, len(lote))
	veces := make(map[llave]int, len(lote))
	for _, e := range lote {
		if !EventoDeUsoValido(e.Pantalla, e.Accion) || !PantallaPermitidaParaRol(e.Pantalla, rol) {
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

// DecisionDeCorte dice qué se puede escribir de una medición sin señalar a una persona.
type DecisionDeCorte int

const (
	// GuardarConRol: ese rol tiene suficiente gente como para que el conteo no sea de nadie.
	GuardarConRol DecisionDeCorte = iota
	// GuardarSinCorte: el rol no se escribe, pero la medición sí — el balde de lo suprimido tapa
	// a más de una persona.
	GuardarSinCorte
	// NoGuardar: ni siquiera sin rol. Escribirla sería escribir a una persona.
	NoGuardar
)

// MinimoParaCortar: por debajo de dos personas, un conteo es de alguien.
const MinimoParaCortar = 2

// CortarPorRol decide qué se guarda de una medición hecha por `rol`, mirando la plantilla ENTERA.
//
// La regla obvia —«si ese rol tiene menos de dos activos, déjalo en blanco»— tiene un agujero que
// costó una auditoría: la supresión se decide rol por rol, pero **todo lo suprimido cae en el mismo
// balde `role is null`**. Cuando exactamente un rol queda por debajo del umbral, ese balde ES esa
// persona, y la consola lo pinta con la etiqueta «sin corte», que promete justo lo contrario.
//
// El caso no es hipotético: con 1 admin, 2 gerentes, 3 cajeros y 2 meseros —la plantilla típica de
// un local— el gerente, el cajero y el mesero se guardan con su rol, y `null` es el dueño.
//
// Por eso hay un tercer resultado: cuando el balde no alcanza a tapar a nadie, la medición **se
// pierde**. Es lo que esta feature tiene permitido hacer; escribir un nombre no lo es.
func CortarPorRol(activosPorRol map[Role]int, rol Role) DecisionDeCorte {
	if !RolMedible(rol) {
		return NoGuardar
	}
	if activosPorRol[rol] >= MinimoParaCortar {
		return GuardarConRol
	}
	// Cuánta gente cae en el balde de lo suprimido: la suma de los roles que tampoco se pueden
	// cortar. Un rol SIN nadie activo aporta cero — no hay a quién tapar con él.
	enElBalde := 0
	for _, activos := range activosPorRol {
		if activos > 0 && activos < MinimoParaCortar {
			enElBalde += activos
		}
	}
	if enElBalde >= MinimoParaCortar {
		return GuardarSinCorte
	}
	return NoGuardar
}

// PantallasMedibles devuelve la lista blanca, ordenada.
//
// La usa el mapa para que las pantallas que NADIE abrió aparezcan en cero: «qué no usa nadie» es la
// mitad de la pregunta, y una pantalla que se omite por no tener filas se lee como que no existe.
func PantallasMedibles() []string {
	nombres := make([]string, 0, len(pantallasMedibles))
	for n := range pantallasMedibles {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	return nombres
}

// RangoDeUsoValido rechaza lo que no se puede contestar.
//
// Un `desde` más viejo que la retención NO se recorta en silencio a lo que hay: devolver otro rango
// del que se pidió es una pantalla que miente, y nadie la audita porque se ve bien (principio V).
func RangoDeUsoValido(desde, hasta time.Time, retencionEnDias int) error {
	if desde.IsZero() || hasta.IsZero() {
		return fmt.Errorf("%w: falta el periodo", ErrValidation)
	}
	if hasta.Before(desde) {
		return fmt.Errorf("%w: el periodo termina antes de empezar", ErrValidation)
	}
	masViejoPosible := time.Now().AddDate(0, 0, -retencionEnDias)
	if desde.Before(masViejoPosible.Truncate(24 * time.Hour)) {
		return fmt.Errorf("%w: solo se conservan %d días de uso", ErrValidation, retencionEnDias)
	}
	return nil
}
