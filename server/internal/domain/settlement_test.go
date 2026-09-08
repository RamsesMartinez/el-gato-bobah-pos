package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// liquidacionDe2x1 son los números MEDIDOS de un 2x1 real de Uber Eats (dos sodas de $110, pedido
// del 5 de septiembre de 2026). Están en docs/plataformas-digitales.md §4-bis y no se re-derivan:
// se usan como caso válido de referencia para que los casos malos se lean como desviaciones de algo
// que de verdad pasó.
func liquidacionDe2x1() Settlement {
	return Settlement{
		ReportedGross:    dec("220.00"),
		CommissionAmount: dec("33.00"),
		CommissionPct:    decPtr("30.00"),
		DiscountTotal:    dec("110.00"),
		DiscountPlatform: dec("0.00"), // en Uber el restaurante absorbe el 100%
		Withholdings:     dec("9.96"),
		NetAmount:        dec("51.77"),
		PayoutReference:  "PAY-2026-09-12-0043",
		DocumentRef:      "uber-payments-2026-09-08.csv",
	}
}

func TestUnaLiquidacionRealSeAcepta(t *testing.T) {
	if err := liquidacionDe2x1().Validate(); err != nil {
		t.Fatalf("el 2x1 medido de Uber se rechazó, y es un documento real: %v", err)
	}
}

func TestLoQueUnaLiquidacionRechaza(t *testing.T) {
	casos := []struct {
		nombre   string
		toca     func(*Settlement)
		queRompe string
	}{
		{
			nombre: "comisión negativa", toca: func(s *Settlement) { s.CommissionAmount = dec("-1") },
			queRompe: "una plataforma no devuelve dinero por comisión; si el documento lo dice, es un error de captura",
		},
		{
			nombre: "bruto negativo", toca: func(s *Settlement) { s.ReportedGross = dec("-220") },
			queRompe: "la plataforma no reporta una venta negativa",
		},
		{
			nombre: "retenciones negativas", toca: func(s *Settlement) { s.Withholdings = dec("-1") },
			queRompe: "una retención negativa sería una devolución de impuesto, que no viene por aquí",
		},
		{
			nombre: "tasa por encima de 100", toca: func(s *Settlement) { s.CommissionPct = decPtr("180") },
			queRompe: "cota de cordura: un 180% es un dedazo, no una tasa",
		},
		{
			nombre: "tasa negativa", toca: func(s *Settlement) { s.CommissionPct = decPtr("-1") },
			queRompe: "idem",
		},
		{
			nombre:   "descuento de la plataforma mayor que el total",
			toca:     func(s *Settlement) { s.DiscountPlatform = dec("150"); s.DiscountTotal = dec("110") },
			queRompe: "la parte no puede ser mayor que el todo, y el derivado del restaurante saldría negativo",
		},
		{
			nombre:   "importe por encima del tope de dinero",
			toca:     func(s *Settlement) { s.ReportedGross = MaxMoney.Add(dec("1")) },
			queRompe: "por encima de MaxMoney desborda el numeric(10,2) y se vuelve un 500",
		},
		{
			nombre: "exponente absurdo", toca: func(s *Settlement) { s.NetAmount = dec("1e100000000") },
			queRompe: "47 bytes de JSON que queman ~25 s de CPU y 279 MiB si alguien los redondea",
		},
		{
			nombre:   "neto por debajo del tope negativo",
			toca:     func(s *Settlement) { s.NetAmount = MaxMoney.Neg().Sub(dec("1")) },
			queRompe: "el neto puede ser negativo, pero no ilimitado: sigue cayendo en numeric(10,2)",
		},
		{
			nombre:   "referencia de depósito más larga que el tope",
			toca:     func(s *Settlement) { s.PayoutReference = strings.Repeat("x", MaxPayoutRefLen+1) },
			queRompe: "misma cota de cordura que el folio, contra un pegado accidental",
		},
		{
			nombre:   "documento más largo que el tope",
			toca:     func(s *Settlement) { s.DocumentRef = strings.Repeat("x", MaxDocumentRefLen+1) },
			queRompe: "idem",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := liquidacionDe2x1()
			c.toca(&s)
			err := s.Validate()
			if err == nil {
				t.Fatalf("se aceptó: %s", c.queRompe)
			}
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("se rechazó pero no como ErrValidation, así que la frontera lo mapea a 500 "+
					"en vez de 400 — y el spec pide 'entrada inválida, no error del servidor': %v", err)
			}
		})
	}
}

