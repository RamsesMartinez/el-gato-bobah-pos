package app

import (
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// La zona con la que se arman los casos. Era `mxLocation`, una constante del servicio; ahora la zona
// sale de la configuración del negocio, así que el test declara la suya y deja de depender de un
// detalle que ya no existe.
var zonaDePrueba = domain.LoadBusinessLocation(domain.DefaultTimezone)

// atTime construye un pick a `daysAgo` días y a la hora `hour` (misma zona que base).
func atTime(base time.Time, daysAgo, hour int) time.Time {
	d := base.AddDate(0, 0, -daysAgo)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, base.Location())
}

func repeat(prod, grp, opt int64, t time.Time, n int) []pick {
	out := make([]pick, n)
	for i := range out {
		out[i] = pick{productID: prod, groupID: grp, optionID: opt, at: t}
	}
	return out
}

// El default debe cambiar con la hora del día: la opción de la mañana gana en la
// mañana y la de la tarde gana en la tarde, con el mismo histórico.
func TestRankDefaults_HourContext(t *testing.T) {
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, zonaDePrueba)
	var picks []pick
	picks = append(picks, repeat(1, 10, 100, atTime(base, 1, 9), 10)...)  // opción mañana
	picks = append(picks, repeat(1, 10, 101, atTime(base, 1, 20), 10)...) // opción tarde

	morning := time.Date(2026, 7, 1, 9, 0, 0, 0, zonaDePrueba)
	if got := rankDefaults(picks, morning)[1][10]; len(got) == 0 || got[0].OptionID != 100 {
		t.Fatalf("mañana: esperaba opción 100 primero, got %v", got)
	}
	evening := time.Date(2026, 7, 1, 20, 0, 0, 0, zonaDePrueba)
	if got := rankDefaults(picks, evening)[1][10]; len(got) == 0 || got[0].OptionID != 101 {
		t.Fatalf("tarde: esperaba opción 101 primero, got %v", got)
	}
}

// La recencia debe pesar más que el volumen antiguo: una opción muy elegida hace 60
// días pierde contra una elegida pocas veces ayer.
func TestRankDefaults_RecencyBeatsVolume(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, zonaDePrueba)
	var picks []pick
	picks = append(picks, repeat(2, 20, 200, atTime(now, 60, 10), 20)...) // viejo, mucho volumen
	picks = append(picks, repeat(2, 20, 201, atTime(now, 1, 10), 5)...)   // reciente, poco volumen

	got := rankDefaults(picks, now)[2][20]
	if len(got) == 0 || got[0].OptionID != 201 {
		t.Fatalf("esperaba que la opción reciente (201) ganara, got %v", got)
	}
}

// Sin umbral de soporte: aun con pocos picks se emite un default (el local es de bajo
// volumen; exigir un mínimo dejaba el recomendador mudo).
func TestRankDefaults_EmitsWithLowSupport(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, zonaDePrueba)
	picks := repeat(3, 30, 300, atTime(now, 1, 10), 2) // solo 2 picks

	got := rankDefaults(picks, now)[3][30]
	if len(got) == 0 || got[0].OptionID != 300 {
		t.Fatalf("esperaba emitir la opción 300 aun con pocos picks, got %v", got)
	}
}
