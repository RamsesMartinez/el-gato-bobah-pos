package domain

import (
	"reflect"
	"testing"
	"time"
)

// La regla de qué avisos deben estar abiertos es la misma que aplica la función de la base
// (evaluate_low_stock); este test fija la tabla de verdad que la prueba de integración compara.
func TestLowStockEffect(t *testing.T) {
	on := LowStockConfig{Enabled: true, OnMin: true, OnOut: true, Min: dp("5")}
	cases := []struct {
		name   string
		cfg    LowStockConfig
		onHand string
		open   []NotificationKind
		manual bool
		opens  []NotificationKind
		closes []NotificationKind
	}{
		{"sin la casilla nunca abre por mínimo", LowStockConfig{OnMin: true, OnOut: true, Min: dp("5")}, "2", nil, false, nil, nil},
		{"bajo el mínimo abre stock_low", on, "2", nil, false, []NotificationKind{KindStockLow}, nil},
		{"justo en el mínimo también", on, "5", nil, false, []NotificationKind{KindStockLow}, nil},
		{"ya abierto no abre otro", on, "2", []NotificationKind{KindStockLow}, false, nil, nil},
		{"en cero abre los dos", on, "0", nil, false, []NotificationKind{KindStockLow, KindStockOut}, nil},
		{"sobre el mínimo cierra", on, "6", []NotificationKind{KindStockLow}, false, nil, []NotificationKind{KindStockLow}},
		{"mínimo cero = sin mínimo: solo el agotado", LowStockConfig{Enabled: true, OnMin: true, OnOut: true, Min: dp("0")}, "0", nil, false, []NotificationKind{KindStockOut}, nil},
		{"mínimo nulo = sin mínimo", LowStockConfig{Enabled: true, OnMin: true, OnOut: true}, "1", nil, false, nil, nil},
		{"apagar cierra lo del sistema", LowStockConfig{Min: dp("5")}, "2", []NotificationKind{KindStockLow, KindStockOut}, false, nil, []NotificationKind{KindStockLow, KindStockOut}},
		{"apagar NO cierra un «se acabó» reportado a mano", LowStockConfig{Min: dp("5")}, "0", []NotificationKind{KindStockOut}, true, nil, nil},
		{"el manual se cierra cuando vuelve a haber", LowStockConfig{}, "3", []NotificationKind{KindStockOut}, true, nil, []NotificationKind{KindStockOut}},
		{"sin avisar al mínimo, solo el agotado", LowStockConfig{Enabled: true, OnOut: true, Min: dp("5")}, "2", nil, false, nil, nil},
		{"negativa cuenta como agotada", on, "-3", nil, false, []NotificationKind{KindStockLow, KindStockOut}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := LowStockEffect(c.cfg, d(c.onHand), c.open, c.manual)
			if !reflect.DeepEqual(e.Opens, orEmpty(c.opens)) || !reflect.DeepEqual(e.Closes, orEmpty(c.closes)) {
				t.Fatalf("abre %v cierra %v; se esperaba abre %v cierra %v", e.Opens, e.Closes, c.opens, c.closes)
			}
		})
	}
}

func orEmpty(k []NotificationKind) []NotificationKind {
	if k == nil {
		return []NotificationKind{}
	}
	return k
}