// Lo que SÍ se acepta aunque parezca raro. Cada uno es un caso real; rechazarlo obligaría a
// capturar una mentira.
func TestLoQueUnaLiquidacionAceptaAunqueParezcaRaro(t *testing.T) {
	casos := []struct {
		nombre string
		toca   func(*Settlement)
		porque string
	}{
		{
			nombre: "neto negativo", toca: func(s *Settlement) { s.NetAmount = dec("-31.20") },
			porque: "con una promoción que financió el restaurante por completo, el neto negativo es lo que pasó",
		},
		{
			nombre: "comisión mayor que la venta reportada",
			toca:   func(s *Settlement) { s.CommissionAmount = dec("250"); s.ReportedGross = dec("110") },
			porque: "en DiDi la comisión se calcula sobre el precio SIN promoción, así que con descuento puede exceder lo reportado",
		},
		{
			nombre: "tasa ausente", toca: func(s *Settlement) { s.CommissionPct = nil },
			porque: "hay documentos que dan el monto y no declaran la tasa; un 0 ahí afirmaría 'cobró 0%', que es falso",
		},
		{
			nombre: "todo en cero", toca: func(s *Settlement) {
				*s = Settlement{ReportedGross: dec("0"), CommissionAmount: dec("0"),
					DiscountTotal: dec("0"), DiscountPlatform: dec("0"),
					Withholdings: dec("0"), NetAmount: dec("0")}
			},
			porque: "una liquidación en ceros es un hecho: la plataforma no cobró nada. Es distinta de no tener liquidación",
		},
		{
			nombre: "sin referencias de texto", toca: func(s *Settlement) { s.PayoutReference = ""; s.DocumentRef = "" },
			porque: "no todo documento trae referencia de depósito, y ausencia se representa como ausencia",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := liquidacionDe2x1()
			c.toca(&s)
			if err := s.Validate(); err != nil {
				t.Fatalf("se rechazó y es un caso real (%s): %v", c.porque, err)
			}
		})
	}
}

// La parte del restaurante es DERIVADA y no una columna: guardar los tres números serían dos
// verdades sobre el mismo hecho, y un documento corregido que mueva una y no la otra deja la fila
// contradiciéndose.
func TestLaParteDelRestauranteSeDeriva(t *testing.T) {
	casos := []struct{ total, plataforma, esperado string }{
		{"110.00", "0.00", "110.00"}, // Uber: el restaurante absorbe todo
		{"100.00", "40.00", "60.00"}, // Rappi con campaña: absorbe una parte
		{"283.50", "283.50", "0.00"}, // Rappi absorbe el total
		{"0.00", "0.00", "0.00"},     // sin promoción
	}
	for _, c := range casos {
		s := liquidacionDe2x1()
		s.DiscountTotal, s.DiscountPlatform = dec(c.total), dec(c.plataforma)
		if got := s.DiscountRestaurant(); !got.Equal(dec(c.esperado)) {
			t.Fatalf("descuento total %s con %s de la plataforma → el restaurante puso %s, se esperaba %s",
				c.total, c.plataforma, got, c.esperado)
		}
	}
}

// El derivado NUNCA sale negativo, porque la validación ya cerró ese caso. Si algún día alguien
// afloja el check, este test dice qué se rompe: un descuento negativo del restaurante se leería
// como que la plataforma le REGALÓ dinero.
func TestLaParteDelRestauranteNuncaEsNegativa(t *testing.T) {
	s := liquidacionDe2x1()
	s.DiscountTotal, s.DiscountPlatform = dec("110"), dec("150")
	if s.Validate() == nil && s.DiscountRestaurant().IsNegative() {
		t.Fatal("una liquidación válida produjo un descuento negativo del restaurante: se leería " +
			"como que la plataforma le regaló dinero al negocio")
	}
}

