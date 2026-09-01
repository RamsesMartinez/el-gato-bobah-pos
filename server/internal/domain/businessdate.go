package domain

import "time"

// DefaultTimezone es la zona con la que nace un negocio. El producto se vende en México y el local
// que lo estrena no debería tener que configurar nada para que su primer corte cuadre.
const DefaultTimezone = "America/Mexico_City"

// BusinessDate devuelve el día de negocio de un instante, en la zona del local.
//
// La base guarda todo en UTC (timestamptz) y eso está bien: un instante es un instante. Lo que NO
// puede salir en UTC es la FECHA, que es una decisión de calendario y depende de dónde está el
// negocio. Con el servidor en UTC, la medianoche cae a las 18:00 en México: todo lo vendido de las
// 6pm en adelante se contaba en el día siguiente —justo la franja donde más vende un lugar de
// comida— y el folio diario se reiniciaba a media cena, dejando dos tickets #1 en la misma noche.
//
// Se devuelve como medianoche EN UTC del día local, que es lo que una columna `date` de Postgres
// espera recibir sin que pgx la desplace.
func BusinessDate(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// LoadBusinessLocation resuelve el nombre IANA de una zona, cayendo a UTC si no existe.
//
// Cae en vez de fallar a propósito: esta función está en el camino de una venta, y un negocio con
// la zona mal escrita prefiere una fecha corrida a no poder cobrar. La validación de que el nombre
// sea real va en la frontera donde se GUARDA la configuración, no donde se usa.
func LoadBusinessLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ValidTimezone dice si un nombre IANA existe. Es lo que usa la frontera al guardar la
// configuración del local, para que un nombre inventado se rechace ahí y no se descubra meses
// después con los cortes corridos.
func ValidTimezone(name string) bool {
	if name == "" {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// Los modos de corte de la VISTA: hasta cuándo se ve en pantalla un pedido ya entregado.
//
// No tienen nada que ver con el día al que pertenece una venta. Ese lo decide el turno y lo guarda
// `orders.business_date`; esto solo decide qué se sigue mostrando.
const (
	// CorteMedianoche: la medianoche del día local. Es el default porque es lo que un operador
	// espera sin que nadie se lo explique, y el único que no depende de que alguien se acuerde de
	// cerrar la caja.
	CorteMedianoche = "medianoche"
	// CorteTurno: desde que abrió el turno vigente.
	CorteTurno = "turno"
	// CorteCierreDeCaja: desde el último cierre.
	CorteCierreDeCaja = "cierre_de_caja"
)

// CorteDeVistaValido dice si el modo es uno de los tres. La frontera lo usa para rechazar cualquier
// otro: un valor desconocido que caiga al default en silencio deja la pantalla mostrando algo que
// nadie configuró.
func CorteDeVistaValido(modo string) bool {
	return modo == CorteMedianoche || modo == CorteTurno || modo == CorteCierreDeCaja
}

// DesdeCuandoSeVen devuelve el instante a partir del cual un pedido entregado sigue en pantalla.
//
// La medianoche se calcula EN la zona, nunca restando 24 horas al día anterior. México quitó el
// horario de verano en 2022, pero `America/Tijuana` sigue cambiando —va con la costa oeste de
// Estados Unidos— y está en la lista de zonas que el producto ofrece: ese día la distancia entre dos
// medianoches es de 23 o 25 horas, y un cálculo que resta 24 se desfasa justo cuando nadie mira.
//
// Un modo desconocido se comporta como el default en vez de devolver un cero que mostraría todo el
// histórico. La frontera ya rechaza los valores inválidos; esto es la última red.
func DesdeCuandoSeVen(modo string, ahora time.Time, zona *time.Location, abrioElTurno, cerroLaCaja time.Time) time.Time {
	switch modo {
	case CorteTurno:
		return abrioElTurno
	case CorteCierreDeCaja:
		return cerroLaCaja
	}
	if zona == nil {
		zona = LoadBusinessLocation(DefaultTimezone)
	}
	y, m, d := ahora.In(zona).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, zona)
}
