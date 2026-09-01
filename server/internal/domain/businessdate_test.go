package domain

import (
	"testing"
	"time"
)

// El día de negocio se calcula en la zona del local, no en UTC. Sonaba a detalle y partía las
// noches en dos: con el servidor en UTC, la medianoche cae a las 18:00 en México, así que todo lo
// vendido de las 6pm en adelante se contaba en el día siguiente — justo la franja donde más vende
// un lugar de comida — y el folio diario se reiniciaba a media cena.
func TestBusinessDate(t *testing.T) {
	mx, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("zona de México: %v", err)
	}
	casos := []struct {
		nombre string
		utc    string
		loc    *time.Location
		quiere string
	}{
		// El caso real que lo destapó: 02:28 UTC del 30 son las 20:28 del 29 en México.
		{"cena, ya pasada la medianoche UTC", "2026-08-30T02:28:00Z", mx, "2026-08-29"},
		{"tarde, antes del corte UTC", "2026-08-29T23:36:00Z", mx, "2026-08-29"},
		{"justo en la medianoche de México", "2026-08-30T06:00:00Z", mx, "2026-08-30"},
		{"un minuto antes de la medianoche local", "2026-08-30T05:59:00Z", mx, "2026-08-29"},
		{"madrugada local", "2026-08-30T08:00:00Z", mx, "2026-08-30"},
		// Sin zona (nil) cae a UTC: es el comportamiento viejo y sirve de referencia.
		{"sin zona cae a UTC", "2026-08-30T02:28:00Z", nil, "2026-08-30"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			momento, err := time.Parse(time.RFC3339, c.utc)
			if err != nil {
				t.Fatal(err)
			}
			got := BusinessDate(momento, c.loc).Format("2006-01-02")
			if got != c.quiere {
				t.Fatalf("BusinessDate(%s) = %s, quería %s", c.utc, got, c.quiere)
			}
		})
	}
}

// Una zona inválida no puede tumbar una venta: se cae a UTC y el pedido entra. Un negocio con la
// configuración mal escrita prefiere una fecha corrida a no poder cobrar.
func TestLoadBusinessLocation(t *testing.T) {
	if loc := LoadBusinessLocation("America/Mexico_City"); loc == nil || loc.String() != "America/Mexico_City" {
		t.Fatalf("una zona válida debe cargarse, dio %v", loc)
	}
	if loc := LoadBusinessLocation("Marte/Olympus"); loc != time.UTC {
		t.Fatalf("una zona inválida debe caer a UTC, dio %v", loc)
	}
	if loc := LoadBusinessLocation(""); loc != time.UTC {
		t.Fatalf("vacío debe caer a UTC, dio %v", loc)
	}
}

// DESDE CUÁNDO SE VEN LOS ENTREGADOS, SEGÚN EL MODO QUE ELIGIÓ EL NEGOCIO.
//
// El caso que decide el diseño es el del CAMBIO DE HORARIO. México quitó el horario de verano en
// 2022, pero `America/Tijuana` sigue cambiando —va alineada con la costa oeste de Estados Unidos— y
// está en la lista de zonas que el producto ofrece. Ese día la distancia entre dos medianoches es de
// 23 o 25 horas, así que un cálculo que reste 24 se desfasa exactamente cuando nadie lo está
// mirando. La medianoche se calcula EN la zona, no restando horas.
func TestDesdeCuandoSeVen(t *testing.T) {
	mx, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("cargar la zona: %v", err)
	}
	tj, err := time.LoadLocation("America/Tijuana")
	if err != nil {
		t.Fatalf("cargar Tijuana: %v", err)
	}
	abrioElTurno := time.Date(2026, 9, 1, 22, 0, 0, 0, time.UTC) // 16:00 en México
	cerroLaCaja := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)   // 22:00 del día anterior en México

	casos := []struct {
		nombre string
		modo   string
		ahora  time.Time
		zona   *time.Location
		quiere time.Time
	}{
		{
			// 23:00 del 1 de septiembre en México = 05:00 UTC del 2. El corte es la medianoche del 1,
			// no la del 2: en UTC ya cambió el día y por eso la lista se vaciaba a media hora pico.
			"medianoche: la del día LOCAL, no la del servidor",
			CorteMedianoche,
			time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC),
			mx,
			time.Date(2026, 9, 1, 0, 0, 0, 0, mx),
		},
		{
			// 2026-11-01 es el domingo del cambio de horario en Tijuana: a las 02:00 el reloj vuelve
			// a la 01:00, así que ese día dura 25 horas. Restar 24 daría las 01:00 del día anterior.
			"cambio de horario: la medianoche real, no 24 horas antes",
			CorteMedianoche,
			time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC),
			tj,
			time.Date(2026, 11, 1, 0, 0, 0, 0, tj),
		},
		{
			"turno: desde que abrió",
			CorteTurno,
			time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC),
			mx,
			abrioElTurno,
		},
		{
			"cierre de caja: desde el último cierre",
			CorteCierreDeCaja,
			time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC),
			mx,
			cerroLaCaja,
		},
		{
			// Un modo que no existe no puede caer al más permisivo en silencio: se comporta como el
			// default, que es lo que el negocio vería si abriera la pantalla.
			"un modo desconocido se comporta como el default",
			"lo-que-sea",
			time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC),
			mx,
			time.Date(2026, 9, 1, 0, 0, 0, 0, mx),
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := DesdeCuandoSeVen(c.modo, c.ahora, c.zona, abrioElTurno, cerroLaCaja)
			if !got.Equal(c.quiere) {
				t.Errorf("DesdeCuandoSeVen = %s, quiere %s", got.Format(time.RFC3339), c.quiere.Format(time.RFC3339))
			}
		})
	}
}
