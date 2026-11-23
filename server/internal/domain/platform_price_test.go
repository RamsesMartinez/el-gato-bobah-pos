package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

// El precio de una plataforma sale del base más su margen, salvo que alguien haya capturado uno a
// mano. El redondeo va en el precio UNITARIO y no en el total de línea: order_lines.unit_price es
// numeric(10,2) y Postgres coacciona al guardar, mientras el total se calcula con el valor sin
// coaccionar. Medido contra el catálogo real, 12 de 215 productos activos dan un tercer decimal al
// 35%, y el ticket que se pega a la bolsa sale con un centavo de diferencia.
func TestPlatformPrice(t *testing.T) {
	casos := []struct {
		nombre string
		base   string
		margen string
		manual *string
		quiere string
	}{
		{"sin manual aplica el margen", "100", "35", nil, "135"},
		{"margen 0 devuelve el base", "100", "0", nil, "100"},
		{"el manual gana sobre el calculado", "100", "35", ptrStr("149"), "149"},
		{"un manual menor al base también gana", "100", "35", ptrStr("80"), "80"},
		// Los dos productos reales que destaparon el redondeo.
		{"BONELESS J 1Kg al 35%", "434.98", "35", nil, "587.22"},
		{"ALITAS J 1Kg al 35%", "398.98", "35", nil, "538.62"},
		// Un extra sin costo sigue sin costo por más margen que tenga la plataforma.
		{"delta 0 con margen sigue en 0", "0", "35", nil, "0"},
		{"margen fraccionario", "100", "32.5", nil, "132.5"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var manual *decimal.Decimal
			if c.manual != nil {
				m := dec(*c.manual)
				manual = &m
			}
			got := PlatformPrice(dec(c.base), dec(c.margen), manual)
			if !got.Equal(dec(c.quiere)) {
				t.Fatalf("PlatformPrice(%s, %s%%) = %s, quería %s", c.base, c.margen, got, c.quiere)
			}
		})
	}
}

// Sin plataforma el precio es el base, tal cual. Es el caso de todos los días y no debe pasar por
// ninguna cuenta que pueda mover un centavo.
func TestSinPlataformaElPrecioEsElBase(t *testing.T) {
	base := dec("434.98")
	if got := PlatformPrice(base, decimal.Zero, nil); !got.Equal(base) {
		t.Fatalf("sin margen el precio debe ser idéntico al base: %s", got)
	}
}

// ptrStr: puntero a un literal, para expresar "hay precio manual" en la tabla de casos.
func ptrStr(s string) *string { return &s }

// El origen del precio es lo que la vista previa de una cuenta de plataforma enseña por renglón: un
// producto sin precio propio en la plataforma toma base × (1 + margen), que puede no coincidir con
// lo que la app publica (#289, 16-nov). Tiene que salir de la misma decisión que PlatformPrice.
func TestPlatformPriceSource(t *testing.T) {
	propio := decimal.RequireFromString("99")
	cases := []struct {
		name     string
		platform bool
		markup   decimal.Decimal
		manual   *decimal.Decimal
		want     string
	}{
		{"mostrador", false, decimal.Zero, nil, PriceFromCounter},
		{"plataforma con precio propio", true, decimal.NewFromInt(30), &propio, PriceFromPlatform},
		{"plataforma sin precio propio: base más margen", true, decimal.NewFromInt(30), nil, PriceFromMarkup},
		{"plataforma sin margen ni precio propio", true, decimal.Zero, nil, PriceFromBase},
		{"precio propio aunque el margen sea cero", true, decimal.Zero, &propio, PriceFromPlatform},
	}
	for _, c := range cases {
		if got := PlatformPriceSource(c.platform, c.markup, c.manual); got != c.want {
			t.Errorf("%s: %q, esperaba %q", c.name, got, c.want)
		}
	}
}
