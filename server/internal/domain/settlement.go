package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// Lo que el documento de pago de una plataforma dice de un pedido.
//
// Es una COPIA del documento (snapshot), nunca un cálculo: las tasas cambian y las promociones las
// alteran, así que recalcular el pasado con la tasa de hoy reescribe la historia. Las comisiones
// medidas —Uber 30%, DiDi 30%, Rappi 20%— son material para PROPONER precios, que es otra feature.

// Cotas de cordura para las dos referencias de texto, del mismo tipo que MaxPlatformRefLen. Son más
// holgadas porque las captura quien administra, no el cajero en hora pico.
const (
	MaxPayoutRefLen   = 128
	MaxDocumentRefLen = 256
)

// Settlement es la liquidación de UN pedido de plataforma.
//
// La parte del descuento que financió el RESTAURANTE no está aquí: es DiscountTotal menos
// DiscountPlatform. Guardarla además serían dos verdades sobre el mismo hecho, y un documento
// corregido que mueva una y no la otra deja la fila contradiciéndose sin que nadie sepa cuál creer.
type Settlement struct {
	// ReportedGross es lo que la plataforma reporta como venta. NO se compara con orders.total:
	// cuadrarlos es la feature de conciliación, y rechazar la discrepancia impediría registrar
	// exactamente lo que se quiere poder ver.
	ReportedGross    decimal.Decimal
	CommissionAmount decimal.Decimal
	// CommissionPct es nil cuando el documento no la declara. Un 0 en su lugar afirmaría "la
	// plataforma cobró 0%", que es medible y falso.
	CommissionPct    *decimal.Decimal
	DiscountTotal    decimal.Decimal
	DiscountPlatform decimal.Decimal
	Withholdings     decimal.Decimal
	// NetAmount es lo que llegó al banco por este pedido y PUEDE SER NEGATIVO: con una promoción
	// que financió el restaurante por completo, eso es lo que de verdad pasó.
	NetAmount decimal.Decimal
	// PayoutReference es el identificador del depósito (payment_id de Rappi, Payout reference ID de
	// Uber). Texto y no una entidad porque EXPIRA: Rappi conserva 3 meses, Uber 31 días.
	PayoutReference string
	DocumentRef     string
}

// DiscountRestaurant es cuánto del descuento lo puso el restaurante. Derivado, nunca almacenado.
// Validate garantiza que no salga negativo.
func (s Settlement) DiscountRestaurant() decimal.Decimal {
	return Round2(s.DiscountTotal.Sub(s.DiscountPlatform))
}

// Rounded devuelve la liquidación con cada importe redondeado a la escala de su columna. Se aplica
// EN LA FRONTERA, antes de tocar la base: un sub-centavo del documento que entra sin redondear hace
// que el resumen del periodo deje de cuadrar contra la suma de sus renglones.
func (s Settlement) Rounded() Settlement {
	s.ReportedGross = Round2(s.ReportedGross)
	s.CommissionAmount = Round2(s.CommissionAmount)
	s.DiscountTotal = Round2(s.DiscountTotal)
	s.DiscountPlatform = Round2(s.DiscountPlatform)
	s.Withholdings = Round2(s.Withholdings)
	s.NetAmount = Round2(s.NetAmount)
	if s.CommissionPct != nil {
		// numeric(5,2) igual que los importes. Redondear no puede INVENTAR una tasa donde no la
		// había: si es nil se queda nil, porque "no se sabe" no es un número.
		pct := Round2(*s.CommissionPct)
		s.CommissionPct = &pct
	}
	return s
}

