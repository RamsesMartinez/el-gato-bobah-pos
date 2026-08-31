package domain

import (
	"errors"
	"testing"
	"time"
)

func mx(t *testing.T) *time.Location {
	t.Helper()
	return LoadBusinessLocation(DefaultTimezone)
}

// El rango de la pantalla se resuelve en la zona del NEGOCIO, no en UTC.
//
// Es el mismo defecto que ya reinició folios a media cena: a las 19:00 de México son las 01:00 del
// día siguiente en UTC, así que "hoy" calculado en UTC devuelve el día equivocado justo en la hora
// de más venta — y el operador ve una pantalla que se ve bien y reporta el día de mañana.
func TestResolveRangeUsaLaZonaDelNegocio(t *testing.T) {
	loc := mx(t)
	// 30 de agosto 19:00 en México = 31 de agosto 01:00 UTC.
	ahora := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)

	r, err := ResolveRange("hoy", time.Time{}, time.Time{}, ahora, loc)
	if err != nil {
		t.Fatalf("hoy: %v", err)
	}
	if r.From.Format(dateOnly) != "2026-08-30" || r.To.Format(dateOnly) != "2026-08-30" {
		t.Fatalf("hoy = %s..%s, quiere 2026-08-30 en los dos: el rango se corrió a UTC",
			r.From.Format(dateOnly), r.To.Format(dateOnly))
	}
}

func TestResolveRangePresets(t *testing.T) {
	loc := mx(t)
	// Miércoles 26 de agosto de 2026, 15:00 en México.
	ahora := time.Date(2026, 8, 26, 21, 0, 0, 0, time.UTC)

	casos := []struct {
		preset, from, to string
	}{
		{"hoy", "2026-08-26", "2026-08-26"},
		{"ayer", "2026-08-25", "2026-08-25"},
		// La semana arranca en LUNES: es como cuenta la semana quien opera el negocio, y con
		// domingo el "esta semana" del lunes en la mañana saldría vacío.
		{"semana", "2026-08-24", "2026-08-26"},
		{"mes", "2026-08-01", "2026-08-26"},
	}
	for _, c := range casos {
		t.Run(c.preset, func(t *testing.T) {
			r, err := ResolveRange(c.preset, time.Time{}, time.Time{}, ahora, loc)
			if err != nil {
				t.Fatalf("%s: %v", c.preset, err)
			}
			if got := r.From.Format(dateOnly); got != c.from {
				t.Fatalf("from = %s, quiere %s", got, c.from)
			}
			if got := r.To.Format(dateOnly); got != c.to {
				t.Fatalf("to = %s, quiere %s", got, c.to)
			}
		})
	}
}

// Un preset desconocido NO cae a "hoy" en silencio. Es la regla que el resto de la frontera ya
// aplica: una pantalla que se ve correcta y reporta un rango que nadie pidió es peor que un error,
// porque nadie la audita.
func TestUnPresetDesconocidoSeRechaza(t *testing.T) {
	if _, err := ResolveRange("el-mes-pasado-pero-solo-martes", time.Time{}, time.Time{}, time.Now(), mx(t)); !errors.Is(err, ErrValidation) {
		t.Fatalf("un preset inventado debe rechazarse, fue %v", err)
	}
}