// Cada importe se redondea EN LA FRONTERA, antes de tocar una columna numeric(10,2). Sin esto, un
// sub-centavo del documento entra a la base y el resumen del periodo deja de cuadrar contra la suma
// de sus renglones.
func TestLaLiquidacionSeRedondeaEnLaFrontera(t *testing.T) {
	s := Settlement{
		ReportedGross:    dec("220.005"),
		CommissionAmount: dec("33.004"),
		DiscountTotal:    dec("110.006"),
		DiscountPlatform: dec("0.004"),
		Withholdings:     dec("9.955"),
		NetAmount:        dec("-31.195"),
		CommissionPct:    decPtr("30.005"),
	}
	r := s.Rounded()
	esperados := map[string]struct{ got, want decimal.Decimal }{
		"bruto":       {r.ReportedGross, dec("220.01")},
		"comisión":    {r.CommissionAmount, dec("33.00")},
		"descuento":   {r.DiscountTotal, dec("110.01")},
		"plataforma":  {r.DiscountPlatform, dec("0.00")},
		"retenciones": {r.Withholdings, dec("9.96")},
		"neto":        {r.NetAmount, dec("-31.20")},
	}
	for nombre, e := range esperados {
		if !e.got.Equal(e.want) {
			t.Fatalf("%s quedó en %s y debía redondear a %s antes de tocar numeric(10,2)", nombre, e.got, e.want)
		}
	}
	if r.CommissionPct == nil || !r.CommissionPct.Equal(dec("30.01")) {
		t.Fatalf("la tasa quedó en %v y debía redondear a 30.01 antes de tocar numeric(5,2)", r.CommissionPct)
	}
	// Redondear no puede inventar una tasa donde no la había.
	sinTasa := liquidacionDe2x1()
	sinTasa.CommissionPct = nil
	if sinTasa.Rounded().CommissionPct != nil {
		t.Fatal("el redondeo le puso tasa a una liquidación que no la declaraba: 'no se sabe' pasaría a ser un número")
	}
}

// Las tres cifras de un periodo: cuánto se vendió, cuánto se quedó la plataforma y cuánto llegó al
// banco. Es SC-006, y el modo de falla que este test vigila es el más caro de la feature.
func totalesDeSeptiembre() PlatformTotals {
	// 96 pedidos de plataforma vendidos, de los cuales SOLO 74 tienen su documento capturado. Los
	// conjuntos son distintos a propósito: es el caso real —el reporte de pago llega días después—
	// y es donde la resta miente.
	//
	// Los importes NO pueden ser tales que `vendido − comisión − retenciones` dé el neto: con esos
	// números el test pasaría igual con la implementación equivocada y no probaría nada. Aquí los 74
	// liquidados vendieron ~14,200 de los 18,420 del periodo, así que la resta da 13,200 y el neto
	// real es 8,980 — la diferencia es justo el dinero de los 22 pedidos cuyo documento no ha llegado.
	return PlatformTotals{
		OrdersVendido:    96,
		Vendido:          dec("18420.00"),
		OrdersLiquidados: 74,
		Comision:         dec("4260.00"),
		Retenciones:      dec("960.00"),
		Neto:             dec("8980.00"),
		SinLiquidar:      22,
		SinFolio:         3,
	}
}

func TestLasTresCifrasDelPeriodoCubrenConjuntosDistintos(t *testing.T) {
	r := SummarizePlatformMoney(totalesDeSeptiembre())

	if r.Vendido.Orders != 96 {
		t.Fatalf("lo vendido cubre %d pedidos y debía cubrir 96", r.Vendido.Orders)
	}
	if r.SeQuedoLaPlataforma.Orders != 74 || r.LlegoAlBanco.Orders != 74 {
		t.Fatalf("las cifras del documento cubren %d y %d pedidos y debían cubrir 74: son las que "+
			"tienen liquidación capturada", r.SeQuedoLaPlataforma.Orders, r.LlegoAlBanco.Orders)
	}
	// El conteo VIAJA con cada cifra. Sin él, tres tiles hermanos con tres importes se restan a ojo
	// — es la forma exacta del fondo de caja que dejó un turno con $4,500 de faltante.
	for nombre, c := range map[string]CifraDePlataforma{
		"vendido": r.Vendido, "se quedó la plataforma": r.SeQuedoLaPlataforma, "llegó al banco": r.LlegoAlBanco,
	} {
		if c.Incluye == "" || c.Excluye == "" {
			t.Fatalf("la cifra %q no declara qué incluye y qué excluye, así que quien la lee no puede "+
				"saber sobre qué pedidos habla", nombre)
		}
	}
}

