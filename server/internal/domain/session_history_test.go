package domain

import (
	"errors"
	"testing"
	"time"
)

// El histórico de cortes se pagina y se puede acotar por día. Un rango al revés o una página fuera
// de tope se rechazan: caer a «los más recientes» en silencio devolvería una lista que se ve bien y
// no es la que se pidió, que es justo como un corte viejo se quedaba sin poder abrirse.
func TestSessionHistoryFilterValidate(t *testing.T) {
	d := func(s string) *time.Time {
		v, _ := time.Parse("2006-01-02", s)
		return &v
	}
	cases := []struct {
		name string
		f    SessionHistoryFilter
		ok   bool
	}{
		{"sin rango", SessionHistoryFilter{Limit: 20}, true},
		{"solo desde", SessionHistoryFilter{From: d("2026-09-01"), Limit: 20}, true},
		{"solo hasta", SessionHistoryFilter{To: d("2026-09-01"), Limit: 20}, true},
		{"un solo día", SessionHistoryFilter{From: d("2026-09-01"), To: d("2026-09-01"), Limit: 20}, true},
		{"rango al revés", SessionHistoryFilter{From: d("2026-09-02"), To: d("2026-09-01"), Limit: 20}, false},
		{"página de cero filas", SessionHistoryFilter{Limit: 0}, false},
		{"página enorme", SessionHistoryFilter{Limit: MaxSalesPageSize + 1}, false},
		{"desplazamiento negativo", SessionHistoryFilter{Limit: 20, Offset: -1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.f.Validate()
			if c.ok && err != nil {
				t.Fatalf("= %v, quería válido", err)
			}
			if !c.ok && !errors.Is(err, ErrValidation) {
				t.Fatalf("= %v, quería ErrValidation", err)
			}
		})
	}
}
