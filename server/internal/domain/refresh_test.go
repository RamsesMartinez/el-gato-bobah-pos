package domain

import (
	"testing"
	"time"
)

func TestClassifyRefresh(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	hour := time.Hour
	cases := []struct {
		name      string
		revoked   bool
		expiresAt time.Time
		want      RefreshVerdict
	}{
		{"vivo y sin revocar", false, now.Add(hour), RefreshValid},
		{"vencido normal", false, now.Add(-hour), RefreshExpired},
		{"revocado y reusado", true, now.Add(hour), RefreshReused},
		// Un revocado que además venció sigue siendo señal de reuso: la revocación gana,
		// porque que reaparezca un token revocado indica robo/rotación, no expiración.
		{"revocado gana a vencido", true, now.Add(-hour), RefreshReused},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyRefresh(c.revoked, c.expiresAt, now); got != c.want {
				t.Fatalf("ClassifyRefresh(%v, exp, now) = %v, want %v", c.revoked, got, c.want)
			}
		})
	}
}
