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
