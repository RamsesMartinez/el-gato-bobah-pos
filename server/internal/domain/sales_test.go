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

// "30d" es la ventana con la que nace la pantalla de Reportes, y son TREINTA días contando hoy.
//
// Antes el handler la armaba con `to.AddDate(0,0,-30)`, que son treinta y un días: el encabezado
// decía "últimos 30 días" y la tabla sumaba uno más. La diferencia es chica y por eso nadie la ve —
// que es justamente lo que la vuelve peligrosa cuando alguien compara dos periodos "de 30 días".
func TestElPresetDeTreintaDiasSonTreintaDiasContandoHoy(t *testing.T) {
	loc := mx(t)
	ahora := time.Date(2026, 8, 26, 21, 0, 0, 0, time.UTC) // 26 de agosto, 15:00 en México.

	r, err := ResolveRange("30d", time.Time{}, time.Time{}, ahora, loc)
	if err != nil {
		t.Fatalf("30d: %v", err)
	}
	if got := r.From.Format(dateOnly); got != "2026-07-28" {
		t.Fatalf("from = %s, quiere 2026-07-28", got)
	}
	if got := r.To.Format(dateOnly); got != "2026-08-26" {
		t.Fatalf("to = %s, quiere 2026-08-26", got)
	}
	if dias := int(r.To.Sub(r.From).Hours()/24) + 1; dias != 30 {
		t.Fatalf("el rango mide %d días, quiere 30", dias)
	}
}

// UNA FECHA QUE SE MANDA Y NO SE USA ES UNA PANTALLA QUE MIENTE.
//
// `from`/`to` solo significan algo con `preset=rango`. Con cualquier otro preset se descartaban en
// silencio: `?preset=hoy&from=2026-01-01&to=2026-01-31` contestaba HOY con la pantalla viéndose
// perfecta, que es el mismo modo de falla que el principio V nombra para un parámetro malformado.
// Un parámetro presente que no se puede atender se rechaza.
func TestUnasFechasQueElPresetNoVaAUsarSeRechazan(t *testing.T) {
	loc := mx(t)
	ahora := time.Date(2026, 8, 26, 21, 0, 0, 0, time.UTC)
	ene := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, preset := range []string{"", "hoy", "ayer", "semana", "mes", "30d"} {
		t.Run("preset="+preset, func(t *testing.T) {
			if _, err := ResolveRange(preset, ene, ene, ahora, loc); !errors.Is(err, ErrValidation) {
				t.Fatalf("preset %q con fechas: err = %v, quiere ErrValidation", preset, err)
			}
			// Una sola de las dos también: media fecha ignorada engaña igual que dos.
			if _, err := ResolveRange(preset, ene, time.Time{}, ahora, loc); !errors.Is(err, ErrValidation) {
				t.Fatalf("preset %q con solo from: err = %v, quiere ErrValidation", preset, err)
			}
			if _, err := ResolveRange(preset, time.Time{}, ene, ahora, loc); !errors.Is(err, ErrValidation) {
				t.Fatalf("preset %q con solo to: err = %v, quiere ErrValidation", preset, err)
			}
		})
	}
}

// UN DÍA QUE NO HA PASADO NO TIENE VENTAS, y un rango que lo incluye devuelve una pantalla corta o
// vacía que se lee como "no vendimos nada".
//
// La pantalla topa el calendario con el día de hoy, pero un tope de calendario no impide teclear la
// fecha: el navegador marca el campo como inválido y ya. La barrera real es esta.
func TestUnRangoQueTerminaEnElFuturoSeRechaza(t *testing.T) {
	loc := mx(t)
	ahora := time.Date(2026, 9, 3, 21, 0, 0, 0, time.UTC) // 3 de septiembre, 15:00 en México.
	d := func(s string) time.Time {
		v, err := time.ParseInLocation(dateOnly, s, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	if _, err := ResolveRange("rango", d("2026-09-01"), d("2026-10-01"), ahora, loc); !errors.Is(err, ErrValidation) {
		t.Fatalf("un rango que termina en el futuro debe rechazarse, fue %v", err)
	}
	// Hasta HOY inclusive sí: el día de hoy ya empezó a vender.
	if _, err := ResolveRange("rango", d("2026-09-01"), d("2026-09-03"), ahora, loc); err != nil {
		t.Fatalf("un rango que termina hoy debe pasar, fue %v", err)
	}
}

// El "hoy" contra el que se compara es el del NEGOCIO. A las 19:00 de México ya es mañana en UTC:
// con el reloj del servidor, un rango que termina mañana pasaría por bueno justo en la hora de más
// venta, que es cuando alguien lo pediría.
func TestElTopeDelFuturoUsaLaZonaDelNegocio(t *testing.T) {
	loc := mx(t)
	// 3 de septiembre 19:00 en México = 4 de septiembre 01:00 UTC.
	ahora := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	d := func(s string) time.Time {
		v, _ := time.ParseInLocation(dateOnly, s, time.UTC)
		return v
	}
	if _, err := ResolveRange("rango", d("2026-09-01"), d("2026-09-04"), ahora, loc); !errors.Is(err, ErrValidation) {
		t.Fatalf("el 4 todavía no es hoy en México; debe rechazarse, fue %v", err)
	}
}