// EL TEST QUE NOMBRA EL CONCEPTO DUPLICADO.
//
// Si alguien "arregla" el resumen haciendo que lo que llegó al banco sea la resta de las otras dos,
// las tres cifras pasan a describir el mismo conjunto y la pantalla afirma un margen que el negocio
// no tuvo: los 22 pedidos sin liquidar entrarían en lo vendido y no en lo retenido.
func TestLoQueLlegoAlBancoNoEsLaRestaDeLasOtrasDos(t *testing.T) {
	tot := totalesDeSeptiembre()
	r := SummarizePlatformMoney(tot)

	resta := Round2(tot.Vendido.Sub(tot.Comision).Sub(tot.Retenciones))
	if r.LlegoAlBanco.Amount.Equal(resta) {
		t.Fatalf("lo que llegó al banco salió de RESTAR (%s) en vez de sumar el neto del documento "+
			"(%s). Los 22 pedidos sin liquidar están en lo vendido y no en la comisión, así que la "+
			"resta inventa dinero que la plataforma nunca retuvo", resta, tot.Neto)
	}
	if !r.LlegoAlBanco.Amount.Equal(tot.Neto) {
		t.Fatalf("lo que llegó al banco es %s y el documento dice %s", r.LlegoAlBanco.Amount, tot.Neto)
	}
}

// La comisión es la comisión MÁS las retenciones: las dos son dinero que el negocio vendió y no
// recibió. Contarlas por separado en tiles hermanos invitaría a sumarlas con lo vendido.
func TestLoQueSeQuedoLaPlataformaIncluyeLasRetenciones(t *testing.T) {
	tot := totalesDeSeptiembre()
	r := SummarizePlatformMoney(tot)
	esperado := Round2(tot.Comision.Add(tot.Retenciones))
	if !r.SeQuedoLaPlataforma.Amount.Equal(esperado) {
		t.Fatalf("se quedó %s y la comisión más las retenciones son %s", r.SeQuedoLaPlataforma.Amount, esperado)
	}
}

// Un periodo sin una sola liquidación capturada no puede afirmar nada sobre el dinero del
// documento, y lo dice con ceros SOBRE CERO PEDIDOS — no con ceros sobre 96, que se leería como
// "las plataformas no cobraron nada este mes".
func TestUnPeriodoSinLiquidacionesNoAfirmaQueLaPlataformaNoCobro(t *testing.T) {
	r := SummarizePlatformMoney(PlatformTotals{
		OrdersVendido: 96, Vendido: dec("18420.00"), OrdersLiquidados: 0, SinLiquidar: 96, SinFolio: 3,
	})
	if r.SeQuedoLaPlataforma.Orders != 0 || r.LlegoAlBanco.Orders != 0 {
		t.Fatal("sin liquidaciones, las cifras del documento tienen que cubrir CERO pedidos")
	}
	if r.SinLiquidar.Orders != 96 {
		t.Fatalf("los 96 pedidos están sin liquidar y el resumen dice %d", r.SinLiquidar.Orders)
	}
}

// El RECHAZO de un importe absurdo tiene que ser barato.
//
// Este test existe por un defecto real: el mensaje de error imprimía el decimal con `%s`, y
// `decimal.String()` de "1e100000000" expande cien millones de dígitos. Rechazar el valor tardaba
// 77 segundos y comía memoria — la misma DoS que Round2 ya cierra, reintroducida por la puerta del
// mensaje. Un endpoint que se defiende gastando lo que el atacante quería que gastara no se defiende.
func TestRechazarUnImporteAbsurdoEsBarato(t *testing.T) {
	for _, absurdo := range []string{"1e100000000", "-1e100000000", "1e-100000000"} {
		s := liquidacionDe2x1()
		s.NetAmount = dec(absurdo)
		hecho := make(chan error, 1)
		go func() { hecho <- s.Validate() }()
		select {
		case err := <-hecho:
			if err == nil {
				t.Fatalf("%s se aceptó", absurdo)
			}
			// Y el mensaje NO trae el número expandido.
			if len(err.Error()) > 500 {
				t.Fatalf("el mensaje de error mide %d caracteres: se materializó el número absurdo",
					len(err.Error()))
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("validar %s tardó más de 2 s: se está expandiendo el número en vez de "+
				"rechazarlo por su exponente", absurdo)
		}
	}
}