// Validate rechaza lo que no puede haber dicho un documento de pago.
//
// Todo sale como ErrValidation —400, no 500— porque son datos que teclea una persona con el
// documento enfrente, y un 500 no le dice qué corregir. Pásale la liquidación ya redondeada, para
// que un sub-centavo que redondea fuera de la cota se rechace igual que el valor explícito.
func (s Settlement) Validate() error {
	// Los cinco que no pueden ser negativos. allowZero: una liquidación en ceros es un hecho
	// —la plataforma no cobró nada— y es distinta de no tener liquidación, que es la ausencia de la
	// fila entera.
	noNegativos := []struct {
		nombre string
		v      decimal.Decimal
	}{
		{"el importe que reporta la plataforma", s.ReportedGross},
		{"la comisión", s.CommissionAmount},
		{"el descuento total", s.DiscountTotal},
		{"el descuento que financió la plataforma", s.DiscountPlatform},
		{"las retenciones", s.Withholdings},
	}
	for _, c := range noNegativos {
		if !ValidMoney(c.v, true) {
			return fmt.Errorf("%w: %s no puede ser negativo ni exceder %s (llegó %s)",
				ErrValidation, c.nombre, MaxMoney, comoTexto(c.v))
		}
	}

	// El neto va con su propio validador: es el único importe del sistema que puede ser negativo.
	if !ValidSignedMoney(s.NetAmount) {
		return fmt.Errorf("%w: el neto puede ser negativo, pero no puede exceder %s en magnitud (llegó %s)",
			ErrValidation, MaxMoney, comoTexto(s.NetAmount))
	}

	if s.CommissionPct != nil {
		pct := *s.CommissionPct
		if !escalaSana(pct) || pct.IsNegative() || pct.GreaterThan(decimal.NewFromInt(100)) {
			return fmt.Errorf("%w: la tasa de comisión va de 0 a 100 (llegó %s)", ErrValidation, comoTexto(pct))
		}
	}

	// La parte no puede ser mayor que el todo. Es la única aritmética que se valida entre campos, y
	// es la que hace que DiscountRestaurant nunca salga negativo — un descuento negativo del
	// restaurante se leería como que la plataforma le regaló dinero al negocio.
	if s.DiscountPlatform.GreaterThan(s.DiscountTotal) {
		return fmt.Errorf("%w: la plataforma no puede haber financiado %s de un descuento de %s",
			ErrValidation, s.DiscountPlatform, s.DiscountTotal)
	}

	if err := refAcotada("la referencia del depósito", s.PayoutReference, MaxPayoutRefLen); err != nil {
		return err
	}
	return refAcotada("la referencia del documento", s.DocumentRef, MaxDocumentRefLen)
}

// comoTexto imprime un importe para un mensaje de error SIN materializarlo si su exponente es
// absurdo.
//
// No es cosmético: `decimal.String()` de "1e100000000" expande cien millones de dígitos, y ese
// mensaje lo produce justamente la validación que rechaza ese valor. Medido: 77 segundos en un
// test que debía tardar milisegundos. Es la misma DoS que `Round2` ya cierra —47 bytes de JSON que
// queman CPU y memoria— reintroducida por la puerta del `%s`.
func comoTexto(v decimal.Decimal) string {
	if !escalaSana(v) {
		return "un número fuera de escala"
	}
	return v.String()
}

// refAcotada acepta el texto vacío —no todo documento trae referencia de depósito— pero no uno que
// sea solo espacios ni uno más largo que su tope. En caracteres, no en bytes, para decir lo mismo
// que el check de Postgres.
func refAcotada(nombre, v string, max int) error {
	if v == "" {
		return nil
	}
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("%w: %s no puede ser solo espacios", ErrValidation, nombre)
	}
	// Mismo motivo que en el folio: un byte NUL pasa hasta el driver y sale como 500.
	if strings.IndexFunc(v, esDeControl) >= 0 {
		return fmt.Errorf("%w: %s trae un carácter que no se puede guardar", ErrValidation, nombre)
	}
	if n := utf8.RuneCountInString(v); n > max {
		return fmt.Errorf("%w: %s tiene %d caracteres y el máximo es %d", ErrValidation, nombre, n, max)
	}
	return nil
}

// El resumen de dinero de plataformas de un periodo: las TRES cifras de SC-006.
//
// Existen porque hoy el sistema solo sabe una: cuánto se vendió. Cuánto se quedó la plataforma y
// cuánto llegó al banco son las que hacen visible una pérdida que lleva años siendo invisible —cero
// registros de comisión en dos años y medio del sistema anterior.