func TestRangoLibre(t *testing.T) {
	loc := mx(t)
	d := func(s string) time.Time {
		v, err := time.ParseInLocation(dateOnly, s, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	r, err := ResolveRange("rango", d("2026-08-01"), d("2026-08-15"), time.Now(), loc)
	if err != nil {
		t.Fatalf("rango: %v", err)
	}
	if r.From.Format(dateOnly) != "2026-08-01" || r.To.Format(dateOnly) != "2026-08-15" {
		t.Fatalf("rango = %s..%s", r.From.Format(dateOnly), r.To.Format(dateOnly))
	}

	// Invertido: devolvería CERO filas en silencio y el operador creería que no vendió.
	if _, err := ResolveRange("rango", d("2026-08-15"), d("2026-08-01"), time.Now(), loc); !errors.Is(err, ErrValidation) {
		t.Fatalf("un rango invertido debe rechazarse, fue %v", err)
	}

	// Sin cota, un rango de años escanea sin límite en el gigabyte del VPS.
	if _, err := ResolveRange("rango", d("2020-01-01"), d("2026-08-15"), time.Now(), loc); !errors.Is(err, ErrValidation) {
		t.Fatalf("un rango de años debe rechazarse, fue %v", err)
	}
}

func TestSalesFilterValidate(t *testing.T) {
	base := func() SalesFilter {
		return SalesFilter{Sort: "fecha", Dir: "desc", Limit: 20}
	}
	casos := []struct {
		nombre string
		f      func(*SalesFilter)
		quiere bool // true = válido
	}{
		{"el default es válido", func(*SalesFilter) {}, true},
		{"sin estado son todos", func(f *SalesFilter) { f.Status = "" }, true},
		{"un estado real pasa", func(f *SalesFilter) { f.Status = StatusEntregada }, true},
		{"un estado inventado se rechaza", func(f *SalesFilter) { f.Status = "pagando" }, false},
		{"un tipo de venta real pasa", func(f *SalesFilter) { f.ServiceType = "domicilio" }, true},
		{"un tipo inventado se rechaza", func(f *SalesFilter) { f.ServiceType = "mesas" }, false},
		// Sin whitelist, un `sort` desconocido se ignora en silencio y la tabla sale ordenada por
		// otra cosa que la que dice el encabezado.
		{"una columna de orden inventada se rechaza", func(f *SalesFilter) { f.Sort = "total; drop table" }, false},
		{"una dirección inventada se rechaza", func(f *SalesFilter) { f.Dir = "arriba" }, false},
		{"un tamaño de página absurdo se rechaza", func(f *SalesFilter) { f.Limit = 100000 }, false},
		{"página negativa se rechaza", func(f *SalesFilter) { f.Offset = -1 }, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := base()
			c.f(&f)
			err := f.Validate()
			if c.quiere && err != nil {
				t.Fatalf("debía ser válido, fue %v", err)
			}
			if !c.quiere && !errors.Is(err, ErrValidation) {
				t.Fatalf("debía rechazarse como validación, fue %v", err)
			}
		})
	}
}

// El resumen clasifica cada peso UNA sola vez.
//
// La propina es dinero del personal que pasa por la caja, no ingreso del negocio. La cancelada y la
// reembolsada son ingreso que NO ocurrió. El envío ya está DENTRO del total del pedido. Un resumen
// que los pone como renglones hermanos invita a sumarlos y a inflar la venta del día — la misma
// forma del fondo de caja que se contó una vez por método y dejó $4,500 de faltante sin explicar.
func TestSummarizeSalesClasificaCadaPesoUnaVez(t *testing.T) {
	filas := []StatusTotals{
		{Status: StatusEntregada, Count: 2, Total: d("300"), Tips: d("15"), DeliveryFee: d("20")},
		{Status: StatusCancelada, Count: 1, Total: d("50")},
		{Status: StatusReembolsada, Count: 1, Total: d("80")},
		{Status: StatusAbierta, Count: 1, Total: d("30")},
	}

	s := SummarizeSales(filas)

	// 100 + 200 + 30: lo entregado y lo abierto. Lo cancelado y lo reembolsado NO son ingreso.
	if !s.Total.Equal(d("330")) {
		t.Fatalf("total = %s, quiere 330: se coló una cancelada o una reembolsada", s.Total)
	}
	if !s.Tips.Equal(d("15")) {
		t.Fatalf("propinas = %s, quiere 15", s.Tips)
	}
	// La propina NO está dentro del total: si lo estuviera, el negocio se estaría contando como
	// ingreso el dinero del personal.
	if s.Total.Add(s.Tips).Equal(s.Total) {
		t.Fatal("propinas y total no pueden ser el mismo número")
	}
	if s.Cancelled.Count != 1 || !s.Cancelled.Amount.Equal(d("50")) {
		t.Fatalf("canceladas = %+v, quiere 1 por 50", s.Cancelled)
	}
	if s.Refunded.Count != 1 || !s.Refunded.Amount.Equal(d("80")) {
		t.Fatalf("reembolsadas = %+v, quiere 1 por 80", s.Refunded)
	}
	// El envío ya viene DENTRO de total: viaja aparte solo como referencia, nunca para sumarse.
	if !s.DeliveryFees.Equal(d("20")) {
		t.Fatalf("envíos = %s, quiere 20", s.DeliveryFees)
	}
	// El conteo cuenta las ventas que suman, no todas las filas.
	if s.Count != 3 {
		t.Fatalf("conteo = %d, quiere 3 (entregadas + abierta)", s.Count)
	}
	if !s.Average.Equal(d("110")) {
		t.Fatalf("promedio = %s, quiere 110 (330/3)", s.Average)
	}
}

// Un rango sin ventas no puede reventar: dividir entre cero es la forma más tonta de tumbar una
// pantalla de reportes, y pasa el primer día que alguien abre "ayer" en un día que no se abrió.
func TestSummarizeSalesSinVentas(t *testing.T) {
	s := SummarizeSales(nil)
	if s.Count != 0 || !s.Total.IsZero() || !s.Average.IsZero() {
		t.Fatalf("resumen vacío = %+v, quiere todo en cero", s)
	}
}

// El promedio se redondea a dos decimales como cualquier otro peso de la frontera: 100/3 son
// 33.3333… y una columna numeric(10,2) los rechazaría.
func TestElPromedioSeRedondea(t *testing.T) {
	s := SummarizeSales([]StatusTotals{{Status: StatusEntregada, Count: 3, Total: d("100.02")}})
	if !s.Average.Equal(d("33.34")) {
		t.Fatalf("promedio = %s, quiere 33.34", s.Average)
	}
}