func TestNeedsCount(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	hace := func(dias int) *time.Time { v := now.AddDate(0, 0, -dias); return &v }
	cases := []struct {
		name   string
		onHand string
		last   *time.Time
		want   bool
	}{
		{"negativa pide conteo aunque se haya contado ayer", "-1", hace(1), true},
		{"nunca contado pide conteo", "3", nil, true},
		{"contado hace 29 días no", "3", hace(29), false},
		{"contado hace 31 días sí", "3", hace(31), true},
	}
	for _, c := range cases {
		if got := NeedsCount(d(c.onHand), c.last, now); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

func TestEmailWindow(t *testing.T) {
	at := time.Date(2026, 11, 14, 20, 37, 59, 0, time.FixedZone("CST", -6*3600))
	start := EmailWindowStart(at)
	if want := time.Date(2026, 11, 15, 2, 35, 0, 0, time.UTC); !start.Equal(want) || start.Location() != time.UTC {
		t.Fatalf("bloque %v, se esperaba %v en UTC", start, want)
	}
	if EmailWindowClosed(start, start.Add(4*time.Minute+59*time.Second)) {
		t.Fatal("el bloque se dio por cerrado antes de los 5 minutos")
	}
	if !EmailWindowClosed(start, start.Add(5*time.Minute)) {
		t.Fatal("el bloque no cerró a los 5 minutos")
	}
	if EmailWindowKey(start) != "2026-11-15T02:35Z" {
		t.Fatalf("llave %q", EmailWindowKey(start))
	}
}

func TestEmailCooldown(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	if InEmailCooldown(nil, now) {
		t.Fatal("sin correo previo no hay enfriamiento")
	}
	hace := func(h float64) *time.Time { v := now.Add(-time.Duration(h * float64(time.Hour))); return &v }
	if !InEmailCooldown(hace(11.9), now) {
		t.Fatal("a las 11.9 h sigue en enfriamiento")
	}
	if InEmailCooldown(hace(12), now) {
		t.Fatal("a las 12 h ya se puede volver a mandar")
	}
}

func TestMorningSummaryDue(t *testing.T) {
	loc, _ := time.LoadLocation("America/Mexico_City")
	at := "08:00"
	cases := []struct {
		name string
		now  time.Time
		due  bool
		date string
	}{
		{"antes de la hora no", time.Date(2026, 11, 14, 7, 59, 0, 0, loc), false, "2026-11-14"},
		{"a la hora sí", time.Date(2026, 11, 14, 8, 0, 0, 0, loc), true, "2026-11-14"},
		{"en la tarde sigue tocando (el envío decide si ya salió)", time.Date(2026, 11, 14, 18, 0, 0, 0, loc), true, "2026-11-14"},
		// 01:30 UTC del 15 = 19:30 del 14 en el centro: la fecha es la del negocio, no la de UTC.
		{"la fecha es la del negocio", time.Date(2026, 11, 15, 1, 30, 0, 0, time.UTC), true, "2026-11-14"},
	}
	for _, c := range cases {
		date, due, err := MorningSummaryDue(c.now, loc, at)
		if err != nil || due != c.due || date != c.date {
			t.Errorf("%s: %s %v %v", c.name, date, due, err)
		}
	}
	if _, _, err := MorningSummaryDue(time.Now(), loc, "8am"); err == nil {
		t.Fatal("una hora mal escrita no puede caer a un default")
	}
}

func TestValidMorningTime(t *testing.T) {
	for in, ok := range map[string]bool{"08:00": true, "23:45": true, "08:10": false, "24:00": false, "8:00": false, "": false} {
		if (ValidMorningTime(in) == nil) != ok {
			t.Errorf("%q: %v", in, ValidMorningTime(in))
		}
	}
}

func TestSearchKeys(t *testing.T) {
	cases := map[string][]string{
		"Quesos":          {"queso"},
		"  platos UNICEL": {"plato", "unicel"},
		"bolsas grandes":  {"bolsa", "grand"},
		"té":              {"te"},
		"gas":             {"gas"},
		"":                {},
	}
	for in, want := range cases {
		if got := SearchKeys(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q → %v, se esperaba %v", in, got, want)
		}
	}
}

func TestValidNotFoundText(t *testing.T) {
	if _, err := CleanNotFoundText("   "); err == nil {
		t.Fatal("un texto vacío no es un aviso")
	}
	if got, err := CleanNotFoundText("  servi   toallas "); err != nil || got != "servi toallas" {
		t.Fatalf("%q %v", got, err)
	}
	long := make([]rune, 81)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := CleanNotFoundText(string(long)); err == nil {
		t.Fatal("más de 80 caracteres debe rechazarse")
	}
}

func TestReportLimit(t *testing.T) {
	if ReportLimitReached(19) {
		t.Fatal("19 en la hora todavía deja reportar")
	}
	if !ReportLimitReached(20) {
		t.Fatal("con 20 en la hora se detiene")
	}
}

func TestUndoWindow(t *testing.T) {
	at := time.Date(2026, 11, 14, 20, 0, 0, 0, time.UTC)
	if !CanUndoOutReport(at, at.Add(90*time.Second)) {
		t.Fatal("a los 90 s todavía se deshace")
	}
	if CanUndoOutReport(at, at.Add(91*time.Second)) {
		t.Fatal("a los 91 s ya no")
	}
}