// PlatformTotals es lo que la base devuelve de un periodo. Los dos conjuntos NO son el mismo y esa
// es la razón de que cada cifra viaje con su conteo.
type PlatformTotals struct {
	// OrdersVendido / Vendido: TODOS los pedidos de plataforma del periodo que sí fueron ingreso.
	OrdersVendido int
	Vendido       decimal.Decimal
	// OrdersLiquidados y lo que sigue: solo los que YA tienen su documento de pago capturado.
	OrdersLiquidados int
	Comision         decimal.Decimal
	Retenciones      decimal.Decimal
	Neto             decimal.Decimal
	// SinLiquidar y SinFolio son lo que falta por capturar, y se dicen para que nadie lea las tres
	// cifras como si cubrieran el mes completo.
	SinLiquidar int
	SinFolio    int
}

// CifraDePlataforma es un importe que NUNCA viaja solo: lleva sobre cuántos pedidos habla y qué
// incluye y qué excluye. Tres importes hermanos sin eso se restan a ojo, que es la forma exacta del
// fondo de caja que dejó un turno con $4,500 de faltante sin explicación.
type CifraDePlataforma struct {
	Amount  decimal.Decimal `json:"amount"`
	Orders  int             `json:"orders"`
	Incluye string          `json:"incluye"`
	Excluye string          `json:"excluye"`
}

// ConteoDePedidos es cuántos pedidos, sin importe. Existe porque `ConceptCount` trae un `Amount` y
// aquí no hay ninguno que decir: lo que falta por capturar no tiene monto conocido, ese es el punto.
type ConteoDePedidos struct {
	Orders int `json:"orders"`
}

// PlatformMoneySummary son las tres cifras, y NO se derivan una de otra.
type PlatformMoneySummary struct {
	Vendido             CifraDePlataforma `json:"vendido"`
	SeQuedoLaPlataforma CifraDePlataforma `json:"seQuedoLaPlataforma"`
	LlegoAlBanco        CifraDePlataforma `json:"llegoAlBanco"`
	// SinLiquidar y SinFolio son CONTEOS y no importes: cuánto dinero hay detrás de un pedido sin
	// liquidar es justamente lo que no se sabe todavía. Un campo `amount` aquí diría siempre $0.00,
	// y una cifra que siempre miente cero se acaba sumando a algo.
	SinLiquidar ConteoDePedidos `json:"sinLiquidar"`
	SinFolio    ConteoDePedidos `json:"sinFolio"`
}

// SummarizePlatformMoney arma las tres cifras del periodo.
//
// LA REGLA QUE SOSTIENE TODO ESTO: `LlegoAlBanco` sale del NETO que dicen los documentos, nunca de
// restar la comisión a lo vendido. Los dos conjuntos son distintos —hay pedidos vendidos cuyo
// documento todavía no llega—, así que la resta inventaría dinero que la plataforma nunca retuvo.
func SummarizePlatformMoney(t PlatformTotals) PlatformMoneySummary {
	return PlatformMoneySummary{
		Vendido: CifraDePlataforma{
			Amount: Round2(t.Vendido), Orders: t.OrdersVendido,
			Incluye: "Lo que cobró el POS en pedidos de plataforma del periodo",
			Excluye: "Canceladas, reembolsadas y todo lo que no es de plataforma",
		},
		SeQuedoLaPlataforma: CifraDePlataforma{
			// Comisión MÁS retenciones: las dos son dinero que el negocio vendió y no recibió, y
			// separarlas en dos tiles invitaría a sumar una de ellas con lo vendido.
			Amount: Round2(t.Comision.Add(t.Retenciones)), Orders: t.OrdersLiquidados,
			Incluye: "Comisión y retenciones de los pedidos con documento de pago capturado",
			Excluye: "Los pedidos sin liquidación: de esos todavía no se sabe",
		},
		LlegoAlBanco: CifraDePlataforma{
			Amount: Round2(t.Neto), Orders: t.OrdersLiquidados,
			Incluye: "El neto que declara el documento, de esos mismos pedidos",
			Excluye: "Depósitos que no se pudieron atribuir a un pedido",
		},
		SinLiquidar: ConteoDePedidos{Orders: t.SinLiquidar},
		SinFolio:    ConteoDePedidos{Orders: t.SinFolio},
	}
}
