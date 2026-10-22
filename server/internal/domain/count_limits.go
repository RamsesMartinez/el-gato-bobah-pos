package domain

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// CountLimitPair son los dos límites del conteo para una clase de unidad, en su unidad base: lo más
// que se puede registrar de un artículo y la cifra a partir de la cual, sin otra referencia, se pide
// confirmar.
type CountLimitPair struct {
	Max     decimal.Decimal `json:"max"`
	Confirm decimal.Decimal `json:"confirm"`
}

// CountLimits son los límites del conteo de una empresa por clase de unidad (spec 044). Solo los usa
// el conteo: compras, recetas y ajustes siguen con MaxStockQty.
type CountLimits struct {
	Mass   CountLimitPair `json:"mass"`
	Volume CountLimitPair `json:"volume"`
	Length CountLimitPair `json:"length"`
	Piece  CountLimitPair `json:"piece"`
}

// MaxCountLimit es el único techo fijo de un límite: existe para que la cifra quepa en numeric(14,4)
// y en los ajustes que genera el cierre, no como regla del negocio (1,000 t en gramos).
var MaxCountLimit = decimal.NewFromInt(1_000_000_000)

// DefaultCountLimits son los valores que regían fijos en el código antes de la 044 (MaxStockQty y
// los topes del aviso). La migración 0098 pone los mismos como default de columna; una prueba de
// integración compara los dos para que no se separen.
func DefaultCountLimits() CountLimits {
	pair := func(confirm int64) CountLimitPair {
		return CountLimitPair{Max: decimal.NewFromInt(1_000_000), Confirm: decimal.NewFromInt(confirm)}
	}
	return CountLimits{Mass: pair(50_000), Volume: pair(50_000), Length: pair(5_000), Piece: pair(5_000)}
}

// ForKind devuelve los límites de una clase de unidad (masa, volumen, longitud o pieza).
func (l CountLimits) ForKind(kind string) (CountLimitPair, bool) {
	switch kind {
	case "masa":
		return l.Mass, true
	case "volumen":
		return l.Volume, true
	case "longitud":
		return l.Length, true
	case "pieza":
		return l.Piece, true
	}
	return CountLimitPair{}, false
}

// Validate rechaza límites que dejarían el conteo sin sentido: cero o negativos, un máximo que no
// pasa la cifra para confirmar (el aviso nunca saltaría antes del rechazo) o sobre MaxCountLimit.
func (l CountLimits) Validate() error {
	for _, c := range []struct {
		label string
		p     CountLimitPair
	}{{"peso", l.Mass}, {"volumen", l.Volume}, {"longitud", l.Length}, {"piezas", l.Piece}} {
		for _, v := range []decimal.Decimal{c.p.Max, c.p.Confirm} {
			if !escalaSana(v) || !v.IsPositive() {
				return fmt.Errorf("%w: en %s, las cifras tienen que ser mayores que 0", ErrValidation, c.label)
			}
			if v.GreaterThan(MaxCountLimit) {
				return fmt.Errorf("%w: en %s, la cifra pasa de lo que se puede guardar", ErrValidation, c.label)
			}
		}
		if !c.p.Max.GreaterThan(c.p.Confirm) {
			return fmt.Errorf("%w: en %s, el máximo tiene que ser mayor que la cifra para confirmar", ErrValidation, c.label)
		}
	}
	return nil
}

// CheckCountMax rechaza una cifra del conteo (en unidad base) que pasa el máximo de su clase, con
// el mensaje que ve quien cuenta. Una clase desconocida cae al techo de almacenamiento.
func (l CountLimits) CheckCountMax(kind string, q decimal.Decimal) error {
	if q.IsNegative() || !escalaSana(q) {
		return ErrValidation
	}
	p, ok := l.ForKind(kind)
	if !ok {
		p.Max = MaxCountLimit
	}
	if q.GreaterThan(p.Max) {
		return fmt.Errorf("%w: %s", ErrValidation, CountMaxMessage(kind, p.Max))
	}
	return nil
}

// countBigUnits es la unidad grande en la que se dice el máximo: nadie piensa en 1,000,000 g.
var countBigUnits = map[string]struct {
	toBase decimal.Decimal
	name   string
}{
	"masa":     {decimal.NewFromInt(1000), "kg"},
	"volumen":  {decimal.NewFromInt(1000), "L"},
	"longitud": {decimal.NewFromInt(100), "m"},
}

// CountMaxMessage dice el máximo de una clase en su unidad grande y con miles: «Máximo 1,000 kg por
// conteo». La pantalla muestra el mismo texto antes de mandar.
func CountMaxMessage(kind string, max decimal.Decimal) string {
	if u, ok := countBigUnits[kind]; ok {
		return fmt.Sprintf("Máximo %s %s por conteo", groupThousands(max.Div(u.toBase)), u.name)
	}
	unit := "piezas"
	if max.Equal(decimal.NewFromInt(1)) {
		unit = "pieza"
	}
	return fmt.Sprintf("Máximo %s %s por conteo", groupThousands(max), unit)
}

// groupThousands escribe la cifra con comas de miles y sin ceros de más a la derecha.
func groupThousands(v decimal.Decimal) string {
	s := v.Round(4).String()
	intPart, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if frac != "" {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// ValidCountQty acepta una cifra del conteo antes de compararla con el máximo de la empresa: cero
// («no hay») o positiva bajo MaxCountLimit, con escala sana. No usa MaxStockQty: el máximo del
// conteo lo decide la empresa y puede pasar de él.
func ValidCountQty(q decimal.Decimal) bool {
	if q.IsZero() {
		return escalaSana(q)
	}
	return ValidQty(q, MaxCountLimit, false)
}
