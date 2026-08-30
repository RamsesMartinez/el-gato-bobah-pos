package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

// d es un helper de test para construir decimales exactos desde string.
func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestValidMoney(t *testing.T) {
	cases := []struct {
		name      string
		v         decimal.Decimal
		allowZero bool
		want      bool
	}{
		{"monto normal", d("150.50"), false, true},
		{"un centavo", d("0.01"), false, true},
		{"cero rechazado sin allowZero", decimal.Zero, false, false},
		{"cero aceptado con allowZero", decimal.Zero, true, true},
		{"negativo", d("-1"), true, false},
		{"tope exacto", MaxMoney, false, true},
		{"sobre el tope (evita overflow numeric(10,2))", MaxMoney.Add(d("0.01")), false, false},
		// El punto de todo el refactor: 0.1 + 0.2 == 0.30 exacto (con float64 daría 0.30000000000000004).
		{"suma exacta sin drift", d("0.1").Add(d("0.2")), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidMoney(c.v, c.allowZero); got != c.want {
				t.Fatalf("ValidMoney(%v, %v) = %v, want %v", c.v, c.allowZero, got, c.want)
			}
		})
	}
}

func TestValidQty(t *testing.T) {
	cases := []struct {
		name          string
		v, max        decimal.Decimal
		allowNegative bool
		want          bool
	}{
		{"venta normal", d("2"), MaxOrderQty, false, true},
		{"fraccionaria", d("1.5"), MaxOrderQty, false, true},
		{"cero nunca", decimal.Zero, MaxOrderQty, true, false},
		{"negativo en venta", d("-1"), MaxOrderQty, false, false},
		{"negativo en stock (merma)", d("-5"), MaxStockQty, true, true},
		{"tope de venta", MaxOrderQty, MaxOrderQty, false, true},
		{"sobre tope de venta (evita int16 wrap del modificador)", MaxOrderQty.Add(d("1")), MaxOrderQty, false, false},
		{"bajo el tope negativo de stock", MaxStockQty.Neg(), MaxStockQty, true, true},
		{"sobre el tope negativo", MaxStockQty.Neg().Sub(d("1")), MaxStockQty, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidQty(c.v, c.max, c.allowNegative); got != c.want {
				t.Fatalf("ValidQty(%v, %v, %v) = %v, want %v", c.v, c.max, c.allowNegative, got, c.want)
			}
		})
	}
}

// Un exponente absurdo no puede costar CPU. `decimal.Round` calcula 10^|exp| como big.Int, así que
// redondear "1e100000000" quema ~25 s y 279 MiB: un cuerpo de 47 bytes tumba la API. El límite de
// 1 MiB del body no ayuda (el payload es diminuto) ni el ReadTimeout (el gasto es después de leer).
//
// Por eso la guarda va ANTES de tocar big.Int y mira solo el exponente, que es O(1).
func TestValidMoneyRechazaExponentesAbsurdosSinGastarCPU(t *testing.T) {
	absurdos := []string{"1e1000", "1e1000000", "1e100000000", "1e-1000000", "-1e100000000"}
	for _, s := range absurdos {
		v := decimal.RequireFromString(s)
		if ValidMoney(v, true) {
			t.Fatalf("%s debe rechazarse", s)
		}
		if ValidQty(v, MaxOrderQty, true) {
			t.Fatalf("%s debe rechazarse como cantidad", s)
		}
	}
}

// Y los valores normales siguen pasando: la guarda no puede volverse un rechazo sorpresa.
func TestValidMoneySigueAceptandoLoNormal(t *testing.T) {
	buenos := []string{"0", "0.01", "1", "434.98", "9999999.99", "-0.01"}
	for _, s := range buenos {
		v := decimal.RequireFromString(s)
		if !ValidMoney(v, true) && !v.IsNegative() {
			t.Fatalf("%s debe aceptarse", s)
		}
	}
	// El tope sigue siendo el tope.
	if ValidMoney(decimal.RequireFromString("10000000.01"), true) {
		t.Fatal("por encima de MaxMoney debe rechazarse")
	}
}

// El redondeo tampoco puede colgarse: todas las fronteras del repo redondean ANTES de validar
// (POST /orders con la cantidad de una línea, el alta de productos, los precios de plataforma), así
// que una guarda que solo viviera en ValidMoney llegaría tarde.
func TestRound2NoSeCuelgaConExponentesAbsurdos(t *testing.T) {
	for _, s := range []string{"1e100000000", "1e-100000000", "-1e100000000"} {
		v := decimal.RequireFromString(s)
		got := Round2(v)
		// Se devuelve sin redondear, y por lo tanto sigue siendo inválido.
		if ValidMoney(got, true) {
			t.Fatalf("%s no debe pasar la validación después de Round2", s)
		}
		if ValidQty(Round4(v), MaxOrderQty, true) {
			t.Fatalf("%s no debe pasar como cantidad después de Round4", s)
		}
	}
	// Y el redondeo normal sigue funcionando.
	if got := Round2(decimal.RequireFromString("587.223")); !got.Equal(decimal.RequireFromString("587.22")) {
		t.Fatalf("Round2(587.223) = %s", got)
	}
}
